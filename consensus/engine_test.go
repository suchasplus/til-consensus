package consensus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type stepClock struct {
	mu    sync.Mutex
	times []time.Time
	idx   int
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.times) == 0 {
		return time.Unix(0, 0).UTC()
	}
	if c.idx >= len(c.times) {
		return c.times[len(c.times)-1]
	}
	value := c.times[c.idx]
	c.idx++
	return value
}

type deterministicIDs struct {
	mu sync.Mutex
	n  int
}

func (i *deterministicIDs) NewSessionID() string { return "session-1" }
func (i *deterministicIDs) NewEntityID(prefix string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.n++
	return fmt.Sprintf("%s-%d", prefix, i.n)
}

type memoryLedger struct {
	entries []EvidenceRecord
}

func (l *memoryLedger) Append(_ context.Context, entry EvidenceRecord) (EvidenceRecord, error) {
	entry.Seq = len(l.entries)
	l.entries = append(l.entries, entry)
	return entry, nil
}

type captureObserver struct {
	mu     sync.Mutex
	events []RunEventType
}

func (o *captureObserver) OnEvent(_ context.Context, event RunEvent) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, event.Type)
	return nil
}

type stubStore struct {
	snapshot SessionSnapshot
}

func (s *stubStore) Save(_ context.Context, snapshot SessionSnapshot) error {
	s.snapshot = snapshot
	return nil
}

func (s *stubStore) Load(_ context.Context, _ string) (*SessionSnapshot, error) {
	cloned := s.snapshot
	return &cloned, nil
}

func (s *stubStore) Patch(_ context.Context, _ string, patch SessionPatch) error {
	if patch.Phase != nil {
		s.snapshot.Phase = *patch.Phase
	}
	if patch.Checkpoint != nil {
		s.snapshot.Checkpoint = patch.Checkpoint
	}
	if patch.CaseManifest != nil {
		s.snapshot.CaseManifest = patch.CaseManifest
	}
	if patch.ClaimGraph != nil {
		s.snapshot.ClaimGraph = append([]ClaimNode(nil), patch.ClaimGraph...)
	}
	if patch.ChallengeTickets != nil {
		s.snapshot.ChallengeTickets = append([]ChallengeTicket(nil), patch.ChallengeTickets...)
	}
	if patch.LedgerEntries != nil {
		s.snapshot.LedgerEntries = append([]EvidenceRecord(nil), patch.LedgerEntries...)
	}
	if patch.LedgerCursor != nil {
		s.snapshot.LedgerCursor = *patch.LedgerCursor
	}
	if patch.VerificationResults != nil {
		s.snapshot.VerificationResults = append([]VerificationResult(nil), patch.VerificationResults...)
	}
	if patch.RevisionRecords != nil {
		s.snapshot.RevisionRecords = append([]ClaimRevisionRecord(nil), patch.RevisionRecords...)
	}
	if patch.AdjudicationRecords != nil {
		s.snapshot.AdjudicationRecords = append([]AdjudicationRecord(nil), patch.AdjudicationRecords...)
	}
	if patch.Observations != nil {
		s.snapshot.Observations = append([]ObservationRecord(nil), patch.Observations...)
	}
	if patch.Metrics != nil {
		s.snapshot.Metrics = *patch.Metrics
	}
	if patch.ResumeCount != nil {
		s.snapshot.ResumeCount = *patch.ResumeCount
	}
	if patch.LastResumedAt != nil {
		s.snapshot.LastResumedAt = *patch.LastResumedAt
	}
	if patch.ArbiterReport != nil {
		s.snapshot.ArbiterReport = patch.ArbiterReport
	}
	if patch.Report != nil {
		s.snapshot.Report = patch.Report
	}
	if patch.Action != nil {
		s.snapshot.Action = patch.Action
	}
	if patch.Result != nil {
		s.snapshot.Result = patch.Result
	}
	if patch.Error != nil {
		s.snapshot.Error = patch.Error
	}
	return nil
}

type stubDelegate struct {
	mu              sync.Mutex
	tasks           map[string]Task
	next            int
	failActionType  bool
	challengeDrafts []ChallengeDraft
	revisionDrafts  []ClaimRevisionDraft
	debateDrafts    []ClaimDraft
	failKinds       map[TaskKind]int
	failAgentKinds  map[string]int // key: agentID+"/"+kind, value: remaining forced failures
	mergeFirstTwo   bool
}

func (d *stubDelegate) Dispatch(_ context.Context, task Task) (DispatchReceipt, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tasks == nil {
		d.tasks = map[string]Task{}
	}
	taskID := fmt.Sprintf("task-%d", d.next)
	d.next++
	d.tasks[taskID] = task
	return DispatchReceipt{TaskID: taskID, AgentID: task.Meta().AgentID, Kind: task.Kind()}, nil
}

func (d *stubDelegate) Await(_ context.Context, taskID string, _ time.Duration) (AwaitedTask, error) {
	d.mu.Lock()
	task := d.tasks[taskID]
	failActionType := d.failActionType
	challengeDrafts := append([]ChallengeDraft(nil), d.challengeDrafts...)
	revisionDrafts := append([]ClaimRevisionDraft(nil), d.revisionDrafts...)
	debateDrafts := append([]ClaimDraft(nil), d.debateDrafts...)
	mergeFirstTwo := d.mergeFirstTwo
	if d.failKinds != nil {
		if remaining := d.failKinds[task.Kind()]; remaining > 0 {
			d.failKinds[task.Kind()] = remaining - 1
			d.mu.Unlock()
			return AwaitedTask{OK: false, Error: "forced failure"}, nil
		}
	}
	if d.failAgentKinds != nil {
		key := task.Meta().AgentID + "/" + string(task.Kind())
		if remaining := d.failAgentKinds[key]; remaining > 0 {
			d.failAgentKinds[key] = remaining - 1
			d.mu.Unlock()
			return AwaitedTask{OK: false, Error: "forced agent failure"}, nil
		}
	}
	d.mu.Unlock()
	switch value := task.(type) {
	case ProposalTask:
		return AwaitedTask{OK: true, Output: ProposalTaskResult{Output: ProposalOutput{
			Summary: "proposal",
			Claims: []ClaimDraft{{
				Title:     "Race fix",
				Statement: "The patch fixes the race condition",
				Metadata:  map[string]any{"touchedPaths": []string{"consensus/engine.go"}},
			}},
		}}}, nil
	case ChallengeTask:
		drafts := challengeDrafts
		if len(drafts) == 0 {
			drafts = []ChallengeDraft{{
				ClaimID:   value.Claims[0].ClaimID,
				Statement: "Need more evidence",
				Kind:      "evidence-gap",
			}}
		} else {
			copied := make([]ChallengeDraft, 0, len(drafts))
			for _, draft := range drafts {
				if strings.TrimSpace(draft.ClaimID) == "" && len(value.Claims) > 0 {
					draft.ClaimID = value.Claims[0].ClaimID
				}
				copied = append(copied, draft)
			}
			drafts = copied
		}
		return AwaitedTask{OK: true, Output: ChallengeTaskResult{Output: ChallengeOutput{
			Summary: "challenge",
			Tickets: drafts,
		}}}, nil
	case ReviseTask:
		drafts := revisionDrafts
		if len(drafts) == 0 {
			drafts = []ClaimRevisionDraft{{
				TargetClaimID:   value.Claims[0].ClaimID,
				Action:          RevisionActionDowngrade,
				ConfidenceDelta: -0.1,
				Caveats:         []string{"需要更多证据"},
				Reason:          "default revision",
			}}
		} else {
			copied := make([]ClaimRevisionDraft, 0, len(drafts))
			for _, draft := range drafts {
				if strings.TrimSpace(draft.TargetClaimID) == "" && len(value.Claims) > 0 {
					draft.TargetClaimID = value.Claims[0].ClaimID
				}
				copied = append(copied, draft)
			}
			drafts = copied
		}
		return AwaitedTask{OK: true, Output: ReviseTaskResult{Output: ReviseOutput{
			Summary:   "revision",
			Revisions: drafts,
		}}}, nil
	case InitialProposalTask:
		return AwaitedTask{OK: true, Output: InitialProposalTaskResult{Output: InitialProposalOutput{
			Summary: "initial proposal",
			Claims: []ClaimDraft{{
				Title:     "Initial claim",
				Statement: fmt.Sprintf("%s initial claim", value.AgentID),
			}},
		}}}, nil
	case DebateRoundTask:
		judgements := make([]DebateJudgementDraft, 0, len(value.PeerClaims))
		for _, claim := range value.PeerClaims {
			judgements = append(judgements, DebateJudgementDraft{
				ClaimID:   claim.ClaimID,
				Judgement: DebateJudgementAgree,
				Rationale: "agree",
			})
		}
		return AwaitedTask{OK: true, Output: DebateRoundTaskResult{Output: DebateRoundOutput{
			Summary:    "debate round",
			NewClaims:  debateDrafts,
			Judgements: judgements,
		}}}, nil
	case SemanticDedupTask:
		merges := []DebateClaimMergeDraft(nil)
		if mergeFirstTwo && len(value.Claims) >= 2 {
			merges = []DebateClaimMergeDraft{{
				SourceClaimID: value.Claims[1].ClaimID,
				TargetClaimID: value.Claims[0].ClaimID,
				Similarity:    value.SimilarityThreshold,
				Rationale:     "same practical position",
			}}
		}
		return AwaitedTask{OK: true, Output: SemanticDedupTaskResult{Output: SemanticDedupOutput{
			Summary: "semantic dedup",
			Merges:  merges,
		}}}, nil
	case FinalVoteTask:
		votes := make([]DebateVoteDraft, 0, len(value.Claims))
		for _, claim := range value.Claims {
			confidence := 1.0
			votes = append(votes, DebateVoteDraft{
				ClaimID:    claim.ClaimID,
				Vote:       DebateVoteAccept,
				Confidence: &confidence,
				Rationale:  "accept",
			})
		}
		return AwaitedTask{OK: true, Output: FinalVoteTaskResult{Output: FinalVoteOutput{
			Summary: "final vote",
			Votes:   votes,
		}}}, nil
	case ArbiterTask:
		decisions := make([]ArbiterDecision, 0, len(value.Claims))
		records := make([]AdjudicationRecord, 0, len(value.Claims))
		for _, claim := range value.Claims {
			decisions = append(decisions, ArbiterDecision{
				ClaimID:    claim.ClaimID,
				Verdict:    ClaimVerdictSupported,
				Confidence: claim.Confidence,
				Rationale:  "supported by stub arbiter",
			})
			records = append(records, AdjudicationRecord{
				TargetClaimID:   claim.ClaimID,
				Disposition:     ClaimDispositionKeep,
				Rationale:       "kept by stub arbiter",
				FinalConfidence: claim.Confidence,
				Actionability:   ActionabilityReady,
			})
		}
		return AwaitedTask{OK: true, Output: ArbiterTaskResult{Output: ArbiterTaskOutput{
			Summary:     "arbiter",
			TaskVerdict: TaskVerdictSupported,
			Decisions:   decisions,
			Records:     records,
		}}}, nil
	case ReportTask:
		return AwaitedTask{OK: true, Output: ReportTaskResult{Output: AdjudicationReport{Summary: "report"}}}, nil
	case DelphiQuestionnaireTask:
		return AwaitedTask{OK: true, Output: DelphiQuestionnaireTaskResult{Output: DelphiQuestionnaireOutput{
			Summary: "delphi questionnaire",
			Responses: []DelphiResponseDraft{{
				Statement: "Use monorepo",
				Rating:    4,
				Rationale: "better coordination",
			}},
		}}}, nil
	case DelphiRevisionTask:
		statementID := ""
		if len(value.StatementSummaries) > 0 {
			statementID = value.StatementSummaries[0].StatementID
		}
		return AwaitedTask{OK: true, Output: DelphiRevisionTaskResult{Output: DelphiRevisionOutput{
			Summary: "delphi revision",
			Responses: []DelphiResponseDraft{{
				StatementID: statementID,
				Rating:      5,
				Rationale:   "stronger support",
			}},
		}}}, nil
	case DelphiFacilitatorSummaryTask:
		return AwaitedTask{OK: true, Output: DelphiFacilitatorSummaryTaskResult{Output: DelphiFacilitatorSummaryOutput{
			Summary:        "facilitator summary",
			Recommendation: "Use monorepo",
		}}}, nil
	case ActionTask:
		if failActionType {
			return AwaitedTask{OK: true, Output: ProposalTaskResult{}}, nil
		}
		return AwaitedTask{OK: true, Output: ActionTaskResult{Output: ActionExecution{FullResponse: "done", Summary: "done"}}}, nil
	default:
		return AwaitedTask{OK: false, Error: "unexpected task"}, nil
	}
}

func (d *stubDelegate) Cancel(_ context.Context, _ string) error { return nil }

type barrierDelegate struct {
	mu            sync.Mutex
	tasks         map[string]Task
	next          int
	want          int
	allDispatched chan struct{}
	closed        bool
}

func newBarrierDelegate(want int) *barrierDelegate {
	return &barrierDelegate{
		want:          want,
		allDispatched: make(chan struct{}),
	}
}

func (d *barrierDelegate) Dispatch(_ context.Context, task Task) (DispatchReceipt, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tasks == nil {
		d.tasks = map[string]Task{}
	}
	taskID := fmt.Sprintf("task-%d", d.next)
	d.next++
	d.tasks[taskID] = task
	if d.next >= d.want && !d.closed {
		close(d.allDispatched)
		d.closed = true
	}
	return DispatchReceipt{TaskID: taskID, AgentID: task.Meta().AgentID, Kind: task.Kind()}, nil
}

func (d *barrierDelegate) Await(_ context.Context, taskID string, timeout time.Duration) (AwaitedTask, error) {
	select {
	case <-d.allDispatched:
	case <-time.After(timeout):
		return AwaitedTask{OK: false, Error: "__timeout__"}, nil
	}
	d.mu.Lock()
	task := d.tasks[taskID]
	d.mu.Unlock()
	return AwaitedTask{OK: true, Output: ProposalTaskResult{Output: ProposalOutput{Summary: "ok " + task.Meta().AgentID}}}, nil
}

func (d *barrierDelegate) Cancel(_ context.Context, _ string) error { return nil }

type stubVerifier struct {
	status VerificationStatus
	kind   string
	name   string
}

func (v stubVerifier) Run(_ context.Context, req VerificationRequest) ([]VerificationResult, error) {
	kind := v.kind
	if kind == "" {
		kind = "allowed_paths"
	}
	name := v.name
	if name == "" {
		name = kind
	}
	return []VerificationResult{{
		VerificationID: "verify-1",
		ClaimID:        req.Claim.ClaimID,
		CheckName:      name,
		Kind:           kind,
		Status:         v.status,
		Summary:        "verification result",
	}}, nil
}

type sequenceArbiter struct {
	reports []ArbiterReport
	calls   int
}

func (a *sequenceArbiter) Decide(_ context.Context, input ArbiterInput) (ArbiterReport, error) {
	idx := a.calls
	a.calls++
	if len(a.reports) == 0 {
		return ArbiterReport{}, nil
	}
	if idx >= len(a.reports) {
		idx = len(a.reports) - 1
	}
	report := a.reports[idx]
	defaultClaimID := ""
	if len(input.Claims) > 0 {
		defaultClaimID = input.Claims[0].ClaimID
	}
	for i := range report.Decisions {
		if strings.TrimSpace(report.Decisions[i].ClaimID) == "" {
			report.Decisions[i].ClaimID = defaultClaimID
		}
	}
	for i := range report.Records {
		if strings.TrimSpace(report.Records[i].TargetClaimID) == "" {
			report.Records[i].TargetClaimID = defaultClaimID
		}
	}
	if len(report.Records) == 0 {
		report.Records = deriveRecordsFromDecisions(input.Request, input.Claims, report.Decisions)
	}
	if report.TaskVerdict == "" {
		report.TaskVerdict = DetermineTaskVerdict(ApplyDecisions(append([]ClaimNode(nil), input.Claims...), report.Decisions))
	}
	return report, nil
}

func baseRequest() StartRequest {
	return StartRequest{
		RequestID: "req-1",
		TaskSpec: TaskSpec{
			Goal: "verify patch",
			Constraints: TaskConstraints{
				AllowedPaths: []string{"consensus"},
			},
		},
		Roles: RoleAssignments{
			Proposers:   []string{"proposer-1"},
			Challengers: []string{"challenger-1"},
		},
		ProposalPolicy: ProposalPolicy{
			MaxPasses:          1,
			MaxClaimsPerWorker: 1,
		},
		VerificationPolicy: VerificationPolicy{
			RequiredChecks:    []VerificationCheck{{Name: "allowed", Kind: "allowed_paths"}},
			MaxParallelChecks: 1,
		},
		ArbiterPolicy: ArbiterPolicy{
			AllowUndetermined: true,
			BlindReview:       true,
		},
		ReportPolicy: ReportPolicy{Style: "builtin"},
		WaitingPolicy: WaitingPolicy{
			PerTaskTimeout: time.Second,
		},
	}
}

func requireAdjudicationSection(t *testing.T, result *RunResult) *AdjudicationResultSection {
	t.Helper()
	if result == nil || result.Adjudication == nil {
		t.Fatalf("expected adjudication result, got %#v", result)
	}
	return result.Adjudication
}

func TestEngineProducesSupportedVerdict(t *testing.T) {
	ids := &deterministicIDs{}
	observer := &captureObserver{}
	ledger := &memoryLedger{}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		Observer:     observer,
		Ledger:       ledger,
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    ids,
	})
	result, err := engine.Start(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if section.TaskVerdict != TaskVerdictSupported {
		t.Fatalf("expected supported verdict, got %s", section.TaskVerdict)
	}
	if len(section.ClaimGraph) != 1 || section.ClaimGraph[0].Verdict != ClaimVerdictSupported {
		t.Fatalf("unexpected claim graph: %#v", section.ClaimGraph)
	}
	if len(section.ChallengeTickets) != 1 || section.ChallengeTickets[0].Status != ChallengeStatusClosed {
		t.Fatalf("expected closed challenge, got %#v", section.ChallengeTickets)
	}
	if len(ledger.entries) == 0 {
		t.Fatal("expected ledger entries to be written")
	}
	if len(observer.events) == 0 || observer.events[0] != RunEventSessionStarted {
		t.Fatalf("unexpected event stream: %#v", observer.events)
	}
}

func TestExecuteTaskBatchDispatchesTasksBeforeAwait(t *testing.T) {
	delegate := newBarrierDelegate(3)
	engine := NewEngine(EngineDeps{
		TaskDelegate: delegate,
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	tasks := []Task{
		ProposalTask{TaskMeta: TaskMeta{SessionID: "session-1", RequestID: request.RequestID, AgentID: "proposer-1"}},
		ProposalTask{TaskMeta: TaskMeta{SessionID: "session-1", RequestID: request.RequestID, AgentID: "proposer-2"}},
		ProposalTask{TaskMeta: TaskMeta{SessionID: "session-1", RequestID: request.RequestID, AgentID: "proposer-3"}},
	}
	results := engine.executeTaskBatch(context.Background(), request, "session-1", tasks, time.Unix(1, 0).UTC(), 500*time.Millisecond, len(tasks))
	if len(results) != len(tasks) {
		t.Fatalf("expected %d results, got %d", len(tasks), len(results))
	}
	for idx, result := range results {
		if result.Err != nil || !result.Awaited.OK {
			t.Fatalf("result %d failed: err=%v awaited=%#v", idx, result.Err, result.Awaited)
		}
		if result.Task.Meta().AgentID != tasks[idx].Meta().AgentID {
			t.Fatalf("result order drifted at %d: got %s want %s", idx, result.Task.Meta().AgentID, tasks[idx].Meta().AgentID)
		}
	}
}

func TestEngineMarksFailedVerificationAsRefuted(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusFailed},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if section.TaskVerdict != TaskVerdictFailed {
		t.Fatalf("expected failed verdict, got %s", section.TaskVerdict)
	}
	if section.ClaimGraph[0].Verdict != ClaimVerdictRefuted {
		t.Fatalf("expected refuted claim, got %#v", section.ClaimGraph[0])
	}
}

func TestEngineKeepsChallengeOpenWhenRequestedChecksRemainUnresolved(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{
			challengeDrafts: []ChallengeDraft{{
				Statement:       "Need workspace proof",
				Kind:            "evidence-gap",
				RequestedChecks: []string{"workspace"},
			}},
		},
		Verifier:     stubVerifier{status: VerificationStatusPassed, kind: "allowed_paths", name: "allowed"},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if len(section.ChallengeTickets) != 1 || section.ChallengeTickets[0].Status != ChallengeStatusOpen {
		t.Fatalf("expected unresolved requested check to keep challenge open, got %#v", section.ChallengeTickets)
	}
}

func TestEngineRunsFreeDebateWorkflow(t *testing.T) {
	request := baseRequest()
	request.Mode = WorkflowModeFreeDebate
	request.Roles = RoleAssignments{
		Participants: []string{"debater-1", "debater-2"},
	}
	request.DebatePolicy = DebatePolicy{
		MinRounds:       1,
		MaxRounds:       2,
		VoteThreshold:   1.0,
		EnableEarlyStop: true,
		PeerContextMode: "summary+active_claims",
	}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.Mode != WorkflowModeFreeDebate || result.FreeDebate == nil {
		t.Fatalf("unexpected result mode: %#v", result)
	}
	if result.FreeDebate.Outcome != FreeDebateOutcomeConsensus {
		t.Fatalf("expected consensus outcome, got %#v", result.FreeDebate)
	}
	if len(result.FreeDebate.Rounds) < 2 {
		t.Fatalf("expected initial + debate rounds, got %#v", result.FreeDebate.Rounds)
	}
	if len(result.FreeDebate.Votes) == 0 {
		t.Fatalf("expected final votes, got %#v", result.FreeDebate)
	}
}

func TestFreeDebateSemanticDedupMergesClaimsBeforeVote(t *testing.T) {
	request := baseRequest()
	request.Mode = WorkflowModeFreeDebate
	request.Roles = RoleAssignments{
		Participants:    []string{"debater-1", "debater-2"},
		SemanticDeduper: "deduper-1",
	}
	request.DebatePolicy = DebatePolicy{
		MinRounds:       1,
		MaxRounds:       1,
		VoteThreshold:   1.0,
		EnableEarlyStop: true,
		PeerContextMode: "summary+active_claims",
		SemanticDedup: DebateSemanticDedupPolicy{
			Enabled:             true,
			SimilarityThreshold: 0.85,
		},
	}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{mergeFirstTwo: true},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.FreeDebate == nil {
		t.Fatal("expected free debate result")
	}
	var canonical *DebateClaim
	var merged *DebateClaim
	for idx := range result.FreeDebate.Claims {
		claim := &result.FreeDebate.Claims[idx]
		if claim.Active {
			canonical = claim
		} else if claim.MergedInto != "" {
			merged = claim
		}
	}
	if canonical == nil || merged == nil {
		t.Fatalf("expected one active canonical claim and one merged claim, got %#v", result.FreeDebate.Claims)
	}
	if merged.MergedInto != canonical.ClaimID {
		t.Fatalf("expected merged claim to point at canonical claim, got %s want %s", merged.MergedInto, canonical.ClaimID)
	}
	if !slices.Contains(canonical.ProposedBy, "debater-1") || !slices.Contains(canonical.ProposedBy, "debater-2") {
		t.Fatalf("expected provenance from both debaters, got %#v", canonical.ProposedBy)
	}
	if !slices.Contains(canonical.MergedClaimIDs, merged.ClaimID) {
		t.Fatalf("expected canonical mergedClaimIds to include merged claim, got %#v", canonical.MergedClaimIDs)
	}
	if len(result.FreeDebate.Votes) != len(request.Roles.Participants) {
		t.Fatalf("expected votes only on active canonical claim, got %#v", result.FreeDebate.Votes)
	}
}

func TestResolveDebateClaimsUsesConfidenceDistribution(t *testing.T) {
	claims := []DebateClaim{{
		ClaimID:   "claim-1",
		Statement: "Use a monorepo when cross-service changes dominate.",
		Active:    true,
	}}
	votes := []DebateVoteRecord{
		{ClaimID: "claim-1", AgentID: "voter-1", Vote: DebateVoteAccept, Confidence: 1.0},
		{ClaimID: "claim-1", AgentID: "voter-2", Vote: DebateVoteAccept, Confidence: 0.52},
		{ClaimID: "claim-1", AgentID: "voter-3", Vote: DebateVoteReject, Confidence: 0.10},
	}
	resolutions, outcome := resolveDebateClaims(claims, claims, votes, DebatePolicy{VoteThreshold: 0.67, VoteAggregation: DebateVoteAggregationMean})
	if outcome != FreeDebateOutcomeNoConsensus {
		t.Fatalf("expected no consensus from low confidence mean, got %s", outcome)
	}
	if len(resolutions) != 1 {
		t.Fatalf("expected one resolution, got %#v", resolutions)
	}
	resolution := resolutions[0]
	if resolution.Accepted {
		t.Fatalf("expected claim not accepted despite binary support ratio, got %#v", resolution)
	}
	if resolution.SupportRatio < 0.66 || resolution.SupportRatio > 0.67 {
		t.Fatalf("expected binary support ratio around 0.67, got %.4f", resolution.SupportRatio)
	}
	if resolution.ConfidenceMean < 0.53 || resolution.ConfidenceMean > 0.55 {
		t.Fatalf("expected confidence mean around 0.54, got %.4f", resolution.ConfidenceMean)
	}
	if resolution.ConfidenceVariance <= 0.1 {
		t.Fatalf("expected high variance from split confidence scores, got %.4f", resolution.ConfidenceVariance)
	}
	if resolution.VoteCount != 3 {
		t.Fatalf("expected vote count 3, got %d", resolution.VoteCount)
	}
}

func TestFreeDebateCanonicalizesStatusPrefixesBeforeDedup(t *testing.T) {
	request := baseRequest()
	request.Mode = WorkflowModeFreeDebate
	request.Roles = RoleAssignments{
		Participants: []string{"debater-1", "debater-2"},
	}
	request.DebatePolicy = DebatePolicy{
		MinRounds:       1,
		MaxRounds:       1,
		VoteThreshold:   1.0,
		EnableEarlyStop: true,
		PeerContextMode: "summary+active_claims",
	}
	statement := "Go 在大团队默认协作成本更低，但 Rust 在安全收益可量化时可能抵消额外成本。"
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{debateDrafts: []ClaimDraft{
			{Title: "[Status: keep] Language TCO", Statement: "[Status: keep] " + statement},
			{Title: "裁决状态：unresolved。Language TCO", Statement: "裁决状态：unresolved。" + statement},
		}},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.FreeDebate == nil {
		t.Fatal("expected free debate result")
	}
	matches := 0
	for _, claim := range result.FreeDebate.Claims {
		if claim.Statement != statement {
			if strings.Contains(claim.Statement, "Status:") || strings.Contains(claim.Statement, "裁决状态") {
				t.Fatalf("status prefix should be stripped before storage: %#v", claim)
			}
			continue
		}
		matches++
		if strings.Contains(claim.Title, "Status:") || strings.Contains(claim.Title, "裁决状态") {
			t.Fatalf("status prefix should be stripped from title: %#v", claim)
		}
	}
	if matches != 1 {
		t.Fatalf("expected status-prefixed duplicates to collapse into one claim, got %d claims: %#v", matches, result.FreeDebate.Claims)
	}
	for _, resolution := range result.FreeDebate.ClaimResolutions {
		if strings.Contains(resolution.FinalStatement, "Status:") || strings.Contains(resolution.FinalStatement, "裁决状态") {
			t.Fatalf("status prefix should not leak to final vote summary: %#v", resolution)
		}
	}
}

func TestFreeDebateSkipsProcessMetaNewClaims(t *testing.T) {
	request := baseRequest()
	request.Mode = WorkflowModeFreeDebate
	request.Roles = RoleAssignments{
		Participants: []string{"debater-1", "debater-2"},
	}
	request.DebatePolicy = DebatePolicy{
		MinRounds:       1,
		MaxRounds:       1,
		VoteThreshold:   1.0,
		EnableEarlyStop: true,
		PeerContextMode: "summary+active_claims",
	}
	metaDraft := ClaimDraft{
		Title:         "43 条 peer claims 可合并为约 12 条独立论点",
		Statement:     "本轮 43 条 peer claims 的实际独立论点约 12 个，建议系统层面实施去重，将声明数量控制在 15 条以内。",
		Applicability: "辩论流程优化",
		ClaimType:     ClaimTypeRecommendation,
		Confidence:    0.95,
	}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{debateDrafts: []ClaimDraft{metaDraft}},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.FreeDebate == nil {
		t.Fatal("expected free debate result")
	}
	for _, claim := range result.FreeDebate.Claims {
		if strings.Contains(claim.Statement, "peer claims") || strings.Contains(claim.Statement, "声明数量") {
			t.Fatalf("process meta claim should not enter free debate claims: %#v", claim)
		}
	}
}

func TestEngineRunsDelphiWorkflow(t *testing.T) {
	request := baseRequest()
	request.Mode = WorkflowModeDelphi
	request.Roles = RoleAssignments{
		Participants: []string{"participant-1", "participant-2"},
		Facilitator:  "facilitator-1",
	}
	request.DelphiPolicy = DelphiPolicy{
		MinRounds:               2,
		MaxRounds:               2,
		ConvergenceThreshold:    0.8,
		RatingScaleMin:          1,
		RatingScaleMax:          5,
		Anonymous:               true,
		FacilitatorSummaryStyle: "anonymous-aggregate",
	}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.Mode != WorkflowModeDelphi || result.Delphi == nil {
		t.Fatalf("unexpected result mode: %#v", result)
	}
	if result.Delphi.Recommendation == "" {
		t.Fatalf("expected recommendation, got %#v", result.Delphi)
	}
	if len(result.Delphi.Rounds) != 2 {
		t.Fatalf("expected two delphi rounds, got %#v", result.Delphi.Rounds)
	}
}

func TestEngineExecuteActionAndFailSession(t *testing.T) {
	ids := &deterministicIDs{}
	delegate := &stubDelegate{}
	engine := NewEngine(EngineDeps{
		TaskDelegate: delegate,
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(2, 0).UTC()},
		IDFactory:    ids,
	})
	request := baseRequest()
	request.ActionPolicy = &ActionPolicy{
		Prompt:  "next action",
		ActorID: "actor-1",
	}
	action, err := engine.executeAction(context.Background(), request, RunResult{
		SchemaVersion: SchemaVersion,
		Mode:          WorkflowModeAdjudication,
		RequestID:     request.RequestID,
		SessionID:     "session-1",
		TaskSpec:      request.TaskSpec,
		Report:        AdjudicationReport{Summary: "report"},
		Adjudication: &AdjudicationResultSection{
			TaskVerdict: TaskVerdictSupported,
			ClaimGraph: []ClaimNode{{
				ClaimID:   "claim-1",
				Statement: "statement",
			}},
		},
	}, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatalf("executeAction failed: %v", err)
	}
	if action == nil || action.Status != "completed" || action.ActorID != "actor-1" {
		t.Fatalf("unexpected action output: %#v", action)
	}

	engine.deps.TaskDelegate = &stubDelegate{failActionType: true}
	if _, err := engine.executeAction(context.Background(), request, RunResult{RequestID: request.RequestID, SessionID: "session-1"}, time.Unix(1, 0).UTC()); err == nil {
		t.Fatal("expected action type mismatch to fail")
	}

	request.ActionPolicy.ActorID = ""
	request.Roles.Actor = ""
	if _, err := engine.executeAction(context.Background(), request, RunResult{RequestID: request.RequestID, SessionID: "session-1"}, time.Unix(1, 0).UTC()); err == nil {
		t.Fatal("expected missing actor to fail")
	}

	state := NewStateMachine()
	if err := state.Transition(SessionPhaseFrame); err != nil {
		t.Fatalf("Transition failed: %v", err)
	}
	if err := state.Transition(SessionPhaseIngest); err != nil {
		t.Fatalf("Transition failed: %v", err)
	}
	store := &stubStore{}
	engine.deps.SessionStore = store
	if err := engine.failSession(context.Background(), baseRequest(), "session-1", state, []ClaimNode{{ClaimID: "claim-1"}}, nil, ptr(3), time.Unix(1, 0).UTC(), errors.New("boom")); err != nil {
		t.Fatalf("failSession failed: %v", err)
	}
	if store.snapshot.Phase != SessionPhaseFailed || store.snapshot.Error == nil || store.snapshot.Error.Message != "boom" {
		t.Fatalf("unexpected failed snapshot: %#v", store.snapshot)
	}
}

func TestEngineBuildsFrameArtifactAndObservation(t *testing.T) {
	ledger := &memoryLedger{}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		Ledger:       ledger,
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	request.ActionPolicy = &ActionPolicy{
		Prompt:   "执行低风险动作",
		ActorID:  "actor-1",
		RiskGate: ActionRiskGateAllowMedium,
	}
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.CaseManifest == nil || result.CaseManifest.CaseID == "" || result.CaseManifest.CanonicalProblemStatement == "" {
		t.Fatalf("expected case manifest, got %#v", result.CaseManifest)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("expected one observation, got %#v", result.Observations)
	}
	if result.Observations[0].Outcome != ObservationOutcomePending {
		t.Fatalf("unexpected observation: %#v", result.Observations[0])
	}
	foundFrame := false
	foundObservation := false
	for _, entry := range ledger.entries {
		if entry.Kind == EvidenceKindCaseFramed {
			foundFrame = true
		}
		if entry.Kind == EvidenceKindObservationRecorded {
			foundObservation = true
		}
	}
	if !foundFrame || !foundObservation {
		t.Fatalf("expected framed and observation evidence, got %#v", ledger.entries)
	}
}

func TestEngineAttachesVerificationResultsToClaims(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusPassed, kind: "allowed_paths", name: "allowed"},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if len(section.VerificationResults) == 0 {
		t.Fatalf("expected verification results, got %#v", section)
	}
	claim := section.ClaimGraph[0]
	if len(claim.SupportingEvidenceIDs) == 0 || len(claim.VerificationRefs) == 0 {
		t.Fatalf("expected supporting evidence and verification refs on claim, got %#v", claim)
	}
}

func TestEngineRevisionWithdrawsClaim(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{
			revisionDrafts: []ClaimRevisionDraft{{
				Action:          RevisionActionWithdraw,
				ConfidenceDelta: -0.6,
				Reason:          "验证失败，撤回 claim",
			}},
		},
		Verifier:     stubVerifier{status: VerificationStatusFailed, kind: "allowed_paths", name: "allowed"},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	request.LoopPolicy.MaxRevisionRounds = 1
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if len(section.RevisionRecords) == 0 || section.RevisionRecords[0].Action != RevisionActionWithdraw {
		t.Fatalf("expected withdraw revision record, got %#v", section.RevisionRecords)
	}
	if section.ClaimGraph[0].Status != ClaimStatusWithdrawn || section.ClaimGraph[0].Disposition != ClaimDispositionReject {
		t.Fatalf("expected withdrawn/rejected claim, got %#v", section.ClaimGraph[0])
	}
}

func TestEngineProducesClaimLevelAdjudicationRecords(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	result, err := engine.Start(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if len(section.AdjudicationRecords) != 1 {
		t.Fatalf("expected adjudication records, got %#v", section.AdjudicationRecords)
	}
	if section.AdjudicationRecords[0].Disposition != ClaimDispositionKeep {
		t.Fatalf("expected keep disposition, got %#v", section.AdjudicationRecords[0])
	}
}

func TestEngineFallbacksFromAdjudicateToRevise(t *testing.T) {
	arbiter := &sequenceArbiter{
		reports: []ArbiterReport{
			{
				TaskVerdict: TaskVerdictSupported,
				Summary:     "需要 caveat",
				Decisions: []ArbiterDecision{{
					Verdict:    ClaimVerdictSupported,
					Confidence: 0.55,
					Rationale:  "先保留 caveat",
				}},
				Records: []AdjudicationRecord{{
					Disposition:     ClaimDispositionKeepWithCaveat,
					Rationale:       "先保留 caveat",
					FinalConfidence: 0.55,
					Actionability:   ActionabilityGated,
				}},
			},
			{
				TaskVerdict: TaskVerdictSupported,
				Summary:     "修订后保留",
				Decisions: []ArbiterDecision{{
					Verdict:    ClaimVerdictSupported,
					Confidence: 0.82,
					Rationale:  "修订后通过",
				}},
				Records: []AdjudicationRecord{{
					Disposition:     ClaimDispositionKeep,
					Rationale:       "修订后通过",
					FinalConfidence: 0.82,
					Actionability:   ActionabilityReady,
				}},
			},
		},
	}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{
			revisionDrafts: []ClaimRevisionDraft{{
				Action:      RevisionActionRevise,
				RevisedText: "The patch likely fixes the race condition under the documented boundary",
				Caveats:     []string{"仍需关注边界条件"},
				Reason:      "根据 adjudication caveat 收紧 claim",
			}},
		},
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		Arbiter:      arbiter,
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	request.FallbackPolicy.OnKeepWithCaveat = FallbackTargetRevise
	request.FallbackPolicy.MaxFallbackRounds = 1
	request.LoopPolicy.MaxRevisionRounds = 1
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if arbiter.calls != 2 {
		t.Fatalf("expected arbiter to run twice, got %d", arbiter.calls)
	}
	if len(section.RevisionRecords) == 0 || section.RevisionRecords[0].Action != RevisionActionRevise {
		t.Fatalf("expected revise fallback records, got %#v", section.RevisionRecords)
	}
	if section.AdjudicationRecords[0].Disposition != ClaimDispositionKeep {
		t.Fatalf("expected final keep disposition, got %#v", section.AdjudicationRecords)
	}
}

func TestEngineFallbacksFromAdjudicateToIngest(t *testing.T) {
	ledger := &memoryLedger{}
	arbiter := &sequenceArbiter{
		reports: []ArbiterReport{
			{
				TaskVerdict: TaskVerdictUndetermined,
				Summary:     "证据不足",
				Decisions: []ArbiterDecision{{
					Verdict:    ClaimVerdictUndetermined,
					Confidence: 0.35,
					Rationale:  "证据不足",
				}},
				Records: []AdjudicationRecord{{
					Disposition:     ClaimDispositionUnresolved,
					Rationale:       "证据不足",
					FinalConfidence: 0.35,
					Actionability:   ActionabilityBlocked,
				}},
			},
			{
				TaskVerdict: TaskVerdictSupported,
				Summary:     "补充证据后保留",
				Decisions: []ArbiterDecision{{
					Verdict:    ClaimVerdictSupported,
					Confidence: 0.8,
					Rationale:  "补充证据后保留",
				}},
				Records: []AdjudicationRecord{{
					Disposition:     ClaimDispositionKeep,
					Rationale:       "补充证据后保留",
					FinalConfidence: 0.8,
					Actionability:   ActionabilityReady,
				}},
			},
		},
	}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		Arbiter:      arbiter,
		Ledger:       ledger,
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
		ArtifactDir:  t.TempDir(),
	})
	request := baseRequest()
	request.TaskSpec.TaskType = CaseTaskTypeCoding
	request.IngestPolicy.Sources = []ExternalCommandSource{{
		Name:       "fresh-evidence",
		Command:    "sh",
		Args:       []string{"-c", "printf fallback-evidence"},
		SourceType: "external_command",
		Reference:  "sh -c printf fallback-evidence",
	}}
	request.FallbackPolicy.OnInsufficientEvidence = FallbackTargetIngest
	request.FallbackPolicy.MaxFallbackRounds = 1
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if arbiter.calls != 2 {
		t.Fatalf("expected arbiter to run twice, got %d", arbiter.calls)
	}
	if section.AdjudicationRecords[0].Disposition != ClaimDispositionKeep {
		t.Fatalf("expected final keep disposition, got %#v", section.AdjudicationRecords)
	}
	foundFallbackIngest := false
	for _, entry := range ledger.entries {
		if entry.Kind != EvidenceKindSourceMaterial {
			continue
		}
		if reason, ok := entry.Metadata["reason"].(string); ok && reason == "fallback-1" {
			foundFallbackIngest = true
			break
		}
	}
	if !foundFallbackIngest {
		t.Fatalf("expected fallback ingest evidence in ledger, got %#v", ledger.entries)
	}
}

func TestEngineReturnsInsufficientEvidenceTerminalState(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusInconclusive},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	request.TaskSpec.TaskType = CaseTaskTypeCoding
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.TerminalState != TerminalStateInsufficientEvidence {
		t.Fatalf("expected insufficient_evidence terminal state, got %#v", result)
	}
}

func TestEngineActionBlockedByRiskAndObserved(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	request.TaskSpec.TaskType = CaseTaskTypeStrategy
	request.ActionPolicy = &ActionPolicy{
		Prompt:   "执行高风险架构变更",
		ActorID:  "actor-1",
		RiskGate: ActionRiskGateLowOnly,
	}
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if result.Action == nil || result.Action.Status != string(TerminalStateActionBlockedByRisk) || result.Action.Executed {
		t.Fatalf("expected action blocked by risk, got %#v", result.Action)
	}
	if result.TerminalState != TerminalStateActionBlockedByRisk {
		t.Fatalf("expected action_blocked_by_risk terminal state, got %s", result.TerminalState)
	}
	if len(result.Observations) != 1 || result.Observations[0].Outcome != ObservationOutcomeFollowUp || !result.Observations[0].Reopen {
		t.Fatalf("expected follow-up observation, got %#v", result.Observations)
	}
}

func TestEngineObserveUsesExternalSources(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
		ArtifactDir:  t.TempDir(),
	})
	request := baseRequest()
	request.ObservePolicy.Sources = []ExternalCommandSource{{
		Name:           "health",
		Command:        "sh",
		Args:           []string{"-c", "printf HEALTHY"},
		SuccessPattern: "HEALTHY",
	}}
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	foundHeldUp := false
	for _, item := range result.Observations {
		if item.Outcome == ObservationOutcomeHeldUp {
			foundHeldUp = true
			break
		}
	}
	if !foundHeldUp {
		t.Fatalf("expected held_up observation, got %#v", result.Observations)
	}
	if result.TerminalState == TerminalStateRequiresHumanReview {
		t.Fatalf("did not expect contradiction-driven reopen, got %#v", result)
	}
}

func TestEngineObserveContradictionReopensCase(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
		ArtifactDir:  t.TempDir(),
	})
	request := baseRequest()
	request.ObservePolicy = ObservePolicy{
		OnContradiction: ObserveContradictionReopen,
		Sources: []ExternalCommandSource{{
			Name:           "health",
			Command:        "sh",
			Args:           []string{"-c", "printf BROKEN"},
			FailurePattern: "BROKEN",
		}},
	}
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	foundContradicted := false
	for _, item := range result.Observations {
		if item.Outcome == ObservationOutcomeContradicted {
			foundContradicted = true
			if !item.Reopen || item.FollowUpCaseID == "" {
				t.Fatalf("expected contradiction to reopen, got %#v", item)
			}
			if item.FollowUpRequestID == "" || item.FollowUpArtifact == nil || item.FollowUpArtifact.Path == "" {
				t.Fatalf("expected follow-up artifact to be created, got %#v", item)
			}
			if _, err := os.Stat(item.FollowUpArtifact.Path); err != nil {
				t.Fatalf("expected follow-up artifact path to exist: %v", err)
			}
		}
	}
	if !foundContradicted {
		t.Fatalf("expected contradicted observation, got %#v", result.Observations)
	}
	if result.TerminalState != TerminalStateRequiresHumanReview {
		t.Fatalf("expected requires_human_review after contradiction, got %#v", result)
	}
}

func TestEngineRevisionLoopHonorsLimit(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{
			revisionDrafts: []ClaimRevisionDraft{{
				Action:          RevisionActionDowngrade,
				ConfidenceDelta: -0.1,
				Reason:          "loop limited",
			}},
		},
		Verifier:     stubVerifier{status: VerificationStatusFailed},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	request.LoopPolicy.MaxRevisionRounds = 1
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if len(section.RevisionRecords) != 1 {
		t.Fatalf("expected exactly one revision round when loop limit is one, got %#v", section.RevisionRecords)
	}
}

func TestEngineVerificationLoopHonorsLimit(t *testing.T) {
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{
			revisionDrafts: []ClaimRevisionDraft{{
				Action:          RevisionActionDowngrade,
				ConfidenceDelta: -0.1,
				Reason:          "需要再次验证",
			}},
		},
		Verifier:     stubVerifier{status: VerificationStatusFailed},
		SessionStore: &stubStore{},
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	request.LoopPolicy.MaxRevisionRounds = 2
	request.LoopPolicy.MaxVerificationRounds = 1
	result, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	section := requireAdjudicationSection(t, result)
	if len(section.VerificationResults) != 1 {
		t.Fatalf("expected exactly one verification round, got %#v", section.VerificationResults)
	}
}

func TestEngineResumeFromSavedCheckpoint(t *testing.T) {
	store := &stubStore{}
	delegate := &stubDelegate{
		failKinds: map[TaskKind]int{
			TaskKindArbitrate: 2,
		},
	}
	engine := NewEngine(EngineDeps{
		TaskDelegate: delegate,
		Verifier:     stubVerifier{status: VerificationStatusPassed},
		SessionStore: store,
		Clock:        fixedClock{now: time.Unix(1, 0).UTC()},
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	request.Roles.Arbiter = "arbiter-1"
	if _, err := engine.Start(context.Background(), request); err == nil {
		t.Fatal("expected initial run to fail at arbiter after retries")
	}
	if store.snapshot.Checkpoint == nil || store.snapshot.Checkpoint.LastCompletedPhase != SessionPhaseVerify {
		t.Fatalf("expected verify checkpoint, got %#v", store.snapshot.Checkpoint)
	}
	resumed, err := engine.Resume(context.Background(), store.snapshot)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if resumed.SessionID != store.snapshot.SessionID {
		t.Fatalf("expected same session id on resume, got %s vs %s", resumed.SessionID, store.snapshot.SessionID)
	}
	if store.snapshot.ResumeCount != 1 {
		t.Fatalf("expected resume count to increment, got %#v", store.snapshot.ResumeCount)
	}
	section := requireAdjudicationSection(t, resumed)
	if len(section.ChallengeTickets) == 0 || len(section.AdjudicationRecords) == 0 {
		t.Fatalf("expected resumed run to continue from checkpoint, got %#v", section)
	}
}

func TestRandomIDFactoryAndHelpers(t *testing.T) {
	factory := randomIDFactory{}
	if got := factory.NewSessionID(); !strings.HasPrefix(got, "session_") || len(got) != len("session_")+12 {
		t.Fatalf("unexpected session id: %s", got)
	}
	if got := factory.NewEntityID("ledger"); !strings.HasPrefix(got, "ledger_") || len(got) != len("ledger_")+12 {
		t.Fatalf("unexpected entity id: %s", got)
	}
	if got := randomHex(3); len(got) != 6 {
		t.Fatalf("unexpected random hex length: %s", got)
	}
}

func TestExecuteTaskStopsAtGlobalDeadline(t *testing.T) {
	startedAt := time.Unix(10, 0).UTC()
	clock := &stepClock{times: []time.Time{
		startedAt.Add(2 * time.Second),
	}}
	engine := NewEngine(EngineDeps{
		TaskDelegate: &stubDelegate{},
		SessionStore: &stubStore{},
		Clock:        clock,
		IDFactory:    &deterministicIDs{},
	})
	request := baseRequest()
	request.WaitingPolicy.GlobalDeadline = time.Second
	_, _, err := engine.executeTask(context.Background(), request, "session-1", ProposalTask{
		TaskMeta: TaskMeta{
			SessionID: "session-1",
			RequestID: request.RequestID,
			AgentID:   "proposer-1",
			Role:      "proposer",
		},
		TaskSpec: request.TaskSpec,
	}, startedAt, request.WaitingPolicy.PerTaskTimeout)
	if !errors.Is(err, ErrGlobalDeadlineExceeded) {
		t.Fatalf("expected global deadline error, got %v", err)
	}
}
