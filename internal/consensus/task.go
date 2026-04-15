package consensus

type TaskKind string

const (
	TaskKindRound  TaskKind = "round"
	TaskKindReport TaskKind = "report"
	TaskKindAction TaskKind = "action"
)

type Task interface {
	Kind() TaskKind
	Meta() TaskMeta
}

type TaskMeta struct {
	SessionID     string         `json:"sessionId"`
	RequestID     string         `json:"requestId"`
	ParticipantID string         `json:"participantId"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type SelfHistoryRef struct {
	StickySession bool `json:"stickySession"`
}

type PeerRoundInput struct {
	ParticipantID string `json:"participantId"`
	Round         int    `json:"round"`
	FullResponse  string `json:"fullResponse"`
	Truncated     bool   `json:"truncated,omitempty"`
}

type RoundTask struct {
	TaskMeta
	Phase           Phase            `json:"phase"`
	Round           int              `json:"round"`
	Prompt          string           `json:"prompt"`
	SelfHistoryRef  *SelfHistoryRef  `json:"selfHistoryRef,omitempty"`
	PeerRoundInputs []PeerRoundInput `json:"peerRoundInputs,omitempty"`
	ClaimCatalog    []Claim          `json:"claimCatalog,omitempty"`
}

func (RoundTask) Kind() TaskKind   { return TaskKindRound }
func (t RoundTask) Meta() TaskMeta { return t.TaskMeta }

type ReportInput struct {
	Status           ConsensusStatus    `json:"status"`
	Representative   Representative     `json:"representative"`
	FinalClaims      []Claim            `json:"finalClaims"`
	ClaimResolutions []ClaimResolution  `json:"claimResolutions"`
	Scoreboard       []ParticipantScore `json:"scoreboard"`
	Rounds           []RoundRecord      `json:"rounds"`
}

type ReportTask struct {
	TaskMeta
	Prompt string      `json:"prompt"`
	Input  ReportInput `json:"reportInput"`
}

func (ReportTask) Kind() TaskKind   { return TaskKindReport }
func (t ReportTask) Meta() TaskMeta { return t.TaskMeta }

type ActionInput struct {
	Status               ConsensusStatus    `json:"status"`
	FinalSummary         string             `json:"finalSummary"`
	RepresentativeSpeech string             `json:"representativeSpeech"`
	Claims               []Claim            `json:"claims"`
	ClaimResolutions     []ClaimResolution  `json:"claimResolutions"`
	Scoreboard           []ParticipantScore `json:"scoreboard"`
	Disagreements        []Disagreement     `json:"disagreements,omitempty"`
}

type ActionTask struct {
	TaskMeta
	Prompt     string           `json:"prompt"`
	Input      ActionInput      `json:"argueResult"`
	FullResult *ConsensusResult `json:"fullResult,omitempty"`
}

func (ActionTask) Kind() TaskKind   { return TaskKindAction }
func (t ActionTask) Meta() TaskMeta { return t.TaskMeta }

type TaskResult interface {
	Kind() TaskKind
}

type RoundTaskResult struct {
	Output ParticipantRoundOutput `json:"output"`
}

func (RoundTaskResult) Kind() TaskKind { return TaskKindRound }

type ReportTaskResult struct {
	Output FinalReport `json:"output"`
}

func (ReportTaskResult) Kind() TaskKind { return TaskKindReport }

type ActionTaskResult struct {
	Output ActionExecution `json:"output"`
}

func (ActionTaskResult) Kind() TaskKind { return TaskKindAction }
