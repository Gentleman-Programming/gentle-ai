//go:build windows

package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"golang.org/x/sys/windows"
)

// managedInstallDisplayTarget is how the detection screen shows the default
// managed install created by platformManagedInstallFixture (USERPROFILE=home).
const managedInstallDisplayTarget = `~\AppData\Local\gentle-shell\install`

// platformManagedInstallFixture builds the Windows-shaped managed Separate
// installation that DiscoverUserShellInstalls finds: the fixed
// LOCALAPPDATA\gentle-shell\install target, private and owned by the current
// user, with a canonical manifest.json bound to selection.json and the
// retained lock. Field order mirrors the product manifest so the canonical
// byte comparison holds; the fixture is validated through real discovery.
// Only the persona and background preference files exist, so the loaded
// profile equals the Unix fixture's (Gentleman, background ON, every optional
// choice and CodeGraph inherited) and the shared Manage expectations hold on
// both platforms. Optional files are absent, which the Windows loader treats
// as inherited.
func platformManagedInstallFixture(t *testing.T) (home, target string, ok bool) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	local := filepath.Join(home, "AppData", "Local")
	t.Setenv("LOCALAPPDATA", local)
	base := filepath.Join(local, "gentle-shell")
	target = filepath.Join(base, "install")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	windowsTestPrivateDir(t, base)
	windowsTestPrivateDir(t, target)
	for _, dir := range []string{"config", "agent", "source"} {
		if err := os.Mkdir(filepath.Join(target, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	selection, err := json.Marshal(shellinstaller.UserInstallRequest{Destination: target, Mode: "separate", Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	lock := []byte(`{"lockfileVersion":3,"packages":{}}`)
	manifest := struct {
		Schema, Destination, SID, Identity, SupervisorSHA string
		PrefixIdentity, AgentIdentity                     string
		SelectionSHA                                      string
		MainCommit, MainArchiveSHA, LockSHA               string
	}{
		Schema: "gentle-shell-windows-separate/v1", Destination: target, SID: windowsTestSID(t),
		Identity: windowsTestIdentity(t, target), SupervisorSHA: "fixture-supervisor",
		SelectionSHA: windowsTestSHA(selection), LockSHA: windowsTestSHA(lock),
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for relative, data := range map[string][]byte{
		`config\persona.json`:              []byte(`{"mode":"gentleman"}`),
		`config\background-subagents.json`: []byte(`{"schema":"gentle-pi.background-subagents/v1","policy":"on"}`),
		`source\package-lock.json`:         lock,
		`selection.json`:                   selection,
		`manifest.json`:                    append(encoded, '\n'),
	} {
		if err := os.WriteFile(filepath.Join(target, relative), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := shellinstaller.DiscoverUserShellInstalls(home)
	if len(got) != 1 || got[0].Target != target || got[0].Mode != "separate" {
		t.Fatalf("Windows fixture is not a discoverable managed installation: %+v", got)
	}
	if profile, err := shellinstaller.LoadUserShellExperience(got[0]); err != nil || profile.Persona != "gentleman" || profile.BackgroundSubagents != "on" || profile.StrictTDD != "" || profile.GlobalRDD != "" || profile.Context7 != "" || profile.CodeGraph != "" {
		t.Fatalf("Windows fixture profile does not load as the default install: %+v %v", profile, err)
	}
	return home, target, true
}

// platformSetInstalledCodeGraph records the installed CodeGraph component the
// way a completed Windows install does: the optional runtime\codegraph tree
// that LoadUserShellExperience reads. Windows has no installation.json bit.
func platformSetInstalledCodeGraph(t *testing.T, target string, on bool) bool {
	t.Helper()
	runtimeDir := filepath.Join(target, "runtime", "codegraph")
	if on {
		if err := os.MkdirAll(runtimeDir, 0700); err != nil {
			t.Fatal(err)
		}
	} else if err := os.RemoveAll(runtimeDir); err != nil {
		t.Fatal(err)
	}
	want := ""
	if on {
		want = "on"
	}
	if profile, err := shellinstaller.LoadUserShellExperience(shellinstaller.UserShellInstall{Target: target, Mode: "separate"}); err != nil || profile.CodeGraph != want {
		t.Fatalf("installed CodeGraph state did not load: %+v %v", profile, err)
	}
	return true
}

// shellDefaultTestDestination is the default Separate target the TUI selects.
func shellDefaultTestDestination() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "gentle-shell", "install")
}

// assertShellPrivateParent requires the default install parent to be a
// directory owned by the current user: Windows custody is owner and DACL,
// not POSIX mode bits.
func assertShellPrivateParent(t *testing.T, base string) {
	t.Helper()
	info, err := os.Lstat(base)
	if err != nil || !info.IsDir() {
		t.Fatalf("default parent is not a directory: %v %v", info, err)
	}
	sd, err := windows.GetNamedSecurityInfo(base, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || owner.String() != windowsTestSID(t) {
		t.Fatalf("default parent is not owned by the current user: %v %v", owner, err)
	}
}

// The Windows counterpart of TestShellExperienceNonPrivateParentRefusesBeforeConsent:
// a default parent that grants another account mutation access is refused by
// the real inspection before any plan, token or change to the parent.
func TestShellExperienceForeignWritableParentRefusesBeforeConsentWindows(t *testing.T) {
	stubShellKernelGate(t) // The real inspection (not the OS guard) must refuse the parent.
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	base := filepath.Join(local, "gentle-shell")
	if err := os.Mkdir(base, 0700); err != nil {
		t.Fatal(err)
	}
	// Everyone (WD) may modify, which includes creating and deleting children.
	windowsTestSetDACL(t, base, "D:P(A;OICI;FA;;;"+windowsTestSID(t)+")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1301bf;;;WD)")
	before := windowsTestDACL(t, base)
	m := NewModel(system.DetectionResult{}, "dev").WithShellInstaller()
	m.Width, m.Height = 120, 30
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnd})
	m = experienceKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.shellPlan != nil || m.shellExperience == nil || m.shellExperience.err == nil || m.shellPlanRequest.Confirmation != "" {
		t.Fatalf("foreign-writable default parent acquired inspection or consent: %+v", m.shellExperience)
	}
	if !strings.Contains(m.shellExperience.err.Error(), "another account mutation access") {
		t.Fatalf("refusal does not name the foreign access: %v", m.shellExperience.err)
	}
	if after := windowsTestDACL(t, base); after != before {
		t.Fatalf("refusal changed the default parent DACL: %s -> %s", before, after)
	}
	if _, err := os.Lstat(filepath.Join(base, "install")); !os.IsNotExist(err) {
		t.Fatal("refusal created the destination", err)
	}
}

func windowsTestSID(t *testing.T) string {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return user.User.Sid.String()
}

func windowsTestSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// windowsTestIdentity mirrors the installer's volume/file identity string.
func windowsTestIdentity(t *testing.T, path string) string {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow)
}

// windowsTestPrivateDir applies the installer's private DACL: the current
// user, SYSTEM and Administrators only, inherited by new children.
func windowsTestPrivateDir(t *testing.T, path string) {
	t.Helper()
	windowsTestSetDACL(t, path, "D:P(A;OICI;FA;;;"+windowsTestSID(t)+")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
}

func windowsTestSetDACL(t *testing.T, path, sddl string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func windowsTestDACL(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}
