package runmanager

import (
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

// The silent delegate call streams a short public summary before returning the
// full P0. Bind both to the same invocation in the proposal commit transaction.
func commitP0Message(r *model.StoredRun, proposal *model.Proposal, attempt string) {
	proposer := workflow.RolePlanFor(r.Frozen).Proposer
	invocationID := ""
	if r.P0AttemptID == attempt {
		invocationID = r.P0InvocationID
	}
	messageID := ""
	if invocationID != "" {
		messageID = "message-" + invocationID
	}
	// This also tolerates pre-upgrade snapshots whose streamed P0 used phase
	// "propose". Do not reuse an invocation from an abandoned attempt.
	if messageID == "" {
		for _, old := range r.Snapshot.Messages {
			if old.AgentID == proposer && old.AttemptID == attempt && old.Kind == "proposal" && (old.Phase == "propose" || old.Phase == "preparing") {
				messageID = old.ID
				invocationID = old.InvocationID
				break
			}
		}
	}
	if messageID == "" {
		messageID = newID("message")
	}
	proposal.InvocationID = invocationID
	actualModel := ""
	if metadata := r.ActualMetadata[invocationID]; metadata != nil {
		actualModel, _ = metadata["model"].(string)
	}
	upsertMessage(&r.Snapshot, model.ChatMessage{ID: messageID, AgentID: proposer, Role: "proposer", Label: workflow.AgentLabel(proposer), Content: proposal.Content, At: now(), Phase: "preparing", AttemptID: attempt, Kind: "proposal", InvocationID: invocationID, Model: actualModel, Streaming: false})
	if invocationID != "" {
		if r.MessageSources == nil {
			r.MessageSources = map[string]model.MessageSource{}
		}
		source := r.MessageSources[messageID]
		source.InvocationID = invocationID
		source.Complete = true
		r.MessageSources[messageID] = source
	}
}
