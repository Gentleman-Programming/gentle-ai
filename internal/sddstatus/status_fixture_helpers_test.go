package sddstatus

import (
	"os"
	"path/filepath"
	"testing"
)

func seedReadyChange(t *testing.T, root string, name string, tasks string) string {
	t.Helper()
	changeRoot := filepath.Join(root, "openspec", "changes", name)
	write(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Expected behavior\n")
	write(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	write(t, filepath.Join(changeRoot, "tasks.md"), tasks)
	return changeRoot
}

func engramPlanningRoute(name string, route string) []engramObservation {
	observations := []engramObservation{}
	if route != "propose" {
		observations = append(observations, engramObservation{Title: "sdd/" + name + "/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"})
	}
	if route == "design" || route == "tasks" {
		observations = append(observations, engramObservation{Title: "sdd/" + name + "/spec", Content: "# Spec\n", Project: "gentle-ai", Scope: "project"})
	}
	if route == "tasks" {
		observations = append(observations, engramObservation{Title: "sdd/" + name + "/design", Content: "# Design\n", Project: "gentle-ai", Scope: "project"})
	}
	if route == "propose" {
		observations = append(observations, engramObservation{Title: "sdd/" + name + "/tasks", Content: "- [ ] 1.1 Work\n", Project: "gentle-ai", Scope: "project"})
	}
	return observations
}

func write(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stubEngramExport(t *testing.T, observations []engramObservation) func() {
	t.Helper()
	original := engramExport
	engramExport = func(_ string) ([]engramObservation, error) { return observations, nil }
	return func() { engramExport = original }
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func ptrValue(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func aliasedRepository(t *testing.T, change string) (string, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realRepo := filepath.Join(base, "real", "repo")
	if err := os.MkdirAll(realRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, realRepo, "init", "-q")
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	seedReadyChange(t, realRepo, change, "- [x] 1.1 Done\n")
	return realRepo, filepath.Join(base, "alias", "repo")
}
