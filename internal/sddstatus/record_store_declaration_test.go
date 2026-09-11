package sddstatus

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDeclaredRecordStore mirrors declaredArtifactStore's own casing and file
// lookup tolerance — record_store is read the same way, on its own axis.
func TestDeclaredRecordStore(t *testing.T) {
	tests := []struct {
		name         string
		fileName     string
		content      string
		wantRaw      string
		wantResolved RecordStore
	}{
		{
			name:         "recognised value",
			fileName:     "config.yaml",
			content:      "record_store: openrecord\n",
			wantRaw:      "openrecord",
			wantResolved: RecordStoreOpenRecord,
		},
		{
			name:         "absent key",
			fileName:     "config.yaml",
			content:      "strict_tdd: true\n",
			wantRaw:      "",
			wantResolved: RecordStoreUndeclared,
		},
		{
			name:         "unrecognised value raises no error",
			fileName:     "config.yaml",
			content:      "record_store: not-a-real-store\n",
			wantRaw:      "not-a-real-store",
			wantResolved: RecordStoreUndeclared,
		},
		{
			name:         "camelCase key",
			fileName:     "config.yaml",
			content:      "recordStore: openrecord\n",
			wantRaw:      "openrecord",
			wantResolved: RecordStoreOpenRecord,
		},
		{
			name:         "yml extension",
			fileName:     "config.yml",
			content:      "record_store: openrecord\n",
			wantRaw:      "openrecord",
			wantResolved: RecordStoreOpenRecord,
		},
		{
			name:         "trailing comment",
			fileName:     "config.yaml",
			content:      "record_store: openrecord # durable records\n",
			wantRaw:      "openrecord",
			wantResolved: RecordStoreOpenRecord,
		},
		{
			name:         "quoted value",
			fileName:     "config.yaml",
			content:      `record_store: "openrecord"` + "\n",
			wantRaw:      "openrecord",
			wantResolved: RecordStoreOpenRecord,
		},
		{
			name:         "commented out line reads as absent",
			fileName:     "config.yaml",
			content:      "# record_store: openrecord\n",
			wantRaw:      "",
			wantResolved: RecordStoreUndeclared,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "openspec", tt.fileName), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}

			raw, resolved := declaredRecordStore(root)
			if raw != tt.wantRaw {
				t.Errorf("raw = %q, want %q", raw, tt.wantRaw)
			}
			if resolved != tt.wantResolved {
				t.Errorf("resolved = %q, want %q", resolved, tt.wantResolved)
			}
		})
	}
}

// TestDeclaredRecordStoreNoConfigFile pins the no-file case distinctly from
// the no-key case: both must resolve undeclared, with no error.
func TestDeclaredRecordStoreNoConfigFile(t *testing.T) {
	root := t.TempDir()
	raw, resolved := declaredRecordStore(root)
	if raw != "" || resolved != RecordStoreUndeclared {
		t.Errorf("declaredRecordStore() = (%q, %q), want (\"\", undeclared)", raw, resolved)
	}
}

// TestBothStoreAxesDeclaredIndependently pins the spec's "A separate axis
// from the artifact store" requirement literally: a config declaring both
// artifact_store and record_store resolves each axis independently from the
// same file read — neither key's value leaks into or displaces the other.
func TestBothStoreAxesDeclaredIndependently(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "artifact_store: engram\nrecord_store: openrecord\n"
	if err := os.WriteFile(filepath.Join(root, "openspec", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	artifactStore, ok := declaredArtifactStore(root)
	if !ok || artifactStore != ArtifactStoreEngram {
		t.Errorf("declaredArtifactStore() = (%q, %v), want (engram, true)", artifactStore, ok)
	}

	raw, resolved := declaredRecordStore(root)
	if raw != "openrecord" || resolved != RecordStoreOpenRecord {
		t.Errorf("declaredRecordStore() = (%q, %q), want (\"openrecord\", openrecord)", raw, resolved)
	}
}
