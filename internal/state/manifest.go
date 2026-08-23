package state

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

// ManifestSchema is the on-disk identity of the gentle-ai managed-assets
// manifest. Bump SUFFIX when the wire shape changes incompatibly.
const ManifestSchema = "gentle-ai.managed-assets/v1"

const (
	manifestFileName = "managed-assets.manifest.json"
	journalFileName  = "managed-assets.journal"
	journalCapBytes  = 1 << 20 // 1 MiB
	StateDirName     = ".gentle-ai"
)

// Producer is the binary identity that wrote a manifest.
type Producer struct {
	BinaryVersion string `json:"binary_version"`
	Commit        string `json:"commit"`
}

// BundleDigest is the bundle identity, derived over a canonicalised subset
// of the manifest (everything except observed digests). Stable across
// observed-only updates; changes whenever owned_extent or desired changes.
type BundleDigest struct {
	Algo   string `json:"algo"`
	Digest string `json:"digest"`
}

// ExtentKind enumerates the supported shapes for a managed resource's owned
// region. "full" means the entire target file; "marker-block" means the
// region between two exclusive markers named in MarkerID.
type ExtentKind string

const (
	ExtentFull        ExtentKind = "full"
	ExtentMarkerBlock ExtentKind = "marker-block"
)

// Ownership enumerates who owns a resource. "managed" means the resource is
// fully owned by gentle-ai and a sync may rewrite it; "user" means the user
// owns the file (e.g. a shared JSON config) and a sync must never overwrite.
type Ownership string

const (
	OwnershipManaged Ownership = "managed"
	OwnershipUser    Ownership = "user"
)

// OwnedExtent describes the region classification may compare against
// desired digests, plus who owns the file outside of any managed injection.
type OwnedExtent struct {
	Kind      ExtentKind `json:"kind"`
	MarkerID  string     `json:"marker_id,omitempty"`
	Start     int        `json:"start,omitempty"`
	End       int        `json:"end,omitempty"`
	Ownership Ownership  `json:"ownership"`
}

// ManifestResource is a single managed-resource entry.
type ManifestResource struct {
	ID          string      `json:"id"`
	Adapter     string      `json:"adapter"`
	Target      string      `json:"target"`
	OwnedExtent OwnedExtent `json:"owned_extent"`
	Desired     string      `json:"desired"`
	Observed    string      `json:"observed,omitempty"`
}

// Manifest is the on-disk bundle identity written by install/sync.
type Manifest struct {
	Schema    string             `json:"schema"`
	Producer  Producer           `json:"producer"`
	Bundle    BundleDigest       `json:"bundle"`
	Resources []ManifestResource `json:"resources"`
}

// ManifestPath returns the absolute path to the manifest for homeDir.
func ManifestPath(homeDir string) string {
	return filepath.Join(homeDir, StateDirName, manifestFileName)
}

// JournalPath returns the absolute path to the co-located journal.
func JournalPath(homeDir string) string {
	return filepath.Join(homeDir, StateDirName, journalFileName)
}

// ReadManifest decodes the manifest at ManifestPath(homeDir). A missing file
// returns os.ErrNotExist so callers (doctor) can treat absence as the legacy
// "unknown" classification rather than a parse error.
func ReadManifest(homeDir string) (Manifest, error) {
	data, err := os.ReadFile(ManifestPath(homeDir))
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// WriteManifestAtomic stages, syncs, and renames the manifest onto disk.
// Uses filemerge.WriteFileAtomic for the same staged+fsync+rename contract
// as state.Write so a crash mid-write does not produce a partial file.
func WriteManifestAtomic(homeDir string, m Manifest) error {
	dir := filepath.Join(homeDir, StateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = filemerge.WriteFileAtomic(ManifestPath(homeDir), data, 0o644)
	return err
}

// canonicalResource is the subset of a ManifestResource that participates in
// the bundle identity; observed digests are deliberately excluded.
type canonicalResource struct {
	ID          string      `json:"id"`
	OwnedExtent OwnedExtent `json:"owned_extent"`
	Desired     string      `json:"desired"`
}

// canonicalManifest is the subset of Manifest that participates in the
// bundle identity; the bundle field itself is excluded to avoid a
// self-referencing digest.
type canonicalManifest struct {
	Producer  Producer            `json:"producer"`
	Resources []canonicalResource `json:"resources"`
}

// ComputeBundleDigest returns the deterministic sha256 hex digest over the
// canonicalised manifest (producer + per-resource owned_extent + desired,
// sorted by stable ID; observed digests excluded).
func ComputeBundleDigest(m Manifest) string {
	resources := make([]canonicalResource, len(m.Resources))
	for i, r := range m.Resources {
		resources[i] = canonicalResource{ID: r.ID, OwnedExtent: r.OwnedExtent, Desired: r.Desired}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].ID < resources[j].ID })

	canonical := canonicalManifest{Producer: m.Producer, Resources: resources}
	data, err := json.Marshal(canonical)
	if err != nil {
		// Marshal of a struct of strings cannot fail; returning an empty
		// digest preserves the contract that ComputeBundleDigest is total.
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// WithBundleDigest returns a copy of m with Bundle.{Algo,Digest} recomputed
// from the canonicalised producer + resources. It does not persist.
func (m Manifest) WithBundleDigest() Manifest {
	m.Bundle = BundleDigest{Algo: "sha256", Digest: ComputeBundleDigest(m)}
	return m
}

// JournalEntry is one record in the co-located journal. The Op field is one
// of OpIntent / OpComplete / OpInterrupted / OpTruncated. "truncated" is a
// single replacement line and signals that all older entries were cleared.
type JournalEntry struct {
	TS       string `json:"ts"`
	Op       string `json:"op"`
	RunID    string `json:"run_id"`
	Resource string `json:"resource,omitempty"`
}

// JournalOp values used by AppendJournal / ReadJournal callers.
const (
	OpIntent      JournalOp = "intent"
	OpComplete    JournalOp = "complete"
	OpInterrupted JournalOp = "interrupted"
	OpTruncated   JournalOp = "truncated"
)

// JournalOp names the legal values of JournalEntry.Op.
type JournalOp string

// AppendJournal adds one entry to the co-located journal and caps the file
// at 1 MiB (older entries are cleared by a single "truncated" marker).
func AppendJournal(homeDir, op, runID, resource string) error {
	dir := filepath.Join(homeDir, StateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entry := JournalEntry{
		TS:       time.Now().UTC().Format(time.RFC3339Nano),
		Op:       op,
		RunID:    runID,
		Resource: resource,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := JournalPath(homeDir)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return capJournal(homeDir)
}

// ReadJournal returns every entry in append order. A "truncated" marker
// clears any older entries (its predecessor was just purged). Missing
// journal returns (nil, nil) — caller treats absence as "clean state".
func ReadJournal(homeDir string) ([]JournalEntry, error) {
	f, err := os.Open(JournalPath(homeDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []JournalEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e JournalEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.Op == "truncated" {
			// A truncated marker is a single replacement line: drop everything
			// older and keep only the marker itself.
			entries = []JournalEntry{e}
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// capJournal truncates the journal to the configured cap, replacing all older
// entries with a single "truncated" marker line.
func capJournal(homeDir string) error {
	path := JournalPath(homeDir)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() <= journalCapBytes {
		return nil
	}
	marker := fmt.Sprintf(`{"ts":%q,"op":"truncated"}`+"\n", time.Now().UTC().Format(time.RFC3339Nano))
	return os.WriteFile(path, []byte(marker), 0o644)
}
