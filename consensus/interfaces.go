package consensus

import (
	"context"
	"time"
)

type DispatchReceipt struct {
	TaskID  string
	AgentID string
	Kind    TaskKind
}

type AwaitedTask struct {
	OK       bool
	Output   TaskResult
	Error    string
	Artifact *ArtifactRef
}

type TaskDelegate interface {
	Dispatch(ctx context.Context, task Task) (DispatchReceipt, error)
	Await(ctx context.Context, taskID string, timeout time.Duration) (AwaitedTask, error)
	Cancel(ctx context.Context, taskID string) error
}

type VerificationRequest struct {
	Request    StartRequest
	SessionID  string
	Claim      ClaimNode
	Challenges []ChallengeTicket
}

type Verifier interface {
	Run(ctx context.Context, req VerificationRequest) ([]VerificationResult, error)
}

type ArbiterInput struct {
	Request    StartRequest
	SessionID  string
	Claims     []ClaimNode
	Challenges []ChallengeTicket
	Ledger     []EvidenceRecord
	Findings   []VerificationResult
}

type Arbiter interface {
	Decide(ctx context.Context, input ArbiterInput) (ArbiterReport, error)
}

type Observer interface {
	OnEvent(ctx context.Context, event RunEvent) error
}

type Ledger interface {
	Append(ctx context.Context, entry EvidenceRecord) (EvidenceRecord, error)
}

type SessionSnapshot struct {
	SessionID                 string
	RequestID                 string
	Request                   *StartRequest
	Phase                     SessionPhase
	Checkpoint                *SessionCheckpoint
	CaseManifest              *CaseManifest
	ClaimGraph                []ClaimNode
	ChallengeTickets          []ChallengeTicket
	LedgerEntries             []EvidenceRecord
	LedgerCursor              int
	VerificationResults       []VerificationResult
	RevisionRecords           []ClaimRevisionRecord
	AdjudicationRecords       []AdjudicationRecord
	Observations              []ObservationRecord
	Metrics                   Metrics
	ResumeCount               int
	LastResumedAt             string
	ArbiterReport             *ArbiterReport
	Report                    *AdjudicationReport
	Action                    *ActionOutput
	DebateClaims              []DebateClaim
	DebateRounds              []DebateRoundRecord
	DebateVotes               []DebateVoteRecord
	DelphiRounds              []DelphiRoundRecord
	DelphiStatements          []DelphiStatement
	DelphiRatingDistributions map[string][]float64
	DelphiConsensusLevel      float64
	DelphiRecommendation      string
	DelphiDissentSummary      []string
	StartedAt                 string
	FinishedAt                string
	FailedAt                  string
	Result                    *RunResult
	Error                     *FailureInfo
}

type SessionPatch struct {
	Phase                     *SessionPhase
	Checkpoint                *SessionCheckpoint
	CaseManifest              *CaseManifest
	ClaimGraph                []ClaimNode
	ChallengeTickets          []ChallengeTicket
	LedgerEntries             []EvidenceRecord
	LedgerCursor              *int
	VerificationResults       []VerificationResult
	RevisionRecords           []ClaimRevisionRecord
	AdjudicationRecords       []AdjudicationRecord
	Observations              []ObservationRecord
	Metrics                   *Metrics
	ResumeCount               *int
	LastResumedAt             *string
	ArbiterReport             *ArbiterReport
	Report                    *AdjudicationReport
	Action                    *ActionOutput
	DebateClaims              []DebateClaim
	DebateRounds              []DebateRoundRecord
	DebateVotes               []DebateVoteRecord
	DelphiRounds              []DelphiRoundRecord
	DelphiStatements          []DelphiStatement
	DelphiRatingDistributions map[string][]float64
	DelphiConsensusLevel      *float64
	DelphiRecommendation      *string
	DelphiDissentSummary      []string
	FinishedAt                *string
	FailedAt                  *string
	Result                    *RunResult
	Error                     *FailureInfo
}

type SessionStore interface {
	Save(ctx context.Context, session SessionSnapshot) error
	Load(ctx context.Context, sessionID string) (*SessionSnapshot, error)
	Patch(ctx context.Context, sessionID string, patch SessionPatch) error
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type IDFactory interface {
	NewSessionID() string
	NewEntityID(prefix string) string
}
