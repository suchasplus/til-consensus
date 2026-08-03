package observer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/internal/securefs"
)

func TestLedgerWriterBuildsHashChainAndRoot(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	manifestPath := filepath.Join(tmp, "artifacts", "manifest.jsonl")
	writer := NewLedger(ledgerPath, manifestPath)

	first, err := writer.Append(ctx, testEvidence(1, true))
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	second, err := writer.Append(ctx, testEvidence(2, true))
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	if first.PrevHash != "" || len(first.EntryHash) != 64 {
		t.Fatalf("unexpected genesis hash fields: %#v", first)
	}
	if second.PrevHash != first.EntryHash || len(second.EntryHash) != 64 {
		t.Fatalf("hash chain is not linked: %#v", second)
	}
	integrity, err := writer.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if integrity.EntryCount != 2 || integrity.RootHash != second.EntryHash || integrity.Legacy {
		t.Fatalf("unexpected integrity result: %#v", integrity)
	}

	rootBody, err := os.ReadFile(LedgerRootPath(ledgerPath))
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	var root consensus.LedgerRoot
	if err := json.Unmarshal(rootBody, &root); err != nil {
		t.Fatalf("decode root: %v", err)
	}
	if root.EntryCount != 2 || root.RootHash != second.EntryHash || root.HashAlgorithm != "sha256" {
		t.Fatalf("unexpected root: %#v", root)
	}

	manifest := readManifestEntries(t, manifestPath)
	if len(manifest) != 2 || manifest[0].LedgerEntryHash != first.EntryHash || manifest[1].LedgerEntryHash != second.EntryHash {
		t.Fatalf("manifest is not anchored to ledger hashes: %#v", manifest)
	}
}

func TestLedgerIntegrityDetectsModificationReorderAndDeletion(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, ledgerPath string)
	}{
		{
			name: "modify",
			mutate: func(t *testing.T, ledgerPath string) {
				lines := readLedgerLines(t, ledgerPath)
				var first consensus.EvidenceRecord
				if err := json.Unmarshal(lines[0], &first); err != nil {
					t.Fatal(err)
				}
				first.Summary = "tampered"
				lines[0], _ = json.Marshal(first)
				writeLedgerLines(t, ledgerPath, lines)
			},
		},
		{
			name: "reorder",
			mutate: func(t *testing.T, ledgerPath string) {
				lines := readLedgerLines(t, ledgerPath)
				lines[0], lines[1] = lines[1], lines[0]
				writeLedgerLines(t, ledgerPath, lines)
			},
		},
		{
			name: "delete-final-entry",
			mutate: func(t *testing.T, ledgerPath string) {
				lines := readLedgerLines(t, ledgerPath)
				writeLedgerLines(t, ledgerPath, lines[:1])
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			tmp := t.TempDir()
			ledgerPath := filepath.Join(tmp, "ledger.jsonl")
			writer := NewLedger(ledgerPath, filepath.Join(tmp, "artifacts", "manifest.jsonl"))
			if _, err := writer.Append(ctx, testEvidence(1, false)); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Append(ctx, testEvidence(2, false)); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, ledgerPath)
			if _, err := VerifyLedger(ctx, ledgerPath); err == nil {
				t.Fatal("expected integrity verification to fail")
			}
			if _, err := NewLedger(ledgerPath, filepath.Join(tmp, "artifacts", "manifest.jsonl")).Append(ctx, testEvidence(3, false)); err == nil {
				t.Fatal("expected startup integrity validation to reject append")
			}
		})
	}
}

func TestLedgerRecoversTornTail(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	manifestPath := filepath.Join(tmp, "artifacts", "manifest.jsonl")
	writer := NewLedger(ledgerPath, manifestPath)
	if _, err := writer.Append(ctx, testEvidence(1, false)); err != nil {
		t.Fatal(err)
	}
	file, err := securefs.OpenAppend(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(`{"partial"`)); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()

	second, err := NewLedger(ledgerPath, manifestPath).Append(ctx, testEvidence(2, false))
	if err != nil {
		t.Fatalf("append after torn tail: %v", err)
	}
	if second.Seq != 1 {
		t.Fatalf("unexpected sequence after recovery: %d", second.Seq)
	}
	integrity, err := VerifyLedger(ctx, ledgerPath)
	if err != nil {
		t.Fatalf("verify recovered ledger: %v", err)
	}
	if integrity.EntryCount != 2 || integrity.RootHash != second.EntryHash {
		t.Fatalf("unexpected recovered integrity: %#v", integrity)
	}
	if bytes.Contains(mustReadFile(t, ledgerPath), []byte("partial")) {
		t.Fatal("torn tail was not removed")
	}
}

func TestLedgerRecoversStaleRootAndManifestFromWAL(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	manifestPath := filepath.Join(tmp, "artifacts", "manifest.jsonl")
	writer := NewLedger(ledgerPath, manifestPath)
	first, err := writer.Append(ctx, testEvidence(1, true))
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a crash after the WAL record was fsynced but before root and
	// manifest derivation completed.
	second := testEvidence(2, true)
	second.Seq = 1
	second.PrevHash = first.EntryHash
	second.EntryHash, err = calculateEntryHash(second)
	if err != nil {
		t.Fatal(err)
	}
	line, _ := json.Marshal(second)
	if err := appendLedgerLine(ledgerPath, append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	third, err := NewLedger(ledgerPath, manifestPath).Append(ctx, testEvidence(3, true))
	if err != nil {
		t.Fatalf("append after simulated crash: %v", err)
	}
	if third.Seq != 2 {
		t.Fatalf("unexpected sequence after recovery: %d", third.Seq)
	}
	integrity, err := VerifyLedger(ctx, ledgerPath)
	if err != nil || integrity.EntryCount != 3 {
		t.Fatalf("unexpected recovered ledger: integrity=%#v err=%v", integrity, err)
	}
	manifest := readManifestEntries(t, manifestPath)
	if len(manifest) != 3 || manifest[1].EntryID != second.EntryID || manifest[2].LedgerEntryHash != third.EntryHash {
		t.Fatalf("manifest was not rebuilt from WAL: %#v", manifest)
	}
}

func TestLedgerSerializesMultipleWriters(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	manifestPath := filepath.Join(tmp, "artifacts", "manifest.jsonl")
	const writerCount = 4
	const entriesPerWriter = 8
	start := make(chan struct{})
	errs := make(chan error, writerCount*entriesPerWriter)
	var ready sync.WaitGroup
	var done sync.WaitGroup
	for writerID := 0; writerID < writerCount; writerID++ {
		writer := NewLedger(ledgerPath, manifestPath)
		ready.Add(1)
		done.Add(1)
		go func(writerID int) {
			defer done.Done()
			ready.Done()
			<-start
			for idx := 0; idx < entriesPerWriter; idx++ {
				entry := testEvidence(writerID*entriesPerWriter+idx+1, false)
				_, err := writer.Append(ctx, entry)
				errs <- err
			}
		}(writerID)
	}
	ready.Wait()
	close(start)
	done.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent append failed: %v", err)
		}
	}
	integrity, err := VerifyLedger(ctx, ledgerPath)
	if err != nil {
		t.Fatalf("VerifyLedger failed: %v", err)
	}
	if want := writerCount * entriesPerWriter; integrity.EntryCount != want {
		t.Fatalf("entry count: got=%d want=%d", integrity.EntryCount, want)
	}
	lines := readLedgerLines(t, ledgerPath)
	for idx, line := range lines {
		var entry consensus.EvidenceRecord
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Seq != idx {
			t.Fatalf("non-monotonic sequence at %d: %d", idx, entry.Seq)
		}
	}
}

func TestLedgerMigratesLegacyEntries(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	manifestPath := filepath.Join(tmp, "artifacts", "manifest.jsonl")
	legacy := []consensus.EvidenceRecord{testEvidence(1, true), testEvidence(2, true)}
	legacy[0].Seq = 0
	legacy[1].Seq = 1
	writeEvidenceRecords(t, ledgerPath, legacy)
	before, err := VerifyLedger(ctx, ledgerPath)
	if err != nil || !before.Legacy {
		t.Fatalf("expected legacy ledger, integrity=%#v err=%v", before, err)
	}
	if _, err := NewLedger(ledgerPath, manifestPath).Append(ctx, testEvidence(3, true)); err != nil {
		t.Fatalf("append with migration failed: %v", err)
	}
	after, err := VerifyLedger(ctx, ledgerPath)
	if err != nil || after.Legacy || after.EntryCount != 3 || len(after.RootHash) != 64 {
		t.Fatalf("unexpected migrated integrity: %#v err=%v", after, err)
	}
}

func testEvidence(id int, withArtifact bool) consensus.EvidenceRecord {
	entry := consensus.EvidenceRecord{
		SchemaVersion: 2,
		EntryID:       fmt.Sprintf("entry-%03d", id),
		RequestID:     "req-1",
		SessionID:     "session-1",
		Kind:          consensus.EvidenceKindClaimProposed,
		Source:        consensus.EvidenceSourceCoordinator,
		Summary:       fmt.Sprintf("summary-%03d", id),
		Metadata:      map[string]any{"ordinal": id},
		CreatedAt:     time.Unix(int64(id), 0).UTC().Format(time.RFC3339Nano),
	}
	if withArtifact {
		entry.Artifact = &consensus.ArtifactRef{
			Path:      fmt.Sprintf("/tmp/artifact-%03d.log", id),
			Hash:      fmt.Sprintf("artifact-hash-%03d", id),
			MediaType: "text/plain",
		}
	}
	return entry
}

func readLedgerLines(t *testing.T, path string) [][]byte {
	t.Helper()
	body := bytes.TrimSpace(mustReadFile(t, path))
	if len(body) == 0 {
		return nil
	}
	return bytes.Split(body, []byte{'\n'})
}

func writeLedgerLines(t *testing.T, path string, lines [][]byte) {
	t.Helper()
	body := bytes.Join(lines, []byte{'\n'})
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeEvidenceRecords(t *testing.T, path string, entries []consensus.EvidenceRecord) {
	t.Helper()
	lines := make([][]byte, 0, len(entries))
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	writeLedgerLines(t, path, lines)
}

func readManifestEntries(t *testing.T, path string) []consensus.ArtifactManifestEntry {
	t.Helper()
	lines := readLedgerLines(t, path)
	out := make([]consensus.ArtifactManifestEntry, 0, len(lines))
	for _, line := range lines {
		var entry consensus.ArtifactManifestEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode manifest: %v", err)
		}
		out = append(out, entry)
	}
	return out
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
