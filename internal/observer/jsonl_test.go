package observer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestJSONLObserverPreservesSequence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	observer := NewJSONL(path)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = observer.OnEvent(context.Background(), consensus.ConsensusEvent{
				SessionID: "s",
				RequestID: "r",
				Type:      consensus.EventSessionStarted,
				At:        "now",
				Payload:   map[string]any{"idx": idx},
			})
		}(i)
	}
	wg.Wait()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 10 {
		t.Fatalf("expected 10 lines, got %d", len(lines))
	}
	for idx, line := range lines {
		var record consensus.ConsensusEventRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.Seq != idx {
			t.Fatalf("expected seq %d, got %d", idx, record.Seq)
		}
	}
}
