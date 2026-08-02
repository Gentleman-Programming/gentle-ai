//go:build windows

package reviewtransaction

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRARSharedOwnerAcceptsOnlyCurrentWindowsPrincipals(t *testing.T) {
	currentUser, err := currentRARWindowsUserSID()
	if err != nil {
		t.Fatal(err)
	}
	tokenOwner, err := currentRARWindowsTokenOwnerSID()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		owner *windows.SID
		want  bool
	}{
		{name: "token user", owner: currentUser, want: true},
		{name: "token owner", owner: tokenOwner, want: true},
	}
	wellKnown := []struct {
		name string
		sid  windows.WELL_KNOWN_SID_TYPE
		want bool
	}{
		// Elevated shells and managed provisioning own repository
		// directories as BUILTIN\Administrators; SYSTEM services and CI
		// runners own theirs as LocalSystem. Both require administrative
		// privilege to forge, so both are trusted shared owners.
		{name: "BUILTIN Administrators", sid: windows.WinBuiltinAdministratorsSid, want: true},
		{name: "LocalSystem", sid: windows.WinLocalSystemSid, want: true},
		// Any standard user can hold these owners; accepting them would
		// let an attacker-controlled directory host review authority.
		{name: "Everyone", sid: windows.WinWorldSid, want: false},
		{name: "Authenticated Users", sid: windows.WinAuthenticatedUserSid, want: false},
	}
	for _, known := range wellKnown {
		sid, err := windows.CreateWellKnownSid(known.sid)
		if err != nil {
			t.Fatal(err)
		}
		tests = append(tests, struct {
			name  string
			owner *windows.SID
			want  bool
		}{name: known.name, owner: sid, want: known.want})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := rarWindowsDescriptorForOwner(t, test.owner)
			if got := rarSharedSecurityDescriptorOwnedByCurrentProcess(
				descriptor,
			); got != test.want {
				t.Fatalf("shared owner accepted = %t, want %t", got, test.want)
			}
		})
	}

	// CI runners hold an elevated token whose token owner IS
	// BUILTIN\Administrators, so the descriptor-level Administrators case
	// above passes there even without the administrative-owner acceptance.
	// These direct assertions prove the token-independent comparison itself,
	// which is what a non-elevated token (the reported onboarding wall)
	// exercises in production.
	t.Run("administrative trust is token independent", func(t *testing.T) {
		for _, trusted := range []windows.WELL_KNOWN_SID_TYPE{
			windows.WinBuiltinAdministratorsSid,
			windows.WinLocalSystemSid,
		} {
			sid, err := windows.CreateWellKnownSid(trusted)
			if err != nil {
				t.Fatal(err)
			}
			if !rarTrustedWindowsAdministrativeOwner(sid) {
				t.Fatalf("administrative owner %s was refused", sid)
			}
		}
		for _, foreign := range []windows.WELL_KNOWN_SID_TYPE{
			windows.WinWorldSid,
			windows.WinAuthenticatedUserSid,
			windows.WinLocalServiceSid,
			windows.WinNetworkServiceSid,
		} {
			sid, err := windows.CreateWellKnownSid(foreign)
			if err != nil {
				t.Fatal(err)
			}
			if rarTrustedWindowsAdministrativeOwner(sid) {
				t.Fatalf("forgeable owner %s was trusted", sid)
			}
		}
		if rarTrustedWindowsAdministrativeOwner(nil) {
			t.Fatal("nil owner was trusted")
		}
	})

	// A real directory created by this process is owned by the current user
	// or, under an elevated token, by BUILTIN\Administrators. Both shapes
	// must proceed through the real ACL read.
	t.Run("real directory proceeds", func(t *testing.T) {
		dir := t.TempDir()
		info, err := os.Lstat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !rarRepositoryDirectorySafe(dir, info) {
			t.Fatalf(
				"real repository parent %q was refused; owner is %s",
				dir, rarRepositoryOwnerDescription(dir),
			)
		}
	})

	// A trusted owner never excuses a reparse point: the redirection half of
	// the check must keep refusing even when the link and its target are
	// owned by an accepted principal.
	t.Run("reparse parent still refused", func(t *testing.T) {
		target := t.TempDir()
		link := filepath.Join(t.TempDir(), "git-common-link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("creating a directory symlink is unavailable: %v", err)
		}
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatal(err)
		}
		if rarRepositoryDirectorySafe(link, info) {
			t.Fatal("reparse-point repository parent was accepted")
		}
	})
}

func TestRARPrivateOwnerRemainsTokenUserOnly(t *testing.T) {
	descriptor, err := ownerOnlyRARSecurityDescriptor(false)
	if err != nil {
		t.Fatal(err)
	}
	if !privateRARSecurityDescriptorSafe(descriptor, false) {
		t.Fatal("current-user-only private descriptor was rejected")
	}

	tokenOwner, err := currentRARWindowsTokenOwnerSID()
	if err != nil {
		t.Fatal(err)
	}
	currentUser, err := currentRARWindowsUserSID()
	if err != nil {
		t.Fatal(err)
	}
	if tokenOwner.Equals(currentUser) {
		// The release blocker runs this test on an account whose token owner
		// differs from its token user; there the rebind class must be proven,
		// never skipped.
		if os.Getenv("GENTLE_AI_REQUIRE_DISTINCT_WINDOWS_TOKEN_OWNER") == "1" {
			t.Fatal(
				"release blocker requires a distinct Windows token owner; the rebind class was not exercised",
			)
		}
		t.Skip("token owner and token user are identical")
	}
	if privateRARSecurityDescriptorSafe(
		rarWindowsDescriptorForOwner(t, tokenOwner),
		false,
	) {
		t.Fatal("token-owner group was accepted for private RAR state")
	}
}

func rarWindowsDescriptorForOwner(
	t *testing.T,
	owner *windows.SID,
) *windows.SECURITY_DESCRIPTOR {
	t.Helper()
	if owner == nil || !owner.IsValid() || owner.String() == "" {
		t.Fatal("test owner SID is invalid")
	}
	sid := owner.String()
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + sid + "D:P(A;;GA;;;" + sid + ")",
	)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor == nil || !descriptor.IsValid() {
		t.Fatal("test security descriptor is invalid")
	}
	return descriptor
}
