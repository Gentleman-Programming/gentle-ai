package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #4119: builds a minimal openspec tree (canonical spec with two
// requirements, a change delta with an ADDED and a MODIFIED requirement) and
// runs the archive merge through the CLI surface the sdd-archive skill
// calls. Before ComposeOpenSpecCanonicalSpec existed, composition was
// model-driven prose with no Go command to call, so this exact shape
// (dropped requirement / unapplied delta) could not be exercised or gated.
func TestRunSDDArchiveComposeMinimalOpenSpecTree(t *testing.T) {
	root := t.TempDir()
	canonicalPath := filepath.Join(root, "openspec", "specs", "widgets", "spec.md")
	deltaPath := filepath.Join(root, "openspec", "changes", "add-widget-tagging", "specs", "widgets", "spec.md")

	writeCLIFixture(t, canonicalPath, "## Requirements\n\n"+
		"### Requirement: Unrelated Listing\n\nThe system MUST list widgets.\n\n"+
		"### Requirement: Widget Expiration\n\nThe system MUST expire widgets after 30 days.\n")
	writeCLIFixture(t, deltaPath, "## ADDED Requirements\n\n"+
		"### Requirement: Widget Tagging\n\nThe system MUST support tagging widgets.\n\n"+
		"## MODIFIED Requirements\n\n### Requirement: Widget Expiration\n\nThe system MUST expire widgets after 90 days.\n")

	var output bytes.Buffer
	if err := runSDDArchiveCompose([]string{"--canonical", canonicalPath, "--delta", deltaPath}, &output); err != nil {
		t.Fatalf("runSDDArchiveCompose() error = %v", err)
	}

	composed := output.String()
	if !strings.Contains(composed, "### Requirement: Unrelated Listing") {
		t.Fatalf("composed spec dropped the unrelated requirement:\n%s", composed)
	}
	if !strings.Contains(composed, "expire widgets after 90 days") || strings.Contains(composed, "expire widgets after 30 days") {
		t.Fatalf("composed spec did not apply the MODIFIED delta:\n%s", composed)
	}
	if !strings.Contains(composed, "### Requirement: Widget Tagging") {
		t.Fatalf("composed spec did not apply the ADDED delta:\n%s", composed)
	}
}

func TestRunSDDArchiveComposeRefusesUnappliedDeltaWithoutWriting(t *testing.T) {
	root := t.TempDir()
	canonicalPath, deltaPath, outputPath := filepath.Join(root, "spec.md"), filepath.Join(root, "delta.md"), filepath.Join(root, "out.md")

	writeCLIFixture(t, canonicalPath, "## Requirements\n\n### Requirement: Existing\n\nBody.\n")
	writeCLIFixture(t, deltaPath, "## MODIFIED Requirements\n\n### Requirement: Missing\n\nNew body.\n")

	err := runSDDArchiveCompose([]string{"--canonical", canonicalPath, "--delta", deltaPath, "--output", outputPath}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MODIFIED") || !strings.Contains(err.Error(), "Missing") {
		t.Fatalf("error = %v, want it to name the MODIFIED delta for %q", err, "Missing")
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("output file was written despite refusal: statErr = %v", statErr)
	}
}

func writeCLIFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
