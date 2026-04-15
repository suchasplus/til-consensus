package observer

import (
	"context"
	"fmt"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type Multi struct {
	observers []consensus.Observer
}

func NewMulti(observers ...consensus.Observer) *Multi {
	filtered := make([]consensus.Observer, 0, len(observers))
	for _, item := range observers {
		if item != nil {
			filtered = append(filtered, item)
		}
	}
	return &Multi{observers: filtered}
}

func (m *Multi) OnEvent(ctx context.Context, event consensus.RunEvent) error {
	for _, item := range m.observers {
		if err := item.OnEvent(ctx, event); err != nil {
			return fmt.Errorf("observer failed: %w", err)
		}
	}
	return nil
}
