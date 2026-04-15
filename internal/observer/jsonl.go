package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type JSONLObserver struct {
	path string
	mu   sync.Mutex
	seq  int
}

func NewJSONL(path string) *JSONLObserver {
	return &JSONLObserver{path: path}
}

func (o *JSONLObserver) OnEvent(_ context.Context, event consensus.RunEvent) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(o.path), 0o755); err != nil {
		return fmt.Errorf("create observer dir: %w", err)
	}
	file, err := os.OpenFile(o.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open jsonl file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	record := consensus.RunEventRecord{
		SchemaVersion: 1,
		Kind:          "til-consensus.event",
		Seq:           o.seq,
		LoggedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Event:         event,
	}
	o.seq++
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal event record: %w", err)
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write event record: %w", err)
	}
	return nil
}
