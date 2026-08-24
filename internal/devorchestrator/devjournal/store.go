package devjournal

import (
	"os"
	"path/filepath"
	"strings"
)

// journalPath builds the per-change journal file path under root (design D1
// layout: <root>/<change>/journal.json).
func journalPath(root, change string) string {
	return filepath.Join(root, change, "journal.json")
}

// resolveJournalRoot resolves the directory holding every change's journal:
//
//   - <git-common-dir>/gentle-ai/dev-orchestrator/v1 when workspaceRoot has a
//     discoverable Git common directory, or
//   - <workspaceRoot>/.gentle-ai/dev-orchestrator/v1 otherwise (usesFallback
//     is true).
//
// This is a pure-os helper mirroring internal/sddstatus/status.go's
// gitConfigPathFor (`.git` is a directory for an ordinary checkout, or a file
// holding a `gitdir:` pointer plus `commondir` for a linked worktree), and is
// deliberately independent of reviewtransaction.CompactAuthoritativeStore:
// devjournal must not depend on reviewtransaction or pathquote (design D1).
// It never errors — an unresolvable Git structure just means "fall back".
func resolveJournalRoot(workspaceRoot string) (root string, usesFallback bool, err error) {
	if commonDir, ok := gitCommonDir(workspaceRoot); ok {
		return filepath.Join(commonDir, "gentle-ai", "dev-orchestrator", "v1"), false, nil
	}
	return filepath.Join(workspaceRoot, ".gentle-ai", "dev-orchestrator", "v1"), true, nil
}

// gitCommonDir resolves the Git common directory for workspaceRoot. ok is
// false when workspaceRoot has no discoverable `.git` entry at all, in which
// case the caller falls back to a workspace-local directory.
func gitCommonDir(workspaceRoot string) (commonDir string, ok bool) {
	gitEntry := filepath.Join(workspaceRoot, ".git")
	info, err := os.Stat(gitEntry)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return gitEntry, true
	}

	pointer, err := os.ReadFile(gitEntry)
	if err != nil {
		return "", false
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(pointer)), "gitdir:"))
	if gitDir == "" {
		return "", false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workspaceRoot, gitDir)
	}

	// A linked worktree records the shared directory in `commondir`, relative
	// to its own gitdir. Without it the gitdir already is the common dir.
	resolved := gitDir
	if content, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		if trimmed := strings.TrimSpace(string(content)); trimmed != "" {
			resolved = trimmed
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(gitDir, resolved)
			}
		}
	}
	return resolved, true
}
