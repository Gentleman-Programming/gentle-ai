// Package managedstate implements the #1876 migration transaction contract:
// a write-ahead journal subordinate to the committed managed-assets manifest
// (internal/state), proving crash-recovery semantics for full-file resources.
//
// Authority split (approved design, gentle-ai#1876):
//
//	~/.gentle-ai/managed-assets.manifest.json  committed ownership truth
//	~/.gentle-ai/transactions/<id>/journal.json temporary recovery truth
//
// The journal never survives as a second committed record: it is deleted on
// completion and one audit line lands in the existing append-only journal.
package managedstate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// JournalSchema is the on-disk identity of the transaction journal.
const JournalSchema = "gentle-ai.migration-journal/v1"

const journalFileName = "journal.json"

// Phase is one durable lifecycle phase of a migration transaction.
type Phase string

const (
	PhaseDiscovered        Phase = "discovered"
	PhaseClassified        Phase = "classified"
	PhaseSnapshotted       Phase = "snapshotted"
	PhasePrepared          Phase = "prepared"
	PhaseApplying          Phase = "applying"
	PhaseVerified          Phase = "verified"
	PhaseManifestCommitted Phase = "manifest_committed"
	PhaseCompleted         Phase = "completed"
)

// phaseOrder ranks the forward phases; prepared is the first restartable one.
var phaseOrder = map[Phase]int{
	PhaseDiscovered:        1,
	PhaseClassified:        2,
	PhaseSnapshotted:       3,
	PhasePrepared:          4,
	PhaseApplying:          5,
	PhaseVerified:          6,
	PhaseManifestCommitted: 7,
	PhaseCompleted:         8,
}

// Status is a terminal, non-forward journal outcome. Empty means active.
type Status string

const (
	StatusBlockedUnknownOwnership Status = "blocked_unknown_ownership"
	StatusBlockedConflict         Status = "blocked_conflict"
	StatusRollbackRequired        Status = "rollback_required"
	StatusRolledBack              Status = "rolled_back"
)

// DigestMode is the durable identity of on-disk bytes: content digest plus
// permissions. Exists=false means the target is absent.
type DigestMode struct {
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
}

// Resource is one managed resource's recovery truth inside the journal.
type Resource struct {
	ID       string            `json:"id"`
	Adapter  string            `json:"adapter"`
	Target   string            `json:"target"`
	Extent   state.OwnedExtent `json:"owned_extent"`
	Before   DigestMode        `json:"before"`
	Desired  DigestMode        `json:"desired"`
	Applied  DigestMode        `json:"applied,omitempty"`
	Snapshot string            `json:"snapshot,omitempty"`
	Foreign  bool              `json:"foreign,omitempty"`
}

// Journal is the write-ahead transaction record for one proposed manifest
// transition. Revisions are atomic and re-read; the file is the only
// restart input.
type Journal struct {
	Schema                 string     `json:"schema"`
	TransactionID          string     `json:"transaction_id"`
	CreatedAt              string     `json:"created_at"`
	HomeDir                string     `json:"home_dir"`
	FromSchema             string     `json:"from_schema"`
	ExpectedManifestDigest string     `json:"expected_manifest_digest,omitempty"`
	ProposedManifestDigest string     `json:"proposed_manifest_digest,omitempty"`
	Resources              []Resource `json:"resources"`
	LastPhase              Phase      `json:"last_phase"`
	Status                 Status     `json:"status,omitempty"`
}

// ErrNoActiveJournal means no transaction directory holds a journal.
var ErrNoActiveJournal = errors.New("managedstate: no active migration journal")

// TransactionsDir is the root holding one directory per transaction.
func TransactionsDir(homeDir string) string {
	return filepath.Join(homeDir, state.StateDirName, "transactions")
}

func transactionDir(homeDir, txID string) string {
	return filepath.Join(TransactionsDir(homeDir), txID)
}

func journalPath(homeDir, txID string) string {
	return filepath.Join(transactionDir(homeDir, txID), journalFileName)
}

// LoadJournal decodes one transaction's journal.
func LoadJournal(homeDir, txID string) (*Journal, error) {
	data, err := os.ReadFile(journalPath(homeDir, txID))
	if err != nil {
		return nil, err
	}
	var j Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("managedstate: journal %s is unreadable: %w", txID, err)
	}
	return &j, nil
}

// LoadActiveJournal returns the single journal of an unfinished transaction.
// Two active journals are an integrity anomaly and never guessed between.
func LoadActiveJournal(homeDir string) (*Journal, error) {
	root := TransactionsDir(homeDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoActiveJournal
		}
		return nil, err
	}
	var active []*Journal
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		j, err := LoadJournal(homeDir, e.Name())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue // directory without a journal is not a transaction
			}
			return nil, err
		}
		// Completed transactions are deleted on completion; anything else,
		// including terminal blocked/rolled-back journals, is active truth.
		if j.LastPhase != PhaseCompleted {
			active = append(active, j)
		}
	}
	switch len(active) {
	case 0:
		return nil, ErrNoActiveJournal
	case 1:
		return active[0], nil
	default:
		return nil, fmt.Errorf("managedstate: two active migration journals (%s, %s); refusing to guess", active[0].TransactionID, active[1].TransactionID)
	}
}

// persist writes one atomic journal revision, re-reads it, and syncs the
// directory (best effort on platforms without directory fsync).
func (j *Journal) persist(homeDir string) error {
	dir := transactionDir(homeDir, j.TransactionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := journalPath(homeDir, j.TransactionID)
	if _, err := filemerge.WriteFileAtomic(path, data, 0o644); err != nil {
		return err
	}
	// Re-read: a journal revision that cannot be read back is not durable.
	reread, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var check Journal
	if err := json.Unmarshal(reread, &check); err != nil {
		return fmt.Errorf("managedstate: journal revision failed re-read: %w", err)
	}
	if check.LastPhase != j.LastPhase || check.Status != j.Status {
		return fmt.Errorf("managedstate: journal revision mismatch after re-read")
	}
	syncDir(dir)
	return nil
}

func syncDir(dir string) {
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = f.Sync()
	_ = f.Close()
}

func newTransactionID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		b[0], b[1], b[2], b[3] = 0xde, 0xad, 0xbe, 0xef
	}
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}
