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

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/internal/securefs"
	"golang.org/x/sys/unix"
)

type Store struct {
	dir          string
	mu           sync.Mutex
	canonicalDir string
}

func New(dir string) *Store {
	return &Store{dir: strings.TrimSpace(dir)}
}

func (s *Store) Save(ctx context.Context, session consensus.SessionSnapshot) error {
	if strings.TrimSpace(session.SessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, unlock, err := s.lockStoreLocked(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()
	path, err := sessionPath(root, session.SessionID)
	if err != nil {
		return err
	}
	if err := securefs.ValidateRegularTarget(path); err != nil {
		return err
	}
	existing, err := loadLocked(root, session.SessionID)
	if err != nil {
		return err
	}
	session.Version = 1
	if existing != nil {
		session.Version = existing.Version + 1
	}
	return writeLocked(root, path, session)
}

func (s *Store) Load(ctx context.Context, sessionID string) (*consensus.SessionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, unlock, err := s.lockStoreLocked(ctx, false)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return loadLocked(root, sessionID)
}

func loadLocked(root string, sessionID string) (*consensus.SessionSnapshot, error) {
	path, err := sessionPath(root, sessionID)
	if err != nil {
		return nil, err
	}
	body, err := securefs.ReadFile(path)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	root, unlock, err := s.lockStoreLocked(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()
	snapshot, err := loadLocked(root, sessionID)
	if err != nil {
		return err
	}
	if snapshot == nil {
		return fmt.Errorf("unknown session id: %s", sessionID)
	}
	if patch.ExpectedVersion != nil && snapshot.Version != *patch.ExpectedVersion {
		return fmt.Errorf("%w: session=%s expected=%d actual=%d", consensus.ErrSessionVersionConflict, sessionID, *patch.ExpectedVersion, snapshot.Version)
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
	if patch.Degradations != nil {
		snapshot.Degradations = append([]consensus.Degradation(nil), patch.Degradations...)
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
	if patch.DelphiConverged != nil {
		snapshot.DelphiConverged = *patch.DelphiConverged
	}
	if patch.DelphiResponseQuorumMet != nil {
		snapshot.DelphiResponseQuorumMet = *patch.DelphiResponseQuorumMet
	}
	if patch.DelphiMinResponsesMet != nil {
		snapshot.DelphiMinResponsesMet = *patch.DelphiMinResponsesMet
	}
	if patch.DelphiStableRounds != nil {
		snapshot.DelphiStableRounds = *patch.DelphiStableRounds
	}
	if patch.DelphiLastStableStatementID != nil {
		snapshot.DelphiLastStableStatementID = *patch.DelphiLastStableStatementID
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
	snapshot.Version++
	path, err := sessionPath(root, sessionID)
	if err != nil {
		return err
	}
	return writeLocked(root, path, *snapshot)
}

func (s *Store) List(ctx context.Context) ([]consensus.SessionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, unlock, err := s.lockStoreLocked(ctx, false)
	if err != nil {
		return nil, err
	}
	defer unlock()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read session store dir: %w", err)
	}
	out := make([]consensus.SessionSnapshot, 0, len(entries))
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("session snapshot is not a regular file: %s", entry.Name())
		}
		path := filepath.Join(root, entry.Name())
		if err := securefs.EnsureContained(root, path); err != nil {
			return nil, err
		}
		body, err := securefs.ReadFile(path)
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

func writeLocked(root string, path string, session consensus.SessionSnapshot) error {
	if err := securefs.EnsureContained(root, path); err != nil {
		return err
	}
	if err := securefs.ValidateRegularTarget(path); err != nil {
		return err
	}
	body, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session snapshot: %w", err)
	}
	body = append(body, '\n')
	tmp, err := os.CreateTemp(root, ".session-*.tmp")
	if err != nil {
		return fmt.Errorf("create session snapshot temp file: %w", err)
	}
	tmpPath := tmp.Name()
	keepTemp := true
	defer func() {
		_ = tmp.Close()
		if keepTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(securefs.PrivateFileMode); err != nil {
		return fmt.Errorf("chmod session snapshot temp file: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		return fmt.Errorf("write session snapshot temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync session snapshot temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close session snapshot temp file: %w", err)
	}
	if err := securefs.ValidateRegularTarget(path); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace session snapshot file: %w", err)
	}
	keepTemp = false
	if err := syncDirectory(root); err != nil {
		return fmt.Errorf("sync session store dir: %w", err)
	}
	return nil
}

func sessionPath(root string, sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	if sessionID == "." || strings.Contains(sessionID, "..") {
		return "", fmt.Errorf("session id must not contain consecutive dots")
	}
	if strings.ContainsAny(sessionID, `/\\`) {
		return "", fmt.Errorf("session id must not contain path separators")
	}
	if strings.ContainsRune(sessionID, '\x00') {
		return "", fmt.Errorf("session id must not contain NUL")
	}
	path := filepath.Join(root, sessionID+".json")
	if err := securefs.EnsureContained(root, path); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) lockStoreLocked(ctx context.Context, exclusive bool) (string, func(), error) {
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	default:
	}
	root, err := s.secureRootLocked()
	if err != nil {
		return "", nil, err
	}
	lockFile, err := securefs.OpenLock(filepath.Join(root, ".store.lock"))
	if err != nil {
		return "", nil, fmt.Errorf("open session store lock: %w", err)
	}
	operation := unix.LOCK_SH
	if exclusive {
		operation = unix.LOCK_EX
	}
	if err := unix.Flock(int(lockFile.Fd()), operation); err != nil {
		_ = lockFile.Close()
		return "", nil, fmt.Errorf("lock session store: %w", err)
	}
	unlock := func() {
		_ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
		_ = lockFile.Close()
	}
	return root, unlock, nil
}

func (s *Store) secureRootLocked() (string, error) {
	if strings.TrimSpace(s.dir) == "" {
		return "", fmt.Errorf("session store dir is required")
	}
	root, err := filepath.Abs(filepath.Clean(s.dir))
	if err != nil {
		return "", fmt.Errorf("resolve session store dir: %w", err)
	}
	if err := securefs.TightenPrivateDir(root); err != nil {
		return "", fmt.Errorf("create session store dir: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve session store symlinks: %w", err)
	}
	if s.canonicalDir != "" && s.canonicalDir != canonical {
		return "", fmt.Errorf("session store directory changed: was=%s now=%s", s.canonicalDir, canonical)
	}
	s.canonicalDir = canonical
	return canonical, nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
