package runmanager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/delegate"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/preparation"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/storage"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
	"github.com/suchasplus/til-consensus/observer"
	filestore "github.com/suchasplus/til-consensus/store/file"
	"golang.org/x/sys/unix"
)

type Options struct {
	CheckCLI           func(context.Context, providers.Config) (providers.CheckResult, error)
	DiscoverModels     func(context.Context, providers.Config) (providers.Catalog, error)
	ExecutePreparation func(context.Context, model.PreparationRecord, providers.Config, string, time.Duration) (model.PreparationRecord, error)
	DataDir            string
	Delay              time.Duration
	Notify             func(model.DesktopEvent)
	Deadline           time.Duration
	ProviderMode       string
}
type Manager struct {
	providerMu     sync.Mutex
	providerChecks map[string]*providerCheckJob
	catalogs       map[string]*catalogJob
	mu             sync.Mutex
	store          *storage.Store
	sessions       *filestore.Store
	opt            Options
	active         map[string]context.CancelFunc
	preparations   map[string]*preparationJob
	wg             sync.WaitGroup
	lock           *os.File
	closing        bool
}

func New(opt Options) (*Manager, error) {
	if opt.ProviderMode == "" {
		opt.ProviderMode = "cli"
	}
	if opt.ProviderMode != "cli" && opt.ProviderMode != "mock" {
		return nil, fmt.Errorf("不支持的 providerMode: %s", opt.ProviderMode)
	}
	if opt.CheckCLI == nil {
		opt.CheckCLI = providers.Check
	}
	if opt.DiscoverModels == nil {
		opt.DiscoverModels = providers.DiscoverModels
	}
	if opt.ExecutePreparation == nil {
		opt.ExecutePreparation = preparation.Execute
	}
	if opt.Deadline <= 0 {
		opt.Deadline = 15 * time.Minute
	}
	if err := os.MkdirAll(opt.DataDir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(opt.DataDir, "host.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("数据目录已由另一个后台进程使用: %w", err)
	}
	s, err := storage.Open(opt.DataDir)
	if err != nil {
		lock.Close()
		return nil, err
	}
	m := &Manager{providerChecks: map[string]*providerCheckJob{}, catalogs: map[string]*catalogJob{}, opt: opt, store: s, sessions: filestore.New(filepath.Join(opt.DataDir, "engine-sessions")), active: map[string]context.CancelFunc{}, preparations: map[string]*preparationJob{}, lock: lock}
	if err = m.reconcilePreparations(); err != nil {
		m.Close()
		return nil, err
	}
	if err = m.reconcile(); err != nil {
		m.Close()
		return nil, err
	}
	return m, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	m.closing = true
	for _, job := range m.preparations {
		job.cancel()
	}
	for id, cancel := range m.active {
		r, err := m.store.Load(context.Background(), id)
		if err == nil {
			if !runtimeConfigChanged(r.StopReason) {
				r.StopReason = "shutdown"
				_ = m.saveLocked(r, "run.interrupting", nil, "", "", "")
			}
		}
		cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
	err := m.store.Close()
	_ = unix.Flock(int(m.lock.Fd()), unix.LOCK_UN)
	_ = m.lock.Close()
	return err
}
func (m *Manager) List() ([]model.RunSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runs, err := m.store.All(context.Background())
	out := []model.RunSummary{}
	for _, r := range runs {
		if r.Snapshot.DeletedAt == "" {
			out = append(out, r.Snapshot.RunSummary)
		}
	}
	return out, err
}
func (m *Manager) Get(id string) (model.RunSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.store.Load(context.Background(), id)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	if err := m.refreshChainSnapshotLocked(r); err != nil {
		return model.RunSnapshot{}, err
	}
	return r.Snapshot, nil
}
func (m *Manager) Events(id string, after int64) ([]model.DesktopEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if after < 0 {
		return nil, fmt.Errorf("afterEventId 必须大于等于 0")
	}
	if _, err := m.store.Load(context.Background(), id); err != nil {
		return nil, err
	}
	return m.store.Events(context.Background(), id, after)
}

func (m *Manager) Start(in model.StartRunInput) (model.RunSnapshot, error) {
	return m.start(in, "", "")
}

func (m *Manager) start(in model.StartRunInput, parentID, parentMaterial string) (model.RunSnapshot, error) {
	in.Goal = strings.TrimSpace(in.Goal)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	materials, normalizeErr := preparation.Normalize(in.Goal, in.Materials)
	if normalizeErr != nil {
		return model.RunSnapshot{}, normalizeErr
	}
	if !validKey(in.IdempotencyKey) {
		return model.RunSnapshot{}, fmt.Errorf("idempotencyKey 需要 1 至 200 个字符")
	}
	switch in.Mode {
	case "auto", "free_debate", "adjudication", "delphi":
	default:
		return model.RunSnapshot{}, fmt.Errorf("不支持的模式: %s", in.Mode)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return model.RunSnapshot{}, fmt.Errorf("后台正在关闭")
	}
	// A branch reads its source outside this lock. Recheck here so a concurrent
	// deletion cannot create a new child from a source that is now in the trash.
	if parentID != "" {
		parent, err := m.store.Load(context.Background(), parentID)
		if err != nil {
			return model.RunSnapshot{}, err
		}
		if parent.Snapshot.DeletedAt != "" {
			return model.RunSnapshot{}, deletedRunError()
		}
	}
	fingerprintBody, _ := json.Marshal(struct{ Goal, Mode, ParentID, ParentHash, MaterialsHash, PreparationID string }{in.Goal, in.Mode, parentID, workflow.Hash(parentMaterial), materialsHash(materials), in.PreparationID})
	fingerprint := workflow.Hash(string(fingerprintBody))
	if id, err := m.store.LookupKey(context.Background(), in.IdempotencyKey, "start", fingerprint); err != nil {
		return model.RunSnapshot{}, err
	} else if id != "" {
		r, err := m.store.Load(context.Background(), id)
		if err != nil {
			return model.RunSnapshot{}, err
		}
		if r.Snapshot.DeletedAt != "" {
			return model.RunSnapshot{}, deletedRunError()
		}
		return r.Snapshot, nil
	}
	selectedPreparation, prepErr := m.matchingPreparationLocked(in, materials)
	if prepErr != nil {
		return model.RunSnapshot{}, prepErr
	}
	finalMode := in.Mode
	if finalMode == "auto" {
		finalMode = selectedPreparation.Classification.Recommendation
	}
	now := time.Now().UTC()
	if selectedPreparation != nil {
		now, _ = time.Parse(time.RFC3339Nano, selectedPreparation.CreatedAt)
	}
	id := newID("run")
	deadline := now.Add(m.opt.Deadline).Format(time.RFC3339Nano)
	title := []rune(in.Goal)
	if len(title) > 36 {
		title = append(title[:36], '…')
	}
	r := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: id, Title: string(title), Goal: in.Goal, Mode: finalMode, Status: "running", Phase: "preparing", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}, HostPhase: "preparing", AttemptID: newID("attempt"), DeadlineAt: deadline, Messages: []model.ChatMessage{}, Reviews: []model.ReviewIssue{}, Claims: []model.ClaimView{}}, Frozen: model.FrozenConfig{EngineVersion: model.EngineVersion, PolicyVersion: model.PolicyVersion, ProviderMode: m.opt.ProviderMode, DelayMS: m.opt.Delay.Milliseconds(), DeadlineAt: deadline, GoalHash: workflow.Hash(in.Goal), Mode: finalMode, RequestedMode: in.Mode, MaterialsHash: materialsHash(materials)}}
	r.Snapshot.Materials = materials
	r.Snapshot.RequestedMode = in.Mode
	adoptPreparation(r, selectedPreparation)
	r.Snapshot.ProviderMode = r.Frozen.ProviderMode
	r.Snapshot.ParentRunID = parentID
	r.Snapshot.LineageRootID = parentID
	if parentID == "" {
		r.Snapshot.LineageRootID = id
	}
	r.ParentMaterial = parentMaterial
	if parentMaterial != "" {
		r.Frozen.ParentMaterialHash = workflow.Hash(parentMaterial)
	}
	r.Snapshot.ProposalHistory = []model.Proposal{}
	r.Snapshot.Ballots = []model.BallotBinding{}
	r.Snapshot.Evidence = []model.Evidence{}

	if parentID != "" {
		if parent, loadErr := m.store.Load(context.Background(), parentID); loadErr == nil {
			if parent.Frozen.ChainRootID != "" {
				r.Snapshot.LineageRootID = parent.Frozen.ChainRootID
			} else if parent.Snapshot.LineageRootID != "" {
				r.Snapshot.LineageRootID = parent.Snapshot.LineageRootID
			} else {
				rootID, rootErr := m.chainRootIDLocked(parent)
				if rootErr != nil {
					return model.RunSnapshot{}, rootErr
				}
				r.Snapshot.LineageRootID = rootID
			}
			if parent.Snapshot.Proposal != nil {
				r.Snapshot.SourceProposalVersion = parent.Snapshot.Proposal.Version
				r.Snapshot.SourceProposalHash = parent.Snapshot.Proposal.Hash
			}
		}
	}

	r.Frozen.ChainRootID = r.Snapshot.LineageRootID
	r.Frozen.ChainLimit = DefaultChainLimit
	if err := m.refreshChainSnapshotLocked(r); err != nil {
		return model.RunSnapshot{}, err
	}
	if r.Snapshot.ChainCalls >= r.Snapshot.ChainLimit {
		return model.RunSnapshot{}, fmt.Errorf("关联讨论链已达到 %d 次 CLI 请求上限，不能再创建分支", r.Snapshot.ChainLimit)
	}
	r.Frozen.ChainLimit = r.Snapshot.ChainLimit
	frozenProviders, err := m.freezeProviders()
	if err != nil {
		return model.RunSnapshot{}, err
	}
	r.Frozen.Providers = frozenProviders
	plan, err := frozenRolePlan(frozenProviders)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	r.Frozen.RolePlan = plan
	r.Snapshot.RolePlan = plan
	r.Snapshot.Providers = r.Frozen.Providers
	if err := validatePreparationSource(r, selectedPreparation); err != nil {
		return model.RunSnapshot{}, err
	}
	deadlineTime, _ := time.Parse(time.RFC3339Nano, deadline)
	if selectedPreparation != nil && !time.Now().Before(deadlineTime) {
		return model.RunSnapshot{}, fmt.Errorf("准备等待已超过原讨论期限，请重新分析")
	}
	b, _ := json.Marshal(r.Frozen)
	r.ConfigHash = workflow.Hash(string(b))
	r.Snapshot.Messages = append(r.Snapshot.Messages, model.ChatMessage{ID: newID("message"), AgentID: "user", Role: "user", Label: "你", Content: in.Goal, At: r.Snapshot.CreatedAt, Phase: "preparing", AttemptID: r.Snapshot.AttemptID, Kind: "system"})
	if err := m.saveLocked(r, "run.started", map[string]any{"mode": in.Mode}, in.IdempotencyKey, "start", fingerprint); err != nil {
		return model.RunSnapshot{}, err
	}
	out := r.Snapshot
	m.launchLocked(r)
	return out, nil
}

func (m *Manager) Resume(in model.ResumeRunInput) (model.RunSnapshot, error) {
	if !validKey(in.IdempotencyKey) {
		return model.RunSnapshot{}, fmt.Errorf("idempotencyKey 需要 1 至 200 个字符")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return model.RunSnapshot{}, fmt.Errorf("后台正在关闭")
	}
	fingerprint := workflow.Hash(in.RunID)
	if id, err := m.store.LookupKey(context.Background(), in.IdempotencyKey, "resume", fingerprint); err != nil {
		return model.RunSnapshot{}, err
	} else if id != "" {
		r, err := m.store.Load(context.Background(), id)
		if err != nil {
			return model.RunSnapshot{}, err
		}
		if r.Snapshot.DeletedAt != "" {
			return model.RunSnapshot{}, deletedRunError()
		}
		return r.Snapshot, nil
	}
	if _, ok := m.active[in.RunID]; ok {
		return model.RunSnapshot{}, fmt.Errorf("讨论已有活跃的执行 lease，不能重复继续")
	}
	r, err := m.store.Load(context.Background(), in.RunID)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	if r.Snapshot.DeletedAt != "" {
		return model.RunSnapshot{}, deletedRunError()
	}
	if r.Snapshot.Status == "completed" {
		return model.RunSnapshot{}, fmt.Errorf("讨论已完成")
	}
	if err = validateFrozen(r); err != nil {
		return model.RunSnapshot{}, err
	}
	if r.Frozen.ProviderMode != m.opt.ProviderMode {
		return model.RunSnapshot{}, fmt.Errorf("此历史讨论使用 %s，不能以 %s 继续；请新建关联讨论", r.Frozen.ProviderMode, m.opt.ProviderMode)
	}
	// A finished snapshot needs only deterministic finalization, even after expiry.
	snap, err := m.loadSession(r)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	if snap == nil || snap.Phase != consensus.SessionPhaseFinished || snap.Result == nil {
		if err := verifyProviderVersions(r.Frozen); err != nil {
			return model.RunSnapshot{}, err
		}
		deadline, _ := time.Parse(time.RFC3339Nano, r.Snapshot.DeadlineAt)
		if !time.Now().Before(deadline) {
			return model.RunSnapshot{}, fmt.Errorf("原讨论期限已过，不能重置期限；请新建讨论")
		}
	}
	for i := range r.Snapshot.Messages {
		r.Snapshot.Messages[i].Streaming = false
	}
	r.Snapshot.AttemptID = newID("attempt")
	r.Snapshot.Status = "running"
	r.Snapshot.Error = ""
	r.StopReason = ""
	if err := m.refreshChainSnapshotLocked(r); err != nil {
		return model.RunSnapshot{}, err
	}
	if err = m.saveLocked(r, "run.resumed", map[string]any{"checkpoint": r.Snapshot.Checkpoint}, in.IdempotencyKey, "resume", fingerprint); err != nil {
		return model.RunSnapshot{}, err
	}
	out := r.Snapshot
	m.launchLocked(r)
	return out, nil
}

func (m *Manager) Stop(id string) (model.RunSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.store.Load(context.Background(), id)
	if err != nil {
		return model.RunSnapshot{}, err
	}
	if runtimeConfigChanged(r.StopReason) {
		return r.Snapshot, nil
	}
	cancel, active := m.active[id]
	if !active {
		return r.Snapshot, nil
	}
	r.StopReason = "user"
	r.Snapshot.Phase = "stopping"
	if err = m.saveLocked(r, "run.stop_requested", nil, "", "", ""); err != nil {
		return model.RunSnapshot{}, err
	}
	cancel()
	return r.Snapshot, nil
}

func (m *Manager) launchLocked(r *model.StoredRun) {
	deadline, _ := time.Parse(time.RFC3339Nano, r.Snapshot.DeadlineAt)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	m.active[r.Snapshot.ID] = cancel
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		err := m.execute(ctx, r.Snapshot.ID, r.Snapshot.AttemptID)
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.active, r.Snapshot.ID)
		current, loadErr := m.store.Load(context.Background(), r.Snapshot.ID)
		if loadErr != nil {
			return
		}
		if err != nil || current.StopReason != "" {
			for i := range current.Snapshot.Messages {
				current.Snapshot.Messages[i].Streaming = false
			}
			if runtimeConfigChanged(current.StopReason) {
				current.Snapshot.Status = "failed"
			} else if current.StopReason == "user" {
				current.Snapshot.Status = "stopped"
				current.Snapshot.Error = ""
			} else if current.StopReason == "shutdown" {
				current.Snapshot.Status = "interrupted"
				current.Snapshot.Error = ""
			} else {
				current.Snapshot.Status = "failed"
				if err != nil {
					current.Snapshot.Error = err.Error()
				}
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					current.Snapshot.Error = "已达到原讨论期限，请新建讨论"
				}
			}
			_ = m.saveLocked(current, "run."+current.Snapshot.Status, map[string]any{"error": current.Snapshot.Error}, "", "", "")
		}
	}()
}

func (m *Manager) execute(ctx context.Context, id, attempt string) error {
	r, err := m.stored(id)
	if err != nil {
		return err
	}
	if err = validateFrozen(r); err != nil {
		return err
	}
	dir := m.runDir(id)
	artifactDir := filepath.Join(dir, "artifacts", attempt)
	if err = os.MkdirAll(artifactDir, 0700); err != nil {
		return err
	}
	if err = writeJSON(filepath.Join(dir, "run-config.json"), r.Frozen); err != nil {
		return err
	}
	d := delegate.New(delegate.Options{Mode: r.Frozen.ProviderMode, RunID: id, Providers: delegateConfigs(r.Frozen), AttemptID: attempt, ArtifactDir: artifactDir, Delay: time.Duration(r.Frozen.DelayMS) * time.Millisecond,
		OnBeforeCall: func(invocationID, agentID string) error { return m.reserveCall(id, attempt, invocationID, agentID) },
		OnMessage: func(msg delegate.Message) error {
			if err := m.enforceCLIVersion(id, attempt, msg.AgentID, msg.CLIVersion, false); err != nil {
				return err
			}
			if err := m.enforceReportedModel(id, attempt, msg.AgentID, msg.InvocationID, msg.Model, msg.ModelSource, msg.Complete); err != nil {
				return err
			}
			if msg.Phase == "preparing" {
				return nil
			}
			return m.update(id, "message.completed", func(r *model.StoredRun) error {
				messageID := "message-" + msg.InvocationID
				if msg.InvocationID == "" {
					messageID = newID("message")
				}
				agentID, label := msg.AgentID, msg.Label
				if r.Snapshot.Mode == "delphi" {
					agentID, label = workflow.AnonymousIdentity(workflow.RolePlanFor(r.Frozen), msg.AgentID)
				}
				message := model.ChatMessage{ID: messageID, AgentID: agentID, Role: msg.Role, Label: label, Content: msg.Content, At: now(), Phase: msg.Phase, AttemptID: attempt, Kind: messageKind(msg.Kind), InvocationID: msg.InvocationID, Round: msg.Round, Model: msg.Model}
				upsertMessage(&r.Snapshot, message)
				recordUsage(r, msg.InvocationID, msg.AgentID, msg.Model, msg.CLIVersion, msg.Usage, msg.Cost, msg.ModelSource)
				if msg.ReportedReasoning != "" {
					r.ActualMetadata[msg.InvocationID]["reportedReasoning"] = msg.ReportedReasoning
				}
				if msg.ProposalContent != "" {
					appendProposal(&r.Snapshot, msg.ProposalContent, "synthesis", msg.InvocationID)
				}
				if r.MessageSources == nil {
					r.MessageSources = map[string]model.MessageSource{}
				}
				r.MessageSources[messageID] = model.MessageSource{InvocationID: msg.InvocationID, ArtifactPath: msg.ArtifactPath, ContextHash: msg.ContextHash, Complete: msg.Complete, Round: msg.Round}
				return nil
			}, nil)
		}, OnProgress: func(event delegate.ProgressEvent) error { return m.progress(id, attempt, event) }, OnBallot: func(ballot delegate.Ballot) error {
			return m.update(id, "ballot.validated", func(r *model.StoredRun) error {
				r.Snapshot.Ballots = append(r.Snapshot.Ballots, model.BallotBinding{AgentID: ballot.AgentID, InvocationID: ballot.InvocationID, ProposalHash: ballot.ProposalHash, ManifestHash: ballot.ManifestHash, Valid: ballot.Valid})
				return nil
			}, nil)
		}, OnReview: func(review delegate.Review) error {
			return m.update(id, "issue.updated", func(r *model.StoredRun) error {
				refs := review.ResolutionRefs
				if refs == nil {
					refs = []string{}
				}
				severity := review.Severity
				if severity == "suggestion" {
					severity = "info"
				}
				if severity != "info" && severity != "warning" && severity != "blocking" {
					severity = "warning"
				}
				issue := model.ReviewIssue{ID: review.ID, ClaimID: review.ClaimID, RaisedBy: review.RaisedBy, Severity: severity, Status: normalizedIssueStatus(review.Status), Statement: review.Statement, ResolutionRefs: refs}
				if issue.Status == "resolved" {
					existing := false
					for _, old := range r.Snapshot.Reviews {
						if old.ID == issue.ID {
							existing = old.RaisedBy == review.RaisedBy
							issue.RaisedBy = old.RaisedBy
							issue.Statement = old.Statement
							issue.Severity = old.Severity
							if issue.ClaimID == "" {
								issue.ClaimID = old.ClaimID
							}
							break
						}
					}
					if !existing || !validResolutionRefs(r, refs, review.InvocationID) {
						issue.Status = "open"
						issue.ResolutionRefs = []string{}
					}
				}
				if issue.ID == "" {
					issue.ID = newID("issue")
				}
				if r.ReviewSources == nil {
					r.ReviewSources = map[string]model.ReviewSource{}
				}
				exclusive := review.ExclusiveWith
				if exclusive == nil {
					exclusive = []string{}
				}
				r.ReviewSources[issue.ID] = model.ReviewSource{InvocationID: review.InvocationID, ClaimDraftIndex: review.ClaimDraftIndex, DraftStatement: review.DraftStatement, AttemptID: attempt, OriginalClaimID: review.ClaimID, ExclusiveWith: exclusive}
				for i, old := range r.Snapshot.Reviews {
					if old.ID == issue.ID {
						r.Snapshot.Reviews[i] = issue
						return nil
					}
				}
				r.Snapshot.Reviews = append(r.Snapshot.Reviews, issue)
				return nil
			}, nil)
		}})
	defer d.Close()
	if r.Snapshot.Proposal == nil {
		proposer := workflow.RolePlanFor(r.Frozen).Proposer
		content, err := d.ProposeP0For(ctx, proposer, r.Snapshot.Goal, workflow.P0Materials(*r))
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		proposal := model.Proposal{ID: id + "-p0", Title: workflow.AgentLabel(proposer) + " 初稿 P0", Content: content, Hash: workflow.Hash(content), Version: 0, CreatedAt: now(), Kind: "initial"}
		deadline, _ := time.Parse(time.RFC3339Nano, r.Snapshot.DeadlineAt)
		request, err := workflow.Request(*r, proposal, time.Until(deadline))
		if err != nil {
			return err
		}
		err = m.update(id, "proposal.created", func(r *model.StoredRun) error {
			commitP0Message(r, &proposal, attempt)
			r.Snapshot.Proposal = &proposal
			r.Snapshot.ProposalHistory = append(r.Snapshot.ProposalHistory, proposal)
			r.Request = &request
			requestBody, _ := json.Marshal(request)
			r.Frozen.RequestHash = workflow.Hash(string(requestBody))
			configBody, _ := json.Marshal(r.Frozen)
			r.ConfigHash = workflow.Hash(string(configBody))
			r.Snapshot.SessionID = "session-" + id
			r.Snapshot.HostPhase = "engine_running"
			r.Snapshot.Phase = "frame"

			return nil
		}, map[string]any{"proposalId": proposal.ID, "hash": proposal.Hash})
		if err != nil {
			return err
		}
		r, err = m.stored(id)
		if err != nil {
			return err
		}
	}
	if err = writeJSON(filepath.Join(dir, "run-config.json"), map[string]any{"config": r.Frozen, "configHash": r.ConfigHash, "request": r.Request}); err != nil {
		return err
	}
	snapshot, err := m.loadSession(r)
	if err != nil {
		return err
	}
	if snapshot != nil && snapshot.Phase == consensus.SessionPhaseFinished && snapshot.Result != nil {
		return m.finalize(id, snapshot.Result)
	}
	if r.Request == nil {
		return fmt.Errorf("已保存的 P0 缺少冻结内核请求")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	engine := consensus.NewEngine(consensus.EngineDeps{TaskDelegate: &boundDelegate{base: d, manager: m, runID: id}, SessionStore: &trackedStore{base: m.sessions, manager: m, runID: id}, Observer: &engineObserver{manager: m, runID: id}, Ledger: observer.NewLedger(filepath.Join(dir, "ledger.jsonl"), filepath.Join(dir, "artifact-manifest.jsonl")), IDFactory: stableIDs{session: r.Snapshot.SessionID}, ArtifactDir: artifactDir})
	var result *consensus.RunResult
	if snapshot == nil {
		result, err = engine.Start(ctx, *r.Request)
	} else {
		result, err = engine.Resume(ctx, *snapshot)
	}
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return m.finalize(id, result)
}

func (m *Manager) finalize(id string, result *consensus.RunResult) error {
	if result == nil {
		return fmt.Errorf("内核没有返回结果")
	}
	record, err := m.stored(id)
	if err != nil {
		return err
	}
	if runtimeConfigChanged(record.StopReason) {
		return fmt.Errorf("%s", record.Snapshot.Error)
	}
	session, err := m.loadSession(record)
	if err != nil {
		return err
	}
	if session == nil || session.Phase != consensus.SessionPhaseFinished || session.Result == nil {
		return fmt.Errorf("最终判定需要已完成的内核快照")
	}
	evidence, err := m.loadEvidence(id)
	if err != nil {
		return err
	}
	if err := m.update(id, "run.finalizing", func(r *model.StoredRun) error {
		r.Snapshot.HostPhase = "finalizing"
		r.Snapshot.Evidence = evidence
		resolveReviewSources(r, session)
		return nil
	}, nil); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(m.runDir(id), "result.json"), result); err != nil {
		return err
	}
	return m.update(id, "decision.finalized", func(r *model.StoredRun) error {
		if runtimeConfigChanged(r.StopReason) {
			return fmt.Errorf("%s", r.Snapshot.Error)
		}
		r.CoreResult = result
		workflow.Finalize(&r.Snapshot, result)
		applyBallotGate(r, result)
		applyEvidenceGate(r, session)
		accepted := map[string]bool{}
		for _, claim := range r.Snapshot.Claims {
			if claim.Accepted != nil && *claim.Accepted {
				accepted[claim.ID] = true
			}
		}
		for _, issue := range r.Snapshot.Reviews {
			for _, other := range r.ReviewSources[issue.ID].ExclusiveWith {
				if accepted[issue.ClaimID] && accepted[canonicalClaimID(r, other)] {
					r.Snapshot.Decision.Outcome = "needs_human"
					r.Snapshot.Decision.Summary += " 存在同时获支持的互斥主张。"
				}
			}
		}
		if err := writeJSON(filepath.Join(m.runDir(id), "decision.json"), r.Snapshot.Decision); err != nil {
			return err
		}
		for i := range r.Snapshot.Messages {
			r.Snapshot.Messages[i].Streaming = false
		}
		r.Snapshot.HostPhase = "completed"
		r.Snapshot.Phase = "finished"
		r.Snapshot.Status = "completed"
		r.Snapshot.Error = ""
		message := model.ChatMessage{ID: id + "-decision-" + r.Frozen.PolicyVersion, AgentID: "coordinator", Role: "coordinator", Label: "协调器", Content: r.Snapshot.Decision.Summary, At: now(), Phase: "finished", AttemptID: r.Snapshot.AttemptID, Kind: "decision"}
		for i, old := range r.Snapshot.Messages {
			if old.ID == message.ID {
				r.Snapshot.Messages[i] = message
				return nil
			}
		}
		r.Snapshot.Messages = append(r.Snapshot.Messages, message)
		return nil
	}, nil)
}

func (m *Manager) reconcile() error {
	runs, err := m.store.All(context.Background())
	if err != nil {
		return err
	}
	for i := range runs {
		r := &runs[i]
		if r.Snapshot.DeletedAt != "" {
			continue
		}
		if r.Snapshot.Status == "completed" {
			continue
		}
		if runtimeConfigChanged(r.StopReason) {
			// Never finalize over an explicit runtime configuration failure, even if
			// the kernel committed a finished checkpoint just before cancellation.
			continue
		}
		if !compatiblePolicyVersion(r.Frozen.PolicyVersion) {
			if r.Snapshot.ProviderMode == "" {
				r.Snapshot.ProviderMode = r.Frozen.ProviderMode
			}
			if r.Snapshot.ProviderMode == "" {
				r.Snapshot.ProviderMode = "mock"
			}
			r.Snapshot.Status = "interrupted"
			r.Snapshot.Error = "历史版本仅供查看，请创建关联讨论使用当前接口"
			if err := m.saveLocked(r, "state.legacy", nil, "", "", ""); err != nil {
				return err
			}
			continue
		}
		if err = validateFrozen(r); err != nil {
			r.Snapshot.Status = "failed"
			r.Snapshot.Error = err.Error()
			if err = m.saveLocked(r, "state.reconciled", map[string]any{"error": r.Snapshot.Error}, "", "", ""); err != nil {
				return err
			}
			continue
		}
		snapshot, err := m.loadSession(r)
		if err != nil {
			r.Snapshot.Status = "failed"
			r.Snapshot.Error = err.Error()
			if err = m.saveLocked(r, "state.reconciled", map[string]any{"error": r.Snapshot.Error}, "", "", ""); err != nil {
				return err
			}
			continue
		}
		if snapshot != nil && snapshot.Phase == consensus.SessionPhaseFinished && snapshot.Result != nil {
			if err = m.finalize(r.Snapshot.ID, snapshot.Result); err != nil {
				return err
			}
			if err = m.update(r.Snapshot.ID, "state.reconciled", nil, map[string]any{"reason": "已从完成的内核快照补齐桌面最终判定"}); err != nil {
				return err
			}
			continue
		}
		if r.Snapshot.Status == "running" || r.Snapshot.Status == "queued" {
			r.Snapshot.Status = "interrupted"
			for i := range r.Snapshot.Messages {
				r.Snapshot.Messages[i].Streaming = false
			}
			r.StopReason = "shutdown"
			if snapshot != nil && snapshot.Checkpoint != nil {
				r.Snapshot.Checkpoint = checkpointLabel(snapshot.Checkpoint)
			}
			if err = m.saveLocked(r, "state.reconciled", map[string]any{"reason": "后台重新启动，从完整检查点继续"}, "", "", ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) stored(id string) (*model.StoredRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.store.Load(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if err := m.refreshChainSnapshotLocked(r); err != nil {
		return nil, err
	}
	return r, nil
}
func (m *Manager) loadSession(r *model.StoredRun) (*consensus.SessionSnapshot, error) {
	if r.Snapshot.SessionID == "" {
		return nil, nil
	}
	s, err := m.sessions.Load(context.Background(), r.Snapshot.SessionID)
	if err != nil || s == nil {
		return s, err
	}
	if s.SessionID != r.Snapshot.SessionID || s.RequestID != r.Snapshot.ID {
		return nil, fmt.Errorf("内核session映射与run不一致")
	}
	if s.Request != nil {
		body, _ := json.Marshal(s.Request)
		if workflow.Hash(string(body)) != r.Frozen.RequestHash {
			return nil, fmt.Errorf("内核冻结请求与宿主快照不一致")
		}
	}
	return s, nil
}
func (m *Manager) update(id, eventType string, fn func(*model.StoredRun) error, payload map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.store.Load(context.Background(), id)
	if err != nil {
		return err
	}
	if fn != nil {
		if err = fn(r); err != nil {
			return err
		}
	}
	return m.saveLocked(r, eventType, payload, "", "", "")
}
func (m *Manager) saveLocked(r *model.StoredRun, eventType string, payload map[string]any, key, action, fingerprint string) error {
	r.Snapshot.UpdatedAt = now()
	if payload == nil {
		payload = map[string]any{}
	}
	e := model.DesktopEvent{RunID: r.Snapshot.ID, AttemptID: r.Snapshot.AttemptID, Type: eventType, At: r.Snapshot.UpdatedAt, Payload: payload}
	if err := m.store.Save(context.Background(), r, &e, key, action, fingerprint); err != nil {
		return err
	}
	if m.opt.Notify != nil {
		m.opt.Notify(e)
	}
	return nil
}
func (m *Manager) runDir(id string) string { return filepath.Join(m.opt.DataDir, "runs", id) }
func now() string                          { return time.Now().UTC().Format(time.RFC3339Nano) }
func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
func validKey(key string) bool { return strings.TrimSpace(key) != "" && len(key) <= 200 }
func validateFrozen(r *model.StoredRun) error {
	b, _ := json.Marshal(r.Frozen)
	if r.ConfigHash != workflow.Hash(string(b)) || r.Frozen.EngineVersion != model.EngineVersion || !compatiblePolicyVersion(r.Frozen.PolicyVersion) || (r.Frozen.ProviderMode != "mock" && r.Frozen.ProviderMode != "cli") || r.Frozen.GoalHash != workflow.Hash(r.Snapshot.Goal) || r.Frozen.Mode != r.Snapshot.Mode || r.Frozen.DeadlineAt != r.Snapshot.DeadlineAt {
		return fmt.Errorf("冻结配置不兼容或已损坏，请新建讨论")
	}
	if r.Frozen.ChainRootID != "" && r.Frozen.ChainRootID != r.Snapshot.LineageRootID {
		return fmt.Errorf("冻结关联链标识已损坏，请新建讨论")
	}
	if (r.ParentMaterial != "" || r.Frozen.ParentMaterialHash != "") && workflow.Hash(r.ParentMaterial) != r.Frozen.ParentMaterialHash {
		return fmt.Errorf("父讨论冻结材料已损坏，请重新创建关联讨论")
	}
	if (r.Frozen.MaterialsHash != "" || len(r.Snapshot.Materials) > 0) && materialsHash(r.Snapshot.Materials) != r.Frozen.MaterialsHash {
		return fmt.Errorf("冻结材料已改变，请重新分析并新建讨论")
	}
	if (r.Frozen.PreparationHash != "" || r.Snapshot.Preparation != nil) && preparationHash(r.Snapshot.Preparation) != r.Frozen.PreparationHash {
		return fmt.Errorf("冻结准备结果已改变，请新建讨论")
	}
	if r.Frozen.RequestedMode != r.Snapshot.RequestedMode {
		return fmt.Errorf("冻结的模式选择已改变，请新建讨论")
	}
	if r.Frozen.RolePlan != nil {
		expected, err := workflow.NewRolePlan(r.Frozen.Providers)
		if err != nil {
			return fmt.Errorf("冻结参与者无效：%w", err)
		}
		frozenBody, _ := json.Marshal(r.Frozen.RolePlan)
		expectedBody, _ := json.Marshal(expected)
		snapshotBody, _ := json.Marshal(r.Snapshot.RolePlan)
		if string(frozenBody) != string(expectedBody) || string(snapshotBody) != string(frozenBody) {
			return fmt.Errorf("冻结的参与者或角色已改变，请新建讨论")
		}
	} else if r.Snapshot.RolePlan != nil {
		return fmt.Errorf("历史讨论不能注入新的角色配置，请新建讨论")
	}
	if r.Request != nil {
		body, _ := json.Marshal(r.Request)
		if workflow.Hash(string(body)) != r.Frozen.RequestHash {
			return fmt.Errorf("冻结内核请求已损坏，请新建讨论")
		}
	}
	return nil
}
func messageKind(kind string) string {
	switch kind {
	case "propose", "initial_proposal", "synthesis":
		return "proposal"
	case "arbitrate", "final_vote":
		return "decision"
	case "report", "delphi_facilitator_summary":
		return "system"
	default:
		return "review"
	}
}
func writeJSON(path string, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(body, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func normalizedIssueStatus(status string) string {
	if status == "resolved" {
		return status
	}
	return "open"
}
func canonicalClaimID(r *model.StoredRun, id string) string {
	seen := map[string]bool{}
	for r.ClaimAliases[id] != "" && !seen[id] {
		seen[id] = true
		id = r.ClaimAliases[id]
	}
	return id
}
