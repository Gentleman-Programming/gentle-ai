package sddstatus

import (
	"path/filepath"
	"testing"
)

// #2480: a checkbox row inside a fenced code block is an example, not a task.
func TestCountTaskProgressSkipsFencedCodeBlocks(t *testing.T) {
	progress := countTaskProgressText("- [ ] 1.1 Real task\n\n~~~text\n- [ ] example row\n~~~\n\n```markdown\n- [x] another example\n```\n")
	if progress.Total != 1 || progress.Pending != 1 || progress.Completed != 0 {
		t.Fatalf("progress = %#v, want exactly the one task outside the fences", progress)
	}
}

// #2317: a legacy openspec/changes/active/ container, or any artifact-less
// directory whose subdirectories hold changes, is not a change and must not
// make selection ambiguous. Empty scaffolds keep their historical standing.
func TestChangeContainersDoNotMakeSelectionAmbiguous(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "feat-x", "- [ ] 1.1 Work\n")
	write(t, filepath.Join(root, "openspec", "changes", "active", "legacy-change", "proposal.md"), "# Proposal\n")
	write(t, filepath.Join(root, "openspec", "changes", "legacy", "older-change", "tasks.md"), "- [ ] 1.1 Old\n")
	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil || status.ChangeName == nil || *status.ChangeName != "feat-x" || status.NextRecommended == "select-change" {
		t.Fatalf("err = %v ChangeName = %v NextRecommended = %q, want feat-x auto-selected\nBlockedReasons: %v",
			err, ptrValue(status.ChangeName), status.NextRecommended, status.BlockedReasons)
	}
}
