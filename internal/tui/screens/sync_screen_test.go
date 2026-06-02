package screens

import (
	"fmt"
	"strings"
	"testing"
)

// ─── RenderSync states ─────────────────────────────────────────────────────

// TestRenderSync_ConfirmState verifies the default confirm state — no operation
// running, no result yet — shows sync description and a prompt.
func TestRenderSync_ConfirmState(t *testing.T) {
	out := RenderSync(nil, nil, false /*operationRunning*/, false /*hasSyncRun*/, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "sync") {
		t.Errorf("RenderSync(confirm) should contain 'sync'; got:\n%s", out)
	}
	// Should show a prompt to press enter.
	if !strings.Contains(lower, "enter") && !strings.Contains(lower, "confirm") {
		t.Errorf("RenderSync(confirm) should show enter/confirm prompt; got:\n%s", out)
	}
}

// TestRenderSync_RunningState verifies that while sync is running the screen
// shows a spinner/progress indicator.
func TestRenderSync_RunningState(t *testing.T) {
	out := RenderSync(nil, nil, true /*operationRunning*/, false, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "syncing") && !strings.Contains(lower, "please wait") {
		t.Errorf("RenderSync(running) should show 'syncing' or 'please wait'; got:\n%s", out)
	}
}

// TestRenderSync_ResultWithFilesChanged verifies that after a successful sync
// with changed files, the screen shows the file count.
func TestRenderSync_ResultWithFilesChanged(t *testing.T) {
	files := []string{"a", "b", "c", "d", "e"}
	out := RenderSync(files, nil, false, true /*hasSyncRun*/, 0)

	if !strings.Contains(out, "5") {
		t.Errorf("RenderSync(filesChanged=5) should show '5'; got:\n%s", out)
	}
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "sync") {
		t.Errorf("RenderSync(result) should mention 'sync'; got:\n%s", out)
	}
	// Verify individual file paths are rendered.
	for _, f := range files {
		if !strings.Contains(out, f) {
			t.Errorf("RenderSync should render file path %q in output; got:\n%s", f, out)
		}
	}
}

// TestRenderSync_ResultWithError verifies that a failed sync shows the error
// message.
func TestRenderSync_ResultWithError(t *testing.T) {
	syncErr := fmt.Errorf("connection refused: agent config dir not writable")
	out := RenderSync(nil, syncErr, false, true /*hasSyncRun*/, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "fail") && !strings.Contains(lower, "error") {
		t.Errorf("RenderSync(error) should show failure indicator; got:\n%s", out)
	}
	if !strings.Contains(out, syncErr.Error()) {
		t.Errorf("RenderSync(error) should show error text %q; got:\n%s", syncErr.Error(), out)
	}
}

// TestRenderSync_TitleAlwaysPresent verifies the screen title is shown in all
// states.
func TestRenderSync_TitleAlwaysPresent(t *testing.T) {
	states := []struct {
		name             string
		files            []string
		syncErr          error
		operationRunning bool
		hasSyncRun       bool
	}{
		{"confirm", nil, nil, false, false},
		{"running", nil, nil, true, false},
		{"success", []string{"a", "b", "c"}, nil, false, true},
		{"error", nil, fmt.Errorf("fail"), false, true},
	}

	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			out := RenderSync(s.files, s.syncErr, s.operationRunning, s.hasSyncRun, 0)
			if !strings.Contains(out, "Sync") {
				t.Errorf("RenderSync state=%q should contain 'Sync'; got:\n%s", s.name, out)
			}
		})
	}
}

// TestRenderSync_ZeroFilesChangedWithNoError verifies the "nothing to update"
// case (hasSyncRun=true, filesChanged=0, no error) shows a completion message.
func TestRenderSync_ZeroFilesChangedWithNoError(t *testing.T) {
	out := RenderSync(nil, nil, false, true /*hasSyncRun*/, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "sync complete") && !strings.Contains(lower, "complete") &&
		!strings.Contains(lower, "no agents") {
		t.Errorf("RenderSync(0 files, no error) should show completion; got:\n%s", out)
	}
}

// TestRenderSync_TruncatesLargeFileList verifies that when more than
// maxFilesToShow files changed, the output shows a truncation indicator.
func TestRenderSync_TruncatesLargeFileList(t *testing.T) {
	files := make([]string, maxFilesToShow+5)
	for i := range files {
		files[i] = fmt.Sprintf("file-%d.txt", i)
	}
	out := RenderSync(files, nil, false, true /*hasSyncRun*/, 0)

	// First file should be rendered.
	if !strings.Contains(out, "file-0.txt") {
		t.Errorf("RenderSync should render first file; got:\n%s", out)
	}
	// Last fully rendered file (index maxFilesToShow-1).
	if !strings.Contains(out, fmt.Sprintf("file-%d.txt", maxFilesToShow-1)) {
		t.Errorf("RenderSync should render file at index %d; got:\n%s", maxFilesToShow-1, out)
	}
	// Truncation message.
	if !strings.Contains(out, "and 5 more") {
		t.Errorf("RenderSync should show truncation message; got:\n%s", out)
	}
}
