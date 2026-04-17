package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

type Store struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) *Store {
	return &Store{dir: strings.TrimSpace(dir)}
}

func (s *Store) Save(_ context.Context, session consensus.SessionSnapshot) error {
	if strings.TrimSpace(session.SessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	return s.write(session)
}

func (s *Store) Load(_ context.Context, sessionID string) (*consensus.SessionSnapshot, error) {
	path, err := s.sessionPath(sessionID)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session snapshot: %w", err)
	}
	var snapshot consensus.SessionSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return nil, fmt.Errorf("decode session snapshot: %w", err)
	}
	return &snapshot, nil
}

func (s *Store) Patch(ctx context.Context, sessionID string, patch consensus.SessionPatch) error {
	snapshot, err := s.Load(ctx, sessionID)
	if err != nil {
		return err
	}
	if snapshot == nil {
		return fmt.Errorf("unknown session id: %s", sessionID)
	}
	if patch.Phase != nil {
		snapshot.Phase = *patch.Phase
	}
	if patch.Checkpoint != nil {
		snapshot.Checkpoint = patch.Checkpoint
	}
	if patch.CaseManifest != nil {
		snapshot.CaseManifest = patch.CaseManifest
	}
	if patch.FinishedAt != nil {
		snapshot.FinishedAt = *patch.FinishedAt
	}
	if patch.FailedAt != nil {
		snapshot.FailedAt = *patch.FailedAt
	}
	if patch.ClaimGraph != nil {
		snapshot.ClaimGraph = append([]consensus.ClaimNode(nil), patch.ClaimGraph...)
	}
	if patch.ChallengeTickets != nil {
		snapshot.ChallengeTickets = append([]consensus.ChallengeTicket(nil), patch.ChallengeTickets...)
	}
	if patch.LedgerEntries != nil {
		snapshot.LedgerEntries = append([]consensus.EvidenceRecord(nil), patch.LedgerEntries...)
	}
	if patch.LedgerCursor != nil {
		snapshot.LedgerCursor = *patch.LedgerCursor
	}
	if patch.VerificationResults != nil {
		snapshot.VerificationResults = append([]consensus.VerificationResult(nil), patch.VerificationResults...)
	}
	if patch.RevisionRecords != nil {
		snapshot.RevisionRecords = append([]consensus.ClaimRevisionRecord(nil), patch.RevisionRecords...)
	}
	if patch.AdjudicationRecords != nil {
		snapshot.AdjudicationRecords = append([]consensus.AdjudicationRecord(nil), patch.AdjudicationRecords...)
	}
	if patch.Observations != nil {
		snapshot.Observations = append([]consensus.ObservationRecord(nil), patch.Observations...)
	}
	if patch.Metrics != nil {
		snapshot.Metrics = *patch.Metrics
	}
	if patch.ResumeCount != nil {
		snapshot.ResumeCount = *patch.ResumeCount
	}
	if patch.LastResumedAt != nil {
		snapshot.LastResumedAt = *patch.LastResumedAt
	}
	if patch.ArbiterReport != nil {
		snapshot.ArbiterReport = patch.ArbiterReport
	}
	if patch.Report != nil {
		snapshot.Report = patch.Report
	}
	if patch.Action != nil {
		snapshot.Action = patch.Action
	}
	if patch.DebateClaims != nil {
		snapshot.DebateClaims = append([]consensus.DebateClaim(nil), patch.DebateClaims...)
	}
	if patch.DebateRounds != nil {
		snapshot.DebateRounds = append([]consensus.DebateRoundRecord(nil), patch.DebateRounds...)
	}
	if patch.DebateVotes != nil {
		snapshot.DebateVotes = append([]consensus.DebateVoteRecord(nil), patch.DebateVotes...)
	}
	if patch.DelphiRounds != nil {
		snapshot.DelphiRounds = append([]consensus.DelphiRoundRecord(nil), patch.DelphiRounds...)
	}
	if patch.DelphiStatements != nil {
		snapshot.DelphiStatements = append([]consensus.DelphiStatement(nil), patch.DelphiStatements...)
	}
	if patch.DelphiRatingDistributions != nil {
		snapshot.DelphiRatingDistributions = make(map[string][]float64, len(patch.DelphiRatingDistributions))
		for key, values := range patch.DelphiRatingDistributions {
			snapshot.DelphiRatingDistributions[key] = append([]float64(nil), values...)
		}
	}
	if patch.DelphiConsensusLevel != nil {
		snapshot.DelphiConsensusLevel = *patch.DelphiConsensusLevel
	}
	if patch.DelphiRecommendation != nil {
		snapshot.DelphiRecommendation = *patch.DelphiRecommendation
	}
	if patch.DelphiDissentSummary != nil {
		snapshot.DelphiDissentSummary = append([]string(nil), patch.DelphiDissentSummary...)
	}
	if patch.Result != nil {
		snapshot.Result = patch.Result
	}
	if patch.Error != nil {
		snapshot.Error = patch.Error
	}
	return s.write(*snapshot)
}

func (s *Store) List(_ context.Context) ([]consensus.SessionSnapshot, error) {
	if strings.TrimSpace(s.dir) == "" {
		return nil, fmt.Errorf("session store dir is required")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create session store dir: %w", err)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read session store dir: %w", err)
	}
	out := make([]consensus.SessionSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read session snapshot %s: %w", entry.Name(), err)
		}
		var snapshot consensus.SessionSnapshot
		if err := json.Unmarshal(body, &snapshot); err != nil {
			return nil, fmt.Errorf("decode session snapshot %s: %w", entry.Name(), err)
		}
		out = append(out, snapshot)
	}
	slices.SortFunc(out, func(left, right consensus.SessionSnapshot) int {
		if left.StartedAt == right.StartedAt {
			return strings.Compare(left.SessionID, right.SessionID)
		}
		return strings.Compare(left.StartedAt, right.StartedAt)
	})
	return out, nil
}

func (s *Store) ListByRequestID(ctx context.Context, requestID string) ([]consensus.SessionSnapshot, error) {
	snapshots, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]consensus.SessionSnapshot, 0)
	for _, snapshot := range snapshots {
		if snapshot.RequestID == requestID {
			out = append(out, snapshot)
		}
	}
	return out, nil
}

func (s *Store) write(session consensus.SessionSnapshot) error {
	path, err := s.sessionPath(session.SessionID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create session store dir: %w", err)
	}
	body, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session snapshot: %w", err)
	}
	body = append(body, '\n')
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, body, 0o644); err != nil {
		return fmt.Errorf("write session snapshot temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace session snapshot file: %w", err)
	}
	return nil
}

func (s *Store) sessionPath(sessionID string) (string, error) {
	if strings.TrimSpace(s.dir) == "" {
		return "", fmt.Errorf("session store dir is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	return filepath.Join(s.dir, sessionID+".json"), nil
}
