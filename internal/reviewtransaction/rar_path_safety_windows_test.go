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
