package sddstatus

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Issue #3278 (secondary defect, tracked as #2790's ambiguity half): the
// ambiguity continuations emitted by ambiguousChangeSelectionReasons rendered
// `gentle-ai sdd-status --cwd <repo> --change <name>`, but ParseCommandArgs
// has no --change flag — the change selector is positional. Every command the
// refusal told the operator to run was rejected by the very parser it targets,
// so following our own guidance burned the operator's single sanctioned
// observation on a syntax error.
//
// The guard: every runnable `gentle-ai sdd-status ...` continuation emitted in
// BlockedReasons must parse through ParseCommandArgs, and ambiguity
// continuations must carry the change selector through to the parsed result.

var sddStatusContinuationRe = regexp.MustCompile("`gentle-ai sdd-status ([^`]+)`")

// parseEmittedContinuations extracts every backticked sdd-status invocation
// from the blocked reasons and feeds its arguments to ParseCommandArgs,
// returning the parsed selectors. Placeholder tokens like <change-name> are
// parseable positionals by design, so no filtering is needed.
func parseEmittedContinuations(t *testing.T, reasons []string) []string {
	t.Helper()
	selectors := []string{}
	found := false
	for _, reason := range reasons {
		for _, match := range sddStatusContinuationRe.FindAllStringSubmatch(reason, -1) {
			found = true
			args := strings.Fields(match[1])
			parsed, err := ParseCommandArgs(args)
			if err != nil {
				t.Fatalf("emitted continuation does not parse through ParseCommandArgs: %q\nerror: %v", "gentle-ai sdd-status "+match[1], err)
			}
			selectors = append(selectors, parsed.ChangeName)
		}
	}
	if !found {
		t.Fatalf("no runnable sdd-status continuation found in blocked reasons: %v", reasons)
	}
	return selectors
}

func TestOpenSpecAmbiguityContinuationsParseAndCarryTheSelector(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "first", "- [ ] 1.1 Work\n")
	seedReadyChange(t, root, "second", "- [ ] 1.1 Work\n")

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "select-change" {
		t.Fatalf("NextRecommended = %q, want select-change", status.NextRecommended)
	}

	selectors := parseEmittedContinuations(t, status.BlockedReasons)
	want := []string{"first", "second"}
	if strings.Join(selectors, ",") != strings.Join(want, ",") {
		t.Fatalf("parsed change selectors = %v, want %v: each continuation must select the change it names", selectors, want)
	}
}

func TestEngramAmbiguityContinuationsParseAndCarryTheSelector(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".engram"))
	runRuntimeLedgerGit(t, root, "init", "-q")
	runRuntimeLedgerGit(t, root, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")

	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/add-auth/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/add-billing/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"},
	})
	defer restore()

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "select-change" {
		t.Fatalf("NextRecommended = %q, want select-change", status.NextRecommended)
	}

	selectors := parseEmittedContinuations(t, status.BlockedReasons)
	want := []string{"add-auth", "add-billing"}
	if strings.Join(selectors, ",") != strings.Join(want, ",") {
		t.Fatalf("parsed change selectors = %v, want %v: each continuation must select the change it names", selectors, want)
	}
}

func TestExplorationOnlyRefusalContinuationParses(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "changes", "explore-widget-idea", "exploration.md"), "# Exploration\n")

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	parseEmittedContinuations(t, status.BlockedReasons)
}
