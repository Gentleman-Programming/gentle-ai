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

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
)

// ManifestSchema is the on-disk identity; bump the suffix on incompatible changes.
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

// BundleDigest is derived over a canonical subset of the manifest (everything except observed).
type BundleDigest struct {
	Algo   string `json:"algo"`
	Digest string `json:"digest"`
}

// ExtentKind: "full" owns the entire target; "marker-block" owns the region between two markers in MarkerID.
type ExtentKind string

const (
	ExtentFull        ExtentKind = "full"
	ExtentMarkerBlock ExtentKind = "marker-block"
)

// Ownership: "managed" lets sync rewrite; "user" must never be overwritten.
type Ownership string

const (
	OwnershipManaged Ownership = "managed"
	OwnershipUser    Ownership = "user"
)

// OwnedExtent describes the region to compare desired against, plus the
// ownership role (managed / user) outside any managed injection.
type OwnedExtent struct {
	Kind      ExtentKind `json:"kind"`
	MarkerID  string     `json:"marker_id,omitempty"`
	Start     int        `json:"start,omitempty"`
	End       int        `json:"end,omitempty"`
	Ownership Ownership  `json:"ownership"`
}

// ManifestResource is one managed resource entry.
type ManifestResource struct {
	ID          string      `json:"id"`
	Adapter     string      `json:"adapter"`
	Target      string      `json:"target"`
	OwnedExtent OwnedExtent `json:"owned_extent"`
	Desired     string      `json:"desired"`
	Observed    string      `json:"observed,omitempty"`
}

// Manifest is the bundle identity written by install/sync.
type Manifest struct {
	Schema    string             `json:"schema"`
	Producer  Producer           `json:"producer"`
	Bundle    BundleDigest       `json:"bundle"`
	Resources []ManifestResource `json:"resources"`
}

func ManifestPath(homeDir string) string {
	return filepath.Join(homeDir, StateDirName, manifestFileName)
}

func JournalPath(homeDir string) string {
	return filepath.Join(homeDir, StateDirName, journalFileName)
}

// ReadManifest decodes the manifest at ManifestPath. Missing file returns
// os.ErrNotExist so the doctor treats absence as "unknown" classification.
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

// WriteManifestAtomic writes the manifest with the staged+fsync+rename
// contract so a crash mid-write cannot produce a partial file.
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

// canonicalResource excludes observed to break the feedback loop.
type canonicalResource struct {
	ID          string      `json:"id"`
	OwnedExtent OwnedExtent `json:"owned_extent"`
	Desired     string      `json:"desired"`
}

// canonicalManifest excludes the bundle field itself to avoid a self-reference.
type canonicalManifest struct {
	Producer  Producer            `json:"producer"`
	Resources []canonicalResource `json:"resources"`
}

// ComputeBundleDigest returns the deterministic sha256 hex digest over
// canonicalised (producer + per-resource owned_extent + desired, sorted by ID).
func ComputeBundleDigest(m Manifest) string {
	resources := make([]canonicalResource, len(m.Resources))
	for i, r := range m.Resources {
		resources[i] = canonicalResource{ID: r.ID, OwnedExtent: r.OwnedExtent, Desired: r.Desired}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].ID < resources[j].ID })

	canonical := canonicalManifest{Producer: m.Producer, Resources: resources}
	data, err := json.Marshal(canonical)
	if err != nil {
		// A struct of strings cannot fail to marshal; keep the function total.
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

// JournalEntry is one record; "truncated" replaces older entries.
type JournalEntry struct {
	TS       string `json:"ts"`
	Op       string `json:"op"`
	RunID    string `json:"run_id"`
	Resource string `json:"resource,omitempty"`
}

// Legal values for JournalEntry.Op.
const (
	OpIntent    = "intent"
	OpComplete  = "complete"
	OpTruncated = "truncated"
)

// AppendJournal appends one entry and caps the file at 1 MiB (older
// entries are replaced by a single "truncated" marker).
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
// clears any older entries. Missing journal returns (nil, nil).
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
			// Drop everything older; keep only the marker itself.
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

// capJournal truncates the journal to the cap, leaving a single "truncated" marker.
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
