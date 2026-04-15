package consensus

type ClaimCategory string

const (
	ClaimCategoryPro      ClaimCategory = "pro"
	ClaimCategoryCon      ClaimCategory = "con"
	ClaimCategoryRisk     ClaimCategory = "risk"
	ClaimCategoryTradeoff ClaimCategory = "tradeoff"
	ClaimCategoryTodo     ClaimCategory = "todo"
)

type ClaimStatus string

const (
	ClaimStatusActive    ClaimStatus = "active"
	ClaimStatusMerged    ClaimStatus = "merged"
	ClaimStatusWithdrawn ClaimStatus = "withdrawn"
)

type Claim struct {
	ClaimID    string        `json:"claimId"`
	Title      string        `json:"title"`
	Statement  string        `json:"statement"`
	Category   ClaimCategory `json:"category,omitempty"`
	ProposedBy []string      `json:"proposedBy"`
	Status     ClaimStatus   `json:"status"`
	MergedInto string        `json:"mergedInto,omitempty"`
}

type ClaimStance string

const (
	ClaimStanceAgree    ClaimStance = "agree"
	ClaimStanceDisagree ClaimStance = "disagree"
	ClaimStanceRevise   ClaimStance = "revise"
)

type ClaimJudgement struct {
	ClaimID          string      `json:"claimId"`
	Stance           ClaimStance `json:"stance"`
	Confidence       float64     `json:"confidence"`
	Rationale        string      `json:"rationale"`
	RevisedStatement string      `json:"revisedStatement,omitempty"`
	MergesWith       string      `json:"mergesWith,omitempty"`
}

type ClaimVoteInput struct {
	ClaimID string `json:"claimId"`
	Vote    string `json:"vote"`
	Reason  string `json:"reason,omitempty"`
}

type ClaimVote struct {
	ParticipantID string `json:"participantId"`
	ClaimID       string `json:"claimId"`
	Vote          string `json:"vote"`
	Reason        string `json:"reason,omitempty"`
}

type Phase string

const (
	PhaseInitial   Phase = "initial"
	PhaseDebate    Phase = "debate"
	PhaseFinalVote Phase = "final_vote"
)

type ExtractedClaim struct {
	ClaimID   string        `json:"claimId,omitempty"`
	Title     string        `json:"title"`
	Statement string        `json:"statement"`
	Category  ClaimCategory `json:"category,omitempty"`
}

type ParticipantRoundOutput struct {
	ParticipantID   string           `json:"participantId"`
	Phase           Phase            `json:"phase"`
	Round           int              `json:"round"`
	FullResponse    string           `json:"fullResponse"`
	ExtractedClaims []ExtractedClaim `json:"extractedClaims,omitempty"`
	Judgements      []ClaimJudgement `json:"judgements"`
	SelfScore       float64          `json:"selfScore,omitempty"`
	Summary         string           `json:"summary"`
	TaskTitle       string           `json:"taskTitle,omitempty"`
	ClaimVotes      []ClaimVoteInput `json:"claimVotes,omitempty"`
	RespondedAt     string           `json:"respondedAt,omitempty"`
}

type ParticipantScoreBreakdown struct {
	Correctness   float64 `json:"correctness,omitempty"`
	Completeness  float64 `json:"completeness,omitempty"`
	Actionability float64 `json:"actionability,omitempty"`
	Consistency   float64 `json:"consistency,omitempty"`
}

type ParticipantRoundScore struct {
	Round int     `json:"round"`
	Score float64 `json:"score"`
}

type ParticipantScore struct {
	ParticipantID string                     `json:"participantId"`
	Total         float64                    `json:"total"`
	ByRound       []ParticipantRoundScore    `json:"byRound"`
	Breakdown     *ParticipantScoreBreakdown `json:"breakdown,omitempty"`
}

type OpinionShift struct {
	ClaimID       string      `json:"claimId"`
	ParticipantID string      `json:"participantId"`
	From          string      `json:"from"`
	To            ClaimStance `json:"to"`
	Round         int         `json:"round"`
	Reason        string      `json:"reason,omitempty"`
}

type ClaimResolutionStatus string

const (
	ClaimResolutionResolved   ClaimResolutionStatus = "resolved"
	ClaimResolutionUnresolved ClaimResolutionStatus = "unresolved"
)

type ClaimResolution struct {
	ClaimID     string                `json:"claimId"`
	Status      ClaimResolutionStatus `json:"status"`
	AcceptCount int                   `json:"acceptCount"`
	RejectCount int                   `json:"rejectCount"`
	TotalVoters int                   `json:"totalVoters"`
	Votes       []ClaimVote           `json:"votes"`
}

type FinalReport struct {
	Mode                 string           `json:"mode"`
	TraceIncluded        bool             `json:"traceIncluded"`
	TraceLevel           TraceLevel       `json:"traceLevel"`
	FinalSummary         string           `json:"finalSummary"`
	RepresentativeSpeech string           `json:"representativeSpeech"`
	OpinionShiftTimeline []OpinionShift   `json:"opinionShiftTimeline,omitempty"`
	RoundHighlights      []RoundHighlight `json:"roundHighlights,omitempty"`
}

type RoundHighlight struct {
	Round         int    `json:"round"`
	ParticipantID string `json:"participantId"`
	Summary       string `json:"summary"`
}

type EliminationRecord struct {
	ParticipantID string `json:"participantId"`
	Round         int    `json:"round"`
	Reason        string `json:"reason"`
	At            string `json:"at"`
}

type RoundAppliedMerge struct {
	SourceClaimID  string   `json:"sourceClaimId"`
	TargetClaimID  string   `json:"targetClaimId"`
	ParticipantIDs []string `json:"participantIds"`
}

type RoundRecord struct {
	Round         int                      `json:"round"`
	Outputs       []ParticipantRoundOutput `json:"outputs"`
	AppliedMerges []RoundAppliedMerge      `json:"appliedMerges,omitempty"`
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

type ConsensusTask struct {
	Prompt string `json:"prompt"`
	Title  string `json:"title"`
}

type ConsensusStatus string

const (
	ConsensusStatusConsensus        ConsensusStatus = "consensus"
	ConsensusStatusPartialConsensus ConsensusStatus = "partial_consensus"
	ConsensusStatusUnresolved       ConsensusStatus = "unresolved"
	ConsensusStatusFailed           ConsensusStatus = "failed"
)

type RepresentativeReason string

const (
	RepresentativeReasonTopScore       RepresentativeReason = "top-score"
	RepresentativeReasonTieBreaker     RepresentativeReason = "tie-breaker"
	RepresentativeReasonHostDesignated RepresentativeReason = "host-designated"
)

type Representative struct {
	ParticipantID string               `json:"participantId"`
	Reason        RepresentativeReason `json:"reason"`
	Score         float64              `json:"score"`
	Speech        string               `json:"speech"`
}

type Disagreement struct {
	ClaimID       string `json:"claimId"`
	ParticipantID string `json:"participantId"`
	Reason        string `json:"reason"`
}

type Metrics struct {
	ElapsedMs          int64 `json:"elapsedMs"`
	TotalRounds        int   `json:"totalRounds"`
	TotalTurns         int   `json:"totalTurns"`
	Retries            int   `json:"retries"`
	WaitTimeouts       int   `json:"waitTimeouts"`
	EarlyStopTriggered bool  `json:"earlyStopTriggered"`
	GlobalDeadlineHit  bool  `json:"globalDeadlineHit"`
}

type FailureInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ConsensusResult struct {
	ResultVersion    int                 `json:"resultVersion"`
	RequestID        string              `json:"requestId"`
	SessionID        string              `json:"sessionId"`
	Task             ConsensusTask       `json:"task"`
	Status           ConsensusStatus     `json:"status"`
	FinalClaims      []Claim             `json:"finalClaims"`
	ClaimResolutions []ClaimResolution   `json:"claimResolutions"`
	Representative   Representative      `json:"representative"`
	Scoreboard       []ParticipantScore  `json:"scoreboard"`
	Eliminations     []EliminationRecord `json:"eliminations"`
	Report           FinalReport         `json:"report"`
	Disagreements    []Disagreement      `json:"disagreements,omitempty"`
	Rounds           []RoundRecord       `json:"rounds"`
	Metrics          Metrics             `json:"metrics"`
	Action           *ActionOutput       `json:"action,omitempty"`
	Error            *FailureInfo        `json:"error,omitempty"`
}
