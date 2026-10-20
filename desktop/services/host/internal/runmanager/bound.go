package runmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

// boundDelegate freezes the desktop review context at a stage/round boundary,
// before fan-out. Core task scheduling remains owned by til-consensus.
type boundDelegate struct {
	base    consensus.TaskDelegate
	manager *Manager
	runID   string
}

func (d *boundDelegate) Dispatch(ctx context.Context, task consensus.Task) (consensus.DispatchReceipt, error) {
	frozen, err := d.manager.bindTask(d.runID, task)
	if err != nil {
		return consensus.DispatchReceipt{}, err
	}
	return d.base.Dispatch(ctx, frozen)
}
func (d *boundDelegate) Await(ctx context.Context, id string, timeout time.Duration) (consensus.AwaitedTask, error) {
	return d.base.Await(ctx, id, timeout)
}
func (d *boundDelegate) Cancel(ctx context.Context, id string) error { return d.base.Cancel(ctx, id) }

func (m *Manager) bindTask(id string, task consensus.Task) (consensus.Task, error) {
	var meta consensus.TaskMeta
	err := m.update(id, "context.frozen", func(r *model.StoredRun) error {
		meta = task.Meta()
		// Never mutate the kernel's shared metadata or claim slices.
		meta.Metadata = cloneMap(meta.Metadata)
		round := taskRound(task)
		key := fmt.Sprintf("%s:%d", task.Kind(), round)
		if r.ReviewContexts == nil {
			r.ReviewContexts = map[string]map[string]any{}
		}
		reviewContext, ok := r.ReviewContexts[key]
		if !ok {
			issues := any(r.Snapshot.Reviews)
			if r.Snapshot.Mode == "delphi" {
				anonymous := []map[string]any{}
				for _, issue := range r.Snapshot.Reviews {
					anonymous = append(anonymous, map[string]any{"severity": issue.Severity, "status": issue.Status, "statement": anonymousStatement(issue.Statement)})
				}
				issues = anonymous
			}
			body, _ := json.Marshal(issues)
			var reviews []any
			_ = json.Unmarshal(body, &reviews)
			reviewContext = map[string]any{"round": round, "phase": string(task.Kind()), "issues": reviews, "versionHash": workflow.Hash(string(body))}
			r.ReviewContexts[key] = reviewContext
		}
		meta.Metadata["desktopReviewContext"] = reviewContext
		if vote, ok := task.(consensus.FinalVoteTask); ok {
			canonical := ""
			for _, claim := range vote.Claims {
				if claim.Active && claim.Category == consensus.DebateClaimCategorySynthesis {
					if canonical != "" {
						return fmt.Errorf("最终投票需要唯一综合方案")
					}
					canonical = claim.Statement
				}
			}
			if canonical == "" {
				return fmt.Errorf("最终投票缺少综合方案，不可批准初稿")
			}
			appendProposal(&r.Snapshot, canonical, "synthesis", "")
			manifestHash := ballotManifestHash(vote.Claims)
			meta.Metadata["proposalHash"] = r.Snapshot.Proposal.Hash
			meta.Metadata["ballotManifestHash"] = manifestHash
			meta.Metadata["proposalVersionId"] = r.Snapshot.Proposal.ID
		}
		return nil
	}, map[string]any{"taskKind": string(task.Kind()), "round": taskRound(task)})
	if err != nil {
		return nil, err
	}
	// All public core task structs embed TaskMeta by value.
	v := reflect.New(reflect.TypeOf(task)).Elem()
	v.Set(reflect.ValueOf(task))
	field := v.FieldByName("TaskMeta")
	if !field.IsValid() || !field.CanSet() {
		return nil, fmt.Errorf("任务缺少可冻结的元数据: %T", task)
	}
	field.Set(reflect.ValueOf(meta))
	frozen := v.Interface().(consensus.Task)
	if task.Kind() == consensus.TaskKindDelphiQuestionnaire || task.Kind() == consensus.TaskKindDelphiRevision || task.Kind() == consensus.TaskKindDelphiFacilitatorSummary {
		frozen = workflow.AnonymousDelphiTask(frozen)
	}
	return frozen, nil
}
func cloneMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func taskRound(task consensus.Task) int {
	v := reflect.ValueOf(task)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if f := v.FieldByName("Round"); f.IsValid() && f.Kind() == reflect.Int {
		return int(f.Int())
	}
	return 0
}
func ballotManifestHash(claims []consensus.DebateClaim) string {
	type item struct {
		ClaimID   string `json:"claimId"`
		Statement string `json:"statement"`
		Category  string `json:"category"`
	}
	items := []item{}
	for _, c := range claims {
		if c.Active {
			items = append(items, item{c.ClaimID, c.Statement, string(c.Category)})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ClaimID < items[j].ClaimID })
	b, _ := json.Marshal(items)
	return workflow.Hash(string(b))
}
func appendProposal(s *model.RunSnapshot, content, kind, invocation string) {
	if content == "" || (s.Proposal != nil && s.Proposal.Content == content) {
		return
	}
	version := 0
	if s.Proposal != nil {
		version = s.Proposal.Version + 1
	}
	p := model.Proposal{ID: fmt.Sprintf("%s-p%d", s.ID, version), Title: fmt.Sprintf("综合方案 P%d", version), Content: content, Hash: workflow.Hash(content), Version: version, CreatedAt: now(), InvocationID: invocation, Kind: kind}
	s.Proposal = &p
	s.ProposalHistory = append(s.ProposalHistory, p)
}

func applyBallotGate(r *model.StoredRun, result *consensus.RunResult) {
	if r.Snapshot.Decision == nil {
		return
	}
	if result.FreeDebate == nil {
		applyIdentityGate(r)
		return
	}
	d := r.Snapshot.Decision
	manifest := ballotManifestHash(result.FreeDebate.Claims)
	d.ManifestHash = manifest
	if r.Snapshot.Proposal != nil {
		d.ProposalHash = r.Snapshot.Proposal.Hash
	}
	bindings := map[string]bool{}
	for _, b := range r.Snapshot.Ballots {
		if b.Valid && b.ProposalHash == d.ProposalHash && b.ManifestHash == manifest {
			bindings[b.AgentID] = true
		}
	}
	for _, id := range workflow.RolePlanFor(r.Frozen).Reviewers {
		if !bindings[id] {
			if d.Outcome == "supported" {
				d.Outcome = "needs_human"
			}
			d.Summary += " 最终票缺少有效版本绑定：" + id + "。"
		}
	}
	applyIdentityGate(r)
}
func applyIdentityGate(r *model.StoredRun) {
	if r.Snapshot.ProviderMode == "mock" || r.Snapshot.Decision == nil {
		return
	}
	d := r.Snapshot.Decision
	identities := map[string]string{}
	for _, id := range workflow.RolePlanFor(r.Frozen).Reviewers {
		identity := normalizeModel(r.ModelBindings[id].Model)
		if identity == "" {
			for _, meta := range r.ActualMetadata {
				if agent, _ := meta["agentId"].(string); agent == id {
					if actual, _ := meta["model"].(string); actual != "" {
						identity = normalizeModel(actual)
					}
				}
			}
		}
		if identity == "" {
			for _, p := range r.Frozen.Providers {
				if p.ID == id {
					identity = normalizeModel(p.Model)
				}
			}
		}
		if identity == "" {
			if d.Outcome == "supported" {
				d.Outcome = "needs_human"
			}
			d.Summary += " 无法确认独立模型身份：" + id + "。"
			continue
		}
		if other := identities[identity]; other != "" {
			d.Outcome = "needs_human"
			d.Summary += " " + id + " 与 " + other + " 使用同一模型，不计为独立票。"
		} else {
			identities[identity] = id
		}
	}
}
func normalizeModel(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func anonymousStatement(s string) string {
	return workflow.AnonymousPeerText(s)
}

func applyEvidenceGate(r *model.StoredRun, session *consensus.SessionSnapshot) {
	if r.Snapshot.Decision == nil || r.Snapshot.Mode != "adjudication" || r.Snapshot.ProviderMode == "mock" {
		return
	}
	for _, c := range session.ClaimGraph {
		if c.ClaimType != consensus.ClaimTypeFact || c.Verdict != consensus.ClaimVerdictSupported {
			continue
		}
		proven := false
		for _, v := range session.VerificationResults {
			if v.ClaimID == c.ClaimID && v.Kind != "semantic" && v.Kind != "" && v.Status == consensus.VerificationStatusPassed {
				proven = true
				break
			}
		}
		if !proven {
			if r.Snapshot.Decision.Outcome == "supported" {
				r.Snapshot.Decision.Outcome = "insufficient_evidence"
			}
			r.Snapshot.Decision.Assumptions = append(r.Snapshot.Decision.Assumptions, "事实主张尚无可复现验证："+c.Statement)
			r.Snapshot.Decision.Summary += " " + c.ClaimID + " 仅获模型语义支持，未记为事实验证通过。"
		}
	}
}
