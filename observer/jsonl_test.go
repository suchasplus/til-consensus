package observer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
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
	manifestPath := filepath.Join(t.TempDir(), "artifacts", "manifest.jsonl")
	writer := NewLedger(path, manifestPath)
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

func TestLedgerWriterWritesArtifactManifest(t *testing.T) {
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	manifestPath := filepath.Join(tmp, "artifacts", "manifest.jsonl")
	writer := NewLedger(ledgerPath, manifestPath)
	_, err := writer.Append(context.Background(), consensus.EvidenceRecord{
		SchemaVersion: 1,
		EntryID:       "entry-1",
		RequestID:     "req-1",
		SessionID:     "session-1",
		ClaimID:       "claim-1",
		Kind:          consensus.EvidenceKindDeterministicCheck,
		Source:        consensus.EvidenceSourceVerifier,
		ProducerRole:  "verifier",
		Summary:       "check result",
		Artifact: &consensus.ArtifactRef{
			Path:      "/tmp/output.log",
			Hash:      "abc123",
			MediaType: "text/plain",
		},
		CreatedAt: time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one manifest line, got %d", len(lines))
	}
	var entry consensus.ArtifactManifestEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("decode manifest entry: %v", err)
	}
	if entry.EntryID != "entry-1" || entry.Artifact.Hash != "abc123" {
		t.Fatalf("unexpected manifest entry: %#v", entry)
	}
}

func TestLedgerWriterPreservesSeqAcrossReopen(t *testing.T) {
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	manifestPath := filepath.Join(tmp, "artifacts", "manifest.jsonl")

	firstWriter := NewLedger(ledgerPath, manifestPath)
	if _, err := firstWriter.Append(context.Background(), consensus.EvidenceRecord{
		SchemaVersion: 1,
		EntryID:       "entry-1",
		RequestID:     "req-1",
		SessionID:     "session-1",
		Kind:          consensus.EvidenceKindClaimProposed,
		Source:        consensus.EvidenceSourceCoordinator,
		Summary:       "summary",
		Artifact: &consensus.ArtifactRef{
			Path:      "/tmp/one.log",
			Hash:      "hash-1",
			MediaType: "text/plain",
		},
		CreatedAt: time.Unix(1, 0).UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	secondWriter := NewLedger(ledgerPath, manifestPath)
	second, err := secondWriter.Append(context.Background(), consensus.EvidenceRecord{
		SchemaVersion: 1,
		EntryID:       "entry-2",
		RequestID:     "req-1",
		SessionID:     "session-1",
		Kind:          consensus.EvidenceKindClaimProposed,
		Source:        consensus.EvidenceSourceCoordinator,
		Summary:       "summary",
		Artifact: &consensus.ArtifactRef{
			Path:      "/tmp/two.log",
			Hash:      "hash-2",
			MediaType: "text/plain",
		},
		CreatedAt: time.Unix(2, 0).UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if second.Seq != 1 {
		t.Fatalf("expected reopened writer to continue seq, got %d", second.Seq)
	}

	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two manifest entries, got %d", len(lines))
	}
	var manifestEntry consensus.ArtifactManifestEntry
	if err := json.Unmarshal([]byte(lines[1]), &manifestEntry); err != nil {
		t.Fatalf("decode manifest entry: %v", err)
	}
	if manifestEntry.Seq != 1 {
		t.Fatalf("expected manifest seq to continue, got %d", manifestEntry.Seq)
	}
}
