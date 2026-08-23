package reviewtransaction

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Correction scope admission (issue #3375).
//
// A bounded correction used to be confined to the frozen genesis manifest:
// every path it touched had to already be one the lenses reviewed. That rule
// is not arbitrary. It is what keeps three separate invariants true at once:
//
//   - Reviewers only ever inspected the genesis paths, so a path outside them
//     entered the approved candidate having been judged by nobody. The same
//     principle already demotes an out-of-genesis severe finding to
//     CausalUnknown (findingLocationInGenesis, compact.go).
//   - The receipt's PathsDigest is derived from that manifest, and every
//     delivery gate re-proves the delivered paths against it. A path that
//     entered behind the manifest's back would either be refused at delivery
//     or, worse, ride along inside a receipt that never named it.
//   - The pre-PR publication range check ("nothing unreviewed rides along in
//     intermediate commits", prepr.go) is stated in the same terms.
//
// So the guard cannot simply be deleted. But it also made the single bounded
// correction structurally unable to satisfy the single most common blocking
// finding a reviewer produces: "this behaviour has no test able to execute
// it". The only remedy for that finding is a new test file, a new path, and
// therefore a scope_changed recovery plus a second full review — a cascade
// caused by the rule rather than by the code.
//
// This file narrows the guard instead of removing it. A correction may add a
// path only when that path is BOTH:
//
//  1. named the way the toolchain that owns its extension names a test, under
//     the closed convention table below, and
//  2. a companion of an already-reviewed genesis path — living in a reviewed
//     directory, or in a test directory immediately inside or immediately
//     beside one.
//
// Everything else keeps today's exact behaviour and still requires recovery.
// The line budget is untouched: an added file's lines are correction lines
// like any other, so the blast radius stays bounded by the same number. The
// single-correction rule is untouched. And an added path is never silent: it
// is persisted on the authority as CorrectionAddedPaths and disclosed on the
// terminal receipt, so an auditor can always see exactly which delivered
// paths no lens inspected.
//
// Rule 1 answers one question and refuses everything it cannot answer: does
// the toolchain that owns THIS extension treat THIS name as a test, rather
// than as source it compiles into the shipped package? Two properties follow,
// and both are load-bearing:
//
//   - A convention is never borrowed across ecosystems. pytest's test_ prefix
//     says nothing about a Go file: only the _test.go suffix keeps a file out
//     of the ordinary build, so test_helper.go is production source and is
//     refused. That refusal is the whole point of the guard.
//   - A directory name never decides for a file. A Go package under a
//     directory called tests is compiled and importable exactly like any
//     other, so an ancestor segment may not answer a question about the file.
//     The reserved directory names below still describe where a test may
//     LIVE relative to the code it covers (rule 2), never what a file IS.
//
// Where the path alone cannot decide the answer, the extension is absent from
// the table and the addition is refused. Java, Kotlin, C#, and Rust are the
// deliberate omissions: their test sources are separated by build
// configuration (a test source root, a separate project, a cargo target)
// rather than by a name a path can be checked against, so FooTest.java under
// src/main/java and widget_test.rs under src/ both compile into the shipped
// artifact. Refusing them costs a recovery, which is the status quo; admitting
// them would ship unreviewed production code inside an approved candidate.

// correctionTestDirectoryNames is the closed set of directory names a
// repository conventionally reserves for tests. It decides only WHERE an
// admitted test file may live relative to the reviewed code it covers; it
// never decides whether a file is a test.
var correctionTestDirectoryNames = map[string]struct{}{
	"test": {}, "tests": {}, "__tests__": {}, "spec": {}, "specs": {},
}

func correctionTestDirectoryName(segment string) bool {
	_, ok := correctionTestDirectoryNames[strings.ToLower(segment)]
	return ok
}

// correctionTestFileConvention is one extension's discovery rule: the stem
// prefixes and suffixes the toolchain that owns that extension uses to tell a
// test apart from source it builds into the shipped package.
type correctionTestFileConvention struct {
	prefixes []string
	suffixes []string
}

// correctionJavaScriptTestConvention is shared by every extension the JS/TS
// runners treat alike.
var correctionJavaScriptTestConvention = correctionTestFileConvention{suffixes: []string{".test", ".spec"}}

// correctionTestFileConventions is the closed table. Matching is
// case-sensitive and extension-exact because every toolchain here matches that
// way: the go tool ignores a file called widget_Test.go as a test and a file
// called widget.GO as Go source at all, so anything looser would admit names
// the toolchain does not.
var correctionTestFileConventions = map[string]correctionTestFileConvention{
	// Go: the compiler itself excludes _test.go from the ordinary build. This
	// is the only entry in the table that is enforced rather than conventional.
	".go": {suffixes: []string{"_test"}},
	// Python: pytest's default python_files, test_*.py and *_test.py.
	".py": {prefixes: []string{"test_"}, suffixes: []string{"_test"}},
	// Ruby: minitest's test/*_test.rb and RSpec's spec/*_spec.rb.
	".rb": {suffixes: []string{"_test", "_spec"}},
	// Elixir: mix test loads *_test.exs, and .exs is never compiled into a
	// release the way .ex is.
	".exs": {suffixes: []string{"_test"}},
	// The JS/TS family: the .test/.spec infix in the default Jest and Vitest
	// patterns. A -test or -spec dash is deliberately absent: no default
	// configuration collects it, so those files are bundled as ordinary
	// modules.
	".js":  correctionJavaScriptTestConvention,
	".jsx": correctionJavaScriptTestConvention,
	".mjs": correctionJavaScriptTestConvention,
	".cjs": correctionJavaScriptTestConvention,
	".ts":  correctionJavaScriptTestConvention,
	".tsx": correctionJavaScriptTestConvention,
	".mts": correctionJavaScriptTestConvention,
	".cts": correctionJavaScriptTestConvention,
}

// correctionTestFileName reports whether one base name is a test file to the
// toolchain that owns its extension. An unrecognized extension is refused
// rather than guessed at, so the failure mode is the status quo (recovery)
// rather than an unreviewed file slipping into an approved candidate.
func correctionTestFileName(base string) bool {
	extension := path.Ext(base)
	convention, recognized := correctionTestFileConventions[extension]
	if !recognized {
		return false
	}
	// A stem no longer than the marker it matches is that marker and nothing
	// else: _test.go names no unit under test, and the go tool ignores a
	// leading underscore outright.
	stem := strings.TrimSuffix(base, extension)
	for _, prefix := range convention.prefixes {
		if len(stem) > len(prefix) && strings.HasPrefix(stem, prefix) {
			return true
		}
	}
	for _, suffix := range convention.suffixes {
		if len(stem) > len(suffix) && strings.HasSuffix(stem, suffix) {
			return true
		}
	}
	return false
}

// correctionCompanionDirectory reports whether a test living in addedDir is
// plausibly evidence about a reviewed file in genesisDir. The three accepted
// shapes are the ones every mainstream layout uses: the same directory
// (Go, JS/TS co-located), a test directory immediately inside the reviewed
// one, and a test directory immediately beside it.
func correctionCompanionDirectory(addedDir, genesisDir string) bool {
	if addedDir == genesisDir {
		return true
	}
	if !correctionTestDirectoryName(path.Base(addedDir)) {
		return false
	}
	parent := path.Dir(addedDir)
	return parent == genesisDir || parent == path.Dir(genesisDir)
}

// correctionAddedPathAdmissible reports whether one path a correction adds
// beyond the frozen genesis scope may enter the reviewed candidate.
func correctionAddedPathAdmissible(added string, genesis []string) bool {
	if !correctionTestFileName(path.Base(added)) {
		return false
	}
	directory := path.Dir(added)
	for _, reviewed := range genesis {
		if correctionCompanionDirectory(directory, path.Dir(reviewed)) {
			return true
		}
	}
	return false
}

// admitCorrectionScope returns the canonical paths a correction adds beyond
// the frozen genesis scope. It answers nil for a correction that stays inside
// genesis — byte-for-byte the pathsAreSubset outcome that preceded it — and
// refuses, naming the exact path, as soon as one addition is inadmissible.
func admitCorrectionScope(paths, genesis []string) ([]string, error) {
	canonicalCandidate, err := canonicalPaths(paths)
	if err != nil || !equalStrings(canonicalCandidate, paths) {
		// refusal:by-design world-action: a non-canonical snapshot path set is derived by native Git, so its repair belongs to code or storage, not to an operator command
		return nil, errors.New("snapshot paths must be canonical")
	}
	canonicalGenesis, err := canonicalPaths(genesis)
	if err != nil || !equalStrings(canonicalGenesis, genesis) {
		// refusal:by-design world-action: the frozen genesis manifest is persisted authority, so a non-canonical one requires code or storage repair
		return nil, errors.New("genesis snapshot paths must be canonical")
	}
	reviewed := make(map[string]struct{}, len(genesis))
	for _, item := range genesis {
		reviewed[item] = struct{}{}
	}
	var added []string
	for _, item := range paths {
		if _, ok := reviewed[item]; ok {
			continue
		}
		if !correctionAddedPathAdmissible(item, genesis) {
			// refusal:by-design operator-knowledge: only the human writing the correction can decide whether to keep it inside the reviewed manifest or take the recovery route, and no command can make that choice for them
			return nil, fmt.Errorf("correction path %q is outside immutable genesis scope", item)
		}
		added = append(added, item)
	}
	return added, nil
}

// correctionScopeRefused is the boolean spelling of admitCorrectionScope, for
// the composite conditions that only need to know whether the paths were
// refused.
func correctionScopeRefused(paths, genesis []string) bool {
	_, err := admitCorrectionScope(paths, genesis)
	return err != nil
}

// CorrectionScopePaths is the path set an approved candidate may actually
// deliver: the frozen genesis manifest plus any companion test paths one
// admitted bounded correction added. It is identical to GenesisPaths for
// every authority that never added one, which is every authority written
// before issue #3375 and every correction that stays inside the manifest.
//
// Genesis itself is never widened. GenesisPaths remains exactly what the
// lenses inspected, because that is what finding causality is judged against.
func (state CompactState) CorrectionScopePaths() []string {
	if len(state.CorrectionAddedPaths) == 0 {
		return state.GenesisPaths
	}
	scope, err := canonicalPaths(append(append([]string(nil), state.GenesisPaths...), state.CorrectionAddedPaths...))
	if err != nil {
		// Fail closed: an authority whose persisted scope cannot be
		// canonicalized delivers under the narrower reviewed manifest.
		return state.GenesisPaths
	}
	return scope
}

// compactCorrectionCandidateScope is the path set a persisted authority may
// legitimately describe: the delivery scope, plus whatever an on-record
// correction attempt actually added.
//
// The two differ for exactly one shape, and it is why this exists. A
// correction that escalated — over budget, or with a failed targeted check —
// consumed the single attempt and is recorded together with the snapshot it
// produced, while its widened delivery scope was rolled back. The authority
// still has to be a valid record of that attempt, so every addition an attempt
// carries is re-checked here against the exact admission the correction itself
// had to pass. Nothing enters this set that the correction could not have
// added, and nothing in it widens what may be delivered.
func compactCorrectionCandidateScope(state CompactState) ([]string, error) {
	scope := state.CorrectionScopePaths()
	if len(state.CorrectionAttempts) == 0 {
		return scope, nil
	}
	known := make(map[string]struct{}, len(scope))
	for _, item := range scope {
		known[item] = struct{}{}
	}
	var additions []string
	for _, attempt := range state.CorrectionAttempts {
		added, err := admitCorrectionScope(attempt.Snapshot.Paths, state.GenesisPaths)
		if err != nil {
			return nil, err
		}
		for _, item := range added {
			if _, ok := known[item]; ok {
				continue
			}
			known[item] = struct{}{}
			additions = append(additions, item)
		}
	}
	if len(additions) == 0 {
		return scope, nil
	}
	return canonicalPaths(append(append([]string(nil), scope...), additions...))
}

// validateCompactCorrectionAddedPaths keeps the widened scope from becoming a
// forgery surface: the field is meaningful only alongside a real completed
// correction, may never restate a genesis path, and every entry must still
// satisfy the same admission the correction had to pass.
func validateCompactCorrectionAddedPaths(state CompactState) error {
	if len(state.CorrectionAddedPaths) == 0 {
		return nil
	}
	canonical, err := canonicalPaths(state.CorrectionAddedPaths)
	if err != nil || !equalStrings(canonical, state.CorrectionAddedPaths) {
		// refusal:by-design world-action: persisted authority that cannot be canonicalized requires code or storage repair
		return errors.New("compact correction added paths must be canonical")
	}
	if len(state.CorrectionAttempts) == 0 && state.ActualCorrectionLines == nil {
		// refusal:by-design world-action: a widened scope with no correction behind it is contradictory persisted authority and cannot be repaired by an operator command
		return errors.New("compact correction added paths require a completed correction")
	}
	for _, added := range state.CorrectionAddedPaths {
		if stringIndex(state.GenesisPaths, added) >= 0 {
			// refusal:by-design world-action: an added path restating the frozen manifest is contradictory persisted authority and requires code or storage repair
			return fmt.Errorf("compact correction added path %q is already inside the frozen genesis scope", added)
		}
		if !correctionAddedPathAdmissible(added, state.GenesisPaths) {
			// refusal:by-design world-action: persisted authority claiming a scope this contract never admits cannot be repaired by an operator command
			return fmt.Errorf("compact correction added path %q is not an admissible companion test path", added)
		}
	}
	return nil
}
