package consensus

import (
	"encoding/json"
	"fmt"
)

type ClaimStatus string

const (
	ClaimStatusProposed    ClaimStatus = "proposed"
	ClaimStatusChallenged  ClaimStatus = "challenged"
	ClaimStatusVerified    ClaimStatus = "verified"
	ClaimStatusRevised     ClaimStatus = "revised"
	ClaimStatusAdjudicated ClaimStatus = "adjudicated"
	ClaimStatusWithdrawn   ClaimStatus = "withdrawn"
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

type CaseTaskType string

const (
	CaseTaskTypeUnknown     CaseTaskType = "unknown"
	CaseTaskTypeFactual     CaseTaskType = "factual"
	CaseTaskTypeCoding      CaseTaskType = "coding"
	CaseTaskTypeStrategy    CaseTaskType = "strategy"
	CaseTaskTypeOperational CaseTaskType = "operational"
)

type RiskLevel string

const (
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
)

type EvidenceLevel string

const (
	EvidenceLevelLow    EvidenceLevel = "low"
	EvidenceLevelMedium EvidenceLevel = "medium"
	EvidenceLevelHigh   EvidenceLevel = "high"
)

type ClaimType string

const (
	ClaimTypeFact           ClaimType = "fact"
	ClaimTypeInference      ClaimType = "inference"
	ClaimTypeRecommendation ClaimType = "recommendation"
	ClaimTypeAssumption     ClaimType = "assumption"
)

type AttackSeverity string

const (
	AttackSeverityLow    AttackSeverity = "low"
	AttackSeverityMedium AttackSeverity = "medium"
	AttackSeverityHigh   AttackSeverity = "high"
)

type ClaimDisposition string

const (
	ClaimDispositionKeep           ClaimDisposition = "keep"
	ClaimDispositionKeepWithCaveat ClaimDisposition = "keep_with_caveat"
	ClaimDispositionUnresolved     ClaimDisposition = "unresolved"
	ClaimDispositionReject         ClaimDisposition = "reject"
)

type Actionability string

const (
	ActionabilityReady   Actionability = "ready"
	ActionabilityGated   Actionability = "gated"
	ActionabilityBlocked Actionability = "blocked"
)

type WorkflowTerminalState string

const (
	TerminalStateCompleted            WorkflowTerminalState = "completed"
	TerminalStateInsufficientEvidence WorkflowTerminalState = "insufficient_evidence"
	TerminalStateUnresolvedConflict   WorkflowTerminalState = "unresolved_conflict"
	TerminalStateRequiresHumanReview  WorkflowTerminalState = "requires_human_review"
	TerminalStateActionBlockedByRisk  WorkflowTerminalState = "action_blocked_by_risk"
)

type RevisionAction string

const (
	RevisionActionRevise     RevisionAction = "revise"
	RevisionActionDowngrade  RevisionAction = "downgrade_confidence"
	RevisionActionWithdraw   RevisionAction = "withdraw"
	RevisionActionUnresolved RevisionAction = "mark_unresolved"
	RevisionActionUnchanged  RevisionAction = "unchanged"
)

type ObservationOutcome string

const (
	ObservationOutcomePending      ObservationOutcome = "pending"
	ObservationOutcomeHeldUp       ObservationOutcome = "held_up"
	ObservationOutcomeContradicted ObservationOutcome = "contradicted"
	ObservationOutcomeNoAction     ObservationOutcome = "no_action"
	ObservationOutcomeFollowUp     ObservationOutcome = "follow_up_recommended"
)

type ActionRiskGate string

const (
	ActionRiskGateLowOnly     ActionRiskGate = "low_only"
	ActionRiskGateAllowMedium ActionRiskGate = "allow_medium"
	ActionRiskGateAllowHigh   ActionRiskGate = "allow_high"
)

type ProvenanceQuality string

const (
	ProvenanceQualityLow    ProvenanceQuality = "low"
	ProvenanceQualityMedium ProvenanceQuality = "medium"
	ProvenanceQualityHigh   ProvenanceQuality = "high"
)

type EvidencePerspective string

const (
	EvidencePerspectiveFirstHand  EvidencePerspective = "first_hand"
	EvidencePerspectiveSecondHand EvidencePerspective = "second_hand"
)

type CaseManifest struct {
	CaseID                    string          `json:"caseId"`
	CanonicalProblemStatement string          `json:"canonicalProblemStatement"`
	TaskType                  CaseTaskType    `json:"taskType"`
	Constraints               TaskConstraints `json:"constraints,omitempty"`
	SuccessCriteria           []string        `json:"successCriteria,omitempty"`
	OutOfScope                []string        `json:"outOfScope,omitempty"`
	RiskLevel                 RiskLevel       `json:"riskLevel"`
	RequiredEvidenceLevel     EvidenceLevel   `json:"requiredEvidenceLevel"`
	AllowedTools              []string        `json:"allowedTools,omitempty"`
	UnresolvedQuestions       []string        `json:"unresolvedQuestions,omitempty"`
}

type ArtifactRef struct {
	Path      string `json:"path,omitempty"`
	Hash      string `json:"hash,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
}

type ArtifactManifestEntry struct {
	SchemaVersion  int          `json:"schemaVersion"`
	Seq            int          `json:"seq"`
	EntryID        string       `json:"entryId"`
	RequestID      string       `json:"requestId"`
	SessionID      string       `json:"sessionId"`
	ClaimID        string       `json:"claimId,omitempty"`
	ChallengeID    string       `json:"challengeId,omitempty"`
	VerificationID string       `json:"verificationId,omitempty"`
	Kind           EvidenceKind `json:"kind"`
	ProducerRole   string       `json:"producerRole,omitempty"`
	Artifact       ArtifactRef  `json:"artifact"`
	LoggedAt       string       `json:"loggedAt"`
}

type ClaimNode struct {
	ClaimID               string           `json:"claimId"`
	Title                 string           `json:"title,omitempty"`
	Statement             string           `json:"statement"`
	ClaimText             string           `json:"claimText,omitempty"`
	ClaimType             ClaimType        `json:"claimType,omitempty"`
	Scope                 string           `json:"scope,omitempty"`
	Dependencies          []string         `json:"dependencies,omitempty"`
	ParentClaimIDs        []string         `json:"parentClaimIds,omitempty"`
	Applicability         string           `json:"applicability,omitempty"`
	BoundaryConditions    []string         `json:"boundaryConditions,omitempty"`
	Status                ClaimStatus      `json:"status"`
	ProposedBy            []string         `json:"proposedBy"`
	EvidenceRefs          []string         `json:"evidenceRefs,omitempty"`
	SupportingEvidenceIDs []string         `json:"supportingEvidenceIds,omitempty"`
	OpposingEvidenceIDs   []string         `json:"opposingEvidenceIds,omitempty"`
	ChallengeRefs         []string         `json:"challengeRefs,omitempty"`
	VerificationRefs      []string         `json:"verificationRefs,omitempty"`
	Verdict               ClaimVerdict     `json:"verdict,omitempty"`
	Disposition           ClaimDisposition `json:"disposition,omitempty"`
	Confidence            float64          `json:"confidence,omitempty"`
	Rationale             string           `json:"rationale,omitempty"`
	Caveats               []string         `json:"caveats,omitempty"`
	LastRevisionRound     int              `json:"lastRevisionRound,omitempty"`
	SourceProposalAgentID string           `json:"sourceProposalAgentId,omitempty"`
	Metadata              map[string]any   `json:"metadata,omitempty"`
}

type ChallengeStatus string

const (
	ChallengeStatusOpen   ChallengeStatus = "open"
	ChallengeStatusClosed ChallengeStatus = "closed"
)

type ChallengeTicket struct {
	TicketID                     string          `json:"ticketId"`
	ClaimID                      string          `json:"claimId"`
	Kind                         string          `json:"kind"`
	AttackType                   string          `json:"attackType,omitempty"`
	Severity                     AttackSeverity  `json:"severity,omitempty"`
	OpenedBy                     string          `json:"openedBy"`
	Statement                    string          `json:"statement"`
	AttackText                   string          `json:"attackText,omitempty"`
	Status                       ChallengeStatus `json:"status"`
	EvidenceRefs                 []string        `json:"evidenceRefs,omitempty"`
	VerificationRefs             []string        `json:"verificationRefs,omitempty"`
	RequestedChecks              []string        `json:"requestedChecks,omitempty"`
	SuggestedFalsificationMethod string          `json:"suggestedFalsificationMethod,omitempty"`
	ResolutionSummary            string          `json:"resolutionSummary,omitempty"`
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
	EvidenceKindCaseFramed           EvidenceKind = "case_framed"
	EvidenceKindTaskIngested         EvidenceKind = "task_ingested"
	EvidenceKindSourceMaterial       EvidenceKind = "source_material"
	EvidenceKindWorkerOutput         EvidenceKind = "worker_output"
	EvidenceKindClaimProposed        EvidenceKind = "claim_proposed"
	EvidenceKindChallengeOpened      EvidenceKind = "challenge_opened"
	EvidenceKindDeterministicCheck   EvidenceKind = "deterministic_check"
	EvidenceKindSemanticVerification EvidenceKind = "semantic_verification"
	EvidenceKindClaimRevised         EvidenceKind = "claim_revised"
	EvidenceKindArbiterDecision      EvidenceKind = "arbiter_decision"
	EvidenceKindAdjudicationFallback EvidenceKind = "adjudication_fallback"
	EvidenceKindFollowUpCaseCreated  EvidenceKind = "follow_up_case_created"
	EvidenceKindReportGenerated      EvidenceKind = "report_generated"
	EvidenceKindActionGenerated      EvidenceKind = "action_generated"
	EvidenceKindObservationRecorded  EvidenceKind = "observation_recorded"
	EvidenceKindDebateRoundOpened    EvidenceKind = "debate_round_opened"
	EvidenceKindDebateRoundOutput    EvidenceKind = "debate_round_output"
	EvidenceKindDebateVoteCast       EvidenceKind = "debate_vote_cast"
	EvidenceKindDelphiRoundOpened    EvidenceKind = "delphi_round_opened"
	EvidenceKindDelphiResponse       EvidenceKind = "delphi_response_recorded"
	EvidenceKindDelphiRoundSummary   EvidenceKind = "delphi_round_summary"
	EvidenceKindDelphiConvergence    EvidenceKind = "delphi_convergence_reached"
)

type EvidenceRecord struct {
	SchemaVersion         int                 `json:"schemaVersion"`
	Seq                   int                 `json:"seq"`
	EntryID               string              `json:"entryId"`
	RequestID             string              `json:"requestId"`
	SessionID             string              `json:"sessionId"`
	ClaimID               string              `json:"claimId,omitempty"`
	ChallengeID           string              `json:"challengeId,omitempty"`
	VerificationID        string              `json:"verificationId,omitempty"`
	Kind                  EvidenceKind        `json:"kind"`
	Source                EvidenceSource      `json:"source"`
	SourceType            string              `json:"sourceType,omitempty"`
	SourceLocator         string              `json:"sourceLocator,omitempty"`
	ProducerID            string              `json:"producerId,omitempty"`
	ProducerRole          string              `json:"producerRole,omitempty"`
	Summary               string              `json:"summary"`
	ContentExcerpt        string              `json:"contentExcerpt,omitempty"`
	ObservedAt            string              `json:"observedAt,omitempty"`
	ProvenanceQuality     ProvenanceQuality   `json:"provenanceQuality,omitempty"`
	FirstHandVsSecondHand EvidencePerspective `json:"firstHandVsSecondHand,omitempty"`
	ConflictsWith         []string            `json:"conflictsWith,omitempty"`
	Notes                 []string            `json:"notes,omitempty"`
	Artifact              *ArtifactRef        `json:"artifact,omitempty"`
	Metadata              map[string]any      `json:"metadata,omitempty"`
	CreatedAt             string              `json:"createdAt"`
}

type VerificationStatus string

const (
	VerificationStatusPassed       VerificationStatus = "passed"
	VerificationStatusFailed       VerificationStatus = "failed"
	VerificationStatusInconclusive VerificationStatus = "inconclusive"
)

type VerificationResult struct {
	VerificationID     string             `json:"verificationId"`
	ClaimID            string             `json:"claimId"`
	ChallengeID        string             `json:"challengeId,omitempty"`
	CheckName          string             `json:"checkName,omitempty"`
	Kind               string             `json:"kind"`
	Method             string             `json:"method,omitempty"`
	Status             VerificationStatus `json:"status"`
	Result             VerificationStatus `json:"result,omitempty"`
	FailureCode        string             `json:"failureCode,omitempty"`
	Summary            string             `json:"summary"`
	EvidenceRef        string             `json:"evidenceRef,omitempty"`
	RawOutputReference string             `json:"rawOutputReference,omitempty"`
	Artifact           *ArtifactRef       `json:"artifact,omitempty"`
	VerdictSuggestion  ClaimVerdict       `json:"verdictSuggestion,omitempty"`
	Confidence         float64            `json:"confidence,omitempty"`
	ConfidenceDelta    float64            `json:"confidenceDelta,omitempty"`
	Notes              []string           `json:"notes,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty"`
}

type AdjudicationRecord struct {
	TargetClaimID   string           `json:"targetClaimId"`
	Disposition     ClaimDisposition `json:"disposition"`
	Rationale       string           `json:"rationale,omitempty"`
	FinalConfidence float64          `json:"finalConfidence,omitempty"`
	BlockingRisks   []string         `json:"blockingRisks,omitempty"`
	Actionability   Actionability    `json:"actionability,omitempty"`
	EvidenceRefs    []string         `json:"evidenceRefs,omitempty"`
}

type ArbiterDecision struct {
	ClaimID      string       `json:"claimId"`
	Verdict      ClaimVerdict `json:"verdict"`
	Confidence   float64      `json:"confidence,omitempty"`
	Rationale    string       `json:"rationale,omitempty"`
	EvidenceRefs []string     `json:"evidenceRefs,omitempty"`
}

type ArbiterReport struct {
	TaskVerdict TaskVerdict          `json:"taskVerdict"`
	Summary     string               `json:"summary"`
	Decisions   []ArbiterDecision    `json:"decisions"`
	Records     []AdjudicationRecord `json:"records,omitempty"`
}

type AdjudicationReport struct {
	Summary             string   `json:"summary"`
	Highlights          []string `json:"highlights,omitempty"`
	RetainedClaims      []string `json:"retainedClaims,omitempty"`
	DowngradedClaims    []string `json:"downgradedClaims,omitempty"`
	UnresolvedQuestions []string `json:"unresolvedQuestions,omitempty"`
	NextActions         []string `json:"nextActions,omitempty"`
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
	Executed     bool   `json:"executed,omitempty"`
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

type ClaimRevisionRecord struct {
	RevisionID         string         `json:"revisionId"`
	TargetClaimID      string         `json:"targetClaimId"`
	ProposerID         string         `json:"proposerId,omitempty"`
	Action             RevisionAction `json:"action"`
	RevisedText        string         `json:"revisedText,omitempty"`
	ConfidenceDelta    float64        `json:"confidenceDelta,omitempty"`
	Caveats            []string       `json:"caveats,omitempty"`
	BoundaryConditions []string       `json:"boundaryConditions,omitempty"`
	Unresolved         bool           `json:"unresolved,omitempty"`
	Reason             string         `json:"reason,omitempty"`
	EvidenceRefs       []string       `json:"evidenceRefs,omitempty"`
	Round              int            `json:"round"`
}

type ObservationRecord struct {
	ObservationID     string             `json:"observationId"`
	Outcome           ObservationOutcome `json:"outcome"`
	Summary           string             `json:"summary"`
	EvidenceRefs      []string           `json:"evidenceRefs,omitempty"`
	FollowUpCaseID    string             `json:"followUpCaseId,omitempty"`
	FollowUpRequestID string             `json:"followUpRequestId,omitempty"`
	FollowUpArtifact  *ArtifactRef       `json:"followUpArtifact,omitempty"`
	Reopen            bool               `json:"reopen,omitempty"`
}

type AdjudicationResultSection struct {
	TaskVerdict         TaskVerdict           `json:"taskVerdict"`
	TerminalState       WorkflowTerminalState `json:"terminalState,omitempty"`
	ClaimGraph          []ClaimNode           `json:"claimGraph"`
	ChallengeTickets    []ChallengeTicket     `json:"challengeTickets"`
	VerificationResults []VerificationResult  `json:"verificationResults,omitempty"`
	RevisionRecords     []ClaimRevisionRecord `json:"revisionRecords,omitempty"`
	AdjudicationRecords []AdjudicationRecord  `json:"adjudicationRecords,omitempty"`
	ArbiterReport       ArbiterReport         `json:"arbiterReport"`
}

type DebateJudgement string

const (
	DebateJudgementAgree    DebateJudgement = "agree"
	DebateJudgementDisagree DebateJudgement = "disagree"
	DebateJudgementRevise   DebateJudgement = "revise"
	DebateJudgementNoChange DebateJudgement = "no_change"
)

type DebateVoteChoice string

const (
	DebateVoteAccept  DebateVoteChoice = "accept"
	DebateVoteReject  DebateVoteChoice = "reject"
	DebateVoteAbstain DebateVoteChoice = "abstain"
)

type FreeDebateOutcome string

const (
	FreeDebateOutcomeConsensus        FreeDebateOutcome = "consensus"
	FreeDebateOutcomePartialConsensus FreeDebateOutcome = "partial_consensus"
	FreeDebateOutcomeNoConsensus      FreeDebateOutcome = "no_consensus"
)

type DebateClaim struct {
	ClaimID      string   `json:"claimId"`
	Title        string   `json:"title,omitempty"`
	Statement    string   `json:"statement"`
	OwnerID      string   `json:"ownerId"`
	Round        int      `json:"round"`
	Active       bool     `json:"active"`
	MergedInto   string   `json:"mergedInto,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}

type DebateJudgementRecord struct {
	ClaimID         string          `json:"claimId"`
	Judgement       DebateJudgement `json:"judgement"`
	Rationale       string          `json:"rationale,omitempty"`
	RevisedClaimID  string          `json:"revisedClaimId,omitempty"`
	MergeWithClaims []string        `json:"mergeWithClaims,omitempty"`
}

type DebateVoteRecord struct {
	ClaimID     string           `json:"claimId"`
	AgentID     string           `json:"agentId"`
	Vote        DebateVoteChoice `json:"vote"`
	Rationale   string           `json:"rationale,omitempty"`
	EvidenceRef string           `json:"evidenceRef,omitempty"`
}

type DebateParticipantOutput struct {
	AgentID     string                  `json:"agentId"`
	Summary     string                  `json:"summary"`
	NewClaimIDs []string                `json:"newClaimIds,omitempty"`
	Judgements  []DebateJudgementRecord `json:"judgements,omitempty"`
	Votes       []DebateVoteRecord      `json:"votes,omitempty"`
}

type DebateRoundRecord struct {
	Round              int                       `json:"round"`
	Phase              string                    `json:"phase"`
	Summary            string                    `json:"summary,omitempty"`
	ParticipantOutputs []DebateParticipantOutput `json:"participantOutputs"`
}

type DebateClaimResolution struct {
	ClaimID          string   `json:"claimId"`
	Accepted         bool     `json:"accepted"`
	SupportRatio     float64  `json:"supportRatio"`
	SupportingVoters []string `json:"supportingVoters,omitempty"`
	OpposingVoters   []string `json:"opposingVoters,omitempty"`
	FinalStatement   string   `json:"finalStatement,omitempty"`
	MergedInto       string   `json:"mergedInto,omitempty"`
}

type FreeDebateResultSection struct {
	Outcome          FreeDebateOutcome       `json:"outcome"`
	Rounds           []DebateRoundRecord     `json:"rounds"`
	Claims           []DebateClaim           `json:"claims"`
	ClaimResolutions []DebateClaimResolution `json:"claimResolutions"`
	Votes            []DebateVoteRecord      `json:"votes"`
}

type DelphiResponseRecord struct {
	StatementID string  `json:"statementId,omitempty"`
	Statement   string  `json:"statement,omitempty"`
	Rating      float64 `json:"rating"`
	Rationale   string  `json:"rationale,omitempty"`
}

type DelphiRoundRecord struct {
	Round      int                    `json:"round"`
	Phase      string                 `json:"phase"`
	Summary    string                 `json:"summary,omitempty"`
	Responses  []DelphiResponseRecord `json:"responses,omitempty"`
	Statements []DelphiStatement      `json:"statements,omitempty"`
}

type DelphiStatement struct {
	StatementID           string   `json:"statementId"`
	Statement             string   `json:"statement"`
	MeanRating            float64  `json:"meanRating"`
	ConsensusLevel        float64  `json:"consensusLevel"`
	ResponseCount         int      `json:"responseCount"`
	LastRound             int      `json:"lastRound"`
	RepresentativeReasons []string `json:"representativeReasons,omitempty"`
}

type DelphiResultSection struct {
	Rounds              []DelphiRoundRecord  `json:"rounds"`
	Statements          []DelphiStatement    `json:"statements"`
	RatingDistributions map[string][]float64 `json:"ratingDistributions,omitempty"`
	ConsensusLevel      float64              `json:"consensusLevel"`
	Recommendation      string               `json:"recommendation"`
	DissentSummary      []string             `json:"dissentSummary,omitempty"`
}

type RunResult struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Mode          WorkflowMode               `json:"mode"`
	RequestID     string                     `json:"requestId"`
	SessionID     string                     `json:"sessionId"`
	Lineage       *RunLineage                `json:"lineage,omitempty"`
	TaskSpec      TaskSpec                   `json:"taskSpec"`
	CaseManifest  *CaseManifest              `json:"caseManifest,omitempty"`
	TerminalState WorkflowTerminalState      `json:"terminalState,omitempty"`
	Report        AdjudicationReport         `json:"report"`
	Action        *ActionOutput              `json:"action,omitempty"`
	Observations  []ObservationRecord        `json:"observations,omitempty"`
	Metrics       Metrics                    `json:"metrics"`
	Error         *FailureInfo               `json:"error,omitempty"`
	Adjudication  *AdjudicationResultSection `json:"adjudication,omitempty"`
	FreeDebate    *FreeDebateResultSection   `json:"freeDebate,omitempty"`
	Delphi        *DelphiResultSection       `json:"delphi,omitempty"`
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

func RunResultFromLegacy(result AdjudicationResult) RunResult {
	return RunResult{
		SchemaVersion: result.SchemaVersion,
		Mode:          WorkflowModeAdjudication,
		RequestID:     result.RequestID,
		SessionID:     result.SessionID,
		TaskSpec:      result.TaskSpec,
		Report:        result.Report,
		Action:        result.Action,
		Metrics:       result.Metrics,
		Error:         result.Error,
		TerminalState: TerminalStateCompleted,
		Adjudication: &AdjudicationResultSection{
			TaskVerdict:      result.TaskVerdict,
			ClaimGraph:       result.ClaimGraph,
			ChallengeTickets: result.ChallengeTickets,
			ArbiterReport:    result.ArbiterReport,
		},
	}
}

func (r RunResult) LegacyAdjudicationResult() *AdjudicationResult {
	if r.Mode != WorkflowModeAdjudication || r.Adjudication == nil {
		return nil
	}
	return &AdjudicationResult{
		SchemaVersion:    r.SchemaVersion,
		RequestID:        r.RequestID,
		SessionID:        r.SessionID,
		TaskSpec:         r.TaskSpec,
		TaskVerdict:      r.Adjudication.TaskVerdict,
		ClaimGraph:       r.Adjudication.ClaimGraph,
		ChallengeTickets: r.Adjudication.ChallengeTickets,
		ArbiterReport:    r.Adjudication.ArbiterReport,
		Report:           r.Report,
		Action:           r.Action,
		Metrics:          r.Metrics,
		Error:            r.Error,
	}
}

func DecodeRunResult(body []byte) (RunResult, error) {
	var probe struct {
		Mode          WorkflowMode `json:"mode"`
		SchemaVersion int          `json:"schemaVersion"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return RunResult{}, fmt.Errorf("decode result probe: %w", err)
	}
	if probe.Mode != "" {
		var result RunResult
		if err := json.Unmarshal(body, &result); err != nil {
			return RunResult{}, fmt.Errorf("decode run result: %w", err)
		}
		if result.SchemaVersion != SchemaVersion {
			return RunResult{}, fmt.Errorf("unsupported result schema version: %d", result.SchemaVersion)
		}
		return result, nil
	}
	var legacy AdjudicationResult
	if err := json.Unmarshal(body, &legacy); err != nil {
		return RunResult{}, fmt.Errorf("decode legacy adjudication result: %w", err)
	}
	if legacy.SchemaVersion != 1 {
		return RunResult{}, fmt.Errorf("unsupported result schema version: %d", legacy.SchemaVersion)
	}
	return RunResultFromLegacy(legacy), nil
}
