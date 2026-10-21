package main

// The canonical path-free projection of recorded strings (issue #1866).
//
// A results file is evidence, and evidence must not depend on the machine it
// was produced on: two runs of the same corpus against the same binary, in
// different sandboxes, have to record the same strings. Every string that
// could carry a machine-specific path is therefore projected onto a closed
// token vocabulary at record time, so the written file is path-free by
// construction rather than by after-the-fact scrubbing.

import (
	"sort"
	"strings"
)

// The closed token vocabulary of the canonical projection. A token stands for
// a path this process knew at record time; everything after the token is the
// sandbox-relative tail, so a reader can still follow the flow without
// learning where the sandbox lived.
const (
	RunRootToken      = "<RUN_ROOT>"
	RepositoryToken   = "<REPOSITORY>"
	HomeToken         = "<HOME>"
	TargetBinaryToken = "<TARGET_BINARY>"
)

// PathNormalizer projects machine-specific paths onto the closed token
// vocabulary at record time. It is wired where records are folded
// (accumulator.observe), so every recorded string is path-free by
// construction.
//
// Replacement order is longest-path-first, so the most specific known path
// always wins over a path that contains it: the repository before the home
// that contains it, the home before the run root, and the target binary's
// exact path before the home it was built under (CI builds it in
// $RUNNER_TEMP, inside the runner's home).
//
// A nil *PathNormalizer is the identity: call sites with no paths to project
// keep working unchanged.
type PathNormalizer struct {
	targetBinary string
	runRoot      string
	home         string
	repository   string
}

// newPathNormalizer builds a normalizer for one run.
func newPathNormalizer(targetBinary, runRoot string) *PathNormalizer {
	return &PathNormalizer{targetBinary: targetBinary, runRoot: runRoot}
}

// setSandbox adds a journey's sandbox layout. The sandbox is created per
// journey inside runJourney, after the run-level normalizer exists.
func (n *PathNormalizer) setSandbox(home, repository string) {
	n.home = home
	n.repository = repository
}

// setRunRoot adds the sandbox root once it is known (it is canonicalized
// after creation, so the recorded spelling is the one replaced).
func (n *PathNormalizer) setRunRoot(root string) {
	n.runRoot = root
}

// Normalize replaces every known machine-specific path with its token,
// longest path first, leaving everything else untouched.
func (n *PathNormalizer) Normalize(value string) string {
	if n == nil || value == "" {
		return value
	}
	for _, replacement := range n.replacements() {
		value = strings.ReplaceAll(value, replacement.from, replacement.to)
	}
	return value
}

type pathReplacement struct{ from, to string }

// replacements returns the known paths ordered longest first. The stable sort
// keeps the declaration order as the tie-break for two paths of equal length.
func (n *PathNormalizer) replacements() []pathReplacement {
	replacements := []pathReplacement{}
	for _, replacement := range []pathReplacement{
		{n.repository, RepositoryToken},
		{n.home, HomeToken},
		{n.runRoot, RunRootToken},
		{n.targetBinary, TargetBinaryToken},
	} {
		if replacement.from != "" {
			replacements = append(replacements, replacement)
		}
	}
	sort.SliceStable(replacements, func(i, j int) bool {
		return len(replacements[i].from) > len(replacements[j].from)
	})
	return replacements
}

// NormalizeAll projects a slice of strings (argv, notes, unsupported steps),
// preserving nil.
func (n *PathNormalizer) NormalizeAll(values []string) []string {
	if n == nil || values == nil {
		return values
	}
	normalized := make([]string, len(values))
	for index, value := range values {
		normalized[index] = n.Normalize(value)
	}
	return normalized
}
