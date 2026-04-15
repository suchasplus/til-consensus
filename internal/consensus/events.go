package consensus

type EventType string

const (
	EventSessionStarted        EventType = "SessionStarted"
	EventRoundDispatched       EventType = "RoundDispatched"
	EventParticipantResponded  EventType = "ParticipantResponded"
	EventParticipantEliminated EventType = "ParticipantEliminated"
	EventClaimsMerged          EventType = "ClaimsMerged"
	EventRoundCompleted        EventType = "RoundCompleted"
	EventEarlyStopTriggered    EventType = "EarlyStopTriggered"
	EventGlobalDeadlineHit     EventType = "GlobalDeadlineHit"
	EventConsensusDrafted      EventType = "ConsensusDrafted"
	EventReportDispatched      EventType = "ReportDispatched"
	EventReportCompleted       EventType = "ReportCompleted"
	EventActionDispatched      EventType = "ActionDispatched"
	EventActionCompleted       EventType = "ActionCompleted"
	EventActionFailed          EventType = "ActionFailed"
	EventFinalized             EventType = "Finalized"
	EventFailed                EventType = "Failed"
)

type ConsensusEvent struct {
	SessionID string         `json:"sessionId"`
	RequestID string         `json:"requestId"`
	Type      EventType      `json:"type"`
	At        string         `json:"at"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type ConsensusEventRecord struct {
	Version  int            `json:"v"`
	Kind     string         `json:"kind"`
	Seq      int            `json:"seq"`
	LoggedAt string         `json:"loggedAt"`
	Event    ConsensusEvent `json:"event"`
}
