package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewStatusSelectorFreeResolvesRepositoryRoot(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	nested := filepath.Join(repo, "nested", "path")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	status := func(t *testing.T, cwd string) string {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview([]string{"status", "--cwd", cwd}, &output); err != nil {
			t.Fatalf("review status --cwd %q: %v", cwd, err)
		}
		var report struct {
			Entries []struct {
				LineageID string `json:"lineage_id"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if len(report.Entries) != 1 || report.Entries[0].LineageID != started.LineageID {
			t.Fatalf("review status --cwd %q entries = %#v, want lineage %q", cwd, report.Entries, started.LineageID)
		}
		return output.String()
	}

	want := status(t, repo)
	if got := status(t, nested); got != want {
		t.Fatalf("nested review status differs from repository root:\nroot:   %s\nnested: %s", want, got)
	}

	t.Run("safe symlink", func(t *testing.T) {
		symlink := filepath.Join(t.TempDir(), "repo-link")
		if err := os.Symlink(repo, symlink); err != nil {
			if errors.Is(err, os.ErrPermission) {
				t.Skipf("creating a directory symlink is unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if got := status(t, symlink); got != want {
			t.Fatalf("symlink review status differs from repository root:\nroot:    %s\nsymlink: %s", want, got)
		}
	})

	if err := RunReview([]string{"status", "--cwd", t.TempDir()}, &bytes.Buffer{}); err == nil {
		t.Fatal("review status accepted a path outside a repository")
	}

	nestedRepository := filepath.Join(repo, "nested-repository")
	if err := os.Mkdir(nestedRepository, 0o755); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, nestedRepository, "init", "-q")
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", nestedRepository}, &output); err != nil {
		t.Fatalf("review status in nested independent repository: %v", err)
	}
	var nestedReport struct {
		Entries []struct {
			LineageID string `json:"lineage_id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output.Bytes(), &nestedReport); err != nil {
		t.Fatal(err)
	}
	if len(nestedReport.Entries) != 0 {
		t.Fatalf("nested independent repository inherited parent authority entries: %#v", nestedReport.Entries)
	}
}
