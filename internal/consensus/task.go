package consensus

type TaskKind string

const (
	TaskKindPropose        TaskKind = "propose"
	TaskKindChallenge      TaskKind = "challenge"
	TaskKindSemanticVerify TaskKind = "semantic_verify"
	TaskKindArbitrate      TaskKind = "arbitrate"
	TaskKindReport         TaskKind = "report"
	TaskKindAction         TaskKind = "action"
)

type Task interface {
	Kind() TaskKind
	Meta() TaskMeta
}

type TaskMeta struct {
	SessionID string         `json:"sessionId"`
	RequestID string         `json:"requestId"`
	AgentID   string         `json:"agentId"`
	Role      string         `json:"role,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ClaimDraft struct {
	Title         string         `json:"title,omitempty"`
	Statement     string         `json:"statement"`
	Scope         string         `json:"scope,omitempty"`
	Dependencies  []string       `json:"dependencies,omitempty"`
	Applicability string         `json:"applicability,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type ProposalTask struct {
	TaskMeta
	TaskSpec       TaskSpec `json:"taskSpec"`
	Scope          string   `json:"scope,omitempty"`
	MaxClaims      int      `json:"maxClaims"`
	DedupeStrategy string   `json:"dedupeStrategy,omitempty"`
}

func (ProposalTask) Kind() TaskKind   { return TaskKindPropose }
func (t ProposalTask) Meta() TaskMeta { return t.TaskMeta }

type ProposalOutput struct {
	Summary string       `json:"summary"`
	Claims  []ClaimDraft `json:"claims"`
}

type ProposalTaskResult struct {
	Output ProposalOutput `json:"output"`
}

func (ProposalTaskResult) Kind() TaskKind { return TaskKindPropose }

type ChallengeDraft struct {
	ClaimID         string   `json:"claimId,omitempty"`
	Statement       string   `json:"statement"`
	Kind            string   `json:"kind"`
	RequestedChecks []string `json:"requestedChecks,omitempty"`
}

type ChallengeTask struct {
	TaskMeta
	TaskSpec TaskSpec    `json:"taskSpec"`
	Claims   []ClaimNode `json:"claims"`
}

func (ChallengeTask) Kind() TaskKind   { return TaskKindChallenge }
func (t ChallengeTask) Meta() TaskMeta { return t.TaskMeta }

type ChallengeOutput struct {
	Summary string           `json:"summary"`
	Tickets []ChallengeDraft `json:"tickets"`
}

type ChallengeTaskResult struct {
	Output ChallengeOutput `json:"output"`
}

func (ChallengeTaskResult) Kind() TaskKind { return TaskKindChallenge }

type SemanticVerificationTask struct {
	TaskMeta
	TaskSpec   TaskSpec          `json:"taskSpec"`
	Claim      ClaimNode         `json:"claim"`
	Challenges []ChallengeTicket `json:"challenges,omitempty"`
}

func (SemanticVerificationTask) Kind() TaskKind   { return TaskKindSemanticVerify }
func (t SemanticVerificationTask) Meta() TaskMeta { return t.TaskMeta }

type SemanticVerificationFinding struct {
	ClaimID    string       `json:"claimId"`
	Verdict    ClaimVerdict `json:"verdict"`
	Confidence float64      `json:"confidence,omitempty"`
	Rationale  string       `json:"rationale"`
}

type SemanticVerificationOutput struct {
	Summary string                        `json:"summary"`
	Results []SemanticVerificationFinding `json:"results"`
}

type SemanticVerificationTaskResult struct {
	Output SemanticVerificationOutput `json:"output"`
}

func (SemanticVerificationTaskResult) Kind() TaskKind { return TaskKindSemanticVerify }

type ArbiterTask struct {
	TaskMeta
	TaskSpec   TaskSpec             `json:"taskSpec"`
	Claims     []ClaimNode          `json:"claims"`
	Challenges []ChallengeTicket    `json:"challenges"`
	Ledger     []EvidenceRecord     `json:"ledger"`
	Findings   []VerificationResult `json:"findings"`
}

func (ArbiterTask) Kind() TaskKind   { return TaskKindArbitrate }
func (t ArbiterTask) Meta() TaskMeta { return t.TaskMeta }

type ArbiterTaskOutput struct {
	Summary     string            `json:"summary"`
	TaskVerdict TaskVerdict       `json:"taskVerdict"`
	Decisions   []ArbiterDecision `json:"decisions"`
}

type ArbiterTaskResult struct {
	Output ArbiterTaskOutput `json:"output"`
}

func (ArbiterTaskResult) Kind() TaskKind { return TaskKindArbitrate }

type ReportTask struct {
	TaskMeta
	TaskSpec    TaskSpec          `json:"taskSpec"`
	TaskVerdict TaskVerdict       `json:"taskVerdict"`
	Claims      []ClaimNode       `json:"claims"`
	Challenges  []ChallengeTicket `json:"challenges"`
	Arbiter     ArbiterReport     `json:"arbiter"`
}

func (ReportTask) Kind() TaskKind   { return TaskKindReport }
func (t ReportTask) Meta() TaskMeta { return t.TaskMeta }

type ReportTaskResult struct {
	Output AdjudicationReport `json:"output"`
}

func (ReportTaskResult) Kind() TaskKind { return TaskKindReport }

type ActionTask struct {
	TaskMeta
	Prompt string             `json:"prompt"`
	Input  AdjudicationResult `json:"input"`
}

func (ActionTask) Kind() TaskKind   { return TaskKindAction }
func (t ActionTask) Meta() TaskMeta { return t.TaskMeta }

type ActionTaskResult struct {
	Output ActionExecution `json:"output"`
}

func (ActionTaskResult) Kind() TaskKind { return TaskKindAction }

type TaskResult interface {
	Kind() TaskKind
}
