package reviewtransaction

import (
	"strings"
	"testing"
)

// The syscall layer that gathers these SIDs is Windows-only and stays
// compile-only off Windows. The decision made from them is not: it is the part
// #3376 got wrong, so it runs here on every platform.
const (
	// A realistic machine-local account and a second account on the same
	// machine, which is the "genuinely foreign principal" this guard exists
	// to refuse.
	ownerTrustTokenUser   = "S-1-5-21-1004336348-1177238915-682003330-1001"
	ownerTrustForeignUser = "S-1-5-21-1004336348-1177238915-682003330-1002"

	ownerTrustAdministrators = "S-1-5-32-544" // BUILTIN\Administrators
	ownerTrustDomainAdmins   = "S-1-5-21-1004336348-1177238915-682003330-512"
	ownerTrustLocalSystem    = "S-1-5-18"     // NT AUTHORITY\SYSTEM
	ownerTrustLocalService   = "S-1-5-19"     // NT AUTHORITY\LOCAL SERVICE
	ownerTrustEveryone       = "S-1-1-0"      // Everyone
	ownerTrustAuthenticated  = "S-1-5-11"     // Authenticated Users
	ownerTrustUsers          = "S-1-5-32-545" // BUILTIN\Users
)

// elevatedAdministrator is the default shape of an elevated administrator's
// token: the default owner is the Administrators group, so every directory the
// process creates without an explicit owner comes out owned by that group and
// not by the account name in the profile path.
func elevatedAdministrator() rarWindowsTokenPrincipals {
	return rarWindowsTokenPrincipals{
		User:          ownerTrustTokenUser,
		DefaultOwner:  ownerTrustAdministrators,
		OwnerEligible: []string{ownerTrustAdministrators},
	}
}

// objectCreatorAdministrator is the same elevated token on a machine where
// NoDefaultAdminOwner has been switched to "object creator": new objects come
// out owned by the user, but the Administrators group stays owner-eligible, so
// a directory an earlier default-policy run created is still ours to repair.
func objectCreatorAdministrator() rarWindowsTokenPrincipals {
	return rarWindowsTokenPrincipals{
		User:          ownerTrustTokenUser,
		DefaultOwner:  ownerTrustTokenUser,
		OwnerEligible: []string{ownerTrustAdministrators, ownerTrustDomainAdmins},
	}
}

// filteredAdministrator is the UAC-filtered token of the same account: the
// Administrators group is present for deny-only evaluation and is NOT
// owner-eligible, so this process really cannot take ownership of an
// Administrators-owned directory and must not pretend it can.
func filteredAdministrator() rarWindowsTokenPrincipals {
	return rarWindowsTokenPrincipals{
		User:          ownerTrustTokenUser,
		DefaultOwner:  ownerTrustTokenUser,
		OwnerEligible: nil,
	}
}

func TestRARWindowsRepairOwnerControlled(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		token rarWindowsTokenPrincipals
		want  bool
	}{
		{
			name:  "token user owns it",
			owner: ownerTrustTokenUser,
			token: elevatedAdministrator(),
			want:  true,
		},
		{
			name:  "token default owner owns it, which is what CreateDirectory stamps",
			owner: ownerTrustAdministrators,
			token: elevatedAdministrator(),
			want:  true,
		},
		{
			name:  "an owner-eligible group owns it while the default owner is the user",
			owner: ownerTrustAdministrators,
			token: objectCreatorAdministrator(),
			want:  true,
		},
		{
			name:  "a second owner-eligible group owns it",
			owner: ownerTrustDomainAdmins,
			token: objectCreatorAdministrator(),
			want:  true,
		},
		{
			name:  "a filtered administrator cannot claim an Administrators-owned directory",
			owner: ownerTrustAdministrators,
			token: filteredAdministrator(),
			want:  false,
		},
		{
			name:  "another account on the same machine owns it",
			owner: ownerTrustForeignUser,
			token: elevatedAdministrator(),
			want:  false,
		},
		{
			name:  "Everyone owns it",
			owner: ownerTrustEveryone,
			token: elevatedAdministrator(),
			want:  false,
		},
		{
			name:  "Authenticated Users owns it",
			owner: ownerTrustAuthenticated,
			token: elevatedAdministrator(),
			want:  false,
		},
		{
			name:  "BUILTIN Users owns it",
			owner: ownerTrustUsers,
			token: elevatedAdministrator(),
			want:  false,
		},
		{
			name:  "LocalSystem owns it and this process is an ordinary administrator",
			owner: ownerTrustLocalSystem,
			token: elevatedAdministrator(),
			want:  false,
		},
		{
			name:  "LocalSystem owns it and this process runs as LocalSystem",
			owner: ownerTrustLocalSystem,
			token: rarWindowsTokenPrincipals{
				User:          ownerTrustLocalSystem,
				DefaultOwner:  ownerTrustAdministrators,
				OwnerEligible: []string{ownerTrustAdministrators},
			},
			want: true,
		},
		{
			name:  "LocalService owns it",
			owner: ownerTrustLocalService,
			token: elevatedAdministrator(),
			want:  false,
		},
		{
			name:  "no owner at all",
			owner: "",
			token: elevatedAdministrator(),
			want:  false,
		},
		{
			name:  "blank owner text",
			owner: "   ",
			token: elevatedAdministrator(),
			want:  false,
		},
		{
			name:  "an unreadable token accepts nothing",
			owner: ownerTrustTokenUser,
			token: rarWindowsTokenPrincipals{},
			want:  false,
		},
		{
			name:  "a token with only blank principals accepts nothing",
			owner: "   ",
			token: rarWindowsTokenPrincipals{
				User:          "",
				DefaultOwner:  "  ",
				OwnerEligible: []string{"", " "},
			},
			want: false,
		},
		{
			name:  "SID text differing only in case is the same principal",
			owner: "s-1-5-32-544",
			token: elevatedAdministrator(),
			want:  true,
		},
		{
			name:  "surrounding whitespace does not change the principal",
			owner: "  " + ownerTrustAdministrators + "  ",
			token: elevatedAdministrator(),
			want:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := rarWindowsRepairOwnerControlled(test.owner, test.token); got != test.want {
				t.Fatalf("owner %q controlled by %+v = %t, want %t",
					test.owner, test.token, got, test.want)
			}
		})
	}
}

// The security property the whole guard exists for: whatever the token looks
// like, a principal that is not in it is never accepted. A widening that lost
// this would be worse than the bug it fixes.
func TestRARWindowsRepairOwnerRefusesEveryPrincipalOutsideTheToken(t *testing.T) {
	tokens := map[string]rarWindowsTokenPrincipals{
		"elevated administrator":    elevatedAdministrator(),
		"object-creator policy":     objectCreatorAdministrator(),
		"filtered administrator":    filteredAdministrator(),
		"unreadable token":          {},
		"service running as SYSTEM": {User: ownerTrustLocalSystem, DefaultOwner: ownerTrustLocalSystem},
	}
	foreign := []string{
		ownerTrustForeignUser,
		ownerTrustEveryone,
		ownerTrustAuthenticated,
		ownerTrustUsers,
		ownerTrustLocalService,
		"S-1-5-21-9999999999-9999999999-9999999999-500",
		"S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464", // NT SERVICE\TrustedInstaller
	}

	for name, token := range tokens {
		for _, owner := range foreign {
			if !rarWindowsRepairOwnerControlled(owner, token) {
				continue
			}
			// Only a principal the token genuinely carries may pass.
			carried := owner == token.User || owner == token.DefaultOwner
			for _, eligible := range token.OwnerEligible {
				carried = carried || owner == eligible
			}
			if !carried {
				t.Fatalf("%s accepted foreign owner %s", name, owner)
			}
		}
	}
}

// ownerOnlyDirectory is exactly what gentle-ai writes for an owner-only RAR
// directory, as it reads back from Windows: protected DACL, this process's
// principal as owner, one inheritable access-allowed entry granting that same
// principal FILE_ALL_ACCESS.
func ownerOnlyDirectory() rarWindowsOwnerOnlyDescriptor {
	return rarWindowsOwnerOnlyDescriptor{
		Readable:      true,
		Control:       0x9014,
		DACLPresent:   true,
		DACLProtected: true,
		DACLReadable:  true,
		Owner:         ownerTrustTokenUser,
		ACECount:      1,
		ACEs: []rarWindowsOwnerOnlyACE{{
			AccessAllowed:  true,
			Flags:          rarWindowsInheritDirectoryACEFlags,
			Mask:           0x001f01ff,
			MaskAcceptable: true,
			SID:            ownerTrustTokenUser,
		}},
	}
}

func withOwnerOnlyDirectory(
	mutate func(*rarWindowsOwnerOnlyDescriptor),
) rarWindowsOwnerOnlyDescriptor {
	observed := ownerOnlyDirectory()
	mutate(&observed)
	return observed
}

func TestRARWindowsOwnerOnlyMismatch(t *testing.T) {
	tests := []struct {
		name      string
		observed  rarWindowsOwnerOnlyDescriptor
		principal string
		directory bool
		accept    bool
		names     string
	}{
		{
			name:      "the descriptor gentle-ai writes for a directory",
			observed:  ownerOnlyDirectory(),
			principal: ownerTrustTokenUser,
			directory: true,
			accept:    true,
		},
		{
			name: "a directory still carrying the unmapped generic mask",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACEs[0].Mask = 0x10000000
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			accept:    true,
		},
		{
			name: "the descriptor gentle-ai writes for a file",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACEs[0].Flags = 0
			}),
			principal: ownerTrustTokenUser,
			accept:    true,
		},
		{
			name:      "the descriptor could not be read",
			observed:  rarWindowsOwnerOnlyDescriptor{},
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "unreadable or invalid",
		},
		{
			name: "no DACL is present",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.DACLPresent = false
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "no DACL is present",
		},
		{
			name: "the DACL is not protected, so inherited entries survive",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.DACLProtected = false
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "not protected",
		},
		{
			name:      "this process has no principal to compare against",
			observed:  ownerOnlyDirectory(),
			principal: "  ",
			directory: true,
			names:     "current Windows user SID is unavailable",
		},
		{
			name: "another principal owns it",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.Owner = ownerTrustAdministrators
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "the owner is " + ownerTrustAdministrators,
		},
		{
			name: "the owner could not be read",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.Owner = ""
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "the owner is <unreadable>",
		},
		{
			name: "the DACL is present but unreadable",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.DACLReadable = false
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "present but unreadable",
		},
		{
			name: "the DACL is defaulted",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.DACLDefaulted = true
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "defaulted",
		},
		{
			name: "an inherited entry survived alongside ours",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACECount = 2
				observed.ACEs = append(observed.ACEs, rarWindowsOwnerOnlyACE{
					AccessAllowed:  true,
					Flags:          0x13,
					Mask:           0x001f01ff,
					MaskAcceptable: true,
					SID:            ownerTrustAdministrators,
				})
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "carries 2 entries",
		},
		{
			name: "the DACL is empty",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACECount = 0
				observed.ACEs = nil
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "carries 0 entries",
		},
		{
			name: "the entry count and the observed entries disagree",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACEs = nil
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "could not be observed",
		},
		{
			name: "the only entry denies rather than allows",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACEs[0].AccessAllowed = false
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "not an access-allowed entry",
		},
		{
			name: "the only entry is malformed",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACEs[0].Malformed = "the entry is too small to carry a SID"
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "too small to carry a SID",
		},
		{
			name: "the entry was marked inherited, so protection did not hold",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACEs[0].Flags = 0x13
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "inheritance flags 0x13, want 0x03",
		},
		{
			name: "a directory entry lost its inheritance flags",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACEs[0].Flags = 0
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "inheritance flags 0x00, want 0x03",
		},
		{
			name:      "a file entry carries directory inheritance flags",
			observed:  ownerOnlyDirectory(),
			principal: ownerTrustTokenUser,
			names:     "inheritance flags 0x03, want 0x00",
		},
		{
			name: "the entry grants less than full access",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACEs[0].Mask = 0x00120089
				observed.ACEs[0].MaskAcceptable = false
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "grants 0x00120089",
		},
		{
			name: "the entry grants somebody else",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.ACEs[0].SID = ownerTrustForeignUser
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			names:     "entry grants " + ownerTrustForeignUser,
		},
		{
			name: "SID text differing only in case is the same principal",
			observed: withOwnerOnlyDirectory(func(observed *rarWindowsOwnerOnlyDescriptor) {
				observed.Owner = strings.ToLower(ownerTrustTokenUser)
				observed.ACEs[0].SID = strings.ToLower(ownerTrustTokenUser)
			}),
			principal: ownerTrustTokenUser,
			directory: true,
			accept:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := rarWindowsOwnerOnlyMismatch(test.observed, test.principal, test.directory)
			if test.accept {
				if got != "" {
					t.Fatalf("descriptor was refused: %s", got)
				}
				return
			}
			if got == "" {
				t.Fatal("descriptor was accepted, want a refusal")
			}
			if !strings.Contains(got, test.names) {
				t.Fatalf("refusal %q does not name %q", got, test.names)
			}
			// Every refusal carries the whole observation, so one CI round
			// closes the question instead of one field per round.
			for _, fact := range []string{"observed control=", "present=", "protected=",
				"owner=", "entries=", "want owner="} {
				if !strings.Contains(got, fact) {
					t.Fatalf("refusal %q does not report %q", got, fact)
				}
			}
		})
	}
}
