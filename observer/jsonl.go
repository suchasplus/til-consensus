package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/internal/securefs"
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

	file, err := securefs.OpenAppend(o.path)
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
