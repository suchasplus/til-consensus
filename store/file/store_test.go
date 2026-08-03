package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	if loaded.Version != 1 {
		t.Fatalf("expected initial version 1, got %d", loaded.Version)
	}
	finished := "2026-04-16T12:00:00Z"
	stableStatementID := "statement-1"
	if err := store.Patch(context.Background(), "session-1", consensus.SessionPatch{
		Phase:                       ptr(consensus.SessionPhaseFinished),
		FinishedAt:                  &finished,
		Degradations:                []consensus.Degradation{{Kind: consensus.DegradationParticipantAbsent, Phase: "delphi_round", Impact: "partial panel"}},
		DelphiLastStableStatementID: &stableStatementID,
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
	if len(loaded.Degradations) != 1 || loaded.DelphiLastStableStatementID != stableStatementID {
		t.Fatalf("expected resumable workflow state to round-trip, got %#v", loaded)
	}
	if loaded.Version != 2 {
		t.Fatalf("expected patched version 2, got %d", loaded.Version)
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

func TestStorePatchSerializesReadModifyWriteAcrossInstances(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	first := New(dir)
	second := New(dir)
	for iteration := 0; iteration < 50; iteration++ {
		sessionID := "session-concurrent-" + fmt.Sprint(iteration)
		if err := first.Save(ctx, consensus.SessionSnapshot{SessionID: sessionID}); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		phase := consensus.SessionPhaseReport
		finishedAt := "2026-08-03T00:00:00Z"
		start := make(chan struct{})
		errs := make(chan error, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		go func() {
			ready.Done()
			<-start
			errs <- first.Patch(ctx, sessionID, consensus.SessionPatch{Phase: &phase})
		}()
		go func() {
			ready.Done()
			<-start
			errs <- second.Patch(ctx, sessionID, consensus.SessionPatch{FinishedAt: &finishedAt})
		}()
		ready.Wait()
		close(start)
		for count := 0; count < 2; count++ {
			if err := <-errs; err != nil {
				t.Fatalf("concurrent Patch failed: %v", err)
			}
		}
		loaded, err := first.Load(ctx, sessionID)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if loaded.Phase != phase || loaded.FinishedAt != finishedAt {
			t.Fatalf("lost concurrent update: %#v", loaded)
		}
		if loaded.Version != 3 {
			t.Fatalf("expected version 3 after two patches, got %d", loaded.Version)
		}
	}
}

func TestStorePatchRejectsStaleVersion(t *testing.T) {
	ctx := context.Background()
	store := New(t.TempDir())
	if err := store.Save(ctx, consensus.SessionSnapshot{SessionID: "session-cas"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	expected := uint64(1)
	phase := consensus.SessionPhaseIngest
	if err := store.Patch(ctx, "session-cas", consensus.SessionPatch{ExpectedVersion: &expected, Phase: &phase}); err != nil {
		t.Fatalf("first CAS patch failed: %v", err)
	}
	if err := store.Patch(ctx, "session-cas", consensus.SessionPatch{ExpectedVersion: &expected, Phase: &phase}); !errors.Is(err, consensus.ErrSessionVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestStoreRejectsUnsafeIDsSymlinksAndDirectories(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := New(dir)
	for _, sessionID := range []string{"../escape", "a/b", `a\b`, "a..b"} {
		if err := store.Save(ctx, consensus.SessionSnapshot{SessionID: sessionID}); err == nil {
			t.Fatalf("expected unsafe session id %q to fail", sessionID)
		}
	}
	if err := store.Save(ctx, consensus.SessionSnapshot{SessionID: "session-safe"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	external := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(external, []byte(`{"SessionID":"external"}`), 0o600); err != nil {
		t.Fatalf("write external target: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(dir, "session-link.json")); err != nil {
		t.Fatalf("create file symlink: %v", err)
	}
	if _, err := store.Load(ctx, "session-link"); err == nil {
		t.Fatal("expected symlink session load to fail")
	}
	if err := os.Mkdir(filepath.Join(dir, "session-dir.json"), 0o700); err != nil {
		t.Fatalf("create directory target: %v", err)
	}
	if _, err := store.Load(ctx, "session-dir"); err == nil {
		t.Fatal("expected directory session load to fail")
	}
	linkedRoot := filepath.Join(t.TempDir(), "linked-store")
	if err := os.Symlink(dir, linkedRoot); err != nil {
		t.Fatalf("create store symlink: %v", err)
	}
	if err := New(linkedRoot).Save(ctx, consensus.SessionSnapshot{SessionID: "session-other"}); err == nil {
		t.Fatal("expected symlink store root to fail")
	}
}

func TestStoreUsesPrivatePermissions(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "sessions")
	store := New(root)
	if err := store.Save(ctx, consensus.SessionSnapshot{SessionID: "session-private"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	assertMode(t, root, 0o700)
	assertMode(t, filepath.Join(root, "session-private.json"), 0o600)
	assertMode(t, filepath.Join(root, ".store.lock"), 0o600)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s: got=%#o want=%#o", path, got, want)
	}
}

func ptr[T any](value T) *T { return &value }
