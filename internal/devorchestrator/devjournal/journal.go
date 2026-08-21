// Package devjournal persists per-repo dev-orchestrator dispatch outcomes and
// a resume cursor across process restarts.
//
// devjournal MUST NEVER author phase truth (binding decision Q5, design D2):
// this package does not import internal/sddstatus, Record has no field
// representing phase, and the journal is written exactly once per dispatch
// run from the facade, never from inside a worker goroutine (see
// executor.ConcurrentEngine.ExecuteBatches). Phase is always read from
// sddstatus.StatusV1Projection by the caller, never derived here.
package devjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

// SchemaV1 identifies the on-disk shape of Record.
const SchemaV1 = "gentle-ai.dev-orchestrator-journal/v1"

// Known Dispatch.Outcome values.
const (
	OutcomePlanned    = "planned"
	OutcomeDispatched = "dispatched"
	OutcomeFailed     = "failed"
	OutcomeSkipped    = "skipped"
)

// ErrJournalRevisionConflict is returned by Store.Save when expectedRevision
// no longer matches the journal's current on-disk revision.
var ErrJournalRevisionConflict = errors.New("dev-orchestrator journal revision conflict")

// Cursor records where a dispatch run should resume from.
type Cursor struct {
	BatchIndex int    `json:"batch_index"`
	RepoSlug   string `json:"repo_slug"`
}

// Dispatch records the outcome of one dispatch attempt for one repo.
type Dispatch struct {
	RepoSlug   string    `json:"repo_slug"`
	Agent      string    `json:"agent"`
	Attempt    int       `json:"attempt"`
	Outcome    string    `json:"outcome"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// Record is the single, atomic, per-change dispatch journal document.
//
// StatusDigest is an opaque caller-supplied string (a sha256 of the projected
// status JSON, in practice) used only to detect "the projection moved since
// the cursor was written" — zero phase semantics.
//
// Record's field set is frozen and MUST NOT grow a phase-shaped field; see
// TestRecordJSONKeySetIsFrozen and TestPackageImportsExcludeSDDStatus.
type Record struct {
	Schema       string     `json:"schema"`
	Change       string     `json:"change"`
	UpdatedAt    time.Time  `json:"updated_at"`
	StatusDigest string     `json:"status_digest"`
	Cursor       Cursor     `json:"cursor"`
	Dispatches   []Dispatch `json:"dispatches"`
}

// Loaded pairs a Record with the revision of the exact bytes it was read
// from, for optimistic-concurrency Save calls.
type Loaded struct {
	Record   Record
	Revision string
}

// Store is a handle to one change's journal file.
type Store struct {
	path         string
	usesFallback bool
}

// Open resolves the journal path for change under workspaceRoot. It performs
// no I/O against the journal itself: a missing journal is a valid empty
// state, discovered on the first Load, never an error from Open.
func Open(workspaceRoot, change string) (Store, error) {
	if strings.TrimSpace(change) == "" {
		return Store{}, fmt.Errorf("devjournal: open: change identifier is empty")
	}
	root, usesFallback, err := resolveJournalRoot(workspaceRoot)
	if err != nil {
		return Store{}, err
	}
	return Store{path: journalPath(root, change), usesFallback: usesFallback}, nil
}

// Path returns the absolute path to the journal file this Store reads and
// writes.
func (s Store) Path() string { return s.path }

// UsesFallback reports whether Open resolved this journal under the
// workspace-local `.gentle-ai` fallback tree rather than under the discovered
// Git common directory (design D1: "reported explicitly, never silently").
func (s Store) UsesFallback() bool { return s.usesFallback }

// Load reads the journal. A journal that does not exist yet is a valid empty
// Record, not an error; its Revision is the fixed digest of zero bytes, so
// the very first Save can succeed by presenting that same Revision.
func (s Store) Load() (Loaded, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Loaded{Record: Record{}, Revision: revisionOf(nil)}, nil
		}
		return Loaded{}, fmt.Errorf("devjournal: read journal %q: %w", s.path, err)
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return Loaded{}, fmt.Errorf("devjournal: parse journal %q: %w", s.path, err)
	}
	return Loaded{Record: rec, Revision: revisionOf(data)}, nil
}

// Save writes rec to the journal only if expectedRevision still matches the
// journal's current on-disk revision; otherwise it returns
// ErrJournalRevisionConflict and leaves the file unchanged.
//
// Save is never called from a worker goroutine: the facade performs exactly
// one Save after every concurrent batch dispatch completes (design D1).
func (s Store) Save(rec Record, expectedRevision string) error {
	current, err := s.Load()
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return ErrJournalRevisionConflict
	}

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("devjournal: encode journal: %w", err)
	}
	if _, err := filemerge.WriteFileAtomic(s.path, data, 0o644); err != nil {
		return fmt.Errorf("devjournal: write journal %q: %w", s.path, err)
	}
	return nil
}

// revisionOf returns the hex sha256 of data. A missing journal is treated as
// zero bytes, giving it a fixed revision the first Save can present.
func revisionOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
