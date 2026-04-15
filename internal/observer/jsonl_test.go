package observer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestJSONLObserverWritesRunEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	observer := NewJSONL(path)
	err := observer.OnEvent(context.Background(), consensus.RunEvent{
		SessionID: "session-1",
		RequestID: "req-1",
		Type:      consensus.RunEventSessionStarted,
		At:        time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("OnEvent failed: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one line, got %d", len(lines))
	}
	var record consensus.RunEventRecord
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode event record: %v", err)
	}
	if record.Event.Type != consensus.RunEventSessionStarted {
		t.Fatalf("unexpected event type: %#v", record)
	}
}

func TestLedgerWriterAppendsMonotonicSeq(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	writer := NewLedger(path)
	for idx := 0; idx < 2; idx++ {
		if _, err := writer.Append(context.Background(), consensus.EvidenceRecord{
			SchemaVersion: 1,
			EntryID:       "entry",
			RequestID:     "req-1",
			SessionID:     "session-1",
			Kind:          consensus.EvidenceKindClaimProposed,
			Source:        consensus.EvidenceSourceCoordinator,
			Summary:       "summary",
			CreatedAt:     time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two lines, got %d", len(lines))
	}
	var first consensus.EvidenceRecord
	var second consensus.EvidenceRecord
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	if first.Seq != 0 || second.Seq != 1 {
		t.Fatalf("unexpected seq values: %d %d", first.Seq, second.Seq)
	}
}
