package communitytool

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// codeGraphInstalledVersion probes the CodeGraph binary that DetectStatus
// already resolved and reports its version. It is a package var so tests
// never shell out to a real binary, and it takes the resolved absolute path
// rather than a bare command name so the probe cannot pick up a different
// binary than the one the install step will invoke.
var codeGraphInstalledVersion = defaultCodeGraphInstalledVersion

// codeGraphVersionPattern matches the dotted numeric core of a version string.
// The upstream CLI prints its version inside a decorated banner
// ("┌  CodeGraph v0.9.3"), so anchoring on the digits is what survives the
// decoration changing.
var codeGraphVersionPattern = regexp.MustCompile(`\d+(?:\.\d+)+`)

func defaultCodeGraphInstalledVersion(cliPath string) (string, bool) {
	cliPath = strings.TrimSpace(cliPath)
	if cliPath == "" {
		return "", false
	}
	// A nonzero exit still carries usable output on CLIs that print the
	// banner before rejecting the flag, so the output is parsed either way
	// and only an unparseable result counts as "cannot determine".
	output, err := exec.Command(cliPath, "--version").CombinedOutput()
	if err != nil && len(output) == 0 {
		return "", false
	}
	return parseCodeGraphVersion(string(output))
}

func parseCodeGraphVersion(output string) (string, bool) {
	match := codeGraphVersionPattern.FindString(output)
	if match == "" {
		return "", false
	}
	return match, true
}

// codeGraphVersionLess reports whether version a precedes version b, comparing
// dotted components numerically. Missing trailing components read as zero, so
// "1.4" precedes "1.4.1".
func codeGraphVersionLess(a, b string) bool {
	left := codeGraphVersionComponents(a)
	right := codeGraphVersionComponents(b)
	for i := 0; i < len(left) || i < len(right); i++ {
		leftPart := 0
		if i < len(left) {
			leftPart = left[i]
		}
		rightPart := 0
		if i < len(right) {
			rightPart = right[i]
		}
		if leftPart != rightPart {
			return leftPart < rightPart
		}
	}
	return false
}

func codeGraphVersionComponents(version string) []int {
	fields := strings.Split(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	components := make([]int, 0, len(fields))
	for _, field := range fields {
		component, err := strconv.Atoi(field)
		if err != nil {
			break
		}
		components = append(components, component)
	}
	return components
}
