package cli

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestRunSDDAttemptGrantPersistsAndReplaysThroughTheCLI(t *testing.T) {
	repo := initReviewCLIRepo(t)
	change := "cli-grant"
	sibling, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seedSDDStatusReadyChange(t, repo, change, "- [ ] 1.1 Update `"+filepath.Join(sibling, "main.go")+"`\n")
	var continued bytes.Buffer
	if err := RunSDDContinue([]string{change, "--cwd", repo, "--json"}, &continued); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(repo, "openspec", "changes", change, ".gentle-ai-instance"))
	if err != nil {
		t.Fatal(err)
	}
	instance := strings.TrimSpace(string(marker))

	grantArgs := []string{
		"grant", "--cwd", repo, "--change", change, "--root", sibling, "--change-instance", instance,
		"--actor", "maintainer", "--reason", "sequential multi-repository rollout", "--request-id", "cli-grant-1",
	}
	granted := runSDDAttemptStatus(t, grantArgs)
	if len(granted.GrantedRoots) != 1 || granted.GrantedRoots[0] != sibling || granted.Revision == "" {
		t.Fatalf("grant CLI status = %#v, want granted root %q with a committed revision", granted, sibling)
	}

	// Read through SDD status, not a retired attempt-status verb.
	var statusOutput bytes.Buffer
	if err := RunSDDStatus([]string{change, "--cwd", repo, "--json"}, &statusOutput); err != nil {
		t.Fatal(err)
	}
	// Decode instead of substring-matching: JSON escapes the backslashes of a
	// Windows path, so the raw path never appears verbatim in the output.
	var projected sddstatus.StatusV2Projection
	if err := json.Unmarshal(statusOutput.Bytes(), &projected); err != nil {
		t.Fatalf("decode SDD status: %v\n%s", err, statusOutput.String())
	}
	if !containsString(projected.ActionContext.AllowedEditRoots, sibling) {
		t.Fatalf("granted root %q absent from SDD status allowedEditRoots %#v", sibling, projected.ActionContext.AllowedEditRoots)
	}
	// An exact duplicate request-id is idempotent through the CLI: same
	// committed revision, no second record.
	replayed := runSDDAttemptStatus(t, grantArgs)
	if replayed.Revision != granted.Revision || !reflect.DeepEqual(replayed.GrantedRoots, []string{sibling}) {
		t.Fatalf("grant CLI replay = %#v, want committed revision %s", replayed, granted.Revision)
	}

	// A widening grant reusing the SAME instance token chains on the first
	// revision, accumulates the new root after the already-granted one, and
	// deduplicates the repeat.
	second, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	accumulated := runSDDAttemptStatus(t, []string{
		"grant", "--cwd", repo, "--change", change, "--expected-revision", granted.Revision,
		"--root", second, "--root", sibling, "--change-instance", instance,
		"--actor", "maintainer", "--reason", "maintainer widened the change to a second sibling", "--request-id", "cli-grant-2",
	})
	if !reflect.DeepEqual(accumulated.GrantedRoots, []string{sibling, second}) {
		t.Fatalf("accumulated CLI granted roots = %#v, want [%q %q]", accumulated.GrantedRoots, sibling, second)
	}
}

func runSDDAttemptStatus(t *testing.T, args []string) sddstatus.RuntimeStatus {
	t.Helper()
	var output bytes.Buffer
	if err := RunSDDAttempt(args, &output); err != nil {
		t.Fatalf("RunSDDAttempt(%v): %v", args, err)
	}
	var status sddstatus.RuntimeStatus
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil {
		t.Fatalf("decode SDD attempt status: %v\n%s", err, output.String())
	}
	if !bytes.HasSuffix(output.Bytes(), []byte("\n")) {
		t.Fatalf("SDD attempt JSON lacks trailing newline: %q", output.Bytes())
	}
	return status
}

func cliAttemptHash(char byte) string {
	return "sha256:" + strings.Repeat(string(char), 64)
}

func snapshotRuntimeAuthorityFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[relative] = string(payload)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return snapshot
}
