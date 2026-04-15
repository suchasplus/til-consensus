package consensus

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type scriptedDelegate struct {
	mu      sync.Mutex
	seq     int
	tasks   map[string]Task
	handler func(Task) (AwaitedTask, error)
}

func (d *scriptedDelegate) Dispatch(_ context.Context, task Task) (DispatchReceipt, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tasks == nil {
		d.tasks = map[string]Task{}
	}
	taskID := fmt.Sprintf("task-%d", d.seq)
	d.seq++
	d.tasks[taskID] = task
	return DispatchReceipt{TaskID: taskID, ParticipantID: task.Meta().ParticipantID, Kind: task.Kind()}, nil
}

func (d *scriptedDelegate) Await(_ context.Context, taskID string, _ time.Duration) (AwaitedTask, error) {
	d.mu.Lock()
	task := d.tasks[taskID]
	d.mu.Unlock()
	return d.handler(task)
}

func (d *scriptedDelegate) Cancel(_ context.Context, _ string) error { return nil }

type fixedIDFactory struct{}

func (fixedIDFactory) NewSessionID() string { return "session-fixed" }

type sequenceClock struct {
	mu    sync.Mutex
	times []time.Time
	index int
}

func (c *sequenceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.times) == 0 {
		return time.Unix(0, 0).UTC()
	}
	if c.index >= len(c.times) {
		return c.times[len(c.times)-1]
	}
	value := c.times[c.index]
	c.index++
	return value
}

func TestEngineConsensusFlow(t *testing.T) {
	delegate := &scriptedDelegate{
		handler: func(task Task) (AwaitedTask, error) {
			switch value := task.(type) {
			case RoundTask:
				switch {
				case value.Phase == PhaseInitial && value.ParticipantID == "a":
					return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
						ParticipantID:   "a",
						Phase:           PhaseInitial,
						Round:           0,
						FullResponse:    "initial a",
						Summary:         "summary a",
						TaskTitle:       "Consensus Title",
						ExtractedClaims: []ExtractedClaim{{Title: "Claim A", Statement: "Claim A"}},
						Judgements:      []ClaimJudgement{},
					}}}, nil
				case value.Phase == PhaseInitial && value.ParticipantID == "b":
					return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
						ParticipantID: "b",
						Phase:         PhaseInitial,
						Round:         0,
						FullResponse:  "initial b",
						Summary:       "summary b",
						TaskTitle:     "Consensus Title",
						Judgements:    []ClaimJudgement{},
					}}}, nil
				case value.Phase == PhaseDebate:
					return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
						ParticipantID: value.ParticipantID,
						Phase:         PhaseDebate,
						Round:         value.Round,
						FullResponse:  "debate",
						Summary:       "debate",
						Judgements:    []ClaimJudgement{{ClaimID: "a:0:0", Stance: ClaimStanceAgree, Confidence: 0.9, Rationale: "agree"}},
					}}}, nil
				default:
					return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
						ParticipantID: value.ParticipantID,
						Phase:         PhaseFinalVote,
						Round:         value.Round,
						FullResponse:  "vote",
						Summary:       "vote",
						Judgements:    []ClaimJudgement{{ClaimID: "a:0:0", Stance: ClaimStanceAgree, Confidence: 0.9, Rationale: "agree"}},
						ClaimVotes:    []ClaimVoteInput{{ClaimID: "a:0:0", Vote: "accept", Reason: "accept"}},
					}}}, nil
				}
			default:
				t.Fatalf("unexpected task: %#v", task)
				return AwaitedTask{}, nil
			}
		},
	}
	engine := NewEngine(EngineDeps{
		TaskDelegate: delegate,
		SessionStore: newTestStore(),
		IDFactory:    fixedIDFactory{},
	})
	result, err := engine.Start(context.Background(), baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ConsensusStatusConsensus {
		t.Fatalf("expected consensus, got %s", result.Status)
	}
	if len(result.Rounds) != 3 {
		t.Fatalf("expected 3 rounds with early stop + final vote, got %d", len(result.Rounds))
	}
	if result.Task.Title != "Consensus Title" {
		t.Fatalf("unexpected task title: %s", result.Task.Title)
	}
	if len(result.ClaimResolutions) != 1 || result.ClaimResolutions[0].Status != ClaimResolutionResolved {
		t.Fatalf("unexpected claim resolutions: %#v", result.ClaimResolutions)
	}
}

func TestEnginePartialConsensus(t *testing.T) {
	delegate := &scriptedDelegate{
		handler: func(task Task) (AwaitedTask, error) {
			switch value := task.(type) {
			case RoundTask:
				switch {
				case value.Phase == PhaseInitial && value.ParticipantID == "a":
					return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
						ParticipantID: "a", Phase: PhaseInitial, Round: 0, FullResponse: "initial a", Summary: "summary a", TaskTitle: "Title",
						ExtractedClaims: []ExtractedClaim{{Title: "Claim A", Statement: "Claim A"}}, Judgements: []ClaimJudgement{},
					}}}, nil
				case value.Phase == PhaseInitial && value.ParticipantID == "b":
					return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
						ParticipantID: "b", Phase: PhaseInitial, Round: 0, FullResponse: "initial b", Summary: "summary b", TaskTitle: "Title",
						ExtractedClaims: []ExtractedClaim{{Title: "Claim B", Statement: "Claim B"}}, Judgements: []ClaimJudgement{},
					}}}, nil
				case value.Phase == PhaseDebate:
					return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
						ParticipantID: value.ParticipantID,
						Phase:         PhaseDebate,
						Round:         1,
						FullResponse:  "debate",
						Summary:       "debate",
						Judgements: []ClaimJudgement{
							{ClaimID: "a:0:0", Stance: ClaimStanceAgree, Confidence: 0.9, Rationale: "agree"},
							{ClaimID: "b:0:0", Stance: ClaimStanceAgree, Confidence: 0.9, Rationale: "agree"},
						},
					}}}, nil
				default:
					votes := []ClaimVoteInput{
						{ClaimID: "a:0:0", Vote: "accept", Reason: "accept"},
					}
					if value.ParticipantID == "a" {
						votes = append(votes, ClaimVoteInput{ClaimID: "b:0:0", Vote: "accept", Reason: "accept"})
					} else {
						votes = append(votes, ClaimVoteInput{ClaimID: "b:0:0", Vote: "reject", Reason: "reject"})
					}
					return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
						ParticipantID: value.ParticipantID,
						Phase:         PhaseFinalVote,
						Round:         value.Round,
						FullResponse:  "vote",
						Summary:       "vote",
						Judgements:    []ClaimJudgement{{ClaimID: "a:0:0", Stance: ClaimStanceAgree, Confidence: 0.9, Rationale: "agree"}},
						ClaimVotes:    votes,
					}}}, nil
				}
			default:
				return AwaitedTask{}, fmt.Errorf("unexpected task")
			}
		},
	}
	engine := NewEngine(EngineDeps{TaskDelegate: delegate, SessionStore: newTestStore(), IDFactory: fixedIDFactory{}})
	result, err := engine.Start(context.Background(), baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ConsensusStatusPartialConsensus {
		t.Fatalf("expected partial consensus, got %s", result.Status)
	}
}

func TestEngineGlobalDeadlineUnresolved(t *testing.T) {
	base := time.Unix(1700000000, 0).UTC()
	clock := &sequenceClock{times: []time.Time{
		base,
		base.Add(100 * time.Millisecond),
		base.Add(200 * time.Millisecond),
		base.Add(2 * time.Second),
		base.Add(3 * time.Second),
		base.Add(4 * time.Second),
		base.Add(5 * time.Second),
	}}
	delegate := &scriptedDelegate{
		handler: func(task Task) (AwaitedTask, error) {
			round := task.(RoundTask)
			return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
				ParticipantID:   round.ParticipantID,
				Phase:           round.Phase,
				Round:           round.Round,
				FullResponse:    "ok",
				Summary:         "ok",
				TaskTitle:       "Title",
				ExtractedClaims: []ExtractedClaim{{Title: "Claim A", Statement: "Claim A"}},
				Judgements:      []ClaimJudgement{},
			}}}, nil
		},
	}
	req := baseRequest()
	req.WaitingPolicy.GlobalDeadline = time.Second
	engine := NewEngine(EngineDeps{
		TaskDelegate: delegate,
		SessionStore: newTestStore(),
		IDFactory:    fixedIDFactory{},
		Clock:        clock,
	})
	result, err := engine.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ConsensusStatusUnresolved {
		t.Fatalf("expected unresolved, got %s", result.Status)
	}
	if !result.Metrics.GlobalDeadlineHit {
		t.Fatal("expected global deadline hit")
	}
}

func TestEngineReportFallback(t *testing.T) {
	delegate := &scriptedDelegate{
		handler: func(task Task) (AwaitedTask, error) {
			switch value := task.(type) {
			case RoundTask:
				return simpleConsensusRound(value), nil
			case ReportTask:
				return AwaitedTask{OK: true, Output: ActionTaskResult{Output: ActionExecution{FullResponse: "bad", Summary: "bad"}}}, nil
			default:
				return AwaitedTask{}, fmt.Errorf("unexpected task")
			}
		},
	}
	req := baseRequest()
	req.ReportPolicy.Composer = ReportComposerRepresentative
	engine := NewEngine(EngineDeps{TaskDelegate: delegate, SessionStore: newTestStore(), IDFactory: fixedIDFactory{}})
	result, err := engine.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report.Mode != "builtin" {
		t.Fatalf("expected builtin fallback report, got %#v", result.Report)
	}
}

func TestEngineActionFailurePaths(t *testing.T) {
	t.Run("inactive actor", func(t *testing.T) {
		delegate := &scriptedDelegate{
			handler: func(task Task) (AwaitedTask, error) {
				switch value := task.(type) {
				case RoundTask:
					if value.ParticipantID == "c" && value.Phase == PhaseInitial {
						return AwaitedTask{OK: false, Error: "__timeout__"}, nil
					}
					return simpleConsensusRound(value), nil
				default:
					return AwaitedTask{}, fmt.Errorf("unexpected task")
				}
			},
		}
		req := baseRequest()
		req.Participants = []Participant{{ID: "a"}, {ID: "b"}, {ID: "c"}}
		req.ActionPolicy = &ActionPolicy{Prompt: "do action", ActorID: "c", IncludeFullResult: true}
		engine := NewEngine(EngineDeps{TaskDelegate: delegate, SessionStore: newTestStore(), IDFactory: fixedIDFactory{}})
		result, err := engine.Start(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if result.Action == nil || result.Action.Status != "failed" {
			t.Fatalf("expected failed action for inactive actor, got %#v", result.Action)
		}
	})

	t.Run("action parse failure", func(t *testing.T) {
		delegate := &scriptedDelegate{
			handler: func(task Task) (AwaitedTask, error) {
				switch value := task.(type) {
				case RoundTask:
					return simpleConsensusRound(value), nil
				case ActionTask:
					return AwaitedTask{OK: true, Output: ReportTaskResult{Output: FinalReport{FinalSummary: "bad", RepresentativeSpeech: "bad"}}}, nil
				default:
					return AwaitedTask{}, fmt.Errorf("unexpected task")
				}
			},
		}
		req := baseRequest()
		req.ActionPolicy = &ActionPolicy{Prompt: "do action", IncludeFullResult: true}
		engine := NewEngine(EngineDeps{TaskDelegate: delegate, SessionStore: newTestStore(), IDFactory: fixedIDFactory{}})
		result, err := engine.Start(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if result.Action == nil || result.Action.Status != "failed" || result.Action.Error != "action parse failed" {
			t.Fatalf("expected parse failure action, got %#v", result.Action)
		}
	})
}

func simpleConsensusRound(value RoundTask) AwaitedTask {
	switch value.Phase {
	case PhaseInitial:
		extractedClaims := []ExtractedClaim{}
		if value.ParticipantID == "a" {
			extractedClaims = []ExtractedClaim{{Title: "Claim A", Statement: "Claim A"}}
		}
		return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
			ParticipantID:   value.ParticipantID,
			Phase:           PhaseInitial,
			Round:           value.Round,
			FullResponse:    "initial",
			Summary:         "initial",
			TaskTitle:       "Title",
			ExtractedClaims: extractedClaims,
			Judgements:      []ClaimJudgement{},
		}}}
	case PhaseDebate:
		return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
			ParticipantID: value.ParticipantID,
			Phase:         value.Phase,
			Round:         value.Round,
			FullResponse:  "debate",
			Summary:       "debate",
			Judgements:    []ClaimJudgement{{ClaimID: "a:0:0", Stance: ClaimStanceAgree, Confidence: 0.9, Rationale: "agree"}},
		}}}
	default:
		return AwaitedTask{OK: true, Output: RoundTaskResult{Output: ParticipantRoundOutput{
			ParticipantID: value.ParticipantID,
			Phase:         value.Phase,
			Round:         value.Round,
			FullResponse:  "vote",
			Summary:       "vote",
			Judgements:    []ClaimJudgement{{ClaimID: "a:0:0", Stance: ClaimStanceAgree, Confidence: 0.9, Rationale: "agree"}},
			ClaimVotes:    []ClaimVoteInput{{ClaimID: "a:0:0", Vote: "accept", Reason: "accept"}},
		}}}
	}
}

func baseRequest() StartRequest {
	return StartRequest{
		RequestID:    "req-1",
		Task:         "test task",
		Participants: []Participant{{ID: "a"}, {ID: "b"}},
		RoundPolicy: RoundPolicy{
			MinRounds: 1,
			MaxRounds: 2,
		},
		ParticipantsPolicy: ParticipantsPolicy{MinParticipants: 2},
		SessionPolicy: SessionPolicy{
			Mode:             "sticky-per-participant",
			SessionKeyPrefix: "consensus",
		},
		PeerContextPolicy: PeerContextPolicy{
			PassMode:                "full-response-preferred",
			MaxCharsPerPeerResponse: DefaultPeerChars,
			MaxPeersPerRound:        DefaultPeerCount,
			OverflowStrategy:        DefaultPeerOverflowStrategy,
		},
		ScoringPolicy: ScoringPolicy{
			Enabled:    true,
			TieBreaker: TieBreakerLatestRoundScore,
			Rubric: RubricWeights{
				Correctness:   0.35,
				Completeness:  0.25,
				Actionability: 0.25,
				Consistency:   0.15,
			},
		},
		ConsensusPolicy: ConsensusPolicy{Threshold: 1.0},
		ReportPolicy: ReportPolicy{
			Composer:   ReportComposerBuiltin,
			TraceLevel: TraceLevelCompact,
		},
		WaitingPolicy: WaitingPolicy{
			PerTaskTimeout:  time.Second,
			PerRoundTimeout: time.Second,
		},
	}
}

type testStore struct {
	mu       sync.Mutex
	sessions map[string]SessionSnapshot
}

func newTestStore() *testStore {
	return &testStore{sessions: map[string]SessionSnapshot{}}
}

func (s *testStore) Save(_ context.Context, session SessionSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.SessionID] = session
	return nil
}

func (s *testStore) Load(_ context.Context, sessionID string) (*SessionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	cloned := session
	return &cloned, nil
}

func (s *testStore) Patch(_ context.Context, sessionID string, patch SessionPatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[sessionID]
	if patch.State != nil {
		session.State = *patch.State
	}
	if patch.Result != nil {
		session.Result = patch.Result
	}
	if patch.Error != nil {
		session.Error = patch.Error
	}
	if patch.ActiveParticipants != nil {
		session.ActiveParticipants = append([]string(nil), patch.ActiveParticipants...)
	}
	if patch.Eliminations != nil {
		session.Eliminations = append([]EliminationRecord(nil), patch.Eliminations...)
	}
	if patch.RunningAt != nil {
		session.RunningAt = *patch.RunningAt
	}
	if patch.FinalizingAt != nil {
		session.FinalizingAt = *patch.FinalizingAt
	}
	if patch.FinishedAt != nil {
		session.FinishedAt = *patch.FinishedAt
	}
	if patch.FailedAt != nil {
		session.FailedAt = *patch.FailedAt
	}
	s.sessions[sessionID] = session
	return nil
}
