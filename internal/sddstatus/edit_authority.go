package sddstatus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
)

// Issue #2547 (S1 of #2540): work units carry no structured target field, so
// the only honest signal that a task plan names an edit path outside its
// authorized roots is the prose itself. Detection is deliberately
// conservative: it inspects only backticked tokens inside markdown checkbox
// lines, and it flags a token only when it resolves to a path outside every
// authorized edit root. Different repositories are represented by their Git
// roots; targets inside the planning repository are narrowed to their
// containing edit roots; a target in no Git repository at all (#3504) is
// represented by its resolved directory. It catches the reported scenario
// (explicit `../sibling/...` and absolute paths); it cannot catch pure prose
// ("update the billing service"), and a context reference can raise a false
// block — acceptable because the consequence is an honest blocked status
// naming its exits, never silent authority. The one deterministic exit for a
// genuine read-only reference (#2934) is the `(read-only)` marker on the
// backticked path, which editTargetTokens honors per token without any prose
// inference.
//
// This derivation deliberately lives outside the #2515 runtime-readiness
// triple (RuntimeStatus.Complete/DecisionRequired/ActiveAttempt): edit
// authority is a property of the task plan, not of runtime execution, so
// TestOneReadinessPredicateHasNoRivalDerivations stays green by design.

var backtickedSpan = regexp.MustCompile("`([^`]+)`")

// readOnlyMarkerAfterToken is the one documented spelling of the read-only
// exit (#2934): `(read-only)` immediately after a backticked path, matched
// case-insensitively. It is token-scoped on purpose: a line that mixes a
// marked input with an unmarked path keeps the unmarked path as an edit
// target, so a marker anywhere on the line can never silence the consent
// gate for a target it does not annotate.
var readOnlyMarkerAfterToken = regexp.MustCompile(`(?i)^\s*\(read-only\)`)

// explicitPathContextMarker recognizes a line-level declared-path context
// (`Files:`, `Edit:`, `Touch:`) that authorizes an otherwise-inconclusive
// backticked token (no extension, does not yet exist) as an edit target.
// It is deliberately line-scoped, not token-scoped: an explicit marker names
// the whole line as a path declaration.
var explicitPathContextMarker = regexp.MustCompile(`(?i)\b(?:files?|edits?|touch(?:es)?)\s*:`)

// editTargetTokens is the one derivation of "which tokens on this line are
// edit targets": none when the line is not a checkbox, otherwise every
// path-like backticked token that is not itself annotated `(read-only)`.
// Both the edit-authority detector and the runtime-topology guard route
// through it so a read-only input never blocks either, and an unmarked
// sibling on the same line still does. workspaceRoot lets pathLikeToken tell
// a genuine repository path from prose that merely contains a slash (#4096,
// #4192, gp#598): a candidate must resolve to an existing path, look like a
// plausible new file, or sit in an explicit Files:/Edit:/Touch: declaration.
func editTargetTokens(line string, workspaceRoot string) []string {
	if taskCheckbox.FindStringIndex(line) == nil {
		return nil
	}
	explicitContext := explicitPathContextMarker.MatchString(line)
	var tokens []string
	for _, span := range backtickedSpan.FindAllStringSubmatchIndex(line, -1) {
		token := line[span[2]:span[3]]
		if !pathLikeToken(token, workspaceRoot, explicitContext) || readOnlyMarkerAfterToken.MatchString(line[span[1]:]) {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

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
		for _, token := range editTargetTokens(line, workspaceRoot) {
			resolved := token
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(workspaceRoot, resolved)
			}
			resolved = resolveExistingPath(filepath.Clean(resolved))
			target := gitRootOf(resolved)
			missing := target
			// #3504: a path in no Git repository is still outside every
			// allowed root when it is; name its resolved directory.
			if target == "" || target == planningGitRoot {
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

// pathLikeToken reports whether one backticked token reads as a genuine
// filesystem edit path rather than prose that merely contains a slash.
//
// Whitespace or a URL scheme always means a command or prose, never a path,
// and a token without any separator is never a path either. Past that, the
// affirmative signals run FIRST: an explicit Files:/Edit:/Touch: declaration
// on the line, or the token (or its parent directory) already existing under
// workspaceRoot, settle the question outright — a real target must never be
// dropped by a shape-based exclusion just because it also happens to look
// route-ish or glob-ish (a real absolute directory with no extension, or an
// explicitly declared wildcard, still counts). Only when neither affirmative
// signal holds do the two shape-based exclusions apply: a glob wildcard
// (`*`, `?`, or an unbalanced/character-class `[...]`) reads as an
// import-specifier or ignore glob, not a literal path (#4096's
// `!../../i18n/*`) — except a `[...]` that is a clean, whole directory
// segment of an otherwise extensioned path is a framework dynamic-route
// name (`src/app/[id]/page.tsx`), not a glob; and a leading `/` with no file
// extension reads as a URL route, not an absolute filesystem path (#4192's
// bare `/`, gp#598's `/api/health`).
func pathLikeToken(token string, workspaceRoot string, explicitContext bool) bool {
	if strings.ContainsAny(token, " \t") || strings.Contains(token, "://") {
		return false
	}
	if !strings.ContainsRune(token, '/') && !strings.ContainsRune(token, filepath.Separator) {
		return false
	}
	if explicitContext || candidatePathExists(token, workspaceRoot) {
		return true
	}
	if looksLikeGlobPattern(token) {
		return false
	}
	if strings.HasPrefix(token, "/") && filepath.Ext(token) == "" {
		return false
	}
	return filepath.Ext(token) != ""
}

// bracketedPathSegment matches a whole path segment that is nothing but a
// `[...]` run (no nested slash or bracket) — a framework dynamic-route
// directory name, e.g. `[id]` or `[...slug]`, not a glob character class.
var bracketedPathSegment = regexp.MustCompile(`^\[[^/\[\]]+\]$`)

// looksLikeGlobPattern reports whether token carries a wildcard that reads as
// an import-specifier or ignore glob rather than a literal path. `*` and `?`
// are unconditional glob signals. A `[`/`]` run only counts when it is not a
// clean, whole bracketed directory segment of an otherwise extensioned path —
// which rules out real dynamic-route files like `src/app/[id]/page.tsx`.
func looksLikeGlobPattern(token string) bool {
	if strings.ContainsAny(token, "*?") {
		return true
	}
	if !strings.ContainsAny(token, "[]") {
		return false
	}
	if filepath.Ext(token) == "" {
		return true
	}
	for _, segment := range strings.Split(token, "/") {
		if strings.ContainsAny(segment, "[]") && !bracketedPathSegment.MatchString(segment) {
			return true
		}
	}
	return false
}

// candidatePathExists reports whether token, resolved against workspaceRoot
// when relative, already exists on disk — either the path itself or its
// containing directory (a plausible new file location). The bare filesystem
// root never counts: every absolute token trivially resolves under it, so
// treating it as "existing" would readmit #4192's bare `/` route literal.
func candidatePathExists(token string, workspaceRoot string) bool {
	resolved := token
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(workspaceRoot, resolved)
	}
	resolved = filepath.Clean(resolved)
	if resolved == string(filepath.Separator) {
		return false
	}
	if _, err := os.Lstat(resolved); err == nil {
		return true
	}
	info, err := os.Stat(filepath.Dir(resolved))
	return err == nil && info.IsDir()
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
// empty result means the path belongs to no repository; the edit-authority
// detector then names the resolved directory itself (#3504), while the
// runtime-topology guard has no common directory to compare and skips it.
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

// editAuthorityBlockedReason names each unauthorized edit root and the three
// exits: keep the plan inside the authorized edit roots, grant authority for
// the named paths, or mark a genuine read-only input with `(read-only)`.
func editAuthorityBlockedReason(roots []string) string {
	quoted := make([]string, 0, len(roots))
	for _, root := range roots {
		quoted = append(quoted, pathquote.Quote(root))
	}
	return fmt.Sprintf(
		"blocked(edit_authority_missing): tasks.md targets edit paths outside the authorized edit roots: %s; edit tasks.md so every work unit stays inside the authorized edit roots, or grant this change edit authority for the named paths, or mark a read-only input with (read-only) right after its backticked path",
		strings.Join(quoted, ", "),
	)
}

// taskWorkUnitPrefix captures a checkbox line's own leading task number (the
// whole-number part before an optional `.sub` suffix, e.g. "1" from "1.1" or
// from a flat "2."), so lines sharing a phase number are one work unit.
var taskWorkUnitPrefix = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+\[[ xX]\]\s*(\d+)(?:\.\d+)*\b`)

// taskWorkUnitKey extracts a checkbox line's leading work-unit number. ok is
// false when the line carries no leading number, and the caller then
// conservatively keeps that line in scope rather than guessing.
func taskWorkUnitKey(line string) (unit int, ok bool) {
	match := taskWorkUnitPrefix.FindStringSubmatch(line)
	if match == nil {
		return 0, false
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// detectUnauthorizedEditRootsForCurrentWorkUnit scopes detection to the
// current work unit (#4103): a later work unit's own external targets must
// not block a distinct, currently-actionable work unit. "Current" is the
// lowest-numbered work unit that still has a pending checkbox; completed
// tasks are always in scope (their edits already happened), and a line whose
// task number cannot be parsed is conservatively kept in scope — matching
// detectUnauthorizedEditRoots's own whole-plan behavior when no work-unit
// structure can be found at all. It returns the in-scope (blocking) roots
// separately from the future work units' own roots, which are informational
// only.
func detectUnauthorizedEditRootsForCurrentWorkUnit(tasksText, workspaceRoot string, allowedEditRoots []string) (blocking []string, future []string) {
	lines := strings.Split(tasksText, "\n")

	currentUnit, hasCurrent := 0, false
	for _, line := range lines {
		state := taskCheckbox.FindStringSubmatch(line)
		if state == nil || state[1] != " " {
			continue
		}
		unit, ok := taskWorkUnitKey(line)
		if !ok {
			continue
		}
		if !hasCurrent || unit < currentUnit {
			currentUnit, hasCurrent = unit, true
		}
	}

	var inScope, outOfScope []string
	for _, line := range lines {
		state := taskCheckbox.FindStringSubmatch(line)
		belongsToLaterUnit := false
		if hasCurrent && state != nil && state[1] == " " {
			if unit, ok := taskWorkUnitKey(line); ok && unit != currentUnit {
				belongsToLaterUnit = true
			}
		}
		if belongsToLaterUnit {
			outOfScope = append(outOfScope, line)
		} else {
			inScope = append(inScope, line)
		}
	}

	blocking = detectUnauthorizedEditRoots(strings.Join(inScope, "\n"), workspaceRoot, allowedEditRoots)
	if len(outOfScope) == 0 {
		return blocking, nil
	}
	blockingSet := make(map[string]bool, len(blocking))
	for _, root := range blocking {
		blockingSet[root] = true
	}
	for _, root := range detectUnauthorizedEditRoots(strings.Join(outOfScope, "\n"), workspaceRoot, allowedEditRoots) {
		if !blockingSet[root] {
			future = append(future, root)
		}
	}
	return blocking, future
}

// futureEditRootsNote reports a later work unit's own unauthorized edit
// targets without blocking the current work unit (#4103). It carries no
// `blocked(...)` reason code, so it never flips applyState — it exists purely
// so a human sees the future authority need before reaching that work unit.
func futureEditRootsNote(roots []string) string {
	quoted := make([]string, 0, len(roots))
	for _, root := range roots {
		quoted = append(quoted, pathquote.Quote(root))
	}
	return fmt.Sprintf(
		"note(future_edit_roots): a later work unit in tasks.md targets edit paths outside the authorized edit roots: %s; this does not block the current work unit and needs no action until that work unit is reached",
		strings.Join(quoted, ", "),
	)
}

// applyEditAuthorityBlock forces applyState blocked before dependencies and
// nextRecommended derive from it, so a plan whose work units the sdd-apply
// outside-root guard would refuse never reports ready/apply. It runs only
// when apply would otherwise be ready: completed work needs no forward edit
// authority, and planning-blocked changes already carry their own reasons.
// It also returns the unauthorized edit roots so the caller can raise the typed
// consent question naming exactly them (#2563, S4b of #2540). Detection is
// scoped to the current work unit (#4103): a future work unit's own
// unauthorized roots are reported through reasons as an informational note,
// never as a blocker.
func applyEditAuthorityBlock(applyState ApplyState, reasons *blockerReasons, tasksText string, workspaceRoot string, allowedEditRoots []string) (ApplyState, []string) {
	if applyState != ApplyReady {
		return applyState, nil
	}
	roots, futureRoots := detectUnauthorizedEditRootsForCurrentWorkUnit(tasksText, workspaceRoot, allowedEditRoots)
	if len(futureRoots) != 0 {
		reasons.genuine = append(reasons.genuine, futureEditRootsNote(futureRoots))
	}
	if len(roots) == 0 {
		return applyState, nil
	}
	reasons.genuine = append(reasons.genuine, editAuthorityBlockedReason(roots))
	return ApplyBlocked, roots
}

// applyRuntimeTopologyBlock stops only a currently routed runtime actor when a
// task target belongs to a different Git common directory. Edit authority is
// deliberately checked first: a grant can authorize a path, but it cannot make
// an independent repository share the planning change's candidate accounting.
func applyRuntimeTopologyBlock(ctx context.Context, applyState *ApplyState, dependencies *Dependencies, nextRecommended *string, reasons *blockerReasons, tasksText, workspaceRoot, change string) {
	if applyState == nil || dependencies == nil || nextRecommended == nil {
		return
	}
	switch *nextRecommended {
	case string(PhaseApply), string(PhaseVerify), string(PhaseRemediate):
	default:
		return
	}
	roots, err := foreignRuntimeTopologyRoots(ctx, tasksText, workspaceRoot, change)
	if err != nil {
		reasons.genuine = append(reasons.genuine, runtimeTopologyBlockedReason(nil, err))
	} else if len(roots) == 0 {
		return
	} else {
		reasons.genuine = append(reasons.genuine, runtimeTopologyBlockedReason(roots, nil))
	}
	if *applyState == ApplyReady {
		*applyState = ApplyBlocked
		dependencies.Apply = DependencyBlocked
	}
	dependencies.Verify = DependencyBlocked
	dependencies.Archive = DependencyBlocked
	*nextRecommended = "resolve-blockers"
}

func foreignRuntimeTopologyRoots(ctx context.Context, tasksText, workspaceRoot, change string) ([]string, error) {
	planningStore, err := OpenRuntimeStore(ctx, workspaceRoot, change)
	if err != nil {
		return nil, fmt.Errorf("resolve the planning repository Git common directory: %w", err)
	}
	foreign := map[string]bool{}
	for _, line := range strings.Split(tasksText, "\n") {
		for _, token := range editTargetTokens(line, workspaceRoot) {
			resolved := token
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(workspaceRoot, resolved)
			}
			target := gitRootOf(resolveExistingPath(filepath.Clean(resolved)))
			if target == "" {
				continue
			}
			targetStore, err := OpenRuntimeStore(ctx, target, change)
			if err != nil {
				return nil, fmt.Errorf("resolve the target repository Git common directory for %s: %w", pathquote.Quote(target), err)
			}
			same, err := sameRuntimeCommonDirectory(planningStore.commonDir, targetStore.commonDir)
			if err != nil {
				return nil, err
			}
			if !same {
				foreign[target] = true
			}
		}
	}
	roots := make([]string, 0, len(foreign))
	for root := range foreign {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots, nil
}

func sameRuntimeCommonDirectory(left, right string) (bool, error) {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false, fmt.Errorf("read planning repository Git common directory: %w", err)
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false, fmt.Errorf("read task target Git common directory: %w", err)
	}
	return os.SameFile(leftInfo, rightInfo), nil
}

func runtimeTopologyBlockedReason(roots []string, err error) string {
	detail := ""
	if err != nil {
		detail = fmt.Sprintf("cannot verify Git common-dir identity: %v", err)
	} else {
		quoted := make([]string, 0, len(roots))
		for _, root := range roots {
			quoted = append(quoted, pathquote.Quote(root))
		}
		detail = fmt.Sprintf("tasks.md targets repositories with a different Git common directory: %s", strings.Join(quoted, ", "))
	}
	return fmt.Sprintf(
		"blocked(cross_common_dir_runtime_target): %s; keep runtime work in the planning repository or a shared linked worktree with the same Git common directory, or split independent repositories into separately planned and runtime-accounted SDD changes; an edit-authority grant does not supply candidate accounting",
		detail,
	)
}
