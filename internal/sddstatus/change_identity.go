package sddstatus

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// One change has one identity contract.
//
// sdd-status resolves a change by name and the native runtime authority opens
// a ledger for that same name. Before #2116 the two disagreed: status resolved
// whatever directory existed under openspec/changes (or whatever Engram topic
// key matched sdd/<change>/<type>), while the runtime authority applied a
// regex of its own. Every identity in the gap between them -- a dot, a space,
// an @, an uppercase letter -- produced planning artifacts that the mandatory
// attempt-acquire gate then refused to serve, with no rename or migration
// operation to recover with.
//
// sddChangeIdentityRefusal is the single predicate both surfaces consult, so
// the gap cannot reopen. It accepts what sdd-status can actually resolve --
// an OpenSpec directory entry name, or an Engram change id -- and refuses only
// what would stop being one addressable path segment. Widening it further is
// not "more permissive", it is a traversal.
const maxSDDChangeIdentityBytes = 96

// sddChangeIdentityForbiddenRunes can never appear in an identity.
//
// '/' and '\' would split one path segment into two, so an identity could name
// a directory outside its own change -- both in the ledger path and in the
// openspec/changes/<change>/ prefix that binds compact review authority.
// ':' names a Windows drive or an NTFS alternate data stream. '*', '?', '"',
// '<', '>' and '|' are refused outright by every Windows filesystem, so an
// identity carrying one would resolve on Linux and be unrepresentable on
// Windows: the same split contract this whole change exists to close.
const sddChangeIdentityForbiddenRunes = `/\:*?"<>|`

// sddChangeIdentityRefusal returns the reason a change identity cannot be
// used, or "" when it is accepted by every SDD surface.
func sddChangeIdentityRefusal(change string) string {
	switch {
	case change == "":
		return "a change identity cannot be empty"
	case len(change) > maxSDDChangeIdentityBytes:
		// The bound is load-bearing, not cosmetic. A non-legacy identity is
		// stored at <common-dir>/gentle-ai/sdd-runtime/v1/_encoded/<label>-<32
		// hex>, so 96 bytes of identity already puts the leaf at 129
		// characters before the repository prefix -- close enough to the
		// Windows 260-character path ceiling that the original reporter of
		// this issue was hitting it.
		return fmt.Sprintf("a change identity is at most %d bytes, this one is %d", maxSDDChangeIdentityBytes, len(change))
	case !utf8.ValidString(change):
		return "a change identity must be valid UTF-8"
	case change == "." || change == "..":
		return `a change identity cannot be "." or "..", because it names a directory rather than one`
	case strings.HasPrefix(change, "-"):
		// sdd-status already refuses a '-'-prefixed positional argument as an
		// unknown flag, so such an identity is reachable through one spelling
		// and not the other. Refusing it everywhere keeps one contract.
		return `a change identity cannot start with "-", because every documented invocation would read it as a flag`
	case strings.TrimSpace(change) != change:
		// sdd-status trims the requested name before resolving anything, so an
		// identity with surrounding whitespace can never be addressed there.
		// Windows also strips a trailing space from a directory name, which
		// would silently merge two identities into one ledger.
		return "a change identity cannot start or end with whitespace"
	case strings.HasSuffix(change, "."):
		// Windows strips a trailing dot from a path segment, so "fix." and
		// "fix" would name the same directory.
		return `a change identity cannot end with ".", because Windows strips it and two identities would share one directory`
	}
	for _, r := range change {
		if r < 0x20 || r == 0x7f {
			return "a change identity cannot contain control characters"
		}
		if strings.ContainsRune(sddChangeIdentityForbiddenRunes, r) {
			return fmt.Sprintf("a change identity cannot contain any of %s, because it must stay one addressable path segment on every platform", sddChangeIdentityForbiddenRunes)
		}
	}
	return ""
}

func validSDDChangeIdentity(change string) bool {
	return sddChangeIdentityRefusal(change) == ""
}

// sddChangeIdentityError names the rejected value, why it was rejected, and
// the command that reports the identity SDD actually resolved. The bare
// "invalid SDD change name" this replaces is what made #2116 take trial and
// error to diagnose across ten independent reports.
func sddChangeIdentityError(change, reason string) error {
	return fmt.Errorf(
		"invalid SDD change name %q: %s; run `gentle-ai sdd-status --cwd <repo> --json` to read the resolved changeName",
		change, reason,
	)
}

// sddChangeIdentityBlockedReason is the sdd-status projection of the same
// refusal. Status blocks rather than erroring, so the reason has to carry the
// exit with it: renaming the change is the only recovery, and doing it before
// artifacts exist is the whole point of refusing here.
func sddChangeIdentityBlockedReason(change, reason string) string {
	return fmt.Sprintf(
		"SDD change identity %q cannot be used: %s. Rename it to an identity every SDD surface accepts, then re-run status.",
		change, reason,
	)
}
