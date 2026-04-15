package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type LedgerWriter struct {
	path string
	mu   sync.Mutex
	seq  int
}

func NewLedger(path string) *LedgerWriter {
	return &LedgerWriter{path: path}
}

func (w *LedgerWriter) Append(_ context.Context, entry consensus.EvidenceRecord) (consensus.EvidenceRecord, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

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
	return entry, nil
}
