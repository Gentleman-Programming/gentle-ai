package sddstatus

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
)

// Issue #2547 (S1 of #2540): work units carry no structured target field, so
// the only honest signal that a task plan names an edit path outside its
// authorized roots is the prose itself. Detection is deliberately
// conservative: it inspects only backticked tokens inside markdown checkbox
// lines, and it flags a token only when it resolves to a path in a Git
// repository outside every authorized edit root. Different repositories are
// represented by their Git roots; targets inside the planning repository are
// narrowed to their containing edit roots. It catches the reported scenario
// (explicit `../sibling/...` and absolute paths); it cannot catch pure prose
// ("update the billing service"), and a context reference can raise a false
// block — acceptable because the consequence is an honest blocked status
// naming its exits, never silent authority.
//
// This derivation deliberately lives outside the #2515 runtime-readiness
// triple (RuntimeStatus.Complete/DecisionRequired/ActiveAttempt): edit
// authority is a property of the task plan, not of runtime execution, so
// TestOneReadinessPredicateHasNoRivalDerivations stays green by design.

var backtickedSpan = regexp.MustCompile("`([^`]+)`")

// detectUnauthorizedEditRoots scans tasks text (both status paths have text;
// the Engram store has no tasks.md path) for path-like tokens in checkbox
// lines, resolves each against workspaceRoot to its nearest existing
// ancestor, and reports every resolved edit root outside allowedEditRoots.
// Targets in a different Git repository are represented by that repository's
// Git root; targets sharing the planning repository are narrowed by
// sameRepositoryEditRoot. allowedEditRoots is a parameter so a later slice can
// extend it with persisted per-change grants without touching detection
// (#2540 S2-S4).
func detectUnauthorizedEditRoots(tasksText string, workspaceRoot string, allowedEditRoots []string) []string {
	planningGitRoot := gitRootOf(resolveExistingPath(workspaceRoot))
	allowed := make([]string, 0, len(allowedEditRoots))
	for _, root := range allowedEditRoots {
		root = filepath.Clean(root)
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
		allowed = append(allowed, root)
	}

	unauthorized := map[string]bool{}
	for _, line := range strings.Split(tasksText, "\n") {
		if len(taskCheckbox.FindStringSubmatch(line)) == 0 {
			continue
		}
		for _, token := range pathLikeTokens(line) {
			resolved := token
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(workspaceRoot, resolved)
			}
			resolved = resolveExistingPath(filepath.Clean(resolved))
			target := gitRootOf(resolved)
			if target == "" {
				continue
			}
			missing := target
			if target == planningGitRoot {
				missing = sameRepositoryEditRoot(resolved)
			}
			if withinAnyRoot(missing, allowed) {
				continue
			}
			unauthorized[missing] = true
		}
	}

	roots := make([]string, 0, len(unauthorized))
	for root := range unauthorized {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

// sameRepositoryEditRoot narrows a target inside the planning repository to
// the smallest existing canonical directory the grant ledger can persist. A
// different repository remains represented by its Git root above.
func sameRepositoryEditRoot(path string) string {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return filepath.Dir(path)
	}
	return path
}

// pathLikeTokens extracts the conservative candidate set from one checkbox
// line: backticked tokens that contain a path separator (which subsumes
// `../` prefixes and absolute paths). Tokens with whitespace are commands or
// prose, and URL-like tokens are references, not filesystem targets.
func pathLikeTokens(line string) []string {
	var tokens []string
	for _, match := range backtickedSpan.FindAllStringSubmatch(line, -1) {
		token := match[1]
		if strings.ContainsAny(token, " \t") || strings.Contains(token, "://") {
			continue
		}
		if !strings.ContainsRune(token, '/') && !strings.ContainsRune(token, filepath.Separator) {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// resolveExistingPath walks up to the nearest existing ancestor (task prose
// routinely names files that do not exist yet, like `../service-a/...`) and
// then evaluates symlinks so root comparisons happen on the paths the
// filesystem knows.
func resolveExistingPath(path string) string {
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	if resolved, err := filepath.EvalSymlinks(current); err == nil {
		return resolved
	}
	return current
}

// gitRootOf walks up from path to the nearest directory containing a `.git`
// entry (a directory for ordinary repositories, a file for worktrees). An
// empty result means the path belongs to no repository and can never be an
// unauthorized edit target.
func gitRootOf(path string) string {
	current := path
	if info, err := os.Stat(current); err != nil || !info.IsDir() {
		current = filepath.Dir(current)
	}
	for {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func withinAnyRoot(target string, roots []string) bool {
	for _, root := range roots {
		if target == root || strings.HasPrefix(target, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// editAuthorityBlockedReason names each unauthorized edit root and both exits:
// keep the plan inside the authorized edit roots, or grant authority for the
// named paths (the grant command is a later slice of #2540).
func editAuthorityBlockedReason(roots []string) string {
	quoted := make([]string, 0, len(roots))
	for _, root := range roots {
		quoted = append(quoted, pathquote.Quote(root))
	}
	return fmt.Sprintf(
		"blocked(edit_authority_missing): tasks.md targets edit paths outside the authorized edit roots: %s; edit tasks.md so every work unit stays inside the authorized edit roots, or grant this change edit authority for the named paths",
		strings.Join(quoted, ", "),
	)
}

// applyEditAuthorityBlock forces applyState blocked before dependencies and
// nextRecommended derive from it, so a plan whose work units the sdd-apply
// outside-root guard would refuse never reports ready/apply. It runs only
// when apply would otherwise be ready: completed work needs no forward edit
// authority, and planning-blocked changes already carry their own reasons.
// It also returns the unauthorized edit roots so the caller can raise the typed
// consent question naming exactly them (#2563, S4b of #2540).
func applyEditAuthorityBlock(applyState ApplyState, reasons *blockerReasons, tasksText string, workspaceRoot string, allowedEditRoots []string) (ApplyState, []string) {
	if applyState != ApplyReady {
		return applyState, nil
	}
	roots := detectUnauthorizedEditRoots(tasksText, workspaceRoot, allowedEditRoots)
	if len(roots) == 0 {
		return applyState, nil
	}
	reasons.genuine = append(reasons.genuine, editAuthorityBlockedReason(roots))
	return ApplyBlocked, roots
}
