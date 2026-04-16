package consensus

type TaskKind string

const (
	TaskKindPropose                TaskKind = "propose"
	TaskKindChallenge              TaskKind = "challenge"
	TaskKindSemanticVerify         TaskKind = "semantic_verify"
	TaskKindRevise                 TaskKind = "revise"
	TaskKindArbitrate              TaskKind = "arbitrate"
	TaskKindReport                 TaskKind = "report"
	TaskKindAction                 TaskKind = "action"
	TaskKindInitialProposal        TaskKind = "initial_proposal"
	TaskKindDebateRound            TaskKind = "debate_round"
	TaskKindFinalVote              TaskKind = "final_vote"
	TaskKindDelphiQuestionnaire    TaskKind = "delphi_questionnaire"
	TaskKindDelphiRevision         TaskKind = "delphi_revision"
	TaskKindDelphiFacilitatorSummary TaskKind = "delphi_facilitator_summary"
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
	Title              string         `json:"title,omitempty"`
	Statement          string         `json:"statement"`
	ClaimType          ClaimType      `json:"claimType,omitempty"`
	Scope              string         `json:"scope,omitempty"`
	Dependencies       []string       `json:"dependencies,omitempty"`
	ParentClaimIDs     []string       `json:"parentClaimIds,omitempty"`
	Applicability      string         `json:"applicability,omitempty"`
	BoundaryConditions []string       `json:"boundaryConditions,omitempty"`
	Confidence         float64        `json:"confidence,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
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
	ClaimID                      string         `json:"claimId,omitempty"`
	Statement                    string         `json:"statement"`
	Kind                         string         `json:"kind"`
	AttackType                   string         `json:"attackType,omitempty"`
	Severity                     AttackSeverity `json:"severity,omitempty"`
	RequestedChecks              []string       `json:"requestedChecks,omitempty"`
	SuggestedFalsificationMethod string         `json:"suggestedFalsificationMethod,omitempty"`
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

type ClaimRevisionDraft struct {
	TargetClaimID      string         `json:"targetClaimId"`
	Action             RevisionAction `json:"action"`
	RevisedText        string         `json:"revisedText,omitempty"`
	ConfidenceDelta    float64        `json:"confidenceDelta,omitempty"`
	Caveats            []string       `json:"caveats,omitempty"`
	BoundaryConditions []string       `json:"boundaryConditions,omitempty"`
	Reason             string         `json:"reason,omitempty"`
	Unresolved         bool           `json:"unresolved,omitempty"`
}

type ReviseTask struct {
	TaskMeta
	TaskSpec   TaskSpec             `json:"taskSpec"`
	Manifest   CaseManifest         `json:"manifest"`
	Round      int                  `json:"round"`
	Claims     []ClaimNode          `json:"claims"`
	Challenges []ChallengeTicket    `json:"challenges,omitempty"`
	Findings   []VerificationResult `json:"findings,omitempty"`
}

func (ReviseTask) Kind() TaskKind   { return TaskKindRevise }
func (t ReviseTask) Meta() TaskMeta { return t.TaskMeta }

type ReviseOutput struct {
	Summary             string               `json:"summary"`
	Revisions           []ClaimRevisionDraft `json:"revisions"`
	UnresolvedQuestions []string             `json:"unresolvedQuestions,omitempty"`
}

type ReviseTaskResult struct {
	Output ReviseOutput `json:"output"`
}

func (ReviseTaskResult) Kind() TaskKind { return TaskKindRevise }

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
	Records     []AdjudicationRecord `json:"records,omitempty"`
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
	Mode        WorkflowMode      `json:"mode,omitempty"`
	Payload     map[string]any    `json:"payload,omitempty"`
}

func (ReportTask) Kind() TaskKind   { return TaskKindReport }
func (t ReportTask) Meta() TaskMeta { return t.TaskMeta }

type ReportTaskResult struct {
	Output AdjudicationReport `json:"output"`
}

func (ReportTaskResult) Kind() TaskKind { return TaskKindReport }

type ActionTask struct {
	TaskMeta
	Prompt string    `json:"prompt"`
	Input  RunResult `json:"input"`
}

func (ActionTask) Kind() TaskKind   { return TaskKindAction }
func (t ActionTask) Meta() TaskMeta { return t.TaskMeta }

type ActionTaskResult struct {
	Output ActionExecution `json:"output"`
}

func (ActionTaskResult) Kind() TaskKind { return TaskKindAction }

type InitialProposalTask struct {
	TaskMeta
	TaskSpec  TaskSpec `json:"taskSpec"`
	Round     int      `json:"round"`
	MaxClaims int      `json:"maxClaims"`
}

func (InitialProposalTask) Kind() TaskKind   { return TaskKindInitialProposal }
func (t InitialProposalTask) Meta() TaskMeta { return t.TaskMeta }

type InitialProposalOutput struct {
	Summary string       `json:"summary"`
	Claims  []ClaimDraft `json:"claims"`
}

type InitialProposalTaskResult struct {
	Output InitialProposalOutput `json:"output"`
}

func (InitialProposalTaskResult) Kind() TaskKind { return TaskKindInitialProposal }

type DebateRoundTask struct {
	TaskMeta
	TaskSpec       TaskSpec         `json:"taskSpec"`
	Round          int              `json:"round"`
	SelfClaims     []DebateClaim    `json:"selfClaims"`
	PeerClaims     []DebateClaim    `json:"peerClaims"`
	RoundSummary   string           `json:"roundSummary,omitempty"`
	PeerContextMode string          `json:"peerContextMode,omitempty"`
}

func (DebateRoundTask) Kind() TaskKind   { return TaskKindDebateRound }
func (t DebateRoundTask) Meta() TaskMeta { return t.TaskMeta }

type DebateJudgementDraft struct {
	ClaimID         string          `json:"claimId"`
	Judgement       DebateJudgement `json:"judgement"`
	Rationale       string          `json:"rationale,omitempty"`
	RevisedStatement string         `json:"revisedStatement,omitempty"`
	MergeWithClaims []string        `json:"mergeWithClaims,omitempty"`
}

type DebateRoundOutput struct {
	Summary    string                `json:"summary"`
	NewClaims  []ClaimDraft          `json:"newClaims,omitempty"`
	Judgements []DebateJudgementDraft `json:"judgements,omitempty"`
}

type DebateRoundTaskResult struct {
	Output DebateRoundOutput `json:"output"`
}

func (DebateRoundTaskResult) Kind() TaskKind { return TaskKindDebateRound }

type FinalVoteTask struct {
	TaskMeta
	TaskSpec TaskSpec      `json:"taskSpec"`
	Round    int           `json:"round"`
	Claims   []DebateClaim `json:"claims"`
}

func (FinalVoteTask) Kind() TaskKind   { return TaskKindFinalVote }
func (t FinalVoteTask) Meta() TaskMeta { return t.TaskMeta }

type DebateVoteDraft struct {
	ClaimID   string           `json:"claimId"`
	Vote      DebateVoteChoice `json:"vote"`
	Rationale string           `json:"rationale,omitempty"`
}

type FinalVoteOutput struct {
	Summary string            `json:"summary"`
	Votes   []DebateVoteDraft `json:"votes"`
}

type FinalVoteTaskResult struct {
	Output FinalVoteOutput `json:"output"`
}

func (FinalVoteTaskResult) Kind() TaskKind { return TaskKindFinalVote }

type DelphiQuestionnaireTask struct {
	TaskMeta
	TaskSpec          TaskSpec            `json:"taskSpec"`
	Round             int                 `json:"round"`
	Questionnaire     string              `json:"questionnaire"`
	PreviousStatements []DelphiStatement  `json:"previousStatements,omitempty"`
	PreviousSummary   string              `json:"previousSummary,omitempty"`
}

func (DelphiQuestionnaireTask) Kind() TaskKind   { return TaskKindDelphiQuestionnaire }
func (t DelphiQuestionnaireTask) Meta() TaskMeta { return t.TaskMeta }

type DelphiResponseDraft struct {
	StatementID string  `json:"statementId,omitempty"`
	Statement   string  `json:"statement,omitempty"`
	Rating      float64 `json:"rating"`
	Rationale   string  `json:"rationale,omitempty"`
}

type DelphiQuestionnaireOutput struct {
	Summary   string               `json:"summary"`
	Responses []DelphiResponseDraft `json:"responses"`
}

type DelphiQuestionnaireTaskResult struct {
	Output DelphiQuestionnaireOutput `json:"output"`
}

func (DelphiQuestionnaireTaskResult) Kind() TaskKind { return TaskKindDelphiQuestionnaire }

type DelphiRevisionTask struct {
	TaskMeta
	TaskSpec            TaskSpec           `json:"taskSpec"`
	Round               int                `json:"round"`
	StatementSummaries  []DelphiStatement  `json:"statementSummaries"`
	PreviousSummary     string             `json:"previousSummary,omitempty"`
}

func (DelphiRevisionTask) Kind() TaskKind   { return TaskKindDelphiRevision }
func (t DelphiRevisionTask) Meta() TaskMeta { return t.TaskMeta }

type DelphiRevisionOutput struct {
	Summary   string                `json:"summary"`
	Responses []DelphiResponseDraft `json:"responses"`
}

type DelphiRevisionTaskResult struct {
	Output DelphiRevisionOutput `json:"output"`
}

func (DelphiRevisionTaskResult) Kind() TaskKind { return TaskKindDelphiRevision }

type DelphiFacilitatorSummaryTask struct {
	TaskMeta
	TaskSpec          TaskSpec          `json:"taskSpec"`
	Round             int               `json:"round"`
	StatementSummaries []DelphiStatement `json:"statementSummaries"`
}

func (DelphiFacilitatorSummaryTask) Kind() TaskKind   { return TaskKindDelphiFacilitatorSummary }
func (t DelphiFacilitatorSummaryTask) Meta() TaskMeta { return t.TaskMeta }

type DelphiFacilitatorSummaryOutput struct {
	Summary         string            `json:"summary"`
	Recommendation  string            `json:"recommendation,omitempty"`
	DissentSummary  []string          `json:"dissentSummary,omitempty"`
	Statements      []DelphiStatement `json:"statements,omitempty"`
}

type DelphiFacilitatorSummaryTaskResult struct {
	Output DelphiFacilitatorSummaryOutput `json:"output"`
}

func (DelphiFacilitatorSummaryTaskResult) Kind() TaskKind { return TaskKindDelphiFacilitatorSummary }

type TaskResult interface {
	Kind() TaskKind
}
