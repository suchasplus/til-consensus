package viewer

import (
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/telemetry"
)

const (
	FormatText     = "text"
	FormatMarkdown = "markdown"
	FormatJSON     = "json"

	SectionAll           = "all"
	SectionOverview      = "overview"
	SectionClaims        = "claims"
	SectionChallenges    = "challenges"
	SectionVerifications = "verifications"
	SectionObservations  = "observations"
	SectionFollowups     = "followups"
	SectionArtifacts     = "artifacts"
	SectionDebug         = "debug"
	SectionRounds        = "rounds"
	SectionVotes         = "votes"
	SectionStatements    = "statements"
	SectionConvergence   = "convergence"
)

type RunFiles struct {
	RunDir                string
	ArtifactsDir          string
	ResultPath            string
	LedgerPath            string
	SummaryPath           string
	ManifestPath          string
	EventsPath            string
	ComplianceSummaryPath string
	ProviderReadinessPath string
	RunTelemetryPath      string
}

type RenderOptions struct {
	Format       string
	Sections     []string
	ClaimVerdict consensus.ClaimVerdict
	Limit        int
	Verbose      bool
}

type Bundle struct {
	Result            consensus.RunResult
	Ledger            []consensus.EvidenceRecord
	Manifest          []consensus.ArtifactManifestEntry
	Events            []consensus.RunEventRecord
	ComplianceSummary ComplianceSummaryFile
	ComplianceReports []ComplianceReportFile
	ProviderReadiness telemetry.ProviderReadinessFile
	RunTelemetry      telemetry.RunTelemetryFile
	Files             RunFiles
	Missing           []string
}

type Overview struct {
	RequestID         string         `json:"requestId"`
	SessionID         string         `json:"sessionId"`
	ParentRequestID   string         `json:"parentRequestId,omitempty"`
	ParentSessionID   string         `json:"parentSessionId,omitempty"`
	ParentCaseID      string         `json:"parentCaseId,omitempty"`
	Trigger           string         `json:"trigger,omitempty"`
	Mode              string         `json:"mode"`
	PrimaryResult     string         `json:"primaryResult"`
	TerminalState     string         `json:"terminalState,omitempty"`
	Elapsed           string         `json:"elapsed"`
	RunDir            string         `json:"runDir"`
	Goal              string         `json:"goal"`
	TaskType          string         `json:"taskType,omitempty"`
	RiskLevel         string         `json:"riskLevel,omitempty"`
	RequiredEvidence  string         `json:"requiredEvidence,omitempty"`
	SuccessCriteria   []string       `json:"successCriteria,omitempty"`
	WorkspaceRoot     string         `json:"workspaceRoot,omitempty"`
	WorkspaceRev      string         `json:"workspaceRevision,omitempty"`
	WorkspacePaths    []string       `json:"workspacePaths,omitempty"`
	ClaimCounts       map[string]int `json:"claimCounts,omitempty"`
	ChallengeCount    int            `json:"challengeCount"`
	VerificationCount int            `json:"verificationCount"`
	ArtifactCount     int            `json:"artifactCount"`
	RoundCount        int            `json:"roundCount"`
}

type ClaimView struct {
	ClaimID       string                 `json:"claimId"`
	Title         string                 `json:"title,omitempty"`
	Statement     string                 `json:"statement"`
	ClaimType     string                 `json:"claimType,omitempty"`
	Verdict       consensus.ClaimVerdict `json:"verdict"`
	Disposition   string                 `json:"disposition,omitempty"`
	Confidence    float64                `json:"confidence,omitempty"`
	Scope         string                 `json:"scope,omitempty"`
	Rationale     string                 `json:"rationale,omitempty"`
	Caveats       []string               `json:"caveats,omitempty"`
	EvidenceRefs  []string               `json:"evidenceRefs,omitempty"`
	ChallengeRefs []string               `json:"challengeRefs,omitempty"`
}

type ChallengeView struct {
	TicketID          string                    `json:"ticketId"`
	ClaimID           string                    `json:"claimId"`
	Kind              string                    `json:"kind"`
	AttackType        string                    `json:"attackType,omitempty"`
	Severity          string                    `json:"severity,omitempty"`
	Status            consensus.ChallengeStatus `json:"status"`
	Statement         string                    `json:"statement"`
	RequestedChecks   []string                  `json:"requestedChecks,omitempty"`
	ResolutionSummary string                    `json:"resolutionSummary,omitempty"`
}

type VerificationView struct {
	EntryID           string                       `json:"entryId"`
	VerificationID    string                       `json:"verificationId,omitempty"`
	ClaimID           string                       `json:"claimId"`
	ChallengeID       string                       `json:"challengeId,omitempty"`
	Kind              string                       `json:"kind"`
	Status            consensus.VerificationStatus `json:"status"`
	FailureCode       string                       `json:"failureCode,omitempty"`
	Summary           string                       `json:"summary"`
	VerdictSuggestion consensus.ClaimVerdict       `json:"verdictSuggestion,omitempty"`
	Confidence        float64                      `json:"confidence,omitempty"`
	ArtifactPath      string                       `json:"artifactPath,omitempty"`
}

type ObservationView struct {
	ObservationID     string `json:"observationId"`
	Outcome           string `json:"outcome"`
	Summary           string `json:"summary"`
	Reopen            bool   `json:"reopen,omitempty"`
	FollowUpCaseID    string `json:"followUpCaseId,omitempty"`
	FollowUpRequestID string `json:"followUpRequestId,omitempty"`
	FollowUpArtifact  string `json:"followUpArtifact,omitempty"`
}

type FollowUpView struct {
	ObservationID     string `json:"observationId,omitempty"`
	ParentRequestID   string `json:"parentRequestId,omitempty"`
	ParentSessionID   string `json:"parentSessionId,omitempty"`
	ParentCaseID      string `json:"parentCaseId,omitempty"`
	Trigger           string `json:"trigger,omitempty"`
	FollowUpCaseID    string `json:"followUpCaseId"`
	FollowUpRequestID string `json:"followUpRequestId,omitempty"`
	ArtifactPath      string `json:"artifactPath,omitempty"`
}

type ArtifactView struct {
	Seq          int                    `json:"seq"`
	EntryID      string                 `json:"entryId"`
	ClaimID      string                 `json:"claimId,omitempty"`
	ChallengeID  string                 `json:"challengeId,omitempty"`
	Kind         consensus.EvidenceKind `json:"kind"`
	ProducerRole string                 `json:"producerRole,omitempty"`
	Path         string                 `json:"path"`
	Hash         string                 `json:"hash,omitempty"`
	MediaType    string                 `json:"mediaType,omitempty"`
}

type RoundView struct {
	Round        int      `json:"round"`
	Phase        string   `json:"phase"`
	Summary      string   `json:"summary,omitempty"`
	Participants []string `json:"participants,omitempty"`
}

type VoteView struct {
	ClaimID    string  `json:"claimId"`
	AgentID    string  `json:"agentId"`
	Vote       string  `json:"vote"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale,omitempty"`
}

type StatementView struct {
	StatementID    string   `json:"statementId"`
	Statement      string   `json:"statement"`
	MeanRating     float64  `json:"meanRating"`
	ConsensusLevel float64  `json:"consensusLevel"`
	ResponseCount  int      `json:"responseCount"`
	Reasons        []string `json:"reasons,omitempty"`
}

type ConvergenceView struct {
	ConsensusLevel float64  `json:"consensusLevel"`
	Recommendation string   `json:"recommendation"`
	DissentSummary []string `json:"dissentSummary,omitempty"`
}

type DebugEventView struct {
	Seq            int                    `json:"seq"`
	LoggedAt       string                 `json:"loggedAt"`
	EventAt        string                 `json:"eventAt,omitempty"`
	Type           consensus.RunEventType `json:"type"`
	Phase          string                 `json:"phase,omitempty"`
	Summary        string                 `json:"summary"`
	RawVerdict     string                 `json:"rawVerdict,omitempty"`
	RawTaskVerdict string                 `json:"rawTaskVerdict,omitempty"`
	Payload        map[string]any         `json:"payload,omitempty"`
	PayloadPretty  string                 `json:"payloadPretty,omitempty"`
	ArtifactHints  []string               `json:"artifactHints,omitempty"`
}

type ComplianceSummaryEntryView struct {
	Provider      string `json:"provider"`
	ProviderType  string `json:"providerType"`
	ProviderModel string `json:"providerModel"`
	TaskKind      string `json:"taskKind"`
	Total         int    `json:"total"`
	Strict        int    `json:"strict"`
	Normalized    int    `json:"normalized"`
	Repaired      int    `json:"repaired"`
	Failed        int    `json:"failed"`
}

type ComplianceReportView struct {
	TaskID               string `json:"taskId"`
	TaskKind             string `json:"taskKind"`
	AgentID              string `json:"agentId"`
	Provider             string `json:"provider"`
	ProviderType         string `json:"providerType"`
	ProviderModel        string `json:"providerModel"`
	StrictCompliant      bool   `json:"strictCompliant"`
	NormalizedWithoutFix bool   `json:"normalizedWithoutFix"`
	RepairAttempted      bool   `json:"repairAttempted"`
	RepairSucceeded      bool   `json:"repairSucceeded"`
	FinalStatus          string `json:"finalStatus"`
	StrictError          string `json:"strictError,omitempty"`
	FinalError           string `json:"finalError,omitempty"`
	RawArtifact          string `json:"rawArtifact,omitempty"`
	InitialErrorArtifact string `json:"initialErrorArtifact,omitempty"`
	FinalArtifact        string `json:"finalArtifact,omitempty"`
}

type TelemetryView struct {
	GeneratedAt string                       `json:"generatedAt,omitempty"`
	Summary     []ComplianceSummaryEntryView `json:"summary,omitempty"`
	Reports     []ComplianceReportView       `json:"reports,omitempty"`
	Readiness   *ProviderReadinessView       `json:"readiness,omitempty"`
	Run         *RunTelemetryView            `json:"run,omitempty"`
}

type ProviderReadinessEntryView struct {
	Provider        string         `json:"provider"`
	ProviderType    string         `json:"providerType,omitempty"`
	Protocol        string         `json:"protocol,omitempty"`
	Model           string         `json:"model,omitempty"`
	BaseURL         string         `json:"baseUrl,omitempty"`
	APIKeyEnv       string         `json:"apiKeyEnv,omitempty"`
	Agent           string         `json:"agent,omitempty"`
	Command         []string       `json:"command,omitempty"`
	RequestContext  map[string]any `json:"requestContext,omitempty"`
	Ready           bool           `json:"ready"`
	StrictJSON      bool           `json:"strictJSON"`
	RecoverableJSON bool           `json:"recoverableJSON"`
	DurationMs      int64          `json:"durationMs"`
	StdoutPreview   string         `json:"stdoutPreview,omitempty"`
	StderrPreview   string         `json:"stderrPreview,omitempty"`
	Error           string         `json:"error,omitempty"`
}

type ProviderReadinessView struct {
	GeneratedAt string                       `json:"generatedAt,omitempty"`
	Providers   []ProviderReadinessEntryView `json:"providers,omitempty"`
}

type RunTaskSummaryView struct {
	TaskKind   string `json:"taskKind"`
	Total      int    `json:"total"`
	Strict     int    `json:"strict"`
	Normalized int    `json:"normalized"`
	Repaired   int    `json:"repaired"`
	Failed     int    `json:"failed"`
}

type RunWorkflowSummaryView struct {
	Claims               int `json:"claims"`
	SupportedClaims      int `json:"supportedClaims"`
	RefutedClaims        int `json:"refutedClaims"`
	InsufficientClaims   int `json:"insufficientClaims"`
	UndeterminedClaims   int `json:"undeterminedClaims"`
	KeepClaims           int `json:"keepClaims"`
	KeepWithCaveatClaims int `json:"keepWithCaveatClaims"`
	UnresolvedClaims     int `json:"unresolvedClaims"`
	RejectClaims         int `json:"rejectClaims"`
	ChallengeCount       int `json:"challengeCount"`
	ObservationCount     int `json:"observationCount"`
	RoundCount           int `json:"roundCount,omitempty"`
	ClaimCount           int `json:"claimCount,omitempty"`
	VoteCount            int `json:"voteCount,omitempty"`
	StatementCount       int `json:"statementCount,omitempty"`
}

type RunVerificationSummaryView struct {
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Inconclusive int `json:"inconclusive"`
}

type RunTelemetryView struct {
	GeneratedAt string                      `json:"generatedAt,omitempty"`
	RequestID   string                      `json:"requestId,omitempty"`
	SessionID   string                      `json:"sessionId,omitempty"`
	Mode        string                      `json:"mode,omitempty"`
	Providers   []string                    `json:"providers,omitempty"`
	Primary     string                      `json:"primaryResult,omitempty"`
	TaskVerdict string                      `json:"taskVerdict,omitempty"`
	Terminal    string                      `json:"terminalState,omitempty"`
	ElapsedMs   int64                       `json:"elapsedMs,omitempty"`
	TaskSummary []RunTaskSummaryView        `json:"taskSummary,omitempty"`
	Workflow    RunWorkflowSummaryView      `json:"workflowSummary"`
	Verify      *RunVerificationSummaryView `json:"verificationSummary,omitempty"`
}

type RiskView struct {
	Category string `json:"category"`
	TargetID string `json:"targetId"`
	Summary  string `json:"summary"`
}

type FileView struct {
	RunDir   string   `json:"runDir"`
	Result   string   `json:"result"`
	Ledger   string   `json:"ledger"`
	Summary  string   `json:"summary"`
	Manifest string   `json:"manifest"`
	Missing  []string `json:"missing,omitempty"`
}

type Document struct {
	Format            string             `json:"format"`
	RequestedSections []string           `json:"requestedSections"`
	Overview          Overview           `json:"overview"`
	Claims            []ClaimView        `json:"claims,omitempty"`
	Challenges        []ChallengeView    `json:"challenges,omitempty"`
	Verifications     []VerificationView `json:"verifications,omitempty"`
	Observations      []ObservationView  `json:"observations,omitempty"`
	FollowUps         []FollowUpView     `json:"followups,omitempty"`
	Rounds            []RoundView        `json:"rounds,omitempty"`
	Votes             []VoteView         `json:"votes,omitempty"`
	Statements        []StatementView    `json:"statements,omitempty"`
	Convergence       *ConvergenceView   `json:"convergence,omitempty"`
	DebugEvents       []DebugEventView   `json:"debugEvents,omitempty"`
	Telemetry         *TelemetryView     `json:"telemetry,omitempty"`
	Artifacts         []ArtifactView     `json:"artifacts,omitempty"`
	Risks             []RiskView         `json:"risks,omitempty"`
	Files             FileView           `json:"files"`
}

type ComplianceSummaryFile struct {
	Version     int                      `json:"version"`
	GeneratedAt string                   `json:"generatedAt,omitempty"`
	Entries     []ComplianceSummaryEntry `json:"entries,omitempty"`
}

type ComplianceSummaryEntry struct {
	Provider      string             `json:"provider"`
	ProviderType  string             `json:"providerType"`
	ProviderModel string             `json:"providerModel"`
	TaskKind      consensus.TaskKind `json:"taskKind"`
	Total         int                `json:"total"`
	Strict        int                `json:"strict"`
	Normalized    int                `json:"normalized"`
	Repaired      int                `json:"repaired"`
	Failed        int                `json:"failed"`
}

type ComplianceReportFile struct {
	Version int `json:"version"`
	Agent   struct {
		ID            string `json:"id"`
		Role          string `json:"role"`
		Provider      string `json:"provider"`
		ProviderType  string `json:"providerType"`
		ProviderModel string `json:"providerModel"`
	} `json:"agent"`
	Task struct {
		TaskID    string `json:"taskId"`
		Kind      string `json:"kind"`
		RequestID string `json:"requestId"`
		SessionID string `json:"sessionId"`
		AgentID   string `json:"agentId"`
	} `json:"task"`
	Compliance struct {
		StrictCompliant      bool                   `json:"strictCompliant"`
		NormalizedWithoutFix bool                   `json:"normalizedWithoutFix"`
		RepairAttempted      bool                   `json:"repairAttempted"`
		RepairSucceeded      bool                   `json:"repairSucceeded"`
		FinalStatus          string                 `json:"finalStatus"`
		StrictError          string                 `json:"strictError,omitempty"`
		FinalError           string                 `json:"finalError,omitempty"`
		RawArtifact          *consensus.ArtifactRef `json:"rawArtifact,omitempty"`
		InitialErrorArtifact *consensus.ArtifactRef `json:"initialErrorArtifact,omitempty"`
		FinalArtifact        *consensus.ArtifactRef `json:"finalArtifact,omitempty"`
	} `json:"compliance"`
}
