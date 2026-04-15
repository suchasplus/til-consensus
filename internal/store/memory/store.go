package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type Store struct {
	mu       sync.RWMutex
	sessions map[string]consensus.SessionSnapshot
}

func New() *Store {
	return &Store{
		sessions: map[string]consensus.SessionSnapshot{},
	}
}

func (s *Store) Save(_ context.Context, session consensus.SessionSnapshot) error {
	if session.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.SessionID] = session
	return nil
}

func (s *Store) Load(_ context.Context, sessionID string) (*consensus.SessionSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	cloned := session
	return &cloned, nil
}

func (s *Store) Patch(_ context.Context, sessionID string, patch consensus.SessionPatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("unknown session id: %s", sessionID)
	}
	if patch.Phase != nil {
		current.Phase = *patch.Phase
	}
	if patch.FinishedAt != nil {
		current.FinishedAt = *patch.FinishedAt
	}
	if patch.FailedAt != nil {
		current.FailedAt = *patch.FailedAt
	}
	if patch.ClaimGraph != nil {
		current.ClaimGraph = append([]consensus.ClaimNode(nil), patch.ClaimGraph...)
	}
	if patch.ChallengeTickets != nil {
		current.ChallengeTickets = append([]consensus.ChallengeTicket(nil), patch.ChallengeTickets...)
	}
	if patch.LedgerCursor != nil {
		current.LedgerCursor = *patch.LedgerCursor
	}
	if patch.Result != nil {
		current.Result = patch.Result
	}
	if patch.Error != nil {
		current.Error = patch.Error
	}
	s.sessions[sessionID] = current
	return nil
}
