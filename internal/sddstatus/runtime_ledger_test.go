package sddstatus

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRuntimeLedgerRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runRuntimeLedgerGit(t, repo, "init", "-q")
	runRuntimeLedgerGit(t, repo, "config", "user.email", "runtime-ledger@example.com")
	runRuntimeLedgerGit(t, repo, "config", "user.name", "Runtime Ledger Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "add", "tracked.txt")
	runRuntimeLedgerGit(t, repo, "commit", "-qm", "base")
	return repo
}

func appendRuntimeLedgerFile(t *testing.T, repo, content string) {
	t.Helper()
	file, err := os.OpenFile(filepath.Join(repo, "tracked.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func runRuntimeLedgerGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repo
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func countRuntimeRecords(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "records"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}

func runtimeTestHash(char byte) string {
	return "sha256:" + strings.Repeat(string(char), 64)
}

func isRuntimeRecordRejection(err error, condition string) bool {
	var rejected *RuntimeRecordRejectedError
	return errors.As(err, &rejected) && rejected.Condition == condition
}
