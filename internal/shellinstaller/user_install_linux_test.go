//go:build linux

package shellinstaller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func userFixture(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "shell")
}

func TestUserInstallSelection(t *testing.T) {
	dest := userFixture(t)
	cases := []struct {
		name string
		req  UserInstallRequest
		bad  bool
	}{
		{"separate", UserInstallRequest{Destination: dest, Mode: "separate"}, false},
		{"relative", UserInstallRequest{Destination: "shell", Mode: "separate"}, true},
		{"unclean", UserInstallRequest{Destination: dest + "/../shell", Mode: "separate"}, true},
		{"unknown mode", UserInstallRequest{Destination: dest, Mode: "link"}, true},
		{"shared missing selection", UserInstallRequest{Destination: dest, Mode: "shared"}, true},
		{"separate with shared prefix", UserInstallRequest{Destination: dest, Mode: "separate", SharedPrefix: dest}, true},
		{"control character", UserInstallRequest{Destination: dest + "\n", Mode: "separate"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateUserInstall(tc.req); (err != nil) != tc.bad {
				t.Fatalf("validation = %v, rejected want %v", err, tc.bad)
			}
		})
	}
}

func TestUserInstallCollisionPreserved(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dest := userFixture(t)
			sentinel := []byte("personal Pi preimage\n")
			switch kind {
			case "file":
				if err := os.WriteFile(dest, sentinel, 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(dest, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("absent-personal-target", dest); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(dest)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateUserInstall(UserInstallRequest{Destination: dest, Mode: "separate"}); err == nil {
				t.Fatal("collision accepted")
			}
			after, err := os.Lstat(dest)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("foreign collision changed: %v", err)
			}
		})
	}
}

func TestUserPhysicalSelection(t *testing.T) {
	dest := userFixture(t)
	parent := filepath.Dir(dest)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(parent, alias); err != nil {
		t.Fatal(err)
	}
	if err := ValidateUserInstall(UserInstallRequest{Destination: filepath.Join(alias, "shell"), Mode: "separate"}); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	if err := os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateUserInstall(UserInstallRequest{Destination: dest, Mode: "separate"}); err == nil {
		t.Fatal("nonprivate parent accepted")
	}
}

func TestUserInventoryAggregateBound(t *testing.T) {
	if testing.Short() {
		t.Skip("inventory boundaries hash sparse GiB-scale fixtures")
	}
	for _, tc := range []struct {
		name  string
		count int
		size  int64
		bad   bool
	}{
		{"above former aggregate", 9, 32 << 20, false},
		{"exact authorized aggregate", 32, 32 << 20, false},
		{"above authorized aggregate", 33, 32 << 20, true},
		{"individual file remains bounded", 1, (32 << 20) + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < tc.count; i++ {
				file, err := os.CreateTemp(root, "part-")
				if err != nil {
					t.Fatal(err)
				}
				err = file.Truncate(tc.size)
				if closeErr := file.Close(); err != nil || closeErr != nil {
					t.Fatal(errors.Join(err, closeErr))
				}
			}
			stamp, err := userTreeStamp(root)
			if (err != nil) != tc.bad {
				t.Fatalf("inventory = %v, rejected want %v", err, tc.bad)
			}
			if tc.bad {
				if !strings.Contains(err.Error(), "byte bound") {
					t.Fatalf("unrelated refusal: %v", err)
				}
			} else if again, err := userTreeStamp(root); err != nil || len(stamp) != 64 || again != stamp {
				t.Fatalf("unchanged inventory stamp differs: %v", err)
			}
		})
	}
}

func TestUserKernelStatus(t *testing.T) {
	qualified := "Uid:\t1000\t1000\t1000\t1000\nCapInh:\t0000000000000000\nCapPrm:\t0000000000000000\nCapEff:\t0000000000000000\nCapAmb:\t0000000000000000\nNoNewPrivs:\t1\n"
	for _, tc := range []struct {
		name, data string
		uid        int
		bad        bool
	}{
		{"normal UID", qualified, 1000, false},
		{"root", qualified, 0, true},
		{"effective capability", strings.Replace(qualified, "CapEff:\t0000000000000000", "CapEff:\t0000000000000001", 1), 1000, true},
		{"ambient capability", strings.Replace(qualified, "CapAmb:\t0000000000000000", "CapAmb:\t0000000000000001", 1), 1000, true},
		{"privilege escalation", strings.Replace(qualified, "NoNewPrivs:\t1", "NoNewPrivs:\t0", 1), 1000, true},
		{"missing status", "", 1000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := userKernelStatus(tc.data, tc.uid); (err != nil) != tc.bad {
				t.Fatalf("kernel status = %v, rejected want %v", err, tc.bad)
			}
		})
	}
}

func TestUserServiceLiteral(t *testing.T) {
	args := userServiceArgs("gentle-shell-0123456789abcdef", false, "/owned/supervisor", "/caller/project with spaces", []string{"internal-install", "/owned/shell", "separate"}, []string{"HOME=/owned/home"})
	joined := strings.Join(args, "\n")
	for _, literal := range []string{
		"--user", "--wait", "--collect", "--service-type=exec", "--expand-environment=no", "--pipe", "--working-directory=/caller/project with spaces",
		"--property=MemoryMax=3221225472", "--property=MemorySwapMax=0", "--property=CPUQuota=100%",
		"--property=CPUQuotaPeriodSec=100ms", "--property=TasksMax=64", "--property=NoNewPrivileges=yes",
		"--property=UMask=0077", "--property=KillMode=control-group", "--property=TimeoutStopSec=2s",
		"--property=UnsetEnvironment=LD_PRELOAD LD_LIBRARY_PATH LD_AUDIT NODE_OPTIONS NODE_PATH",
		"/usr/bin/env\n-i\nHOME=/owned/home\n/usr/bin/setpriv\n--inh-caps=-all\n--ambient-caps=-all\n--no-new-privs\n--\n/owned/supervisor\nshell\ninternal-install",
	} {
		if !strings.Contains(joined, literal) {
			t.Errorf("missing service literal %q", literal)
		}
	}
	if strings.Contains(joined, "--system") || strings.Contains(joined, "sudo") {
		t.Fatal("privileged service fallback")
	}
	interactive := userServiceArgs("gentle-shell-0123456789abcdef", true, "/owned/supervisor", "/caller/project", nil, nil)
	if !strings.Contains(strings.Join(interactive, "\n"), "--pty") {
		t.Fatal("interactive service has no PTY")
	}
}

func TestUserEnvironmentSealed(t *testing.T) {
	t.Setenv("GENTLE_PI_NO_SKILL_REGISTRY", "0")
	env := userEnvironment("/owned/shell", "/owned/prefix", "/owned/agent")
	for _, literal := range []string{
		"HOME=/owned/shell/home", "TMPDIR=/owned/shell/tmp", "PI_CODING_AGENT_DIR=/owned/agent", "GENTLE_PI_NO_SKILL_REGISTRY=1",
		"NPM_CONFIG_IGNORE_SCRIPTS=true", "npm_config_ignore_scripts=true", "NPM_CONFIG_PREFIX=/owned/prefix", "NODE_USE_SYSTEM_CA=1",
	} {
		if !containsUserEnv(env, literal) {
			t.Errorf("missing environment %q", literal)
		}
	}
	for _, item := range env {
		if strings.HasPrefix(item, "NODE_OPTIONS=") || strings.HasPrefix(item, "LD_") || strings.Contains(item, "TOKEN=") || strings.Contains(item, "PROXY=") {
			t.Fatalf("inherited injection: %q", item)
		}
	}
	if !reflect.DeepEqual(env, userEnvironment("/owned/shell", "/owned/prefix", "/owned/agent")) {
		t.Fatal("environment depends on caller configuration")
	}
}

func containsUserEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}

func TestUserInstallCanceledBeforeEffects(t *testing.T) {
	dest := userFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunUserInstall(ctx, UserInstallRequest{Destination: dest, Mode: "separate"})
	if err == nil {
		t.Fatal("canceled API accepted")
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatal("canceled API changed destination")
	}
}

func TestUserUncertainFinish(t *testing.T) {
	dest := userFixture(t)
	workspace, err := os.MkdirTemp(filepath.Dir(dest), ".user-test-")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := privateDirectory(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Fatal(err)
	}
	_, err = userFinish(context.Background(), UserInstallResult{}, errors.New("readback failed after publication"), workspace, identity, dest)
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "uncertain" {
		t.Fatalf("publication ambiguity lost: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("published evidence removed: %v", err)
	}
	_, err = userFinish(context.Background(), UserInstallResult{}, nil, workspace, identity, dest)
	if err == nil {
		t.Fatal("cleanup identity failure hidden")
	}
}

func TestUserFinishPreservesCleanupUncertainty(t *testing.T) {
	dest := userFixture(t)
	workspace, err := os.MkdirTemp(filepath.Dir(dest), ".user-test-")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := privateDirectory(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(workspace, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(workspace, 0700)
	_, err = userFinish(context.Background(), UserInstallResult{}, privateError("source", errors.New("digest differs")), workspace, identity, dest)
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "uncertain" || failure.Workspace != workspace {
		t.Fatalf("source failure hid cleanup uncertainty: %v", err)
	}
	if _, err := os.Lstat(workspace); err != nil {
		t.Fatal("uncertain evidence removed", err)
	}
}

func TestUserSupervisorPreimages(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "supervisor")
	if err := os.WriteFile(path, []byte("trusted-build DATA preimage"), 0700); err != nil {
		t.Fatal(err)
	}
	before, err := userSupervisorSHA(context.Background(), path)
	if err != nil || len(before) != 64 {
		t.Fatalf("owned physical preimage: %v", err)
	}
	if err := os.Chmod(path, 0770); err != nil {
		t.Fatal(err)
	}
	if _, err := userSupervisorSHA(context.Background(), path); err == nil {
		t.Fatal("group-writable controller accepted")
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(path), 0770); err != nil {
		t.Fatal(err)
	}
	_, err = userSupervisorSHA(context.Background(), path)
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "source" || failure.Cause == nil || !strings.Contains(failure.Cause.Error(), "supervisor ancestor refused") {
		t.Fatalf("unsafe controller ancestor lost refusal cause: %v", err)
	}
}

func TestUserSharedConsentBindsFiles(t *testing.T) {
	dest := userFixture(t)
	prefix, agent := filepath.Join(filepath.Dir(dest), "prefix"), filepath.Join(filepath.Dir(dest), "agent")
	cli := filepath.Join(prefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js")
	for _, directory := range []string{filepath.Dir(cli), agent} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(cli, []byte("selected Pi DATA"), 0600); err != nil {
		t.Fatal(err)
	}
	req := UserInstallRequest{Destination: dest, Mode: "shared", SharedPrefix: prefix, SharedAgent: agent}
	token, err := InspectUserInstall(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []struct{ path, bytes string }{
		{filepath.Join(agent, "settings.json"), "{}\n"},
		{cli, "changed Pi DATA"},
	} {
		if err := os.WriteFile(mutation.path, []byte(mutation.bytes), 0600); err != nil {
			t.Fatal(err)
		}
		next, err := InspectUserInstall(req)
		if err != nil || next == token {
			t.Fatalf("selected file mutation did not invalidate consent: %v", err)
		}
		token = next
	}
	if err := os.Symlink("/outside-selection", filepath.Join(agent, "foreign-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectUserInstall(req); err == nil {
		t.Fatal("escaping selected symlink accepted")
	}
}

func TestUserModernNativeRejectsForeignState(t *testing.T) {
	for _, name := range []string{"v3.7.0", ".v4.0.0.install.lock", "v4.0.0"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Dir(userFixture(t))
			if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
				t.Fatal(err)
			}
			if err := userNativeReadback(context.Background(), root); err == nil {
				t.Fatal("foreign/incomplete native state accepted")
			}
		})
	}
}

func TestUserRecoveryRequiresSelection(t *testing.T) {
	root := filepath.Dir(userFixture(t))
	if err := userRecover(context.Background(), []string{root, "inspect"}, io.Discard); err == nil {
		t.Fatal("missing recovery preimages accepted")
	}
	if err := userUnitReadback(filepath.Join(root, "absent.json")); err == nil {
		t.Fatal("missing unit receipt treated as termination proof")
	}
}

func TestUserRecoveryRejectsFinalPrefixDrift(t *testing.T) {
	root := filepath.Dir(userFixture(t))
	if err := os.Mkdir(filepath.Join(root, "state"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "relative", filepath.Join(root, "other")} {
		selection := map[string]string{"mode": "shared", "prefix": filepath.Join(root, "prefix"), "agent": filepath.Join(root, "agent"), "finalRoot": root, "finalPrefix": value}
		data, err := json.Marshal(selection)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "state/selection.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		err = userRecover(context.Background(), []string{root, "inspect"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "selection is absent or malformed") {
			t.Fatalf("unchecked final prefix %q reached recovery: %v", value, err)
		}
	}
}

func TestUserServiceDescriptor(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	for _, input := range []io.Reader{strings.NewReader(""), read} {
		if interactive, err := userInteractive(input); err != nil || interactive {
			t.Fatalf("nonterminal service selection: interactive=%v error=%v", interactive, err)
		}
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := userInteractive(read); err == nil {
		t.Fatal("invalid service descriptor silently downgraded")
	}
}

func TestUserLaunchInheritsProject(t *testing.T) {
	manifest := userManifest{Prefix: "/selected/prefix", Agent: "/selected/agent"}
	cmd := userLaunchCommand(context.Background(), "/owned/shell", manifest, "/selected/cli.js", []string{"--help"})
	if cmd.Dir != "" {
		t.Fatalf("ordinary launch overrides caller cwd: %q", cmd.Dir)
	}
	if !reflect.DeepEqual(cmd.Args, []string{"/owned/shell/runtime/node/bin/node", "/selected/cli.js", "--help"}) {
		t.Fatalf("selected runtime/CLI/arguments changed: %q", cmd.Args)
	}
	if !containsUserEnv(cmd.Env, "PI_CODING_AGENT_DIR=/selected/agent") {
		t.Fatal("launch lost selected agent")
	}
}

func TestUserForegroundNonterminal(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	for _, input := range []io.Reader{strings.NewReader(""), read} {
		cmd := &exec.Cmd{}
		restore, err := userForeground(cmd, input)
		if err != nil || restore == nil {
			t.Fatalf("nonterminal preparation: %v", err)
		}
		if !cmd.SysProcAttr.Setpgid || cmd.SysProcAttr.Foreground {
			t.Fatal("nonterminal must retain owned group without tty handoff")
		}
		if err := restore(); err != nil {
			t.Fatal(err)
		}
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := userForeground(&exec.Cmd{}, read); err == nil {
		t.Fatal("invalid terminal descriptor silently downgraded")
	}
}

func TestUserDelegatedManagerLaunch(t *testing.T) {
	if testing.Short() || os.Getenv("GENTLE_USER_MANAGER_GUEST") != "approved" {
		t.Skip("PENDING: actual externally provided delegated manager/PTY Guest")
	}
	self, root, project := os.Getenv("GENTLE_USER_SUPERVISOR"), os.Getenv("GENTLE_USER_INSTALLED"), os.Getenv("GENTLE_USER_PROJECT")
	if _, err := privatePhysical(self); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := userService(ctx, self, []string{"internal-launch", root}, os.Stdin, os.Stdout, os.Stderr); err != nil {
		t.Fatalf("manager normal launch/controller/termination readback: %v", err)
	}
}

// No synthetic manager satisfies this integration prerequisite. The external
// harness must provide a real, already-delegated >=254 user manager and a
// physical source-built supervisor; it must not create delegation for the test.
func TestUserDelegatedManagerIntegration(t *testing.T) {
	if testing.Short() || os.Getenv("GENTLE_USER_MANAGER_GUEST") != "approved" {
		t.Skip("PENDING: independently provided delegated user-manager Guest")
	}
	self := os.Getenv("GENTLE_USER_SUPERVISOR")
	if _, err := privatePhysical(self); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := userService(ctx, self, []string{"internal-check"}, os.Stdin, os.Stdout, os.Stderr); err != nil {
		t.Fatalf("real inner kernel qualification through user manager: %v", err)
	}
}

// Synthetic archive DATA only, never a runnable helper or execution witness.
func userToolFixture(t *testing.T, kind byte, copies int, size int) []byte {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for i := 0; i < copies; i++ {
		h := &tar.Header{Name: "fixed/tool", Typeflag: kind, Mode: 0700, Size: int64(size)}
		if kind != tar.TypeReg {
			h.Size, h.Linkname = 0, "elsewhere"
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write(make([]byte, size)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := errors.Join(tw.Close(), gz.Close()); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func TestUserToolArchiveAuthority(t *testing.T) {
	good := userToolFixture(t, tar.TypeReg, 1, 16)
	for _, tc := range []struct {
		name, member string
		data         []byte
		badPin       bool
		bad          bool
	}{
		{"regular", "fixed/tool", good, false, false},
		{"wrong pin", "fixed/tool", good, true, true},
		{"missing member", "fixed/missing", good, false, true},
		{"path escape", "../tool", good, false, true},
		{"duplicate", "fixed/tool", userToolFixture(t, tar.TypeReg, 2, 16), false, true},
		{"link", "fixed/tool", userToolFixture(t, tar.TypeSymlink, 1, 0), false, true},
		{"truncated", "fixed/tool", good[:len(good)-1], false, true},
		{"inflation", "fixed/tool", userToolFixture(t, tar.TypeReg, 1, (32<<20)+1), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pin := fmt.Sprintf("%x", sha256.Sum256(tc.data))
			if tc.badPin {
				pin = strings.Repeat("0", 64)
			}
			data, err := userToolMember(context.Background(), tc.data, pin, tc.member)
			if (err != nil) != tc.bad || (!tc.bad && !bytes.Equal(data, make([]byte, 16))) {
				t.Fatalf("archive authority: err=%v bytes=%d", err, len(data))
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := userToolMember(ctx, good, fmt.Sprintf("%x", sha256.Sum256(good)), "fixed/tool"); err == nil {
		t.Fatal("canceled archive admitted")
	}
}

func TestUserToolExclusivePhysicalReadback(t *testing.T) {
	path, data := filepath.Join(t.TempDir(), "tool"), []byte("synthetic DATA only")
	if err := userToolWrite(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userToolWrite(path, []byte("replacement"), 0700); err == nil {
		t.Fatal("occupied tool overwritten")
	}
	pin := fmt.Sprintf("%x", sha256.Sum256(data))
	if _, err := privateNativeFile(context.Background(), path, 0700, int64(len(data)), pin); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte{'X'}, len(data)), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := privateNativeFile(context.Background(), path, 0700, int64(len(data)), pin); err == nil {
		t.Fatal("helper byte drift admitted")
	}
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := privateNativeFile(context.Background(), path, 0700, int64(len(data)), pin); err == nil {
		t.Fatal("helper mode drift admitted")
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	alias := path + "-alias"
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := privateNativeFile(context.Background(), alias, 0700, int64(len(data)), pin); err == nil {
		t.Fatal("helper alias admitted")
	}
}
