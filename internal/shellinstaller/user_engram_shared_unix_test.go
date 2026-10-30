//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// userEngramFakeCore records its pid, arguments and environment under its
// HOME, then stays alive like a server would.
const userEngramFakeCore = "#!/bin/sh\nenv > \"$HOME/core.env\"\nprintf '%s\\n' \"$*\" > \"$HOME/core.args\"\nprintf '%s\\n' \"$$\" > \"$HOME/core.pid.tmp\"\nmv \"$HOME/core.pid.tmp\" \"$HOME/core.pid\"\nexec sleep 30\n"

func userEngramPrivateDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// userEngramClearEnv removes the runner's own Engram configuration.
func userEngramClearEnv(t *testing.T) {
	t.Helper()
	for _, key := range append(append([]string{}, userEngramSelections...), userEngramPolicyEnv, userEngramNoticeEnv) {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

// userEngramSeparateFixture is a Separate root whose agent holds a fake core.
func userEngramSeparateFixture(t *testing.T, mode string) (root, core string) {
	t.Helper()
	root = filepath.Join(userEngramPrivateDir(t), "shell")
	agent := filepath.Join(root, "agent")
	for _, dir := range []string{root, agent, filepath.Join(agent, "bin")} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := json.Marshal(userManifest{Schema: userSchema, Destination: root, Prefix: filepath.Join(root, "prefix"), Agent: agent, Mode: mode})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "installation.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	core = filepath.Join(agent, "bin", "engram")
	if err := os.WriteFile(core, []byte(userEngramFakeCore), 0o700); err != nil {
		t.Fatal(err)
	}
	return root, core
}

func userEngramWaitPID(t *testing.T, home string, within time.Duration) (int, bool) {
	t.Helper()
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if data, err := os.ReadFile(filepath.Join(home, "core.pid")); err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			return pid, true
		}
	}
	return 0, false
}

func userEngramFreePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func userEngramServeReady(t *testing.T, address string) {
	t.Helper()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(userEngramReadyBody)) })}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
}

func userEngramUseProbeBase(t *testing.T, base string) {
	t.Helper()
	original := userEngramProbeBase
	userEngramProbeBase = base
	t.Cleanup(func() { userEngramProbeBase = original })
}

func TestUserEngramUnixStartDetachesWithRealHomeAndMinimalEnv(t *testing.T) {
	home := userEngramPrivateDir(t)
	_, core := userEngramSeparateFixture(t, "separate")
	t.Setenv("ENGRAM_DATA_DIR", "/caller/selection")
	t.Setenv("GH_TOKEN", "caller-secret")
	exited, err := userEngramUnixStart(core, home)
	if err != nil {
		t.Fatal(err)
	}
	pid, ok := userEngramWaitPID(t, home, 5*time.Second)
	if !ok {
		t.Fatal("fake core never started")
	}
	select {
	case <-exited:
		t.Fatal("fake core exited")
	default:
	}
	sid, err := unix.Getsid(pid)
	if err != nil || sid != pid {
		t.Fatalf("core session = %d (%v), want its own session %d", sid, err, pid)
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil || pgid != pid || pgid == syscall.Getpgrp() {
		t.Fatalf("core process group = %d (%v), want its own group outside the caller's %d", pgid, err, syscall.Getpgrp())
	}
	args, err := os.ReadFile(filepath.Join(home, "core.args"))
	if err != nil || string(args) != "serve\n" {
		t.Fatalf("core arguments = %q (%v), want serve", args, err)
	}
	data, err := os.ReadFile(filepath.Join(home, "core.env"))
	if err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		// The shell adds its own bookkeeping variables.
		if name, _, _ := strings.Cut(line, "="); name != "PWD" && name != "SHLVL" && name != "_" && name != "OLDPWD" {
			env = append(env, line)
		}
	}
	slices.Sort(env)
	if want := []string{"ENGRAM_NO_UPDATE_CHECK=1", "HOME=" + home, "PATH=/usr/bin:/bin"}; !reflect.DeepEqual(env, want) {
		t.Fatalf("core environment = %q, want exactly %q", env, want)
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("core did not survive its starter: %v", err)
	}
	if _, err := userEngramUnixStart(core, filepath.Join(home, "missing")); err == nil {
		t.Fatal("a missing real HOME started the core")
	}
}

func TestUserEngramUnixCoreRevalidation(t *testing.T) {
	root, core := userEngramSeparateFixture(t, "separate")
	agent := filepath.Join(root, "agent")
	if got, ok := userEngramUnixCore(context.Background(), agent); !ok || got != core {
		t.Fatalf("verified core = %q,%v want %q", got, ok, core)
	}
	if err := os.Chmod(core, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := userEngramUnixCore(context.Background(), agent); ok {
		t.Fatal("a 0755 core was accepted")
	}
	if err := os.Rename(core, core+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(core+".real", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(core+".real", core); err != nil {
		t.Fatal(err)
	}
	if _, ok := userEngramUnixCore(context.Background(), agent); ok {
		t.Fatal("a symlinked core was accepted")
	}
	if _, ok := userEngramUnixCore(context.Background(), filepath.Join(root, "missing")); ok {
		t.Fatal("a missing agent was accepted")
	}
}

func TestUserEngramUnixLaunchStartsSharedServerWhenNothingListens(t *testing.T) {
	userEngramClearEnv(t)
	home := userEngramPrivateDir(t)
	t.Setenv("HOME", home)
	root, _ := userEngramSeparateFixture(t, "separate")
	address := userEngramFreePort(t)
	userEngramUseProbeBase(t, "http://"+address)
	go func() {
		// The started core is a fake: once it runs, a ready server binds.
		for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(filepath.Join(home, "core.pid")); err == nil {
				userEngramServeReady(t, address)
				return
			}
		}
	}()
	got := userEngramUnixLaunch(context.Background(), root)
	if want := []string{"ENGRAM_STARTUP_POLICY=allow", "ENGRAM_URL=http://127.0.0.1:7437"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("launch entries = %q, want %q", got, want)
	}
	pid, ok := userEngramWaitPID(t, home, time.Second)
	if !ok {
		t.Fatal("the shared server was not started")
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("started server did not outlive the decision: %v", err)
	}
}

func TestUserEngramUnixLaunchNeverStartsBesideAnotherDecision(t *testing.T) {
	ready := func(t *testing.T) string {
		address := userEngramFreePort(t)
		userEngramServeReady(t, address)
		return "http://" + address
	}
	silent := func(t *testing.T) string {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })}
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(func() { _ = server.Close() })
		return "http://" + listener.Addr().String()
	}
	refused := func(t *testing.T) string { return "http://" + userEngramFreePort(t) }
	newer := append(userEngramSQLiteHeader(4096, userEngramCoreSchema+1), make([]byte, 4096-100)...)
	tests := []struct {
		name   string
		mode   string
		base   func(*testing.T) string
		store  []byte
		noCore bool
		user   string
		want   []string
	}{
		{name: "compatible listener", mode: "separate", base: ready, want: userEngramAllow},
		{name: "listener without compatibility", mode: "separate", base: silent, want: userEngramDisabled(userEngramRecoveryNotice)},
		{name: "refused without core", mode: "separate", base: refused, noCore: true, want: userEngramDisabled(userEngramCoreMissingNotice)},
		{name: "newer store schema", mode: "separate", base: refused, store: newer, want: userEngramDisabled(userEngramNewerSchemaNotice)},
		{name: "user selection", mode: "separate", base: refused, user: "/user/engram", want: append([]string{"ENGRAM_BIN=/user/engram"}, userEngramDisabled(userEngramRecoveryNotice)...)},
		{name: "shared mode decides nothing", mode: "shared", base: refused},
		{name: "agent mode decides nothing", mode: "agent", base: refused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userEngramClearEnv(t)
			home := userEngramPrivateDir(t)
			t.Setenv("HOME", home)
			if tt.user != "" {
				t.Setenv("ENGRAM_BIN", tt.user)
			}
			if tt.store != nil {
				if err := os.Mkdir(filepath.Join(home, ".engram"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, ".engram", "engram.db"), tt.store, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			root, core := userEngramSeparateFixture(t, tt.mode)
			if tt.noCore {
				if err := os.Remove(core); err != nil {
					t.Fatal(err)
				}
			}
			userEngramUseProbeBase(t, tt.base(t))
			if got := userEngramUnixLaunch(context.Background(), root); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("launch entries = %q, want %q", got, tt.want)
			}
			if _, started := userEngramWaitPID(t, home, 200*time.Millisecond); started {
				t.Fatal("a server was started")
			}
		})
	}
}

// With nothing listening a launch would start the verified core; a version or
// help run through the platform's launch entry starts nothing, leaves the
// real HOME without a store and gets memory disabled without a notice.
func TestUserEngramVersionAndHelpLaunchesStartNoServer(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"--help"}} {
		t.Run(args[0], func(t *testing.T) {
			userEngramClearEnv(t)
			home := userEngramPrivateDir(t)
			t.Setenv("HOME", home)
			root, _ := userEngramSeparateFixture(t, "separate")
			userEngramUseProbeBase(t, "http://"+userEngramFreePort(t))
			if env := userEngramEntryEnv(context.Background(), root, args); !reflect.DeepEqual(env, userEngramQuiet) {
				t.Fatalf("launch entries = %q, want %q", env, userEngramQuiet)
			}
			if _, started := userEngramWaitPID(t, home, 200*time.Millisecond); started {
				t.Fatal("a server was started")
			}
			if _, err := os.Lstat(filepath.Join(home, ".engram")); !os.IsNotExist(err) {
				t.Fatalf("the real HOME gained a store: %v", err)
			}
		})
	}
}

func TestUserLaunchCommandAddsOnlyTheEngramDecision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shell")
	t.Setenv("ENGRAM_URL", "http://caller.invalid")
	t.Setenv("ENGRAM_BIN", "/caller/engram")
	for _, mode := range []string{"separate", "shared", "agent"} {
		t.Run(mode, func(t *testing.T) {
			manifest := userManifest{Mode: mode, Prefix: filepath.Join(root, "prefix"), Agent: filepath.Join(root, "agent")}
			baseline := append(userEnvironment(root, manifest.Prefix, manifest.Agent), "GIT_OPTIONAL_LOCKS=0")
			// A Separate launch without a decision fails closed: the declared
			// companion would otherwise run an unverified PATH engram.
			want := baseline
			if mode == "separate" {
				want = append(append([]string{}, baseline...), userEngramDisabled(userEngramNoDecisionNotice)...)
			}
			if got := userLaunchCommand(context.Background(), root, manifest, "/not-executed/cli.js", nil).Env; !reflect.DeepEqual(got, want) {
				t.Fatalf("launch env without a decision = %q, want %q", got, want)
			}
			for _, decided := range [][]string{userEngramAllow, userEngramDisabled(userEngramRecoveryNotice), {"ENGRAM_BIN=/user/engram"}} {
				ctx := userEngramWithLaunch(context.Background(), func() []string { return decided })
				got := userLaunchCommand(ctx, root, manifest, "/not-executed/cli.js", nil).Env
				if want := append(append([]string{}, baseline...), decided...); !reflect.DeepEqual(got, want) {
					t.Fatalf("launch env = %q, want the unchanged baseline plus %q", got, decided)
				}
			}
		})
	}
}
