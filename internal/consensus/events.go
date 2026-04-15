package consensus

type RunEventType string

const (
	RunEventSessionStarted   RunEventType = "session_started"
	RunEventPhaseChanged     RunEventType = "phase_changed"
	RunEventTaskDispatched   RunEventType = "task_dispatched"
	RunEventTaskCompleted    RunEventType = "task_completed"
	RunEventTaskFailed       RunEventType = "task_failed"
	RunEventLedgerAppended   RunEventType = "ledger_appended"
	RunEventSessionFinalized RunEventType = "session_finalized"
	RunEventSessionFailed    RunEventType = "session_failed"
)

type RunEvent struct {
	SessionID string         `json:"sessionId"`
	RequestID string         `json:"requestId"`
	Type      RunEventType   `json:"type"`
	Phase     SessionPhase   `json:"phase,omitempty"`
	At        string         `json:"at"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type RunEventRecord struct {
	SchemaVersion int      `json:"schemaVersion"`
	Kind          string   `json:"kind"`
	Seq           int      `json:"seq"`
	LoggedAt      string   `json:"loggedAt"`
	Event         RunEvent `json:"event"`
}
