//go:build windows

package reviewtransaction

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
		{name: "BUILTIN Administrators", sid: windows.WinBuiltinAdministratorsSid, want: true},
		{name: "LocalSystem", sid: windows.WinLocalSystemSid, want: true},
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
			if got := rarSharedSecurityDescriptorOwnedByCurrentProcess(descriptor); got != test.want {
				t.Fatalf("shared owner accepted = %t, want %t", got, test.want)
			}
		})
	}

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

	t.Run("real directory proceeds", func(t *testing.T) {
		dir := t.TempDir()
		info, err := os.Lstat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !rarRepositoryDirectorySafe(dir, info) {
			t.Fatalf("real repository parent %q was refused", dir)
		}
	})

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

// TestRARWindowsOwnerOnlyConstantsMatchWindows binds the constants the pure
// rule is table-tested against to the real Windows values, so the
// cross-platform table cannot drift away from the platform it describes.
func TestRARWindowsOwnerOnlyConstantsMatchWindows(t *testing.T) {
	wantFlags := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
	if rarWindowsInheritDirectoryACEFlags != wantFlags {
		t.Fatalf("directory ACE flags = %#04x, want %#04x",
			rarWindowsInheritDirectoryACEFlags, wantFlags)
	}
	// The written mask and the accepted masks have to be the same access set,
	// which is the whole reason the SDDL may say FA instead of GA.
	if !ownerOnlyRARWindowsAccessMask(windows.GENERIC_ALL) {
		t.Fatal("GENERIC_ALL is not an accepted owner-only mask")
	}
	const fileAllAccess = windows.ACCESS_MASK(0x001f01ff)
	if !ownerOnlyRARWindowsAccessMask(fileAllAccess) {
		t.Fatal("FILE_ALL_ACCESS is not an accepted owner-only mask")
	}
	if ownerOnlyRARWindowsAccessMask(windows.FILE_GENERIC_READ) {
		t.Fatal("a read-only mask is accepted as owner-only")
	}
}

// TestRARWindowsOwnerOnlyDescriptorRoundTrips proves the descriptor the repair
// writes is the descriptor the validator accepts, through the real SDDL
// builder and the real observation layer rather than a hand-built table.
func TestRARWindowsOwnerOnlyDescriptorRoundTrips(t *testing.T) {
	for _, directory := range []bool{false, true} {
		descriptor, err := ownerOnlyRARSecurityDescriptor(directory)
		if err != nil {
			t.Fatal(err)
		}
		if mismatch := privateRARSecurityDescriptorMismatch(descriptor, directory); mismatch != "" {
			t.Fatalf("the descriptor gentle-ai writes (directory=%t) is refused by its own rule: %s",
				directory, mismatch)
		}
		if !privateRARSecurityDescriptorSafe(descriptor, directory) {
			t.Fatalf("mismatch and safe disagree for directory=%t", directory)
		}
	}
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
		if os.Getenv("GENTLE_AI_REQUIRE_DISTINCT_WINDOWS_TOKEN_OWNER") == "1" {
			t.Fatal("release blocker requires a distinct Windows token owner")
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

// TestRARWindowsTokenPrincipalsDescribeThisProcess covers the half of the
// ownership decision that cannot be executed off Windows: reading the real
// token. The rule applied to what it reads is covered cross-platform by
// TestRARWindowsRepairOwnerControlled.
func TestRARWindowsTokenPrincipalsDescribeThisProcess(t *testing.T) {
	token, err := currentRARWindowsTokenPrincipals()
	if err != nil {
		t.Fatal(err)
	}
	if token.User == "" || token.DefaultOwner == "" {
		t.Fatalf("token principals are incomplete: %+v", token)
	}
	for _, owned := range []string{token.User, token.DefaultOwner} {
		if !rarWindowsRepairOwnerControlled(owned, token) {
			t.Fatalf("this process does not control its own principal %s", owned)
		}
	}

	// A directory this process creates without an explicit owner comes out
	// owned by the token's default owner, so the repair must accept exactly
	// that SID. This is the invariant #3376's predicate could not express.
	dir := filepath.Join(t.TempDir(), "created-by-this-process")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.GetNamedSecurityInfo(
		dir,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatal(err)
	}
	controlled, err := rarWindowsDescriptorOwnerControlled(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if !controlled {
		owner, _, ownerErr := descriptor.Owner()
		t.Fatalf("a directory this process just created is owned by %v (err %v), "+
			"which %+v does not control", owner, ownerErr, token)
	}

	// The squattable well-known groups stay outside the accepted set whatever
	// this token turns out to be.
	for _, forgeable := range []windows.WELL_KNOWN_SID_TYPE{
		windows.WinWorldSid,
		windows.WinAuthenticatedUserSid,
	} {
		sid, err := windows.CreateWellKnownSid(forgeable)
		if err != nil {
			t.Fatal(err)
		}
		if rarWindowsRepairOwnerControlled(sid.String(), token) {
			t.Fatalf("forgeable owner %s is treated as controlled", sid)
		}
	}

	// A foreign account SID derived from this machine's own domain is the
	// closest thing to a real attacker-owned directory a test can build.
	if rarWindowsRepairOwnerControlled(
		"S-1-5-21-1004336348-1177238915-682003330-31337",
		token,
	) {
		t.Fatal("a foreign account SID is treated as controlled")
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

// ignoreRequestedRARDescriptor reproduces a filesystem or token that accepts the
// descriptor handed to CreateDirectory and then ignores it.
func ignoreRequestedRARDescriptor(t *testing.T) {
	t.Helper()
	previous := rarPrivateDirectoryCreate
	rarPrivateDirectoryCreate = func(name *uint16, _ *windows.SecurityAttributes) error {
		return windows.CreateDirectory(name, nil)
	}
	t.Cleanup(func() { rarPrivateDirectoryCreate = previous })
}

func TestCreatePrivateRARDirectoryRepairsADescriptorThatDidNotStick(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1")
	ignoreRequestedRARDescriptor(t)

	// A nil error is the revalidation: createPrivateRARDirectory only reports
	// success once validatePrivateRARDirectory accepted the repaired handle.
	created, err := createPrivateRARDirectory(path)
	if err != nil || !created {
		t.Fatalf("createPrivateRARDirectory on a mount that ignored the descriptor = (%t, %v), want (true, nil)", created, err)
	}
}

func TestCreatePrivateRARDirectoryRepairsWhatAnInterruptedRunLeftBehind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1")
	// What a run killed between CreateDirectory and its repair leaves behind: a
	// directory carrying inherited ACLs. CreateDirectory answers every later
	// attempt with ERROR_ALREADY_EXISTS, so a `created` gate never repairs it.
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	created, err := createPrivateRARDirectory(path)
	if err != nil || created {
		t.Fatalf("invocation after an interrupted run = (%t, %v), want (false, nil)", created, err)
	}
}

func TestCreatePrivateRARDirectoryKeepsTheUnsafePathItNamesOnWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1")
	ignoreRequestedRARDescriptor(t)
	previous := rarPrivateDirectoryRepair
	// A repair that reports success and changes nothing, the way a mount
	// without persistent ACL semantics behaves.
	rarPrivateDirectoryRepair = func(string, fs.FileMode) error { return nil }
	t.Cleanup(func() { rarPrivateDirectoryRepair = previous })

	created, err := createPrivateRARDirectory(path)
	var unsafePath *UnsafeRARPathError
	if created || !errors.As(err, &unsafePath) || unsafePath.Path != path || !unsafePath.Directory {
		t.Fatalf("createPrivateRARDirectory = (%t, %v), want (false, *UnsafeRARPathError naming %q)", created, err, path)
	}
	// Removing the refused directory is what made the printed repair unrunnable.
	if _, statErr := os.Lstat(path); statErr != nil {
		t.Fatalf("refused path %q is gone, so the printed repair cannot run: %v", path, statErr)
	}
}

// classifierStub records every path it receives and returns a fixed filesystem type.
type classifierStub struct {
	calls []string
	give  string
}

func (s *classifierStub) classify(path string) string {
	s.calls = append(s.calls, path)
	return s.give
}

// ownerStubAlwaysReject forces rarRepositoryDirectorySafe to return false.
func ownerStubAlwaysReject(path string, info fs.FileInfo) bool {
	return false
}

func TestRARWindowsAuthorityFilesystemClassifier(t *testing.T) {
	origOwner := rarWindowsAuthorityOwnerUnsafe
	t.Cleanup(func() { rarWindowsAuthorityOwnerUnsafe = origOwner })
	rarWindowsAuthorityOwnerUnsafe = ownerStubAlwaysReject

	stub := &classifierStub{give: "NTFS"}
	origClassifier := rarWindowsAuthorityFilesystemClassifier
	t.Cleanup(func() { rarWindowsAuthorityFilesystemClassifier = origClassifier })
	rarWindowsAuthorityFilesystemClassifier = stub.classify

	dir := t.TempDir()

	tests := []struct {
		name            string
		path            string
		fsType          string
		wantBase        error
		wantUnknown     bool
		wantUnsupported bool
		wantSubStr      string
		wantNilStr      string
	}{
		{"NTFS → ACL guidance", "", "NTFS", errUnsafeRARAuthorityPath, false, false, "takeown", "exFAT"},
		{"ReFS → ACL guidance", "", "ReFS", errUnsafeRARAuthorityPath, false, false, "icacls", "exFAT"},
		{"exFAT → unsupported", "", "exFAT", errUnsafeRARAuthorityPath, false, true, "exFAT", "takeown"},
		{"FAT32 → unsupported", "", "FAT32", errUnsafeRARAuthorityPath, false, true, "FAT32", "icacls"},
		{"empty → unknown", "", "", errUnsafeRARAuthorityPath, true, false, "", "takeown"},
		{"FOOFS → unknown", "", "FOOFS", errUnsafeRARAuthorityPath, true, false, "", "takeown"},
		{"UNC → unknown", `\\server\share`, "NTFS", errUnsafeRARAuthorityPath, true, false, "", "takeown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testDir := dir
			if tc.path != "" {
				testDir = tc.path + `\temp`
			}
			stub.give = tc.fsType
			stub.calls = nil

			err := validateRARRepositoryParent(testDir)

			if !errors.Is(err, tc.wantBase) {
				t.Fatalf("err = %v, want errors.Is(_, %v)", err, tc.wantBase)
			}
			if tc.wantUnknown && !errors.Is(err, errUnknownWindowsFilesystem) {
				t.Fatalf("err = %v, want errUnknownWindowsFilesystem", err)
			}
			if tc.wantUnsupported && !errors.Is(err, errUnsupportedWindowsFilesystem) {
				t.Fatalf("err = %v, want errUnsupportedWindowsFilesystem", err)
			}
			if tc.wantSubStr != "" && !strings.Contains(err.Error(), tc.wantSubStr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSubStr)
			}
			if tc.wantNilStr != "" && strings.Contains(err.Error(), tc.wantNilStr) {
				t.Fatalf("error %q unexpectedly contains %q", err.Error(), tc.wantNilStr)
			}
			if tc.path == "" && (len(stub.calls) != 1 || stub.calls[0] != testDir) {
				t.Fatalf("classifier called with calls=%v, want [%q]", stub.calls, testDir)
			}
		})
	}
}

func TestRARWindowsAuthorityFilesystemClassifierFS5WorktreeOnExFAT(t *testing.T) {
	stub := &classifierStub{give: "exFAT"}
	origClassifier := rarWindowsAuthorityFilesystemClassifier
	t.Cleanup(func() { rarWindowsAuthorityFilesystemClassifier = origClassifier })
	rarWindowsAuthorityFilesystemClassifier = stub.classify

	dir := t.TempDir()
	err := validateRARRepositoryParent(dir)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(stub.calls) > 0 {
		t.Fatalf("classifier was called for trusted-owner path: calls=%v", stub.calls)
	}
}
