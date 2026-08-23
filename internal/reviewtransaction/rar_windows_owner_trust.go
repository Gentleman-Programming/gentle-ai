package reviewtransaction

import (
	"fmt"
	"strings"
)

// rarWindowsInheritDirectoryACEFlags is OBJECT_INHERIT_ACE|CONTAINER_INHERIT_ACE,
// the exact ACE inheritance flags gentle-ai writes on an owner-only RAR
// directory. A Windows-tagged test binds it to the real constants.
const rarWindowsInheritDirectoryACEFlags uint8 = 0x03

// rarWindowsOwnerOnlyACE is one access-control entry reduced to the facts the
// owner-only rule decides from, plus the raw values a refusal reports.
type rarWindowsOwnerOnlyACE struct {
	AccessAllowed bool
	Flags         uint8
	Mask          uint32
	// MaskAcceptable is ownerOnlyRARWindowsAccessMask's verdict. The typed
	// Windows comparison stays where it is; only its answer crosses over.
	MaskAcceptable bool
	SID            string
	// Malformed is non-empty when the raw entry could not be read safely.
	Malformed string
}

// rarWindowsOwnerOnlyDescriptor is a Windows security descriptor reduced to
// the facts the owner-only rule decides from.
type rarWindowsOwnerOnlyDescriptor struct {
	Readable      bool
	Control       uint16
	DACLPresent   bool
	DACLProtected bool
	DACLReadable  bool
	DACLDefaulted bool
	Owner         string
	ACECount      int
	// ACEs is the observed prefix of the DACL, not necessarily all ACECount
	// of them: a refusal needs the first entry and the count, not an
	// unbounded copy of somebody else's ACL.
	ACEs []rarWindowsOwnerOnlyACE
}

// rarWindowsOwnerOnlyMismatch returns "" when observed is exactly the
// owner-only descriptor gentle-ai writes for principal, and otherwise names
// the first fact that differs followed by everything it observed.
//
// This is the rule privateRARSecurityDescriptorSafe applies, moved out of the
// Windows file unchanged so it can be table-tested on any platform. It is also
// why the RAR directory repair can now say which field disagreed: #3376's
// readback could only report that the descriptor it had just written was "not
// owner-only", which on a platform this repository cannot execute locally is
// indistinguishable from every other reason the same call could refuse.
//
// The rule itself is not relaxed anywhere. The directory must still grant
// exactly one principal -- the one this process controls and has just been
// made the owner of -- through a single access-allowed entry, with the DACL
// protected so no inherited entry applies, and with a foreign owner or a
// foreign grantee refused.
func rarWindowsOwnerOnlyMismatch(
	observed rarWindowsOwnerOnlyDescriptor,
	principal string,
	directory bool,
) string {
	// Local, so the whole rule stays one function: a package-level helper
	// reachable only from Windows code is another entry in the deadcode
	// baseline for no gain.
	sid := func(value string) string {
		if strings.TrimSpace(value) == "" {
			return "<unreadable>"
		}
		return value
	}
	same := func(left, right string) bool {
		return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
	}

	wantFlags := uint8(0)
	if directory {
		wantFlags = rarWindowsInheritDirectoryACEFlags
	}
	var entry rarWindowsOwnerOnlyACE
	if len(observed.ACEs) > 0 {
		entry = observed.ACEs[0]
	}

	reason := ""
	switch {
	case !observed.Readable:
		reason = "the descriptor is unreadable or invalid"
	case !observed.DACLPresent:
		reason = "no DACL is present"
	case !observed.DACLProtected:
		reason = "the DACL is not protected, so inherited entries still apply"
	case strings.TrimSpace(principal) == "":
		reason = "the current Windows user SID is unavailable"
	case !same(observed.Owner, principal):
		reason = fmt.Sprintf("the owner is %s, want %s", sid(observed.Owner), principal)
	case !observed.DACLReadable:
		reason = "the DACL is present but unreadable"
	case observed.DACLDefaulted:
		reason = "the DACL is defaulted rather than explicitly set"
	case observed.ACECount != 1:
		reason = fmt.Sprintf("the DACL carries %d entries, want exactly 1", observed.ACECount)
	case len(observed.ACEs) == 0:
		reason = "the only DACL entry could not be observed"
	case !entry.AccessAllowed:
		reason = "the only DACL entry is not an access-allowed entry"
	case entry.Malformed != "":
		reason = "the only DACL entry is malformed: " + entry.Malformed
	case entry.Flags != wantFlags:
		reason = fmt.Sprintf("the only DACL entry has inheritance flags %#02x, want %#02x",
			entry.Flags, wantFlags)
	case !entry.MaskAcceptable:
		reason = fmt.Sprintf("the only DACL entry grants %#08x, which is neither GENERIC_ALL nor FILE_ALL_ACCESS",
			entry.Mask)
	case !same(entry.SID, principal):
		reason = fmt.Sprintf("the only DACL entry grants %s, want %s",
			sid(entry.SID), principal)
	}
	if reason == "" {
		return ""
	}

	// Everything observed follows the reason, not just the field that lost.
	// One more Windows run then closes whatever this turns out to be, instead
	// of peeling off one field per round.
	rendered := &strings.Builder{}
	fmt.Fprintf(rendered, "%s; observed control=%#04x present=%t protected=%t"+
		" readable=%t defaulted=%t owner=%s entries=%d",
		reason, observed.Control, observed.DACLPresent, observed.DACLProtected,
		observed.DACLReadable, observed.DACLDefaulted,
		sid(observed.Owner), observed.ACECount)
	for index, ace := range observed.ACEs {
		fmt.Fprintf(rendered, " [%d]{allowed=%t flags=%#02x mask=%#08x maskok=%t sid=%s",
			index, ace.AccessAllowed, ace.Flags, ace.Mask, ace.MaskAcceptable, sid(ace.SID))
		if ace.Malformed != "" {
			fmt.Fprintf(rendered, " malformed=%q", ace.Malformed)
		}
		rendered.WriteString("}")
	}
	fmt.Fprintf(rendered, " want owner=%s entry flags=%#02x", sid(principal), wantFlags)
	return rendered.String()
}

// rarWindowsTokenPrincipals is the token-derived evidence the Windows RAR
// directory repair decides ownership from.
//
// Every field is a string SID rather than a *windows.SID on purpose. The
// syscalls that gather these values cannot run anywhere but Windows, but the
// rule they feed is the part that has to be right, and keeping the rule a pure
// function over plain strings is what lets it be executed by tests on every
// platform instead of shipped on the strength of a successful compile.
type rarWindowsTokenPrincipals struct {
	// User is TOKEN_USER: the account this process runs as.
	User string

	// DefaultOwner is TOKEN_OWNER: the SID Windows stamps as the owner of
	// every object this token creates when the creator supplies no explicit
	// owner in the security descriptor. It is NOT always the token user --
	// see rarWindowsRepairOwnerControlled.
	DefaultOwner string

	// OwnerEligible is every group SID in the token carrying SE_GROUP_OWNER:
	// exactly the principals Windows itself will accept from this token as an
	// owner SID. Mere membership is not enough and must not be treated as
	// enough: a filtered, non-elevated administrator's token carries
	// BUILTIN\Administrators as SE_GROUP_USE_FOR_DENY_ONLY rather than as an
	// owner-eligible group, and such a process genuinely cannot take
	// ownership of an Administrators-owned directory.
	OwnerEligible []string
}

// rarWindowsRepairOwnerControlled reports whether an existing directory is
// owned by a principal this process controls, which is the property the
// bounded repair actually needs.
//
// The predicate #3376 shipped asked a narrower question -- is the owner SID
// equal to the token user, or to the token's default owner -- and answered the
// wrong one. Windows assigns the owner of a newly created object from the
// creating token's DEFAULT OWNER, never from the token user, unless the
// creator handed CreateDirectory an explicit owner. On a token that belongs to
// a member of the Administrators group and is running elevated, that default
// owner is BUILTIN\Administrators unless the "System objects: Default owner
// for objects created by members of the Administrators group" policy
// (HKLM\SYSTEM\CurrentControlSet\Control\Lsa\NoDefaultAdminOwner) has been
// switched to the object creator. So a directory this very process created
// moments ago is routinely owned by a SID that is not the token user, and a
// directory an earlier elevated run created is routinely owned by a SID that
// is neither the token user nor the current token's default owner. Refusing
// those is what makes the clone-scope kill switch -- the documented
// self-service delivery exit -- unable to self-heal on ordinary Windows
// machines, not just in CI.
//
// Accepting them is not a loosening, because the accepted set is defined by
// Windows rather than by this file: it is precisely the set of SIDs this token
// could itself stamp as an owner. A different user's SID is never in it. The
// squattable well-known groups this guard exists to refuse -- Everyone
// (S-1-1-0) and Authenticated Users (S-1-5-11) -- are never owner-eligible
// groups in any token. NT AUTHORITY\SYSTEM clears the bar only when this
// process actually runs as SYSTEM. And a genuinely foreign owner still refuses
// with the same message and the same operator repair (takeown / icacls
// /setowner) as before.
//
// Nothing downstream is widened either. Clearing this precondition only earns
// the directory a repair that rewrites its owner to the token user and
// reapplies the protected owner-only DACL; the caller then revalidates with
// the unchanged strict predicate, which still demands owner-only current-token
// user. A directory that clears this check and cannot be brought to that state
// is still refused.
func rarWindowsRepairOwnerControlled(owner string, token rarWindowsTokenPrincipals) bool {
	candidate := strings.TrimSpace(owner)
	if candidate == "" {
		return false
	}
	controlled := make([]string, 0, len(token.OwnerEligible)+2)
	controlled = append(controlled, token.User, token.DefaultOwner)
	controlled = append(controlled, token.OwnerEligible...)
	for _, principal := range controlled {
		principal = strings.TrimSpace(principal)
		if principal == "" {
			continue
		}
		if strings.EqualFold(candidate, principal) {
			return true
		}
	}
	return false
}
