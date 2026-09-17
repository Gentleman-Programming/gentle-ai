package communitytool

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

const rtkStatusTestLimit = 64 << 10

type rtkRecordingRunner struct {
	home      string
	available *bool
	commands  []string
	envs      []map[string]string
	fail      error
}

func (r *rtkRecordingRunner) Run(name string, args ...string) error {
	return r.RunWithEnv(nil, name, args...)
}
func (r *rtkRecordingRunner) RunWithEnv(env map[string]string, name string, args ...string) error {
	r.commands = append(r.commands, strings.Join(append([]string{name}, args...), " "))
	r.envs = append(r.envs, env)
	*r.available = true
	switch strings.Join(args, " ") {
	case "init -g --auto-patch":
		rtkMustWrite(r.home, ".claude/RTK.md", "RTK\n")
		rtkMustWrite(r.home, ".claude/CLAUDE.md", "@RTK.md\n")
		rtkMustWrite(r.home, ".claude/settings.json", `{"hooks":{"PreToolUse":["rtk hook claude"]}}`)
	case "init -g --opencode":
		rtkMustWrite(r.home, ".config/opencode/plugins/rtk.ts", "tool.execute.before rtk rewrite\n")
	case "init -g --codex":
		rtkMustWrite(r.home, ".codex/RTK.md", "RTK\n")
		rtkMustWrite(r.home, ".codex/AGENTS.md", filepath.Join(r.home, ".codex", "RTK.md"))
	case "init -g --agent pi --auto-patch":
		rtkMustWrite(r.home, ".pi/agent/extensions/rtk.ts", `exec("rtk", ["rewrite", cmd])`)
	}
	return r.fail
}

func TestRTKInstallClientHasFiniteTimeout(t *testing.T) {
	client, ok := rtkHTTPClientForInstall.(*http.Client)
	if !ok || client.Timeout <= 0 {
		t.Fatalf("RTK client = %#v, want finite-timeout *http.Client", rtkHTTPClientForInstall)
	}
}

func TestRTKInstallPinnedArchiveConfiguresDetectedAgentsAndDisablesTelemetry(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{filepath.Join(home, ".claude"), filepath.Join(home, ".config", "opencode"), filepath.Join(home, ".codex"), filepath.Join(home, ".pi", "agent")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	archive := rtkTarGz(t, "rtk", "binary")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	t.Cleanup(server.Close)
	restoreRTKInstallTestHooks(t, server.URL, archive)
	available := false
	runner := &rtkRecordingRunner{home: home, available: &available}
	result, err := InstallWithHome(model.CommunityToolRTK, "", home, runner, DetectorFunc(func(string) (string, error) {
		if available {
			return filepath.Join(home, ".local", "bin", "rtk"), nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusAfter == nil || result.StatusAfter.CLI != AvailabilityAvailable {
		t.Fatalf("status after = %#v", result.StatusAfter)
	}
	want := []string{
		filepath.Join(home, ".local", "bin", "rtk") + " init -g --auto-patch",
		filepath.Join(home, ".local", "bin", "rtk") + " init -g --opencode",
		filepath.Join(home, ".local", "bin", "rtk") + " init -g --codex",
		filepath.Join(home, ".local", "bin", "rtk") + " init -g --agent pi --auto-patch",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
	for _, env := range runner.envs {
		if env["RTK_TELEMETRY_DISABLED"] != "1" || env["HOME"] != home || env["XDG_CONFIG_HOME"] != filepath.Join(home, ".config") {
			t.Fatalf("environment = %#v, want isolated telemetry-disabled RTK child", env)
		}
	}
	for _, agent := range result.StatusAfter.Agents {
		if !agent.Configured || agent.Status != AgentStatusConfigured {
			t.Fatalf("agent status = %#v", agent)
		}
	}
}

func TestRTKInstallUsesAndReusesAttestedDestinationInsteadOfPATH(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := rtkTarGz(t, "rtk", "binary")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; _, _ = w.Write(archive) }))
	t.Cleanup(server.Close)
	restoreRTKInstallTestHooks(t, server.URL, archive)
	available := false
	runner := &rtkRecordingRunner{home: home, available: &available}
	external := filepath.Join(home, "external", "rtk")
	detector := DetectorFunc(func(string) (string, error) { return external, nil })
	var second Result
	for run := 0; run < 2; run++ {
		result, err := InstallWithHome(model.CommunityToolRTK, "", home, runner, detector)
		if err != nil {
			t.Fatalf("install run %d: %v", run+1, err)
		}
		second = result
	}
	if len(second.ManualActions) != 1 || !strings.Contains(second.ManualActions[0], `export PATH="$HOME/.local/bin:$PATH"`) {
		t.Fatalf("external PATH shadow must retain manual PATH guidance: %#v", second.ManualActions)
	}
	installed := rtkInstallPath(home)
	if requests != 1 {
		t.Fatalf("download requests = %d, want one first-install acquisition", requests)
	}
	if !reflect.DeepEqual(runner.commands, []string{installed + " init -g --auto-patch", installed + " init -g --auto-patch"}) {
		t.Fatalf("setup commands = %#v, want attested destination %q and never %q", runner.commands, installed, external)
	}
}

func TestRTKScopedInstallConfiguresOnlySelectedDetectedAgent(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{filepath.Join(home, ".claude"), filepath.Join(home, ".config", "opencode"), filepath.Join(home, ".codex"), filepath.Join(home, ".pi", "agent")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	archive := rtkTarGz(t, "rtk", "binary")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	t.Cleanup(server.Close)
	restoreRTKInstallTestHooks(t, server.URL, archive)
	available := false
	runner := &rtkRecordingRunner{home: home, available: &available}
	_, err := InstallWithHomeAndAgents(model.CommunityToolRTK, "", home, []model.AgentID{model.AgentOpenCode}, runner, DetectorFunc(func(string) (string, error) {
		if available {
			return rtkInstallPath(home), nil
		}
		return "", errors.New("not found")
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runner.commands, []string{rtkInstallPath(home) + " init -g --opencode"}) {
		t.Fatalf("commands = %v, want only selected OpenCode", runner.commands)
	}
	if paths := RTKManagedPathsForAgents(home, []model.AgentID{model.AgentOpenCode}); slices.Contains(paths, filepath.Join(home, ".claude", "RTK.md")) || !slices.Contains(paths, filepath.Join(home, ".config", "opencode", "plugins", "rtk.ts")) {
		t.Fatalf("scoped paths = %v", paths)
	}
	if _, err = InstallWithHomeAndAgents(model.CommunityToolRTK, "", t.TempDir(), []model.AgentID{}, &rtkRecordingRunner{available: new(bool)}, DetectorFunc(func(string) (string, error) { return "", errors.New("not found") })); err == nil || !strings.Contains(err.Error(), "no selected supported detected agents") {
		t.Fatalf("empty scope error = %v", err)
	}
}

func TestRTKInstallRollsBackTheBinaryAndAgentFilesOnSetupFailure(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "user Claude instructions\n"
	mustWrite(t, filepath.Join(home, ".claude", "CLAUDE.md"), original)
	archive := rtkTarGz(t, "rtk", "binary")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	t.Cleanup(server.Close)
	restoreRTKInstallTestHooks(t, server.URL, archive)
	available := false
	_, err := InstallWithHome(model.CommunityToolRTK, "", home, &rtkRecordingRunner{home: home, available: &available, fail: errors.New("setup failed")}, DetectorFunc(func(string) (string, error) { return "", errors.New("not found") }))
	if err == nil || !strings.Contains(err.Error(), "setup failed") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "rtk")); !os.IsNotExist(err) {
		t.Fatalf("RTK binary remains after rollback: %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if readErr != nil || string(data) != original {
		t.Fatalf("Claude instructions after rollback = %q, %v; want %q", data, readErr, original)
	}
}

func TestRTKInstallRejectsUnverifiableExistingBinary(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{name: "directory", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "non executable", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("binary"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "unverifiable executable", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlink", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(filepath.Dir(path), "other")
			if err := os.WriteFile(target, []byte("binary"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			withRTKRuntime(t, "linux", "amd64")
			test.setup(t, rtkInstallPath(home))
			available := false
			_, err := InstallWithHome(model.CommunityToolRTK, "", home, &rtkRecordingRunner{home: home, available: &available}, DetectorFunc(func(string) (string, error) { return "", errors.New("not found") }))
			if err == nil || !strings.Contains(err.Error(), "cannot verify existing RTK") {
				t.Fatalf("error = %v, want actionable existing-binary rejection", err)
			}
		})
	}
}

func TestRTKStatusBoundsUnsafeAgentFiles(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{name: "directory", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "oversized", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(strings.Repeat("x", rtkStatusTestLimit+1)), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlink", setup: func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(filepath.Dir(path), "user-plugin.ts")
			if err := os.WriteFile(target, []byte("tool.execute.before rtk rewrite"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			withRTKRuntime(t, "linux", "amd64")
			path := filepath.Join(home, ".config", "opencode", "plugins", "rtk.ts")
			test.setup(t, path)
			status := DetectStatus(model.CommunityToolRTK, home, DetectorFunc(func(string) (string, error) { return "/bin/rtk", nil }))
			if len(status.Agents) != 1 || status.Agents[0].Configured {
				t.Fatalf("status = %#v, want missing unsafe OpenCode status", status)
			}
		})
	}
}

func TestRTKManagedPathsIgnoreAmbientXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "outside"))
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths := RTKManagedPaths(home)
	want := filepath.Join(home, ".config", "opencode", "plugins", "rtk.ts")
	if !slices.Contains(paths, want) {
		t.Fatalf("managed paths = %v, want %q", paths, want)
	}
}

func TestRTKStatusRequiresAttestedManagedPATHResolution(t *testing.T) {
	home := t.TempDir()
	withRTKRuntime(t, "linux", "amd64")
	binary := []byte("verified-rtk")
	withRTKExecutableIdentity(t, binary)
	managed := rtkInstallPath(home)
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(home, ".codex", "RTK.md"), "RTK\n")
	mustWrite(t, filepath.Join(home, ".codex", "AGENTS.md"), filepath.Join(home, ".codex", "RTK.md"))
	external := filepath.Join(home, "external", "rtk")
	if err := os.MkdirAll(filepath.Dir(external), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(external, binary, 0o700); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name      string
		resolved  string
		lookupErr error
		available bool
	}{
		{name: "exact managed destination", resolved: managed, available: true},
		{name: "clean absolute managed destination", resolved: filepath.Join(filepath.Dir(managed), "..", "bin", "rtk"), available: true},
		{name: "home relative managed destination", resolved: filepath.Join(".local", "bin", "..", "bin", "rtk"), available: true},
		{name: "missing PATH", lookupErr: errors.New("not found")},
		{name: "external shadow", resolved: external},
	} {
		t.Run(test.name, func(t *testing.T) {
			status := DetectStatus(model.CommunityToolRTK, home, DetectorFunc(func(string) (string, error) { return test.resolved, test.lookupErr }))
			if got := status.CLI == AvailabilityAvailable; got != test.available {
				t.Fatalf("CLI availability = %v, status=%#v, want %v", got, status, test.available)
			}
			if len(status.Agents) != 1 || status.Agents[0].Configured != test.available {
				t.Fatalf("agent status = %#v, want configured=%v", status.Agents, test.available)
			}
		})
	}
}

func TestRTKStatusDoesNotClaimConfiguredWhenLocalBinaryIsNotOnPath(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(home, ".codex", "RTK.md"), "RTK\n")
	mustWrite(t, filepath.Join(home, ".codex", "AGENTS.md"), filepath.Join(home, ".codex", "RTK.md"))
	withRTKRuntime(t, "linux", "amd64")
	status := DetectStatus(model.CommunityToolRTK, home, DetectorFunc(func(string) (string, error) { return "", errors.New("not found") }))
	if status.CLI != AvailabilityMissing || len(status.Agents) != 1 || status.Agents[0].Configured {
		t.Fatalf("status = %#v", status)
	}
	if status.Agents[0].Reason != "RTK is not effective on PATH" {
		t.Fatalf("reason = %q", status.Agents[0].Reason)
	}
}

func TestRTKAcquireTarGzRejectsUnsafeContent(t *testing.T) {
	for _, name := range []string{"../rtk", "/rtk", "rtk/", "other"} {
		t.Run(name, func(t *testing.T) {
			archive := rtkTarGz(t, name, "binary")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
			defer server.Close()
			asset := rtkTestAsset(server.URL, archive)
			asset.Name, asset.ExecutableMember = "rtk-linux.tar.gz", "rtk"
			if _, err := acquireRTKCandidate(server.Client(), t.TempDir(), asset); err == nil {
				t.Fatal("error = nil")
			}
		})
	}
}

func rtkMustWrite(home, relative, content string) {
	path := filepath.Join(home, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		panic(err)
	}
}

func rtkTarGz(t *testing.T, name, data string) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	header := &tar.Header{Name: name, Mode: 0o700, Size: int64(len(data)), Typeflag: tar.TypeReg}
	if strings.HasSuffix(name, "/") {
		header.Size, header.Typeflag = 0, tar.TypeDir
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if header.Typeflag == tar.TypeReg {
		if _, err := io.WriteString(tarWriter, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func withRTKExecutableIdentity(t *testing.T, binary []byte) {
	t.Helper()
	oldAssets := rtkCandidateAssets
	sum := sha256.Sum256(binary)
	rtkCandidateAssets = []rtkCandidateAsset{{Platform: rtkPlatformLinuxAMD64, ExecutableMember: "rtk", ExecutableSizeBytes: int64(len(binary)), ExecutableSHA256: hex.EncodeToString(sum[:])}}
	t.Cleanup(func() { rtkCandidateAssets = oldAssets })
}

func withRTKRuntime(t *testing.T, goos, goarch string) {
	t.Helper()
	oldOS, oldArch := rtkGOOS, rtkGOARCH
	rtkGOOS, rtkGOARCH = goos, goarch
	t.Cleanup(func() { rtkGOOS, rtkGOARCH = oldOS, oldArch })
}

func restoreRTKInstallTestHooks(t *testing.T, url string, archive []byte) {
	t.Helper()
	oldAssets, oldClient := rtkCandidateAssets, rtkHTTPClientForInstall
	withRTKRuntime(t, "linux", "amd64")
	sum := sha256.Sum256(archive)
	binarySum := sha256.Sum256([]byte("binary"))
	rtkCandidateAssets = []rtkCandidateAsset{{Platform: rtkPlatformLinuxAMD64, Name: "rtk-linux.tar.gz", URL: url, ExecutableMember: "rtk", SizeBytes: int64(len(archive)), SHA256: hex.EncodeToString(sum[:]), ChecksumSHA256: rtkChecksumsSHA256, ExecutableSizeBytes: int64(len("binary")), ExecutableSHA256: hex.EncodeToString(binarySum[:])}}
	rtkHTTPClientForInstall = http.DefaultClient
	t.Cleanup(func() { rtkCandidateAssets, rtkHTTPClientForInstall = oldAssets, oldClient })
}
