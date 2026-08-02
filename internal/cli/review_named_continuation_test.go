package cli

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// This file is the cheap tier of the "a refusal must name something that
// works" contract.
//
// WHAT IT PROVES: every `gentle-ai review <verb>` continuation the product
// names in its own source -- refusals, hints, usage text -- resolves to a verb
// the CLI really dispatches, and every `--flag` that continuation names is
// really defined on that verb's flag.FlagSet. In other words: the named
// command PARSES.
//
// WHAT IT DOES NOT PROVE: that running the named command RESOLVES the block it
// was named for. A structurally perfect `gentle-ai review recover
// --predecessor-lineage ... --successor-lineage ...` can still dead-end,
// freezing a successor that re-trips the very rule that denied delivery. That
// stronger, expensive property is proven by execution in
// TestPrePushScopeChangeNamedRecoveryReachesAllow
// (review_scope_change_recovery_test.go), which runs exactly the recovery a
// denial names and requires the same gate to then return allow. Do not read
// this file as a substitute for that one: passing here only means the caller
// will not be told to run something the parser rejects outright.
//
// The set is derived from source, never from a hand-maintained table: a
// refusal added tomorrow that names a nonexistent verb or a flag that verb
// does not define fails here without anyone editing this file.

// reviewNamedContinuationInvocationRegexp matches the explicit invocation form
// -- the only form that unambiguously names a command rather than merely
// mentioning the word "review". Requiring a lowercase letter directly after
// "review " excludes the usage banner's `gentle-ai review <capabilities|...>`
// placeholder, which names no single verb.
var reviewNamedContinuationInvocationRegexp = regexp.MustCompile(`gentle-ai review ([a-z][a-z-]*)`)

// reviewNamedContinuationSelfReferenceRegexp matches the self-referential
// refusal form: a message that opens by naming the verb it is refusing ("review
// start with --base-ref omits ...") and then tells the caller to rerun with
// different flags. Those flags are argv for the verb the message opened with,
// even though the message never repeats the tool name.
var reviewNamedContinuationSelfReferenceRegexp = regexp.MustCompile(`^review ([a-z][a-z-]*)[ :]`)

// reviewNamedContinuationFlagRegexp matches a named argv flag. It requires a
// lowercase letter directly after "--" so prose that uses " -- " as a dash is
// never mistaken for a flag.
var reviewNamedContinuationFlagRegexp = regexp.MustCompile(`--([a-z][a-z0-9-]*)`)

// reviewNamedContinuationRerunTriggers are the phrases that turn a
// self-referential refusal into a named continuation.
var reviewNamedContinuationRerunTriggers = []string{"rerun with", "re-run with"}

type reviewNamedContinuationSite struct {
	file    string
	line    int
	verb    string
	flags   []string
	literal string
}

// TestEveryNamedReviewContinuationIsStructurallyReal is the guard. It walks the
// package's own source (plus internal/reviewtransaction, which raises review
// refusals of its own), enumerates every named `gentle-ai review <verb>`
// continuation, and requires the verb to dispatch and every flag it names to be
// defined on that verb's real FlagSet.
func TestEveryNamedReviewContinuationIsStructurallyReal(t *testing.T) {
	sites := collectReviewNamedContinuationSites(t)
	// The survey that motivated this guard found continuations across
	// review_facade.go, review_narration.go, review_capabilities.go,
	// review_mode.go, review.go and internal/reviewtransaction. A walk that
	// suddenly sees only a handful is broken, not clean.
	if len(sites) < 15 {
		t.Fatalf("enumerated only %d named review continuations; the survey found far more, so the AST walk is broken", len(sites))
	}
	t.Logf("enumerated %d named review continuations across %d source files", len(sites), reviewNamedContinuationFileCount(sites))

	verbs := reviewDispatchableReviewVerbs(t)
	flagCache := map[string]map[string]bool{}
	namedFlags := 0
	for _, site := range sites {
		if !verbs[site.verb] {
			t.Errorf("%s:%d names the continuation `gentle-ai review %s`, but no such verb is dispatched (%q)",
				site.file, site.line, site.verb, site.literal)
			continue
		}
		// A continuation that names no flag needs no FlagSet. Some verbs
		// legitimately have none at all -- `review schema` takes a positional
		// name -- so resolving flags unconditionally would fail the guard on a
		// perfectly valid refusal. The verb check above still applies to every
		// site; only flag resolution is skipped, and only when there is nothing
		// to resolve.
		if len(site.flags) == 0 {
			continue
		}
		flags, cached := flagCache[site.verb]
		if !cached {
			flags = reviewNamedContinuationVerbFlags(t, site.verb)
			flagCache[site.verb] = flags
		}
		for _, name := range site.flags {
			namedFlags++
			if !flags[name] {
				t.Errorf("%s:%d names `gentle-ai review %s --%s`, but that verb's FlagSet defines no --%s (%q)",
					site.file, site.line, site.verb, name, name, site.literal)
			}
		}
	}
	if namedFlags == 0 {
		t.Fatal("no named continuation carried a single --flag; the flag extraction is broken")
	}
	t.Logf("checked %d named flags against real FlagSets", namedFlags)
}

func reviewNamedContinuationFileCount(sites []reviewNamedContinuationSite) int {
	files := map[string]bool{}
	for _, site := range sites {
		files[site.file] = true
	}
	return len(files)
}

// collectReviewNamedContinuationSites walks every production source file of
// this package and of internal/reviewtransaction, and returns one site per
// named `gentle-ai review <verb>` continuation found in a string literal,
// carrying the flags that continuation names.
//
// Attribution is positional and line-scoped: a literal is split at each
// explicit invocation, and the flags inside one slice belong to the verb that
// opened it, up to the end of that invocation's own line. The slice
// BEFORE the first invocation is attributed only to a self-referential opener
// ("review start with ... rerun with --committed-only"); otherwise its flags
// are left unattributed rather than guessed at, because a flag with no
// resolvable owner cannot be checked without inventing one.
func collectReviewNamedContinuationSites(t *testing.T) []reviewNamedContinuationSite {
	t.Helper()
	var sites []reviewNamedContinuationSite
	unattributedRerun := 0
	for _, dir := range []string{".", filepath.Join("..", "reviewtransaction")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		fset := token.NewFileSet()
		for _, name := range names {
			path := filepath.Join(dir, name)
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				t.Fatalf("parse %s: %v", path, parseErr)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, unquoteErr := strconv.Unquote(literal.Value)
				if unquoteErr != nil {
					return true
				}
				found, unattributed := reviewNamedContinuationsInLiteral(value)
				unattributedRerun += unattributed
				for _, site := range found {
					site.file, site.line = path, fset.Position(literal.Pos()).Line
					sites = append(sites, site)
				}
				return true
			})
		}
	}
	if unattributedRerun > 0 {
		// Honesty: a "rerun with --flag" message that names no verb anywhere in
		// its own text cannot be attributed without guessing which command the
		// caller was running. Those are counted, never silently dropped, and
		// never asserted against a verb this guard picked for them.
		t.Logf("%d rerun-style literal(s) named a flag with no resolvable verb in the same literal; out of scope for this guard", unattributedRerun)
	}
	return sites
}

// reviewNamedContinuationsInLiteral extracts the named continuations of one
// string literal, and returns how many rerun-style flag groups it could not
// attribute to any verb.
func reviewNamedContinuationsInLiteral(value string) ([]reviewNamedContinuationSite, int) {
	matches := reviewNamedContinuationInvocationRegexp.FindAllStringSubmatchIndex(value, -1)
	prefixEnd := len(value)
	if len(matches) > 0 {
		prefixEnd = matches[0][0]
	}
	sites := make([]reviewNamedContinuationSite, 0, len(matches))
	unattributed := 0
	if prefixFlags := reviewNamedContinuationFlags(value[:prefixEnd]); len(prefixFlags) > 0 && reviewNamedContinuationRerunsItself(value) {
		if opener := reviewNamedContinuationSelfReferenceRegexp.FindStringSubmatch(value); opener != nil {
			sites = append(sites, reviewNamedContinuationSite{verb: opener[1], flags: prefixFlags, literal: value})
		} else {
			unattributed++
		}
	}
	for index, match := range matches {
		end := len(value)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		sites = append(sites, reviewNamedContinuationSite{
			verb:    value[match[2]:match[3]],
			flags:   reviewNamedContinuationFlags(reviewNamedContinuationArgv(value[match[1]:end])),
			literal: value,
		})
	}
	return sites, unattributed
}

// reviewNamedContinuationArgv narrows one invocation's segment to the argv that
// really belongs to it. A command line ends at its newline -- unless that line
// ends in a backslash continuation -- so a multi-line message that names a
// command on one line and then explains, on later lines, which flags of the
// REFUSING verb the caller must fill in never has that prose mistaken for argv
// of the command named above it. Without this, a refusal is structurally
// forbidden from naming a lookup command and describing its own inputs in the
// same message, which is exactly the shape an actionable refusal needs.
func reviewNamedContinuationArgv(segment string) string {
	for offset := 0; ; {
		index := strings.IndexByte(segment[offset:], '\n')
		if index < 0 {
			return segment
		}
		line := segment[:offset+index]
		if !strings.HasSuffix(strings.TrimRight(line, " \t"), "\\") {
			return line
		}
		offset += index + 1
	}
}

func reviewNamedContinuationRerunsItself(value string) bool {
	lowered := strings.ToLower(value)
	for _, trigger := range reviewNamedContinuationRerunTriggers {
		if strings.Contains(lowered, trigger) {
			return true
		}
	}
	return false
}

func reviewNamedContinuationFlags(segment string) []string {
	matches := reviewNamedContinuationFlagRegexp.FindAllStringSubmatch(segment, -1)
	if len(matches) == 0 {
		return nil
	}
	flags := make([]string, 0, len(matches))
	for _, match := range matches {
		flags = append(flags, match[1])
	}
	return flags
}

// reviewAppReviewPreDispatchRegexp extracts the review sub-verbs internal/app
// dispatches BEFORE handing the rest to RunReview. `review mode` is exactly
// that: the kill switch has to stay reachable when review authority itself is
// disabled, so it never appears in review_facade.go's own switch.
var reviewAppReviewPreDispatchRegexp = regexp.MustCompile(`args\[1\] == "([a-z][a-z-]*)"`)

// reviewDispatchableReviewVerbs returns every verb `gentle-ai review <verb>`
// really reaches, from the two mechanical extractions that own the answer:
// review_facade.go's own dispatch switches, and internal/app's review
// pre-dispatch. Both fail closed when their extraction stops finding anything.
func reviewDispatchableReviewVerbs(t *testing.T) map[string]bool {
	t.Helper()
	verbs := reviewCommandDispatchVerbs(t)
	if len(verbs) == 0 {
		t.Fatal("found no dispatched review command verbs in review_facade.go; the extraction is stale")
	}
	source, err := os.ReadFile(filepath.Join("..", "app", "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	block := reviewAppReviewPreDispatchBlock(t, string(source))
	preDispatched := reviewAppReviewPreDispatchRegexp.FindAllStringSubmatch(block, -1)
	if len(preDispatched) == 0 {
		t.Fatal("found no review sub-verb pre-dispatch in internal/app/app.go's `case \"review\":` block; the extraction is stale")
	}
	for _, match := range preDispatched {
		verbs[match[1]] = true
	}
	return verbs
}

// reviewAppReviewPreDispatchBlock returns the source text of internal/app's
// `case "review":` arm only, so the pre-dispatch extraction can never pick up
// an args[1] comparison belonging to a different command.
func reviewAppReviewPreDispatchBlock(t *testing.T, source string) string {
	t.Helper()
	const marker = "\t\tcase \"review\":\n"
	start := strings.Index(source, marker)
	if start < 0 {
		t.Fatal("internal/app/app.go has no `case \"review\":` arm; the extraction is stale")
	}
	rest := source[start+len(marker):]
	if end := strings.Index(rest, "\t\tcase \""); end >= 0 {
		return rest[:end]
	}
	return rest
}

// reviewNamedContinuationVerbFlagRegexp matches one flag line of the usage
// block newReviewFlagSet renders, which is a VisitAll over the verb's real
// FlagSet -- so parsing it enumerates exactly the flags that verb defines,
// with no second source of truth to drift from.
var reviewNamedContinuationVerbFlagRegexp = regexp.MustCompile(`(?m)^\s+--([a-z][a-z0-9-]*) `)

// reviewNamedContinuationVerbFlags asks the real command for its own flags by
// running it with --help through the exact route the CLI dispatches. --help is
// handled before any repository work in every review verb, so this observes the
// FlagSet without mutating anything.
func reviewNamedContinuationVerbFlags(t *testing.T, verb string) map[string]bool {
	t.Helper()
	var usage bytes.Buffer
	var err error
	if verb == reviewModeDispatchVerb {
		// `review mode` builds its FlagSet per operation, after the operation
		// word; every operation builds the identical set. Mirroring
		// internal/app's own pre-dispatch keeps this honest, and
		// reviewDispatchableReviewVerbs above fails closed if that pre-dispatch
		// ever stops existing.
		err = RunReviewMode([]string{"status", "--help"}, &usage)
	} else {
		err = RunReview([]string{verb, "--help"}, &usage)
	}
	if err != nil {
		t.Fatalf("`gentle-ai review %s --help` failed: %v\n%s", verb, err, usage.String())
	}
	flags := map[string]bool{"help": true, "h": true}
	for _, match := range reviewNamedContinuationVerbFlagRegexp.FindAllStringSubmatch(usage.String(), -1) {
		flags[match[1]] = true
	}
	if len(flags) <= 2 {
		t.Fatalf("`gentle-ai review %s --help` listed no flags; the usage parsing is stale:\n%s", verb, usage.String())
	}
	return flags
}

// reviewModeDispatchVerb is the one review verb internal/app routes before the
// facade; see reviewAppReviewPreDispatchRegexp.
const reviewModeDispatchVerb = "mode"
