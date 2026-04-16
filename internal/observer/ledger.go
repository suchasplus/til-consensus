package observer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type LedgerWriter struct {
	path         string
	manifestPath string
	mu           sync.Mutex
	seq          int
	manifestSeq  int
	loaded       bool
}

func NewLedger(path string, manifestPath string) *LedgerWriter {
	return &LedgerWriter{path: path, manifestPath: manifestPath}
}

func (w *LedgerWriter) Append(_ context.Context, entry consensus.EvidenceRecord) (consensus.EvidenceRecord, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.loadExistingSeqLocked(); err != nil {
		return consensus.EvidenceRecord{}, err
	}

	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return consensus.EvidenceRecord{}, fmt.Errorf("create ledger dir: %w", err)
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return consensus.EvidenceRecord{}, fmt.Errorf("open ledger file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()
	entry.Seq = w.seq
	w.seq++
	body, err := json.Marshal(entry)
	if err != nil {
		return consensus.EvidenceRecord{}, fmt.Errorf("marshal ledger entry: %w", err)
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		return consensus.EvidenceRecord{}, fmt.Errorf("write ledger entry: %w", err)
	}
	if entry.Artifact != nil && entry.Artifact.Path != "" && w.manifestPath != "" {
		manifest := consensus.ArtifactManifestEntry{
			SchemaVersion:  entry.SchemaVersion,
			Seq:            w.manifestSeq,
			EntryID:        entry.EntryID,
			RequestID:      entry.RequestID,
			SessionID:      entry.SessionID,
			ClaimID:        entry.ClaimID,
			ChallengeID:    entry.ChallengeID,
			VerificationID: entry.VerificationID,
			Kind:           entry.Kind,
			ProducerRole:   entry.ProducerRole,
			Artifact:       *entry.Artifact,
			LoggedAt:       entry.CreatedAt,
		}
		if err := appendJSONL(w.manifestPath, manifest); err != nil {
			return consensus.EvidenceRecord{}, fmt.Errorf("write artifact manifest: %w", err)
		}
		w.manifestSeq++
	}
	return entry, nil
}

func (w *LedgerWriter) loadExistingSeqLocked() error {
	if w.loaded {
		return nil
	}
	seq, err := countJSONLLines(w.path)
	if err != nil {
		return fmt.Errorf("read existing ledger seq: %w", err)
	}
	manifestSeq, err := countJSONLLines(w.manifestPath)
	if err != nil {
		return fmt.Errorf("read existing manifest seq: %w", err)
	}
	w.seq = seq
	w.manifestSeq = manifestSeq
	w.loaded = true
	return nil
}

func countJSONLLines(path string) (int, error) {
	if path == "" {
		return 0, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer func() {
		_ = file.Close()
	}()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		count++
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return 0, err
	}
	return count, nil
}

func appendJSONL(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		return err
	}
	return nil
}
