package sddstatus

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestVerifyReportPathUnderAnchorsDecidesSpellingsFromAnyPlatform is the
// cross-platform half of this guard.
//
// The defect it pins only ever reproduced on Windows, but nothing about it is
// Windows-specific once the filesystem is out of the way: it was a lexical
// comparison between an anchor in one spelling and a change root in another.
// verifyReportPathUnderAnchors is pure and takes slash-form absolute paths and
// the platform's case semantics as operands, so genuine Windows spellings --
// 8.3 short names, drive letters, mixed case -- are stated and refuted here
// from a Linux runner, under either platform's rules.
func TestVerifyReportPathUnderAnchorsDecidesSpellingsFromAnyPlatform(t *testing.T) {
	const change = "post-review-verify-report"
	const canonical = "openspec/changes/post-review-verify-report/verify-report.md"
	const windowsLong = "C:/Users/runneradmin/AppData/Local/Temp/TestBound/001"
	const windowsShort = "C:/Users/RUNNER~1/AppData/Local/Temp/TestBound/001"

	for _, testCase := range []struct {
		name       string
		semantics  anchorCaseSemantics
		repo       string
		workspace  string
		changeRoot string
		want       string
		wantErr    bool
	}{
		{
			name:       "posix anchors in one spelling",
			semantics:  caseSensitiveAnchors,
			repo:       "/srv/repo",
			workspace:  "/srv/repo",
			changeRoot: "/srv/repo/openspec/changes/" + change,
			want:       canonical,
		},
		{
			name:       "windows long-form anchors in one spelling",
			semantics:  caseFoldingAnchors,
			repo:       windowsLong,
			workspace:  windowsLong,
			changeRoot: windowsLong + "/openspec/changes/" + change,
			want:       canonical,
		},
		{
			// The exact CI failure. GetTempPath hands out the short spelling
			// and filepath.EvalSymlinks expands it, so the workspace anchor and
			// the change root named one directory in two spellings.
			name:       "windows 8.3 short workspace against an expanded change root",
			semantics:  caseFoldingAnchors,
			repo:       windowsLong,
			workspace:  windowsShort,
			changeRoot: windowsLong + "/openspec/changes/" + change,
			wantErr:    true,
		},
		{
			name:       "windows 8.3 short repository against an expanded change root",
			semantics:  caseFoldingAnchors,
			repo:       windowsShort,
			workspace:  windowsLong,
			changeRoot: windowsLong + "/openspec/changes/" + change,
			wantErr:    true,
		},
		{
			// The POSIX half of the platform boundary: /srv/Repo and /srv/repo
			// are two directories on ext4, so this must stay a refusal. It is
			// unreachable in production because every anchor is resolved by the
			// same canonicalization, which returns the on-disk case.
			name:       "differently cased anchor is two directories under posix semantics",
			semantics:  caseSensitiveAnchors,
			repo:       "/srv/repo",
			workspace:  "/srv/Repo",
			changeRoot: "/srv/repo/openspec/changes/" + change,
			wantErr:    true,
		},
		{
			// The Windows half, on the very same operands. filepath.Rel
			// compares path words with sameWord, which is strings.EqualFold
			// there, so these name one directory and this comparison must not
			// be narrower than the platform it runs on.
			name:       "the same differently cased anchor is one directory under windows semantics",
			semantics:  caseFoldingAnchors,
			repo:       "/srv/repo",
			workspace:  "/srv/Repo",
			changeRoot: "/srv/repo/openspec/changes/" + change,
			want:       canonical,
		},
		{
			// Volume names count: filepath.Rel compares them with the same
			// sameWord, and in slash form the volume is just the leading word.
			name:       "windows volume letter case is not a difference",
			semantics:  caseFoldingAnchors,
			repo:       "c:/Users/runneradmin/AppData/Local/Temp/TestBound/001",
			workspace:  windowsLong,
			changeRoot: windowsLong + "/openspec/changes/" + change,
			want:       canonical,
		},
		{
			// The case difference on the target side rather than the anchor
			// side. The returned tail keeps the change root's own spelling,
			// which is what addresses the blob in the settled candidate tree.
			name:       "a differently cased change root is contained under windows semantics",
			semantics:  caseFoldingAnchors,
			repo:       windowsLong,
			workspace:  windowsLong,
			changeRoot: "C:/USERS/RunnerAdmin/appdata/LOCAL/temp/TestBound/001/openspec/changes/" + change,
			want:       canonical,
		},
		{
			// Folding is per word, never over a byte-length prefix: U+212A
			// KELVIN SIGN folds to "k" and is three bytes wide, so a prefix
			// compare would refuse a pair Windows itself calls equal.
			name:       "windows folding spans words whose byte lengths differ",
			semantics:  caseFoldingAnchors,
			repo:       "C:/srv/KELVIN",
			workspace:  "C:/srv/\u212Aelvin",
			changeRoot: "C:/srv/kelvin/openspec/changes/" + change,
			want:       canonical,
		},
		{
			// The other side of that boundary: folding case must never fold two
			// genuinely different directories into one.
			name:       "case folding does not contain a differently named sibling",
			semantics:  caseFoldingAnchors,
			repo:       windowsLong,
			workspace:  windowsLong,
			changeRoot: windowsLong + "-2/openspec/changes/" + change,
			wantErr:    true,
		},
		{
			name:       "trailing separators are not a difference",
			semantics:  caseFoldingAnchors,
			repo:       windowsLong + "/",
			workspace:  windowsLong + "/",
			changeRoot: windowsLong + "/openspec/changes/" + change + "/",
			want:       canonical,
		},
		{
			name:       "redundant separators and dot segments are not a difference",
			semantics:  caseSensitiveAnchors,
			repo:       "/srv//repo/.",
			workspace:  "/srv/./repo",
			changeRoot: "/srv/repo/openspec/changes/" + change,
			want:       canonical,
		},
		{
			// Separator normalization is the caller's filepath.ToSlash, and
			// this states why: a backslash is an ordinary filename character on
			// POSIX, so this comparison must never treat it as a separator.
			// Case folding does not rescue it either -- the whole backslashed
			// path is one word, and no folding makes it equal to "C:".
			name:       "native windows separators are the caller's job to normalize",
			semantics:  caseFoldingAnchors,
			repo:       `C:\Users\runneradmin\repo`,
			workspace:  `C:\Users\runneradmin\repo`,
			changeRoot: "C:/Users/runneradmin/repo/openspec/changes/" + change,
			wantErr:    true,
		},
		{
			name:       "workspace below the repository keeps the repository-relative path",
			semantics:  caseSensitiveAnchors,
			repo:       "/srv/repo",
			workspace:  "/srv/repo/service",
			changeRoot: "/srv/repo/service/openspec/changes/" + change,
			want:       "service/" + canonical,
		},
		{
			name:       "a sibling whose name merely shares the anchor prefix is not contained",
			semantics:  caseSensitiveAnchors,
			repo:       "/srv/repo",
			workspace:  "/srv/repo",
			changeRoot: "/srv/repo-2/openspec/changes/" + change,
			wantErr:    true,
		},
		{
			name:       "a change root above the workspace is refused, never described with ..",
			semantics:  caseSensitiveAnchors,
			repo:       "/srv/repo",
			workspace:  "/srv/repo/service",
			changeRoot: "/srv/repo/openspec/changes/" + change,
			wantErr:    true,
		},
		{
			name:       "a report outside the canonical active-change path is refused",
			semantics:  caseSensitiveAnchors,
			repo:       "/srv/repo",
			workspace:  "/srv/repo",
			changeRoot: "/srv/repo/openspec/archive/" + change,
			wantErr:    true,
		},
		{
			name:       "a change root named for another change is refused",
			semantics:  caseSensitiveAnchors,
			repo:       "/srv/repo",
			workspace:  "/srv/repo",
			changeRoot: "/srv/repo/openspec/changes/other-change",
			wantErr:    true,
		},
		{
			name:       "a workspace outside its repository is refused",
			semantics:  caseSensitiveAnchors,
			repo:       "/srv/other",
			workspace:  "/srv/repo",
			changeRoot: "/srv/repo/openspec/changes/" + change,
			wantErr:    true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := verifyReportPathUnderAnchors(testCase.repo, testCase.workspace, testCase.changeRoot, change, testCase.semantics)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("verifyReportPathUnderAnchors(%q, %q, %q) = %q, nil; want a refusal",
						testCase.repo, testCase.workspace, testCase.changeRoot, got)
				}
				if got != "" {
					t.Fatalf("refused anchoring returned %q; want no path alongside the refusal", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("verifyReportPathUnderAnchors(%q, %q, %q) error = %v",
					testCase.repo, testCase.workspace, testCase.changeRoot, err)
			}
			if got != testCase.want {
				t.Fatalf("repository-relative report path = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestRelativePathUnderMatchesFilepathRelOnThisPlatform is the anti-drift half
// of the platform boundary. The table above states both platforms' rules from
// any runner, but nothing there proves this platform picks the right one, and
// picking the wrong one is exactly the defect: filepath.Rel folds case on
// Windows and this comparison replaced it, so anything narrower silently
// refuses anchors the platform calls equal.
//
// It runs meaningfully on both sides. On POSIX filepath.Rel walks out of the
// differently-cased base with "..", so containment must be refused; on Windows
// sameWord folds and Rel names the tail, so containment must be granted with
// exactly that tail.
func TestRelativePathUnderMatchesFilepathRelOnThisPlatform(t *testing.T) {
	separator := string(filepath.Separator)
	base := separator + filepath.Join("srv", "repo")
	target := separator + filepath.Join("srv", "REPO", "openspec", "changes", "some-change")

	relative, err := filepath.Rel(base, target)
	relContains := err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+separator)

	got, contained := relativePathUnder(filepath.ToSlash(base), filepath.ToSlash(target), platformAnchorCaseSemantics())
	if contained != relContains {
		t.Fatalf("relativePathUnder(%q, %q) contained = %v; filepath.Rel says %v (rel %q, err %v)",
			base, target, contained, relContains, relative, err)
	}
	if contained && got != filepath.ToSlash(relative) {
		t.Fatalf("relativePathUnder(%q, %q) = %q; filepath.Rel says %q", base, target, got, filepath.ToSlash(relative))
	}
}

// TestCanonicalVerifyReportPathsAcceptsTwoSpellingsOfOneWorkspace proves the
// filesystem half: the anchors reach the pure comparison in one spelling
// because canonicalVerifyReportPaths resolves every one of them the same way.
// A symlinked ancestor is the POSIX shape of the divergence; on Windows the
// same resolution expands an 8.3 short name to its long, real-case form.
func TestCanonicalVerifyReportPathsAcceptsTwoSpellingsOfOneWorkspace(t *testing.T) {
	const change = "aliased-anchor"
	realRepo, aliasedRepo := aliasedRepository(t, change)
	changeRoot, err := resolveBindingChangeRoot(context.Background(), realRepo, realRepo, change)
	if err != nil {
		t.Fatal(err)
	}
	canonical := "openspec/changes/" + change + "/verify-report.md"
	for _, spelling := range []struct {
		name      string
		repo      string
		workspace string
	}{
		{name: "canonical", repo: realRepo, workspace: realRepo},
		{name: "aliased workspace", repo: realRepo, workspace: aliasedRepo},
		{name: "aliased repository", repo: aliasedRepo, workspace: realRepo},
		{name: "both aliased", repo: aliasedRepo, workspace: aliasedRepo},
	} {
		t.Run(spelling.name, func(t *testing.T) {
			got, err := canonicalVerifyReportPaths(spelling.repo, spelling.workspace, changeRoot, change)
			if err != nil {
				t.Fatalf("canonicalVerifyReportPaths(%q, %q) error = %v", spelling.repo, spelling.workspace, err)
			}
			if got != canonical {
				t.Fatalf("repository-relative report path = %q, want %q", got, canonical)
			}
		})
	}
}
