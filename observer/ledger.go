package observer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/internal/securefs"
	"golang.org/x/sys/unix"
)

const (
	ledgerRootSchemaVersion = 1
	ledgerHashAlgorithm     = "sha256"
	ledgerHashDomain        = "til-consensus-ledger-entry-v1\n"
)

type LedgerWriter struct {
	path         string
	manifestPath string
	mu           sync.Mutex
}

type LedgerIntegrity struct {
	EntryCount int
	RootHash   string
	Legacy     bool
}

type ledgerState struct {
	entries  []consensus.EvidenceRecord
	rootHash string
	legacy   bool
}

func NewLedger(path string, manifestPath string) *LedgerWriter {
	return &LedgerWriter{path: strings.TrimSpace(path), manifestPath: strings.TrimSpace(manifestPath)}
}

func (w *LedgerWriter) Append(ctx context.Context, entry consensus.EvidenceRecord) (consensus.EvidenceRecord, error) {
	if strings.TrimSpace(entry.EntryID) == "" {
		return consensus.EvidenceRecord{}, fmt.Errorf("ledger entryId is required")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	var committed consensus.EvidenceRecord
	err := withLedgerLock(ctx, w.path, true, func() error {
		state, err := loadAndRepairLedgerLocked(w.path, w.manifestPath)
		if err != nil {
			return err
		}
		if existing, ok, err := findExistingEntry(state.entries, entry); err != nil {
			return err
		} else if ok {
			committed = existing
			return nil
		}

		entry.Seq = len(state.entries)
		entry.PrevHash = state.rootHash
		entry.EntryHash = ""
		entry.EntryHash, err = calculateEntryHash(entry)
		if err != nil {
			return fmt.Errorf("hash ledger entry: %w", err)
		}
		line, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal ledger entry: %w", err)
		}
		if err := appendLedgerLine(w.path, append(line, '\n')); err != nil {
			return err
		}
		committed = entry
		state.entries = append(state.entries, entry)
		state.rootHash = entry.EntryHash
		if err := writeLedgerRoot(w.path, state.entries); err != nil {
			return fmt.Errorf("write ledger root: %w", err)
		}
		if err := rebuildManifest(w.manifestPath, state.entries); err != nil {
			return fmt.Errorf("rebuild artifact manifest after ledger commit: %w", err)
		}
		return nil
	})
	return committed, err
}

func (w *LedgerWriter) Verify(ctx context.Context) (LedgerIntegrity, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var integrity LedgerIntegrity
	err := withLedgerLock(ctx, w.path, false, func() error {
		var err error
		integrity, err = verifyLedgerLocked(w.path)
		return err
	})
	return integrity, err
}

func (w *LedgerWriter) RootHash(ctx context.Context) (string, error) {
	integrity, err := w.Verify(ctx)
	if err != nil {
		return "", err
	}
	return integrity.RootHash, nil
}

// VerifyLedger validates the complete hash chain and its persisted root
// checkpoint without modifying either file. Legacy unhashed ledgers are
// reported as Legacy and are migrated on the next LedgerWriter.Append.
func VerifyLedger(ctx context.Context, path string) (LedgerIntegrity, error) {
	var integrity LedgerIntegrity
	err := withLedgerLock(ctx, strings.TrimSpace(path), false, func() error {
		var err error
		integrity, err = verifyLedgerLocked(path)
		return err
	})
	return integrity, err
}

func LedgerRootPath(ledgerPath string) string {
	ext := filepath.Ext(ledgerPath)
	if ext == "" {
		return ledgerPath + ".root.json"
	}
	return strings.TrimSuffix(ledgerPath, ext) + ".root.json"
}

func verifyLedgerLocked(path string) (LedgerIntegrity, error) {
	_, integrity, err := readVerifiedLedgerLocked(path)
	return integrity, err
}

// ReadVerifiedLedger reads and verifies one consistent ledger snapshot while
// holding the shared process lock, avoiding a verify-then-read TOCTOU window.
func ReadVerifiedLedger(ctx context.Context, path string) ([]consensus.EvidenceRecord, LedgerIntegrity, error) {
	var entries []consensus.EvidenceRecord
	var integrity LedgerIntegrity
	err := withLedgerLock(ctx, strings.TrimSpace(path), false, func() error {
		var err error
		entries, integrity, err = readVerifiedLedgerLocked(path)
		return err
	})
	return entries, integrity, err
}

func readVerifiedLedgerLocked(path string) ([]consensus.EvidenceRecord, LedgerIntegrity, error) {
	body, err := readOptionalFile(path)
	if err != nil {
		return nil, LedgerIntegrity{}, fmt.Errorf("read ledger: %w", err)
	}
	state, rewrite, err := parseLedger(body, false)
	if err != nil {
		return nil, LedgerIntegrity{}, err
	}
	consumerEntries, err := decodeLedgerForConsumer(body)
	if err != nil {
		return nil, LedgerIntegrity{}, err
	}
	if state.legacy {
		return consumerEntries, LedgerIntegrity{EntryCount: len(state.entries), Legacy: true}, nil
	}
	if rewrite {
		return nil, LedgerIntegrity{}, fmt.Errorf("ledger requires crash recovery before verification")
	}
	root, exists, err := readLedgerRoot(path)
	if err != nil {
		return nil, LedgerIntegrity{}, err
	}
	if !exists {
		if len(state.entries) == 0 {
			return consumerEntries, LedgerIntegrity{}, nil
		}
		return nil, LedgerIntegrity{}, fmt.Errorf("ledger root checkpoint is missing")
	}
	if err := validateExactRoot(root, state.entries); err != nil {
		return nil, LedgerIntegrity{}, err
	}
	return consumerEntries, LedgerIntegrity{EntryCount: len(state.entries), RootHash: state.rootHash}, nil
}

func decodeLedgerForConsumer(body []byte) ([]consensus.EvidenceRecord, error) {
	body = bytes.TrimSuffix(body, []byte{'\n'})
	if len(body) == 0 {
		return nil, nil
	}
	lines := bytes.Split(body, []byte{'\n'})
	entries := make([]consensus.EvidenceRecord, 0, len(lines))
	for idx, line := range lines {
		var entry consensus.EvidenceRecord
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("decode ledger entry %d for consumer: %w", idx, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func loadAndRepairLedgerLocked(path string, manifestPath string) (ledgerState, error) {
	body, err := readOptionalFile(path)
	if err != nil {
		return ledgerState{}, fmt.Errorf("read ledger: %w", err)
	}
	state, rewrite, err := parseLedger(body, true)
	if err != nil {
		return ledgerState{}, err
	}
	root, rootExists, err := readLedgerRoot(path)
	if err != nil {
		return ledgerState{}, err
	}
	if rootExists && state.legacy {
		return ledgerState{}, fmt.Errorf("legacy ledger unexpectedly has a hash root checkpoint")
	}
	if rootExists {
		if err := validateRecoverableRoot(root, state.entries); err != nil {
			return ledgerState{}, err
		}
	}
	if rewrite {
		if err := rewriteLedger(path, state.entries); err != nil {
			return ledgerState{}, fmt.Errorf("recover ledger: %w", err)
		}
	}
	if !rootExists || root.EntryCount != len(state.entries) || root.RootHash != state.rootHash {
		if err := writeLedgerRoot(path, state.entries); err != nil {
			return ledgerState{}, fmt.Errorf("repair ledger root: %w", err)
		}
	}
	if err := rebuildManifest(manifestPath, state.entries); err != nil {
		return ledgerState{}, fmt.Errorf("recover artifact manifest: %w", err)
	}
	state.legacy = false
	return state, nil
}

func parseLedger(body []byte, recoverTail bool) (ledgerState, bool, error) {
	state := ledgerState{}
	if len(body) == 0 {
		return state, false, nil
	}
	hasFinalNewline := body[len(body)-1] == '\n'
	lines := bytes.Split(body, []byte{'\n'})
	if hasFinalNewline {
		lines = lines[:len(lines)-1]
	}
	rewrite := !hasFinalNewline
	seenEntryIDs := make(map[string]struct{}, len(lines))
	hashedEntries := 0
	legacyEntries := 0
	for idx, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			return ledgerState{}, false, fmt.Errorf("ledger entry %d is blank", idx)
		}
		entry, err := decodeLedgerEntry(line)
		if err != nil {
			if recoverTail && idx == len(lines)-1 && !hasFinalNewline {
				break
			}
			return ledgerState{}, false, fmt.Errorf("decode ledger entry %d: %w", idx, err)
		}
		if entry.Seq != len(state.entries) {
			return ledgerState{}, false, fmt.Errorf("ledger sequence mismatch at entry %d: got=%d", len(state.entries), entry.Seq)
		}
		if strings.TrimSpace(entry.EntryID) == "" {
			return ledgerState{}, false, fmt.Errorf("ledger entry %d has no entryId", entry.Seq)
		}
		if _, exists := seenEntryIDs[entry.EntryID]; exists {
			return ledgerState{}, false, fmt.Errorf("duplicate ledger entryId: %s", entry.EntryID)
		}
		seenEntryIDs[entry.EntryID] = struct{}{}
		if entry.EntryHash == "" {
			legacyEntries++
		} else {
			hashedEntries++
		}
		state.entries = append(state.entries, entry)
	}
	if hashedEntries > 0 && legacyEntries > 0 {
		return ledgerState{}, false, fmt.Errorf("ledger mixes hashed and legacy entries")
	}
	if legacyEntries > 0 {
		state.legacy = true
		rewrite = true
		root := ""
		for idx := range state.entries {
			state.entries[idx].PrevHash = root
			state.entries[idx].EntryHash = ""
			hash, err := calculateEntryHash(state.entries[idx])
			if err != nil {
				return ledgerState{}, false, fmt.Errorf("hash migrated ledger entry %d: %w", idx, err)
			}
			state.entries[idx].EntryHash = hash
			root = hash
		}
		state.rootHash = root
		return state, rewrite, nil
	}
	root := ""
	for idx := range state.entries {
		entry := state.entries[idx]
		if entry.PrevHash != root {
			return ledgerState{}, false, fmt.Errorf("ledger prevHash mismatch at entry %d", idx)
		}
		calculated, err := calculateEntryHash(entry)
		if err != nil {
			return ledgerState{}, false, fmt.Errorf("hash ledger entry %d: %w", idx, err)
		}
		if entry.EntryHash != calculated {
			return ledgerState{}, false, fmt.Errorf("ledger entryHash mismatch at entry %d", idx)
		}
		root = entry.EntryHash
	}
	state.rootHash = root
	return state, rewrite, nil
}

func decodeLedgerEntry(line []byte) (consensus.EvidenceRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var entry consensus.EvidenceRecord
	if err := decoder.Decode(&entry); err != nil {
		return consensus.EvidenceRecord{}, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return consensus.EvidenceRecord{}, err
	}
	return entry, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("multiple JSON values")
}

func calculateEntryHash(entry consensus.EvidenceRecord) (string, error) {
	entry.EntryHash = ""
	body, err := canonicalLedgerEntry(entry)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(append([]byte(ledgerHashDomain), body...))
	return hex.EncodeToString(hash[:]), nil
}

func appendLedgerLine(path string, line []byte) error {
	file, err := securefs.OpenAppend(path)
	if err != nil {
		return fmt.Errorf("open ledger file: %w", err)
	}
	if err := writeAll(file, line); err != nil {
		_ = file.Close()
		return fmt.Errorf("write ledger entry: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync ledger entry: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close ledger file: %w", err)
	}
	if err := syncParentDirectory(path); err != nil {
		return fmt.Errorf("sync ledger directory: %w", err)
	}
	return nil
}

func syncParentDirectory(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

func writeAll(writer io.Writer, body []byte) error {
	for len(body) > 0 {
		written, err := writer.Write(body)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		body = body[written:]
	}
	return nil
}

func rewriteLedger(path string, entries []consensus.EvidenceRecord) error {
	body := bytes.NewBuffer(nil)
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		body.Write(line)
		body.WriteByte('\n')
	}
	return securefs.WriteFileAtomic(path, body.Bytes())
}

func writeLedgerRoot(path string, entries []consensus.EvidenceRecord) error {
	root := consensus.LedgerRoot{
		SchemaVersion: ledgerRootSchemaVersion,
		HashAlgorithm: ledgerHashAlgorithm,
		EntryCount:    len(entries),
	}
	if len(entries) > 0 {
		root.RootHash = entries[len(entries)-1].EntryHash
	}
	body, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return securefs.WriteFileAtomic(LedgerRootPath(path), body)
}

func readLedgerRoot(path string) (consensus.LedgerRoot, bool, error) {
	body, err := securefs.ReadFile(LedgerRootPath(path))
	if err != nil {
		if os.IsNotExist(err) {
			return consensus.LedgerRoot{}, false, nil
		}
		return consensus.LedgerRoot{}, false, fmt.Errorf("read ledger root: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var root consensus.LedgerRoot
	if err := decoder.Decode(&root); err != nil {
		return consensus.LedgerRoot{}, false, fmt.Errorf("decode ledger root: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return consensus.LedgerRoot{}, false, fmt.Errorf("decode ledger root: %w", err)
	}
	if root.SchemaVersion != ledgerRootSchemaVersion || root.HashAlgorithm != ledgerHashAlgorithm {
		return consensus.LedgerRoot{}, false, fmt.Errorf("unsupported ledger root format")
	}
	if root.EntryCount < 0 {
		return consensus.LedgerRoot{}, false, fmt.Errorf("ledger root entryCount must be non-negative")
	}
	return root, true, nil
}

func validateExactRoot(root consensus.LedgerRoot, entries []consensus.EvidenceRecord) error {
	if root.EntryCount != len(entries) {
		return fmt.Errorf("ledger root entry count mismatch: checkpoint=%d ledger=%d", root.EntryCount, len(entries))
	}
	expected := hashAtEntryCount(entries, len(entries))
	if root.RootHash != expected {
		return fmt.Errorf("ledger root hash mismatch")
	}
	return nil
}

func validateRecoverableRoot(root consensus.LedgerRoot, entries []consensus.EvidenceRecord) error {
	if root.EntryCount > len(entries) {
		return fmt.Errorf("ledger was truncated: checkpoint=%d ledger=%d", root.EntryCount, len(entries))
	}
	if root.RootHash != hashAtEntryCount(entries, root.EntryCount) {
		return fmt.Errorf("ledger root checkpoint does not match its ledger prefix")
	}
	return nil
}

func hashAtEntryCount(entries []consensus.EvidenceRecord, count int) string {
	if count <= 0 {
		return ""
	}
	return entries[count-1].EntryHash
}

func rebuildManifest(path string, entries []consensus.EvidenceRecord) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	body := bytes.NewBuffer(nil)
	seq := 0
	for _, entry := range entries {
		if entry.Artifact == nil || entry.Artifact.Path == "" {
			continue
		}
		manifest := consensus.ArtifactManifestEntry{
			SchemaVersion:   entry.SchemaVersion,
			Seq:             seq,
			EntryID:         entry.EntryID,
			LedgerEntryHash: entry.EntryHash,
			RequestID:       entry.RequestID,
			SessionID:       entry.SessionID,
			ClaimID:         entry.ClaimID,
			ChallengeID:     entry.ChallengeID,
			VerificationID:  entry.VerificationID,
			Kind:            entry.Kind,
			ProducerRole:    entry.ProducerRole,
			Artifact:        *entry.Artifact,
			LoggedAt:        entry.CreatedAt,
		}
		line, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		body.Write(line)
		body.WriteByte('\n')
		seq++
	}
	return securefs.WriteFileAtomic(path, body.Bytes())
}

func findExistingEntry(entries []consensus.EvidenceRecord, candidate consensus.EvidenceRecord) (consensus.EvidenceRecord, bool, error) {
	if strings.TrimSpace(candidate.EntryID) == "" {
		return consensus.EvidenceRecord{}, false, nil
	}
	for _, existing := range entries {
		if existing.EntryID != candidate.EntryID {
			continue
		}
		existingPayload, err := ledgerPayload(existing)
		if err != nil {
			return consensus.EvidenceRecord{}, false, err
		}
		candidatePayload, err := ledgerPayload(candidate)
		if err != nil {
			return consensus.EvidenceRecord{}, false, err
		}
		if !bytes.Equal(existingPayload, candidatePayload) {
			return consensus.EvidenceRecord{}, false, fmt.Errorf("duplicate ledger entryId with different payload: %s", candidate.EntryID)
		}
		return existing, true, nil
	}
	return consensus.EvidenceRecord{}, false, nil
}

func ledgerPayload(entry consensus.EvidenceRecord) ([]byte, error) {
	entry.Seq = 0
	entry.PrevHash = ""
	entry.EntryHash = ""
	return canonicalLedgerEntry(entry)
}

func canonicalLedgerEntry(entry consensus.EvidenceRecord) ([]byte, error) {
	raw, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	normalized, err := decodeLedgerEntry(raw)
	if err != nil {
		return nil, err
	}
	normalized.EntryHash = ""
	return json.Marshal(normalized)
}

func readOptionalFile(path string) ([]byte, error) {
	body, err := securefs.ReadFile(path)
	if err != nil && os.IsNotExist(err) {
		return nil, nil
	}
	return body, err
}

func withLedgerLock(ctx context.Context, ledgerPath string, exclusive bool, action func() error) error {
	if strings.TrimSpace(ledgerPath) == "" {
		return fmt.Errorf("ledger path is required")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	lockPath := ledgerLockPath(ledgerPath)
	if err := securefs.TightenPrivateDir(filepath.Dir(lockPath)); err != nil {
		return fmt.Errorf("prepare ledger lock directory: %w", err)
	}
	lock, err := securefs.OpenLock(lockPath)
	if err != nil {
		return fmt.Errorf("open ledger lock: %w", err)
	}
	operation := unix.LOCK_SH
	if exclusive {
		operation = unix.LOCK_EX
	}
	if err := unix.Flock(int(lock.Fd()), operation); err != nil {
		_ = lock.Close()
		return fmt.Errorf("lock ledger: %w", err)
	}
	defer func() {
		_ = unix.Flock(int(lock.Fd()), unix.LOCK_UN)
		_ = lock.Close()
	}()
	return action()
}

func ledgerLockPath(ledgerPath string) string {
	canonicalPath, err := filepath.Abs(filepath.Clean(ledgerPath))
	if err != nil {
		canonicalPath = filepath.Clean(ledgerPath)
	}
	if resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(canonicalPath)); err == nil {
		canonicalPath = filepath.Join(resolvedParent, filepath.Base(canonicalPath))
	}
	digest := sha256.Sum256([]byte(canonicalPath))
	lockDir := filepath.Join(os.TempDir(), fmt.Sprintf("til-consensus-ledger-locks-%d", unix.Getuid()))
	return filepath.Join(lockDir, hex.EncodeToString(digest[:])+".lock")
}
