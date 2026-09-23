package main

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
)

// TestInitMutatesSelfToolEntryFieldByField pins the D-01 invariant: init() must
// mutate update.Tools field by field — never replace the whole ToolInfo struct —
// so that GoModulePath and any future registry fields survive the rename.
func TestInitMutatesSelfToolEntryFieldByField(t *testing.T) {
	var selfTool *update.ToolInfo
	for i := range update.Tools {
		if update.IsSelfToolName(update.Tools[i].Name) {
			selfTool = &update.Tools[i]
			break
		}
	}
	if selfTool == nil {
		t.Fatal("no self-tool entry found in update.Tools after init()")
	}

	if selfTool.Name != "axiom" {
		t.Errorf("Name = %q, want %q", selfTool.Name, "axiom")
	}
	if selfTool.Owner != "IGutierrezZ" {
		t.Errorf("Owner = %q, want %q", selfTool.Owner, "IGutierrezZ")
	}
	if selfTool.Repo != "axiom" {
		t.Errorf("Repo = %q, want %q", selfTool.Repo, "axiom")
	}
	if selfTool.GoImportPath != "github.com/IGutierrezZ/axiom/cmd/axiom" {
		t.Errorf("GoImportPath = %q, want %q", selfTool.GoImportPath, "github.com/IGutierrezZ/axiom/cmd/axiom")
	}
	if selfTool.DetectCmd != nil {
		t.Errorf("DetectCmd = %v, want nil", selfTool.DetectCmd)
	}
	if selfTool.VersionPrefix != "v" {
		t.Errorf("VersionPrefix = %q, want %q", selfTool.VersionPrefix, "v")
	}

	// Key assertion: GoModulePath must survive the rename. Full-struct
	// replacement would zero this field (D-01).
	if selfTool.GoModulePath != "github.com/gentleman-programming/gentle-ai/v3" {
		t.Errorf("GoModulePath = %q, want %q (must be preserved by field-by-field mutation)",
			selfTool.GoModulePath, "github.com/gentleman-programming/gentle-ai/v3")
	}
}

// TestInitPreservesSelfToolNameRecognition verifies that IsSelfToolName still
// recognizes both historical names after the mutation renames the entry.
func TestInitPreservesSelfToolNameRecognition(t *testing.T) {
	if !update.IsSelfToolName("axiom") {
		t.Error(`IsSelfToolName("axiom") = false, want true`)
	}
	if !update.IsSelfToolName("gentle-ai") {
		t.Error(`IsSelfToolName("gentle-ai") = false, want true`)
	}
}
