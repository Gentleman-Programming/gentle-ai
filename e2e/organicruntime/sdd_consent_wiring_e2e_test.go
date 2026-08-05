package organicruntime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #2563 (S4b of #2540): the built-binary consent loop. A blocked
// multi-repository change's status carries the typed consent envelope; the
// agent executes the envelope's EXACT grant invocation verbatim; the next
// status projects the granted roots into allowedEditRoots and apply becomes
// ready. Declining runs the envelope's decline invocation and the change
// stays blocked under the same instance identity. A single-repository change
// stays byte-identical with zero consent footprint.

type sddConsentStatusE2E struct {
	ApplyState    string `json:"applyState"`
	ActionContext struct {
		AllowedEditRoots []string `json:"allowedEditRoots"`
	} `json:"actionContext"`
	NextRecommended string   `json:"nextRecommended"`
	BlockedReasons  []string `json:"blockedReasons"`
	Consent         *struct {
		Schema       string   `json:"schema"`
		Change       string   `json:"change"`
		MissingRoots []string `json:"missing_roots"`
		Choices      []struct {
			Answer     string `json:"answer"`
			Invocation string `json:"invocation"`
		} `json:"choices"`
	} `json:"consent"`
}

func seedConsentChange(t *testing.T, planning, name, tasks string) {
	t.Helper()
	changeRoot := filepath.Join(planning, "openspec", "changes", name)
	files := map[string]string{
		"proposal.md":        "# Proposal\n",
		"specs/auth/spec.md": "### Requirement: Auth\n#### Scenario: Expected behavior\n",
		"design.md":          "# Design\n",
		"tasks.md":           tasks,
	}
	for relative, content := range files {
		path := filepath.Join(changeRoot, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func initConsentGitRepo(t *testing.T, dir string, commit bool) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := organicGitOutput(context.Background(), dir, "init", "--quiet", "--initial-branch=main", "."); err != nil {
		t.Fatal(err)
	}
	if !commit {
		return
	}
	for _, arguments := range [][]string{
		{"config", "user.email", "consent-e2e@example.com"},
		{"config", "user.name", "Consent E2E"},
	} {
		if _, err := organicGitOutput(context.Background(), dir, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"add", "tracked.txt"},
		{"commit", "--quiet", "-m", "base"},
	} {
		if _, err := organicGitOutput(context.Background(), dir, arguments...); err != nil {
			t.Fatal(err)
		}
	}
}

func consentRealPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func consentStatus(t *testing.T, environment []string, planning, change string) (sddConsentStatusE2E, string) {
	t.Helper()
	stdout, stderr, err := runOrganicCommand(t, organicBinary, planning, environment,
		"sdd-status", change, "--cwd", planning, "--json")
	if err != nil {
		t.Fatalf("sdd-status: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	var status sddConsentStatusE2E
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatalf("decode sdd-status: %v\n%s", err, stdout)
	}
	return status, stdout
}

func consentInstanceToken(t *testing.T, invocation string) string {
	t.Helper()
	fields := strings.Fields(invocation)
	for index, field := range fields {
		if field == "--change-instance" && index+1 < len(fields) {
			return fields[index+1]
		}
	}
	t.Fatalf("grant invocation carries no --change-instance token: %q", invocation)
	return ""
}

// consentShellEnvironment prepends a directory holding a `gentle-ai` shim for
// the built binary, so the envelope's invocation runs VERBATIM through a
// POSIX shell exactly as the agent would run it (the invocation's paths are
// pathquote-quoted shell tokens, not pre-split argv).
func consentShellEnvironment(t *testing.T, home string) []string {
	t.Helper()
	binDir := t.TempDir()
	if err := os.Symlink(organicBinary, filepath.Join(binDir, "gentle-ai")); err != nil {
		t.Fatal(err)
	}
	environment := organicEnvironment(home)
	for index, entry := range environment {
		if strings.HasPrefix(entry, "PATH=") {
			environment[index] = "PATH=" + binDir + string(os.PathListSeparator) + strings.TrimPrefix(entry, "PATH=")
		}
	}
	return environment
}

func runConsentInvocation(t *testing.T, environment []string, dir, invocation string) {
	t.Helper()
	if !strings.HasPrefix(invocation, "gentle-ai ") {
		t.Fatalf("invocation is not a gentle-ai command: %q", invocation)
	}
	if stdout, stderr, err := runOrganicCommand(t, "sh", dir, environment, "-c", invocation); err != nil {
		t.Fatalf("invocation %q failed: %v\nstdout:\n%s\nstderr:\n%s", invocation, err, stdout, stderr)
	}
}

func TestSDDEditAuthorityConsentGrantLoop(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir() // the workspace itself is NOT a Git repository
	home := t.TempDir()
	environment := consentShellEnvironment(t, home)
	planning := filepath.Join(workspace, "planning")
	serviceA := filepath.Join(workspace, "service-a")
	serviceB := filepath.Join(workspace, "service-b")
	initConsentGitRepo(t, planning, true)
	initConsentGitRepo(t, serviceA, false)
	initConsentGitRepo(t, serviceB, false)
	planning = consentRealPath(t, planning)
	const change = "multi-repo-rollout"
	seedConsentChange(t, planning, change, strings.Join([]string{
		"- [ ] 1.1 Update `../service-a/internal/api/handler.go` to accept the new header",
		"- [ ] 1.2 Update `../service-b/internal/worker/consume.go` to forward the header",
		"",
	}, "\n"))

	// Blocked status carries the envelope with the exact grant invocation.
	blocked, blockedPayload := consentStatus(t, environment, planning, change)
	if blocked.ApplyState != "blocked" ||
		!strings.Contains(strings.Join(blocked.BlockedReasons, "\n"), "blocked(edit_authority_missing)") {
		t.Fatalf("fixture is not blocked on edit authority: %s", blockedPayload)
	}
	if blocked.Consent == nil {
		t.Fatalf("blocked(edit_authority_missing) status carries no consent envelope: %s", blockedPayload)
	}
	if blocked.Consent.Schema != "gentle-ai.sdd-integration.consent/v1" || blocked.Consent.Change != change ||
		len(blocked.Consent.Choices) != 2 || blocked.Consent.Choices[0].Answer != "granted" ||
		blocked.Consent.Choices[1].Answer != "declined" {
		t.Fatalf("consent envelope identity is wrong: %s", blockedPayload)
	}
	wantA := consentRealPath(t, serviceA)
	wantB := consentRealPath(t, serviceB)
	if len(blocked.Consent.MissingRoots) != 2 || blocked.Consent.MissingRoots[0] != wantA || blocked.Consent.MissingRoots[1] != wantB {
		t.Fatalf("consent envelope missing_roots = %v, want [%s %s]", blocked.Consent.MissingRoots, wantA, wantB)
	}
	grantInvocation := blocked.Consent.Choices[0].Invocation
	token := consentInstanceToken(t, grantInvocation)

	// Decline path: the human says no, the agent runs the envelope's decline
	// invocation (native status re-entry). Nothing is persisted; the change
	// stays blocked under the SAME instance identity.
	runConsentInvocation(t, environment, planning, blocked.Consent.Choices[1].Invocation)
	declined, declinedPayload := consentStatus(t, environment, planning, change)
	if declined.ApplyState != "blocked" || declined.Consent == nil {
		t.Fatalf("declined change did not stay blocked with the envelope: %s", declinedPayload)
	}
	if declinedToken := consentInstanceToken(t, declined.Consent.Choices[0].Invocation); declinedToken != token {
		t.Fatalf("decline minted a rival instance token: %q then %q", token, declinedToken)
	}

	// Grant path: execute the envelope's EXACT invocation from the built
	// binary, verbatim.
	runConsentInvocation(t, environment, planning, grantInvocation)

	// Post-grant status: the covering grant clears detection, apply is ready,
	// and allowedEditRoots = planning + granted roots.
	granted, grantedPayload := consentStatus(t, environment, planning, change)
	if granted.ApplyState != "ready" || granted.NextRecommended != "apply" {
		t.Fatalf("post-grant status is not ready/apply: %s", grantedPayload)
	}
	if strings.Contains(strings.Join(granted.BlockedReasons, "\n"), "edit_authority_missing") {
		t.Fatalf("post-grant status still blocked on edit authority: %s", grantedPayload)
	}
	wantRoots := []string{planning, wantA, wantB}
	if len(granted.ActionContext.AllowedEditRoots) != len(wantRoots) {
		t.Fatalf("allowedEditRoots = %v, want %v", granted.ActionContext.AllowedEditRoots, wantRoots)
	}
	for index, want := range wantRoots {
		if granted.ActionContext.AllowedEditRoots[index] != want {
			t.Fatalf("allowedEditRoots = %v, want %v", granted.ActionContext.AllowedEditRoots, wantRoots)
		}
	}
	if granted.Consent != nil {
		t.Fatalf("post-grant status still carries a consent envelope: %s", grantedPayload)
	}
}

func TestSDDSingleRepoStatusStaysByteIdenticalWithZeroConsentFootprint(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	environment := organicEnvironment(home)
	planning := filepath.Join(t.TempDir(), "planning")
	initConsentGitRepo(t, planning, true)
	planning = consentRealPath(t, planning)
	const change = "single-repo-change"
	seedConsentChange(t, planning, change, strings.Join([]string{
		"- [ ] 1.1 Update `internal/auth/login.go` with the new claim check",
		"- [ ] 1.2 Run `go test ./...` and fix regressions",
		"",
	}, "\n"))

	_, first := consentStatus(t, environment, planning, change)
	_, second := consentStatus(t, environment, planning, change)
	if first != second {
		t.Fatalf("single-repo status is not byte-identical across runs:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	if strings.Contains(first, "\"consent\"") || strings.Contains(first, "edit_authority_missing") {
		t.Fatalf("single-repo status carries a consent footprint: %s", first)
	}
	if _, err := os.Lstat(filepath.Join(planning, "openspec", "changes", change, ".gentle-ai-instance")); !os.IsNotExist(err) {
		t.Fatalf("single-repo status minted an instance marker: %v", err)
	}
}
