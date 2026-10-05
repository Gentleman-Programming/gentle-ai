package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type userPathRunnerFunc func(context.Context, ...string) ([]byte, error)

func (f userPathRunnerFunc) Run(ctx context.Context, args ...string) ([]byte, error) {
	return f(ctx, args...)
}

func useWindowsUserPathSeam(t *testing.T, runner userPathPowerShellRunner) {
	t.Helper()
	originalGOOS := userPathGOOS
	originalTest := userPathRunningInGoTest
	originalRunner := newUserPathPowerShellRunner
	userPathGOOS = "windows"
	userPathRunningInGoTest = func() bool { return false }
	newUserPathPowerShellRunner = func() userPathPowerShellRunner { return runner }
	t.Cleanup(func() {
		userPathGOOS = originalGOOS
		userPathRunningInGoTest = originalTest
		newUserPathPowerShellRunner = originalRunner
	})
}

// TestAddToUserPathAlreadyPresent verifies that if the directory is already in PATH,
// AddToUserPath returns nil and does not duplicate it.
func TestAddToUserPathAlreadyPresent(t *testing.T) {
	// Set up a PATH that already contains the target dir.
	targetDir := filepath.Join(t.TempDir(), "already-present")
	original := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", original) })

	os.Setenv("PATH", targetDir+string(os.PathListSeparator)+original)

	err := AddToUserPath(targetDir)
	if err != nil {
		t.Fatalf("AddToUserPath returned unexpected error: %v", err)
	}

	// PATH should not have duplicates.
	currentPath := os.Getenv("PATH")
	count := 0
	for _, p := range filepath.SplitList(currentPath) {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(targetDir)) {
			count++
		}
	}
	if count > 1 {
		t.Fatalf("expected dir to appear at most once in PATH, got %d occurrences", count)
	}
}

// TestAddToProcessPathAddsToProcessEnv verifies the process-local PATH update
// without mutating the persistent user PATH on Windows.
func TestAddToProcessPathAddsToProcessEnv(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "new-bin-dir")
	original := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", original) })

	// Ensure target is NOT currently in PATH.
	os.Setenv("PATH", strings.ReplaceAll(original, targetDir, ""))

	err := addToProcessPath(targetDir)
	if err != nil {
		t.Fatalf("addToProcessPath returned unexpected error: %v", err)
	}

	// The directory must now be in the process PATH.
	found := false
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(targetDir)) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %q to be present in process PATH after AddToUserPath, got: %s", targetDir, os.Getenv("PATH"))
	}
}

// TestAddToUserPathNoOpOnNonWindows verifies that on non-Windows platforms the
// PowerShell persistence call is skipped (no error, and we can't run powershell).
func TestAddToUserPathNoOpOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping non-Windows no-op test on Windows")
	}

	targetDir := filepath.Join(t.TempDir(), "bin")
	original := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", original) })

	// Remove targetDir from PATH to force the add path.
	os.Setenv("PATH", strings.ReplaceAll(original, targetDir, ""))

	// Must not error even though powershell is unavailable on Linux/macOS.
	err := AddToUserPath(targetDir)
	if err != nil {
		t.Fatalf("AddToUserPath should be a no-op on non-Windows but returned error: %v", err)
	}
}

func TestAddToUserPathUsesProcessPathInGoTests(t *testing.T) {
	if !runningInGoTest() {
		t.Fatal("runningInGoTest() = false in go test binary")
	}

	targetDir := filepath.Join(t.TempDir(), "test-bin")
	original := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", original) })

	os.Setenv("PATH", strings.ReplaceAll(original, targetDir, ""))

	if err := AddToUserPath(targetDir); err != nil {
		t.Fatalf("AddToUserPath() error = %v", err)
	}

	entries := filepath.SplitList(os.Getenv("PATH"))
	if len(entries) == 0 || !strings.EqualFold(filepath.Clean(entries[0]), filepath.Clean(targetDir)) {
		t.Fatalf("PATH first entry = %q, want %q; full PATH=%q", entries, targetDir, os.Getenv("PATH"))
	}
}

func TestPrioritizeUserPathUsesProcessPathInGoTests(t *testing.T) {
	if !runningInGoTest() {
		t.Fatal("runningInGoTest() = false in go test binary")
	}

	firstDir := filepath.Join(t.TempDir(), "first")
	targetDir := filepath.Join(t.TempDir(), "target")
	original := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", original) })

	os.Setenv("PATH", strings.Join([]string{firstDir, targetDir}, string(os.PathListSeparator)))

	if err := PrioritizeUserPath(targetDir); err != nil {
		t.Fatalf("PrioritizeUserPath() error = %v", err)
	}

	entries := filepath.SplitList(os.Getenv("PATH"))
	if len(entries) == 0 || !strings.EqualFold(filepath.Clean(entries[0]), filepath.Clean(targetDir)) {
		t.Fatalf("PATH first entry = %q, want %q; full PATH=%q", entries, targetDir, os.Getenv("PATH"))
	}
}

func TestPrioritizeProcessPathMovesExistingEntryToFront(t *testing.T) {
	firstDir := filepath.Join(t.TempDir(), "first")
	targetDir := filepath.Join(t.TempDir(), "target")
	lastDir := filepath.Join(t.TempDir(), "last")
	original := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", original) })

	os.Setenv("PATH", strings.Join([]string{firstDir, targetDir, lastDir}, string(os.PathListSeparator)))

	if err := prioritizeProcessPath(targetDir); err != nil {
		t.Fatalf("prioritizeProcessPath() error = %v", err)
	}

	entries := filepath.SplitList(os.Getenv("PATH"))
	if len(entries) != 3 {
		t.Fatalf("PATH entries = %v, want three preserved entries", entries)
	}
	if entries[0] != targetDir {
		t.Fatalf("PATH first entry = %q, want %q", entries[0], targetDir)
	}
	if entries[1] != firstDir || entries[2] != lastDir {
		t.Fatalf("PATH should preserve non-target order after target move, got %v", entries)
	}
}

func TestRemoveFromUserPathRemovesOneProcessEntryWithoutRegistryMutation(t *testing.T) {
	firstDir := filepath.Join(t.TempDir(), "first")
	targetDir := filepath.Join(t.TempDir(), "target")
	lastDir := filepath.Join(t.TempDir(), "last")
	original := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", original) })
	os.Setenv("PATH", strings.Join([]string{firstDir, targetDir, targetDir, lastDir}, string(os.PathListSeparator)))

	if err := RemoveFromUserPath(targetDir); err != nil {
		t.Fatalf("RemoveFromUserPath() error = %v", err)
	}
	entries := filepath.SplitList(os.Getenv("PATH"))
	if got, want := strings.Join(entries, ","), strings.Join([]string{firstDir, targetDir, lastDir}, ","); got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}
}

func TestAddToUserPathWindowsEmptyPersistentPathDoesNotWriteTrailingEntry(t *testing.T) {
	targetDir := `C:\gentle-ai\bin`
	t.Setenv("PATH", os.Getenv("PATH"))
	var script string
	useWindowsUserPathSeam(t, userPathRunnerFunc(func(_ context.Context, args ...string) ([]byte, error) {
		script = args[len(args)-1]
		return []byte("changed"), nil
	}))

	if err := AddToUserPath(targetDir); err != nil {
		t.Fatalf("AddToUserPath() error = %v", err)
	}
	if !strings.Contains(script, `$updated = if ($current) { 'C:\gentle-ai\bin;' + $current } else { 'C:\gentle-ai\bin' }`) {
		t.Fatalf("persistent PATH script = %q, want empty path to persist exactly the managed directory", script)
	}
}

func TestAddToUserPathWithResultWindowsOwnership(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "bin")
	otherDir := filepath.Join(t.TempDir(), "tools")

	for _, tt := range []struct {
		name                  string
		processPath           string
		persistentResult      string
		want                  UserPathAddition
		wantRollbackCalls     int
		wantProcessPresent    bool
		wantPersistentPresent bool
	}{
		{
			name:                  "persistent present process absent",
			processPath:           otherDir,
			persistentResult:      "unchanged",
			want:                  UserPathAddition{ProcessAdded: true},
			wantPersistentPresent: true,
		},
		{
			name:              "both absent",
			processPath:       otherDir,
			persistentResult:  "changed",
			want:              UserPathAddition{ProcessAdded: true, PersistentAdded: true},
			wantRollbackCalls: 1,
		},
		{
			name:                  "both present",
			processPath:           strings.Join([]string{targetDir, otherDir}, string(os.PathListSeparator)),
			persistentResult:      "unchanged",
			want:                  UserPathAddition{},
			wantProcessPresent:    true,
			wantPersistentPresent: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", tt.processPath)
			calls := 0
			persistentPresent := tt.persistentResult == "unchanged"
			useWindowsUserPathSeam(t, userPathRunnerFunc(func(_ context.Context, args ...string) ([]byte, error) {
				calls++
				if strings.Contains(args[len(args)-1], `$removed = $false`) {
					persistentPresent = false
					return nil, nil
				}
				persistentPresent = true
				return []byte(tt.persistentResult), nil
			}))

			addition, err := AddToUserPathWithResult(targetDir)
			if err != nil {
				t.Fatalf("AddToUserPathWithResult() error = %v", err)
			}
			if addition != tt.want {
				t.Fatalf("addition = %+v, want %+v", addition, tt.want)
			}
			if err := RollbackUserPathAddition(targetDir, addition); err != nil {
				t.Fatalf("RollbackUserPathAddition() error = %v", err)
			}
			if got, want := calls, 1+tt.wantRollbackCalls; got != want {
				t.Fatalf("PowerShell calls = %d, want %d", got, want)
			}
			if got := processPathContains(targetDir); got != tt.wantProcessPresent {
				t.Fatalf("process PATH contains target = %t, want %t", got, tt.wantProcessPresent)
			}
			if persistentPresent != tt.wantPersistentPresent {
				t.Fatalf("persistent PATH contains target = %t, want %t", persistentPresent, tt.wantPersistentPresent)
			}
		})
	}
}

func TestAddToUserPathWithResultPersistentFailurePreservesPreexistingProcessEntry(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "bin")
	original := strings.Join([]string{targetDir, filepath.Join(t.TempDir(), "tools")}, string(os.PathListSeparator))
	t.Setenv("PATH", original)
	useWindowsUserPathSeam(t, userPathRunnerFunc(func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("persistent PATH unavailable")
	}))

	addition, err := AddToUserPathWithResult(targetDir)
	if err == nil {
		t.Fatal("AddToUserPathWithResult() error = nil, want persistent failure")
	}
	if addition.ProcessAdded || addition.PersistentAdded {
		t.Fatalf("addition = %+v, want no owned mutations", addition)
	}
	if got := os.Getenv("PATH"); got != original {
		t.Fatalf("PATH = %q, want pre-existing process entry preserved as %q", got, original)
	}
}

func TestRemoveFromUserPathWindowsPersistentFailureLeavesProcessPathUnchanged(t *testing.T) {
	firstDir := filepath.Join(t.TempDir(), "first")
	targetDir := filepath.Join(t.TempDir(), "target")
	lastDir := filepath.Join(t.TempDir(), "last")
	original := strings.Join([]string{firstDir, targetDir, lastDir}, string(os.PathListSeparator))
	t.Setenv("PATH", original)
	useWindowsUserPathSeam(t, userPathRunnerFunc(func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("persistent PATH unavailable")
	}))

	if err := RemoveFromUserPath(targetDir); err == nil {
		t.Fatal("RemoveFromUserPath() error = nil, want persistent failure")
	}
	if got := os.Getenv("PATH"); got != original {
		t.Fatalf("PATH = %q, want unchanged %q after persistent failure", got, original)
	}
}

func TestRemoveFromUserPathWindowsPersistsBeforeRemovingProcessEntry(t *testing.T) {
	firstDir := filepath.Join(t.TempDir(), "first")
	targetDir := filepath.Join(t.TempDir(), "target")
	lastDir := filepath.Join(t.TempDir(), "last")
	original := strings.Join([]string{firstDir, targetDir, lastDir}, string(os.PathListSeparator))
	t.Setenv("PATH", original)
	var script string
	useWindowsUserPathSeam(t, userPathRunnerFunc(func(_ context.Context, args ...string) ([]byte, error) {
		if got := os.Getenv("PATH"); got != original {
			t.Fatalf("PATH during persistent removal = %q, want unchanged %q", got, original)
		}
		script = args[len(args)-1]
		return nil, nil
	}))

	if err := RemoveFromUserPath(targetDir); err != nil {
		t.Fatalf("RemoveFromUserPath() error = %v", err)
	}
	if !strings.Contains(script, `$removed = $false`) || !strings.Contains(script, `($entries -join ';')`) {
		t.Fatalf("persistent PATH script = %q, want ordered exact-entry removal", script)
	}
	if got, want := filepath.SplitList(os.Getenv("PATH")), []string{firstDir, lastDir}; !slices.Equal(got, want) {
		t.Fatalf("process PATH entries = %v, want %v after persistent removal", got, want)
	}
}
