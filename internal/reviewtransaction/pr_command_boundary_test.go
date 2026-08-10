package reviewtransaction

import (
	"context"
	"errors"
	"testing"
)

// TestPrePRBoundaryRefsAreOwnedByStructuredBindingsNotCommandStrings proves the
// "PR commands" threat-matrix row at the only surface RAR exposes: the pre-PR
// and pre-push publication boundary is selected by a structured ref, never a
// command line. The gate itself never composes a shell command, so the
// fail-closed obligation here is that no environment-prefix form and no
// composed-shell form can ever become a bound boundary selector. A selector
// that survives validation is later replayed verbatim as a Git process
// argument (`check-ref-format`, `ls-remote`, `merge-base`, `rev-list`), so
// accepting `refs/heads/main;id` or a leading `-` would hand an attacker
// either a shell fragment or an argument-injection option.
func TestPrePRBoundaryRefsAreOwnedByStructuredBindingsNotCommandStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ref    string
		accept bool
	}{
		{name: "structured branch head", ref: "refs/heads/feature/organic-recovery", accept: true},
		{name: "structured default head", ref: "refs/heads/main", accept: true},

		// Environment-prefix forms: the head is smuggled behind an assignment
		// or an `env` invocation that only a shell would strip.
		{name: "env command prefix", ref: "env GIT_SSH_COMMAND=id refs/heads/main"},
		{name: "assignment prefix", ref: "GIT_DIR=/tmp/evil refs/heads/main"},
		{name: "unspaced assignment prefix", ref: "GIT_DIR=/tmp/evil;refs/heads/main"},

		// Composed-shell forms: separators, pipes, substitutions, and
		// redirections that only mean anything to a shell.
		{name: "semicolon composition", ref: "refs/heads/main;id"},
		{name: "and composition", ref: "refs/heads/main&&id"},
		{name: "background composition", ref: "refs/heads/main&id"},
		{name: "pipe composition", ref: "refs/heads/main|id"},
		{name: "dollar substitution", ref: "refs/heads/$(id)"},
		{name: "backtick substitution", ref: "refs/heads/`id`"},
		{name: "brace expansion", ref: "refs/heads/{a,b}"},
		{name: "output redirection", ref: "refs/heads/main>/tmp/owned"},
		{name: "input redirection", ref: "refs/heads/main</tmp/owned"},
		{name: "glob expansion", ref: "refs/heads/*"},
		{name: "newline composition", ref: "refs/heads/main\nid"},
		{name: "quote escape", ref: "refs/heads/main'"},

		// Argument-injection forms: a value that a structured argument list
		// would still hand to the tool as an option.
		{name: "long option", ref: "--upload-pack=id"},
		{name: "short option", ref: "-o"},

		// Git ref-format forms that fail closed for the same reason.
		{name: "parent traversal", ref: "refs/heads/../../etc"},
		{name: "reflog selector", ref: "refs/heads/main@{1}"},
		{name: "control character", ref: "refs/heads/main\x00id"},
		{name: "leading whitespace", ref: " refs/heads/main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateBoundaryRefSelector("pre-PR base selector", tt.ref)
			if tt.accept && err != nil {
				t.Fatalf("validateBoundaryRefSelector(%q) error = %v, want accepted", tt.ref, err)
			}
			if !tt.accept && !errors.Is(err, errCommandLikeBoundaryRef) {
				t.Fatalf("validateBoundaryRefSelector(%q) error = %v, want errCommandLikeBoundaryRef", tt.ref, err)
			}
		})
	}
}

// TestPrePRBoundarySelectionRejectsCommandLikeSelectorsBeforeGit proves the
// validator is wired at the selector entry point rather than existing beside
// it: a hostile selector reaching selectPrePRBoundary — the single funnel for
// the CLI --base-ref flag, a stored gate-request boundary, and the pre-push
// selector — is rejected as a structured-ref violation, not resolved against
// remotes and reported as merely missing.
func TestPrePRBoundarySelectionRejectsCommandLikeSelectorsBeforeGit(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	gitSnapshot(t, repo, "init")

	for _, ref := range []string{
		"refs/heads/main;id", "--upload-pack=id", "-o",
		"env GIT_SSH_COMMAND=id refs/heads/main", "refs/heads/main@{1}",
		"refs/heads/../../etc", "refs/heads/$(id)",
	} {
		if _, err := selectPrePRBoundary(context.Background(), repo, ref); !errors.Is(err, errCommandLikeBoundaryRef) {
			t.Fatalf("selectPrePRBoundary(%q) error = %v, want errCommandLikeBoundaryRef", ref, err)
		}
	}
}

// TestBoundaryRefSelectorKeepsOpaqueRefShapes proves the hardening above stays
// a shell/argument boundary rather than a ref-name policy: the characters the
// validator keeps legal (`:`, `/`, `.`, `-` inside a token, `_`, `+`, `,`,
// `%`, `@`) cover both the remote/branch selectors the gate resolves and the
// scheme-prefixed opaque owner labels used across the review surface. Git's
// own check-ref-format still governs branch resolution downstream.
func TestBoundaryRefSelectorKeepsOpaqueRefShapes(t *testing.T) {
	t.Parallel()

	for _, ref := range []string{
		"candidate:normalized/1", "candidate:owner-verified", "work-run:1794",
		"maintainer:one", "github:issue/1794", "bare:organic-runtime-e2e",
		"candidate:human-readable-label", "refs/heads/main",
		"origin/main", "main", "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
	} {
		t.Run(ref, func(t *testing.T) {
			t.Parallel()
			if err := validateBoundaryRefSelector("pre-PR base selector", ref); err != nil {
				t.Fatalf("validateBoundaryRefSelector(%q) error = %v, want accepted", ref, err)
			}
		})
	}
}
