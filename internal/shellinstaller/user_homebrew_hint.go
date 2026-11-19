package shellinstaller

import "strings"

// userHomebrewPrefixes are the shared Homebrew trees whose admin-group 0775
// directories the supervisor provenance check refuses as ancestors (#5814
// keeps that refusal). Paths use "/" on every OS so the rule stays testable.
var userHomebrewPrefixes = []string{"/opt/homebrew", "/usr/local/Cellar", "/usr/local/Homebrew"}

const userHomebrewRemediation = "; gentle-ai was started from Homebrew's shared prefix, whose group-writable directories it does not trust as a supervisor ancestor. " +
	"Keep Homebrew as it is (do not change Homebrew permissions) and run gentle-ai from a private directory instead: " +
	"either `go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@latest` into your own GOBIN, " +
	"or copy the gentle-ai binary into a user-owned 0755 directory (for example ~/.local/bin) and run it from there"

// userHomebrewHint returns the remediation appended to a supervisor ancestor
// refusal when the refused path lies in a Homebrew prefix, and "" otherwise.
func userHomebrewHint(path string) string {
	for _, prefix := range userHomebrewPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return userHomebrewRemediation
		}
	}
	return ""
}
