package file

import (
	"context"
	"testing"

	"github.com/suchasplus/til-consensus/consensus"
)

func TestStoreSaveLoadPatchAndListByRequestID(t *testing.T) {
	store := New(t.TempDir())
	first := consensus.SessionSnapshot{
		SessionID: "session-1",
		RequestID: "request-a",
		Phase:     consensus.SessionPhaseIngest,
		StartedAt: "2026-04-16T10:00:00Z",
	}
	second := consensus.SessionSnapshot{
		SessionID: "session-2",
		RequestID: "request-a",
		Phase:     consensus.SessionPhaseReport,
		StartedAt: "2026-04-16T11:00:00Z",
	}
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatalf("Save first failed: %v", err)
	}
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatalf("Save second failed: %v", err)
	}
	loaded, err := store.Load(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded == nil || loaded.RequestID != "request-a" || loaded.Phase != consensus.SessionPhaseIngest {
		t.Fatalf("unexpected loaded snapshot: %#v", loaded)
	}
	finished := "2026-04-16T12:00:00Z"
	if err := store.Patch(context.Background(), "session-1", consensus.SessionPatch{
		Phase:      ptr(consensus.SessionPhaseFinished),
		FinishedAt: &finished,
	}); err != nil {
		t.Fatalf("Patch failed: %v", err)
	}
	loaded, err = store.Load(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Load after patch failed: %v", err)
	}
	if loaded.Phase != consensus.SessionPhaseFinished || loaded.FinishedAt != finished {
		t.Fatalf("unexpected patched snapshot: %#v", loaded)
	}
	items, err := store.ListByRequestID(context.Background(), "request-a")
	if err != nil {
		t.Fatalf("ListByRequestID failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two snapshots, got %d", len(items))
	}
	if items[0].SessionID != "session-1" || items[1].SessionID != "session-2" {
		t.Fatalf("unexpected snapshot order: %#v", items)
	}
}

func ptr[T any](value T) *T { return &value }
