package sddstatus

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// verifyReportFileName is the single canonical final-verification report an
// active OpenSpec change owns.
const verifyReportFileName = "verify-report.md"

// canonicalVerifyReportPaths proves that the change's final-verification
// report is the one canonical report its planning workspace owns, and returns
// the repository-relative slash path the settled candidate tree addresses it
// by.
//
// Every anchor is canonicalized here, through exactly the resolution
// resolveBindingChangeRoot already applied to the anchors it derived changeRoot
// from. Canonicalizing one operand and comparing it lexically against the raw
// other operand is the defect this exists to remove: a raw --cwd and a resolved
// change root are two spellings of one directory, and comparing spellings
// answers about the strings instead of about the directory.
//
// That is what broke the Windows suite. GetTempPath hands out the process
// temporary directory in its 8.3 short form (C:\Users\RUNNER~1\AppData\...),
// so filepath.Abs of --cwd kept the short spelling while
// filepath.EvalSymlinks expanded every component to its long, real-case name.
// filepath.Rel then walked out of the workspace with ".." and the change's own
// canonical report was refused as foreign, which projected as "current
// repository target no longer matches the reviewed scope". The same divergence
// exists wherever one directory has two spellings: a symlinked ancestor on
// POSIX, /var and /private/var on macOS.
func canonicalVerifyReportPaths(repo, workspace, changeRoot, change string) (repositoryRelative string, err error) {
	canonicalRepo, err := canonicalBindingPath(repo)
	if err != nil {
		return "", err
	}
	canonicalWorkspace, err := canonicalBindingPath(workspace)
	if err != nil {
		return "", err
	}
	canonicalChangeRoot, err := canonicalBindingPath(changeRoot)
	if err != nil {
		return "", err
	}
	return verifyReportPathUnderAnchors(
		filepath.ToSlash(canonicalRepo),
		filepath.ToSlash(canonicalWorkspace),
		filepath.ToSlash(canonicalChangeRoot),
		change,
		platformAnchorCaseSemantics(),
	)
}

// anchorCaseSemantics is how one platform's own path package decides whether
// two path words name the same thing. It is an explicit operand rather than a
// build-tagged constant so the comparison stays pure and every platform's
// semantics are exercisable from any runner -- the property that made this
// class of defect diagnosable from CI output in the first place.
type anchorCaseSemantics bool

const (
	// caseSensitiveAnchors compares path words byte-exactly. This is what
	// path/filepath does everywhere except Windows, and it is correct there:
	// on POSIX /srv/Repo and /srv/repo are genuinely two directories.
	caseSensitiveAnchors anchorCaseSemantics = false
	// caseFoldingAnchors compares path words case-insensitively. This is what
	// path/filepath does on Windows, for the volume name as much as for every
	// other element; the volume reaches this comparison as the leading word of
	// the caller's slash form, so it folds with the rest.
	caseFoldingAnchors anchorCaseSemantics = true
)

// platformAnchorCaseSemantics is this platform's answer, taken from
// path/filepath itself rather than restated from GOOS, so this comparison
// cannot drift from the one filepath.Rel makes on the same operands.
//
// filepath.Rel compares the volume name and every path element with sameWord,
// which is strings.EqualFold on Windows and byte equality on Unix and Plan 9.
// So two paths differing only in case are one directory (".") exactly where
// the platform folds, and two directories ("../A") where it does not.
func platformAnchorCaseSemantics() anchorCaseSemantics {
	separator := string(filepath.Separator)
	relative, err := filepath.Rel(separator+"a", separator+"A")
	if err == nil && relative == "." {
		return caseFoldingAnchors
	}
	return caseSensitiveAnchors
}

// verifyReportPathUnderAnchors is the pure decision behind that anchoring. It
// never touches the filesystem, so the whole path algebra is exercisable from
// any platform against any platform's spellings -- which is the only way this
// class of defect is catchable without a Windows runner.
//
// Every operand must already be an absolute path in slash form (the caller's
// filepath.ToSlash) and in one shared canonical spelling. This answers
// containment, never identity, and it does not resolve links, because the
// caller resolved every operand the same way before asking. Case is folded
// exactly where the named platform's own path semantics fold it, no more.
//
// The canonical suffix is still matched byte-exactly under either semantics,
// which is what filepath.Rel gave this comparison before: Rel returns the
// target's spelling, never the anchor's, so the returned repository-relative
// path is the one the settled candidate tree is addressed by. Git tree paths
// are byte-exact, so folding that suffix would hand the blob read a path the
// tree cannot resolve.
func verifyReportPathUnderAnchors(repo, workspace, changeRoot, change string, semantics anchorCaseSemantics) (repositoryRelative string, err error) {
	reportPath := path.Join(changeRoot, verifyReportFileName)
	canonical := path.Join("openspec", "changes", change, verifyReportFileName)
	workspaceRelative, contained := relativePathUnder(workspace, reportPath, semantics)
	if !contained || workspaceRelative != canonical {
		return "", fmt.Errorf("final verification report is not the canonical active-change path %q", canonical) // refusal:by-design world-action: a report outside the canonical active change cannot be safely attested
	}
	repositoryRelative, contained = relativePathUnder(repo, reportPath, semantics)
	if !contained {
		return "", errors.New("final verification report resolves outside the repository root") // refusal:by-design world-action: a report outside the repository has no path the settled candidate tree can be read by
	}
	return repositoryRelative, nil
}

// relativePathUnder reports target's slash-separated position beneath anchor.
//
// Both operands are cleaned the same way, so a trailing separator on either
// side is not a difference. The prefix is matched on whole path words, so a
// sibling whose name merely starts with the anchor's name ("/repo-2" under
// "/repo") is not contained, and the result never escapes the anchor with
// "..": a target outside the anchor is refused rather than described.
//
// Words are walked one at a time rather than compared as a single byte prefix
// because that is how filepath.Rel walks them, and because case folding can
// change a word's byte length -- U+212A KELVIN SIGN folds to "k" -- so a
// byte-length prefix would refuse pairs the platform itself calls equal.
func relativePathUnder(anchor, target string, semantics anchorCaseSemantics) (string, bool) {
	anchorWords := pathWords(anchor)
	targetWords := pathWords(target)
	if len(targetWords) < len(anchorWords) {
		return "", false
	}
	for index, anchorWord := range anchorWords {
		if !samePathWord(anchorWord, targetWords[index], semantics) {
			return "", false
		}
	}
	if len(targetWords) == len(anchorWords) {
		return ".", true
	}
	return strings.Join(targetWords[len(anchorWords):], "/"), true
}

// pathWords splits a slash-form path into the words filepath.Rel would walk.
// The path is cleaned first, so redundant separators and dot segments are not
// differences, and an absolute path keeps exactly one leading empty word so it
// can never match a relative one.
func pathWords(slashPath string) []string {
	cleaned := path.Clean(slashPath)
	if cleaned == "/" {
		return []string{""}
	}
	return strings.Split(cleaned, "/")
}

// samePathWord is path/filepath's own sameWord with the platform half passed
// in rather than chosen by build tag.
func samePathWord(anchorWord, targetWord string, semantics anchorCaseSemantics) bool {
	if semantics == caseFoldingAnchors {
		return strings.EqualFold(anchorWord, targetWord)
	}
	return anchorWord == targetWord
}
