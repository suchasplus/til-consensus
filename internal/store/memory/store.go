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
	if patch.Checkpoint != nil {
		current.Checkpoint = patch.Checkpoint
	}
	if patch.CaseManifest != nil {
		current.CaseManifest = patch.CaseManifest
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
	if patch.LedgerEntries != nil {
		current.LedgerEntries = append([]consensus.EvidenceRecord(nil), patch.LedgerEntries...)
	}
	if patch.LedgerCursor != nil {
		current.LedgerCursor = *patch.LedgerCursor
	}
	if patch.VerificationResults != nil {
		current.VerificationResults = append([]consensus.VerificationResult(nil), patch.VerificationResults...)
	}
	if patch.RevisionRecords != nil {
		current.RevisionRecords = append([]consensus.ClaimRevisionRecord(nil), patch.RevisionRecords...)
	}
	if patch.AdjudicationRecords != nil {
		current.AdjudicationRecords = append([]consensus.AdjudicationRecord(nil), patch.AdjudicationRecords...)
	}
	if patch.Observations != nil {
		current.Observations = append([]consensus.ObservationRecord(nil), patch.Observations...)
	}
	if patch.Metrics != nil {
		current.Metrics = *patch.Metrics
	}
	if patch.ResumeCount != nil {
		current.ResumeCount = *patch.ResumeCount
	}
	if patch.LastResumedAt != nil {
		current.LastResumedAt = *patch.LastResumedAt
	}
	if patch.ArbiterReport != nil {
		current.ArbiterReport = patch.ArbiterReport
	}
	if patch.Report != nil {
		current.Report = patch.Report
	}
	if patch.Action != nil {
		current.Action = patch.Action
	}
	if patch.DebateClaims != nil {
		current.DebateClaims = append([]consensus.DebateClaim(nil), patch.DebateClaims...)
	}
	if patch.DebateRounds != nil {
		current.DebateRounds = append([]consensus.DebateRoundRecord(nil), patch.DebateRounds...)
	}
	if patch.DebateVotes != nil {
		current.DebateVotes = append([]consensus.DebateVoteRecord(nil), patch.DebateVotes...)
	}
	if patch.DelphiRounds != nil {
		current.DelphiRounds = append([]consensus.DelphiRoundRecord(nil), patch.DelphiRounds...)
	}
	if patch.DelphiStatements != nil {
		current.DelphiStatements = append([]consensus.DelphiStatement(nil), patch.DelphiStatements...)
	}
	if patch.DelphiRatingDistributions != nil {
		current.DelphiRatingDistributions = map[string][]float64{}
		for key, values := range patch.DelphiRatingDistributions {
			current.DelphiRatingDistributions[key] = append([]float64(nil), values...)
		}
	}
	if patch.DelphiConsensusLevel != nil {
		current.DelphiConsensusLevel = *patch.DelphiConsensusLevel
	}
	if patch.DelphiRecommendation != nil {
		current.DelphiRecommendation = *patch.DelphiRecommendation
	}
	if patch.DelphiDissentSummary != nil {
		current.DelphiDissentSummary = append([]string(nil), patch.DelphiDissentSummary...)
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
