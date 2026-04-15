package consensus

type ClaimStatus string

const (
	ClaimStatusProposed    ClaimStatus = "proposed"
	ClaimStatusChallenged  ClaimStatus = "challenged"
	ClaimStatusVerified    ClaimStatus = "verified"
	ClaimStatusAdjudicated ClaimStatus = "adjudicated"
)

type ClaimVerdict string

const (
	ClaimVerdictSupported            ClaimVerdict = "supported"
	ClaimVerdictRefuted              ClaimVerdict = "refuted"
	ClaimVerdictInsufficientEvidence ClaimVerdict = "insufficient_evidence"
	ClaimVerdictUndetermined         ClaimVerdict = "undetermined"
)

type TaskVerdict string

const (
	TaskVerdictSupported          TaskVerdict = "supported"
	TaskVerdictPartiallySupported TaskVerdict = "partially_supported"
	TaskVerdictUndetermined       TaskVerdict = "undetermined"
	TaskVerdictFailed             TaskVerdict = "failed"
)

type ArtifactRef struct {
	Path      string `json:"path,omitempty"`
	Hash      string `json:"hash,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
}

type ClaimNode struct {
	ClaimID          string         `json:"claimId"`
	Title            string         `json:"title,omitempty"`
	Statement        string         `json:"statement"`
	Scope            string         `json:"scope,omitempty"`
	Dependencies     []string       `json:"dependencies,omitempty"`
	Applicability    string         `json:"applicability,omitempty"`
	Status           ClaimStatus    `json:"status"`
	ProposedBy       []string       `json:"proposedBy"`
	EvidenceRefs     []string       `json:"evidenceRefs,omitempty"`
	ChallengeRefs    []string       `json:"challengeRefs,omitempty"`
	VerificationRefs []string       `json:"verificationRefs,omitempty"`
	Verdict          ClaimVerdict   `json:"verdict,omitempty"`
	Confidence       float64        `json:"confidence,omitempty"`
	Rationale        string         `json:"rationale,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type ChallengeStatus string

const (
	ChallengeStatusOpen   ChallengeStatus = "open"
	ChallengeStatusClosed ChallengeStatus = "closed"
)

type ChallengeTicket struct {
	TicketID          string          `json:"ticketId"`
	ClaimID           string          `json:"claimId"`
	Kind              string          `json:"kind"`
	OpenedBy          string          `json:"openedBy"`
	Statement         string          `json:"statement"`
	Status            ChallengeStatus `json:"status"`
	EvidenceRefs      []string        `json:"evidenceRefs,omitempty"`
	VerificationRefs  []string        `json:"verificationRefs,omitempty"`
	RequestedChecks   []string        `json:"requestedChecks,omitempty"`
	ResolutionSummary string          `json:"resolutionSummary,omitempty"`
}

type EvidenceSource string

const (
	EvidenceSourceCoordinator EvidenceSource = "coordinator"
	EvidenceSourceWorker      EvidenceSource = "worker"
	EvidenceSourceVerifier    EvidenceSource = "verifier"
	EvidenceSourceArbiter     EvidenceSource = "arbiter"
	EvidenceSourceReporter    EvidenceSource = "reporter"
	EvidenceSourceActor       EvidenceSource = "actor"
)

type EvidenceKind string

const (
	EvidenceKindTaskIngested         EvidenceKind = "task_ingested"
	EvidenceKindWorkerOutput         EvidenceKind = "worker_output"
	EvidenceKindClaimProposed        EvidenceKind = "claim_proposed"
	EvidenceKindChallengeOpened      EvidenceKind = "challenge_opened"
	EvidenceKindDeterministicCheck   EvidenceKind = "deterministic_check"
	EvidenceKindSemanticVerification EvidenceKind = "semantic_verification"
	EvidenceKindArbiterDecision      EvidenceKind = "arbiter_decision"
	EvidenceKindReportGenerated      EvidenceKind = "report_generated"
	EvidenceKindActionGenerated      EvidenceKind = "action_generated"
)

type EvidenceRecord struct {
	SchemaVersion  int            `json:"schemaVersion"`
	Seq            int            `json:"seq"`
	EntryID        string         `json:"entryId"`
	RequestID      string         `json:"requestId"`
	SessionID      string         `json:"sessionId"`
	ClaimID        string         `json:"claimId,omitempty"`
	ChallengeID    string         `json:"challengeId,omitempty"`
	VerificationID string         `json:"verificationId,omitempty"`
	Kind           EvidenceKind   `json:"kind"`
	Source         EvidenceSource `json:"source"`
	ProducerID     string         `json:"producerId,omitempty"`
	ProducerRole   string         `json:"producerRole,omitempty"`
	Summary        string         `json:"summary"`
	Artifact       *ArtifactRef   `json:"artifact,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      string         `json:"createdAt"`
}

type VerificationStatus string

const (
	VerificationStatusPassed       VerificationStatus = "passed"
	VerificationStatusFailed       VerificationStatus = "failed"
	VerificationStatusInconclusive VerificationStatus = "inconclusive"
)

type VerificationResult struct {
	VerificationID    string             `json:"verificationId"`
	ClaimID           string             `json:"claimId"`
	ChallengeID       string             `json:"challengeId,omitempty"`
	Kind              string             `json:"kind"`
	Status            VerificationStatus `json:"status"`
	Summary           string             `json:"summary"`
	EvidenceRef       string             `json:"evidenceRef,omitempty"`
	Artifact          *ArtifactRef       `json:"artifact,omitempty"`
	VerdictSuggestion ClaimVerdict       `json:"verdictSuggestion,omitempty"`
	Confidence        float64            `json:"confidence,omitempty"`
	Metadata          map[string]any     `json:"metadata,omitempty"`
}

type ArbiterDecision struct {
	ClaimID      string       `json:"claimId"`
	Verdict      ClaimVerdict `json:"verdict"`
	Confidence   float64      `json:"confidence,omitempty"`
	Rationale    string       `json:"rationale,omitempty"`
	EvidenceRefs []string     `json:"evidenceRefs,omitempty"`
}

type ArbiterReport struct {
	TaskVerdict TaskVerdict       `json:"taskVerdict"`
	Summary     string            `json:"summary"`
	Decisions   []ArbiterDecision `json:"decisions"`
}

type AdjudicationReport struct {
	Summary     string   `json:"summary"`
	Highlights  []string `json:"highlights,omitempty"`
	NextActions []string `json:"nextActions,omitempty"`
}

type ActionExecution struct {
	FullResponse string `json:"fullResponse"`
	Summary      string `json:"summary"`
}

type ActionOutput struct {
	ActorID      string `json:"actorId"`
	Status       string `json:"status"`
	FullResponse string `json:"fullResponse,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Error        string `json:"error,omitempty"`
}

type Metrics struct {
	ElapsedMs         int64 `json:"elapsedMs"`
	ClaimsProposed    int   `json:"claimsProposed"`
	ChallengesOpened  int   `json:"challengesOpened"`
	VerificationsRun  int   `json:"verificationsRun"`
	TasksDispatched   int   `json:"tasksDispatched"`
	WaitTimeouts      int   `json:"waitTimeouts"`
	GlobalDeadlineHit bool  `json:"globalDeadlineHit"`
}

type FailureInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AdjudicationResult struct {
	SchemaVersion    int                `json:"schemaVersion"`
	RequestID        string             `json:"requestId"`
	SessionID        string             `json:"sessionId"`
	TaskSpec         TaskSpec           `json:"taskSpec"`
	TaskVerdict      TaskVerdict        `json:"taskVerdict"`
	ClaimGraph       []ClaimNode        `json:"claimGraph"`
	ChallengeTickets []ChallengeTicket  `json:"challengeTickets"`
	ArbiterReport    ArbiterReport      `json:"arbiterReport"`
	Report           AdjudicationReport `json:"report"`
	Action           *ActionOutput      `json:"action,omitempty"`
	Metrics          Metrics            `json:"metrics"`
	Error            *FailureInfo       `json:"error,omitempty"`
}
