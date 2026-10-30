//go:build linux || darwin || windows

package shellinstaller

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const userEngramReadyBody = `{"status":"ready","active_reuse_authorized":true,"capability":"startup-verified-store-v1","revision":1,"source":"active",` +
	`"instance_id":"0123456789abcdef0123456789abcdef","runtime_id":"fedcba9876543210fedcba9876543210"}`

// userEngramFakeHost records every effect the decision may cause.
type userEngramFakeHost struct {
	env       map[string]string
	endpoints []userEngramEndpoint
	core      bool
	header    uint32
	hasHeader bool
	startErr  error
	probes    int
	starts    []string
	headers   int
}

func (f *userEngramFakeHost) host() userEngramHost {
	return userEngramHost{
		lookup: func(key string) (string, bool) { value, ok := f.env[key]; return value, ok },
		probe: func(context.Context, time.Duration) userEngramEndpoint {
			endpoint := f.endpoints[min(f.probes, len(f.endpoints)-1)]
			f.probes++
			return endpoint
		},
		core: func() (string, bool) {
			if !f.core {
				return "", false
			}
			return "/agent/bin/engram", true
		},
		header: func() (uint32, bool) { f.headers++; return f.header, f.hasHeader },
		start: func(core string) (<-chan struct{}, error) {
			f.starts = append(f.starts, core)
			return make(chan struct{}), f.startErr
		},
		timeout: 300 * time.Millisecond, grace: 100 * time.Millisecond, interval: time.Millisecond,
	}
}

func userEngramPolicy(entries []string) string {
	for _, entry := range entries {
		if value, ok := strings.CutPrefix(entry, "ENGRAM_STARTUP_POLICY="); ok {
			return value
		}
	}
	return ""
}

// Bundled's rule for an explicit selection: forwarded untouched, memory
// disabled (keeping a user's own non-allow policy), and never a PATH engram:
// an absent ENGRAM_BIN names the verified core when it revalidates, otherwise
// the disabled policy keeps the companion from running any binary.
func TestUserEngramDecideSelectionNeverFallsBackToPath(t *testing.T) {
	disabled := []string{"ENGRAM_STARTUP_POLICY=disabled", "ENGRAM_STARTUP_NOTICE=" + userEngramRecoveryNotice}
	tests := []struct {
		name string
		env  map[string]string
		core bool
		want []string
	}{
		{"data directory with core", map[string]string{"ENGRAM_DATA_DIR": "/home/u/mem"}, true, append([]string{"ENGRAM_DATA_DIR=/home/u/mem", "ENGRAM_BIN=/agent/bin/engram"}, disabled...)},
		{"data directory without core", map[string]string{"ENGRAM_DATA_DIR": "/home/u/mem"}, false, append([]string{"ENGRAM_DATA_DIR=/home/u/mem"}, disabled...)},
		{"port with core", map[string]string{"ENGRAM_PORT": "7438"}, true, append([]string{"ENGRAM_PORT=7438", "ENGRAM_BIN=/agent/bin/engram"}, disabled...)},
		{"explicit URL keeps URL and gets the core", map[string]string{"ENGRAM_URL": "http://127.0.0.1:9000"}, true, append([]string{"ENGRAM_URL=http://127.0.0.1:9000", "ENGRAM_BIN=/agent/bin/engram"}, disabled...)},
		{"explicit BIN is never replaced", map[string]string{"ENGRAM_BIN": " /user/engram "}, true, append([]string{"ENGRAM_BIN= /user/engram "}, disabled...)},
		{"allow policy cannot enable a selection", map[string]string{"ENGRAM_PORT": "7438", "ENGRAM_STARTUP_POLICY": "allow"}, false, append([]string{"ENGRAM_PORT=7438"}, disabled...)},
		{"user notice is kept after ours", map[string]string{"ENGRAM_PORT": "7438", "ENGRAM_STARTUP_NOTICE": "mine"}, false, []string{"ENGRAM_PORT=7438", "ENGRAM_STARTUP_POLICY=disabled", "ENGRAM_STARTUP_NOTICE=" + userEngramRecoveryNotice + " mine"}},
		{"user's own policy is kept", map[string]string{"ENGRAM_STARTUP_POLICY": "off", "ENGRAM_STARTUP_NOTICE": "mine"}, true, []string{"ENGRAM_BIN=/agent/bin/engram", "ENGRAM_STARTUP_POLICY=off", "ENGRAM_STARTUP_NOTICE=mine"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.env["PATH"], tt.env["ENGRAM_HTTP_TOKEN"] = "/caller/bin", "secret"
			fake := &userEngramFakeHost{env: tt.env, endpoints: []userEngramEndpoint{userEngramCompatible}, core: tt.core}
			got := userEngramDecide(context.Background(), fake.host())
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("selection forwarded %q, want %q", got, tt.want)
			}
			if fake.probes != 0 || len(fake.starts) != 0 || fake.headers != 0 {
				t.Fatalf("an explicit selection must decide nothing: probes=%d starts=%v headers=%d", fake.probes, fake.starts, fake.headers)
			}
		})
	}
	// Blank selections select nothing.
	fake := &userEngramFakeHost{env: map[string]string{"ENGRAM_URL": "  ", "ENGRAM_STARTUP_POLICY": "allow"}, endpoints: []userEngramEndpoint{userEngramCompatible}}
	if got := userEngramDecide(context.Background(), fake.host()); !reflect.DeepEqual(got, userEngramAllow) || fake.probes != 1 {
		t.Fatalf("blank selection forwarded %q (probes %d), want the decided %q", got, fake.probes, userEngramAllow)
	}
}

func TestUserEngramDecideOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		fake      userEngramFakeHost
		policy    string
		starts    int
		wantNotes string
	}{
		{name: "compatible listener is reused", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramCompatible}, core: true}, policy: "allow"},
		{name: "listener without compatibility is never replaced", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramIncompatible}, core: true}, policy: "disabled", wantNotes: userEngramRecoveryNotice},
		{name: "unanswered probe is never replaced", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramUnknown}, core: true}, policy: "disabled", wantNotes: userEngramRecoveryNotice},
		{name: "refused without core starts nothing", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramRefused}}, policy: "disabled", wantNotes: userEngramCoreMissingNotice},
		{name: "newer store schema starts nothing", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramRefused}, core: true, header: userEngramCoreSchema + 1, hasHeader: true}, policy: "disabled", wantNotes: userEngramNewerSchemaNotice},
		{name: "refused with core starts and verifies", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramRefused, userEngramRefused, userEngramCompatible}, core: true, header: userEngramCoreSchema, hasHeader: true}, policy: "allow", starts: 1},
		{name: "refused with core and no store starts", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramRefused, userEngramCompatible}, core: true}, policy: "allow", starts: 1},
		{name: "start failure disables", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramRefused}, core: true, startErr: errors.New("exec failed")}, policy: "disabled", starts: 1, wantNotes: userEngramStartFailedNotice},
		{name: "never ready disables", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramRefused}, core: true}, policy: "disabled", starts: 1, wantNotes: userEngramStartFailedNotice},
		{name: "incompatible winner after start disables", fake: userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramRefused, userEngramIncompatible}, core: true}, policy: "disabled", starts: 1, wantNotes: userEngramStartFailedNotice},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := tt.fake
			got := userEngramDecide(context.Background(), fake.host())
			if userEngramPolicy(got) != tt.policy || len(fake.starts) != tt.starts {
				t.Fatalf("decision %q starts %v, want policy %q and %d starts", got, fake.starts, tt.policy, tt.starts)
			}
			if tt.policy == "allow" && !reflect.DeepEqual(got, userEngramAllow) {
				t.Fatalf("allowed decision %q, want exactly %q", got, userEngramAllow)
			}
			if tt.policy == "disabled" && !reflect.DeepEqual(got, []string{"ENGRAM_STARTUP_POLICY=disabled", "ENGRAM_STARTUP_NOTICE=" + tt.wantNotes}) {
				t.Fatalf("disabled decision %q, want the %q notice and no URL", got, tt.wantNotes)
			}
			for _, started := range fake.starts {
				if started != "/agent/bin/engram" {
					t.Fatalf("started %q, want the verified core", started)
				}
			}
		})
	}
}

func TestUserEngramDecideStopsPollingSoonAfterStartedCoreExits(t *testing.T) {
	fake := &userEngramFakeHost{endpoints: []userEngramEndpoint{userEngramRefused}, core: true}
	host := fake.host()
	host.timeout, host.grace = time.Minute, 50*time.Millisecond
	host.start = func(string) (<-chan struct{}, error) {
		exited := make(chan struct{})
		close(exited)
		return exited, nil
	}
	began := time.Now()
	if got := userEngramDecide(context.Background(), host); userEngramPolicy(got) != "disabled" {
		t.Fatalf("decision %q, want disabled", got)
	}
	if elapsed := time.Since(began); elapsed > 5*time.Second {
		t.Fatalf("polling took %v after the core exited, want the short grace", elapsed)
	}
}

func TestUserEngramProbe(t *testing.T) {
	serve := func(handler http.HandlerFunc) string {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		return server.URL
	}
	ready := serve(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compatibility" || r.Header.Get("Authorization") != "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(userEngramReadyBody))
	})
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refusedURL := "http://" + closed.Addr().String()
	_ = closed.Close()
	tests := []struct {
		name string
		base string
		want userEngramEndpoint
	}{
		{"ready compatibility", ready, userEngramCompatible},
		{"not ready", serve(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Replace(userEngramReadyBody, `"ready"`, `"starting"`, 1)))
		}), userEngramIncompatible},
		{"offline opening authorized", serve(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Replace(userEngramReadyBody, `"source":"active"`, `"source":"active","offline_automatic_opening_authorized":true`, 1)))
		}), userEngramIncompatible},
		{"short instance id", serve(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Replace(userEngramReadyBody, "0123456789abcdef0123456789abcdef", "0123", 1)))
		}), userEngramIncompatible},
		{"ready body with error status", serve(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(userEngramReadyBody))
		}), userEngramIncompatible},
		{"redirect to a ready server is not followed", serve(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, ready+"/compatibility", http.StatusFound)
		}), userEngramIncompatible},
		{"keys differ only in case", serve(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Replace(userEngramReadyBody, `"status"`, `"STATUS"`, 1)))
		}), userEngramIncompatible},
		{"oversized body", serve(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(userEngramReadyBody + strings.Repeat(" ", 32<<10)))
		}), userEngramIncompatible},
		{"nothing listens", refusedURL, userEngramRefused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := userEngramProbe(context.Background(), tt.base, time.Second); got != tt.want {
				t.Fatalf("probe = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("silent listener is unknown within the bound", func(t *testing.T) {
		silent, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = silent.Close() })
		began := time.Now()
		if got := userEngramProbe(context.Background(), "http://"+silent.Addr().String(), 200*time.Millisecond); got != userEngramUnknown {
			t.Fatalf("probe = %v, want unknown", got)
		}
		if time.Since(began) > 5*time.Second {
			t.Fatal("probe exceeded its bound")
		}
	})
}

func userEngramSQLiteHeader(pageSize uint16, userVersion uint32) []byte {
	header := make([]byte, 100)
	copy(header, "SQLite format 3\x00")
	binary.BigEndian.PutUint16(header[16:], pageSize)
	header[18], header[19] = 1, 1
	binary.BigEndian.PutUint32(header[60:], userVersion)
	return header
}

func TestUserEngramHeaderVersion(t *testing.T) {
	tests := []struct {
		name   string
		header []byte
		size   int64
		want   uint32
		ok     bool
	}{
		{"rollback database", userEngramSQLiteHeader(4096, 3), 8192, 3, true},
		{"64 KiB pages", userEngramSQLiteHeader(1, 1), 65536, 1, true},
		{"not sqlite", append([]byte("SQLite format 2\x00"), make([]byte, 84)...), 4096, 0, false},
		{"short header", userEngramSQLiteHeader(4096, 1)[:60], 4096, 0, false},
		{"page size not a power of two", userEngramSQLiteHeader(3000, 1), 6000, 0, false},
		{"size not a page multiple", userEngramSQLiteHeader(4096, 1), 5000, 0, false},
		{"bad file format", func() []byte { h := userEngramSQLiteHeader(4096, 1); h[18] = 3; return h }(), 4096, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := userEngramHeaderVersion(tt.header, tt.size)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("header version = %d,%v want %d,%v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestUserEngramReadHeader(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "engram.db")
	if _, ok := userEngramReadHeader(db); ok {
		t.Fatal("a missing store must have no header")
	}
	data := append(userEngramSQLiteHeader(4096, 2), make([]byte, 4096-100)...)
	if err := os.WriteFile(db, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := userEngramReadHeader(db); !ok || got != 2 {
		t.Fatalf("header = %d,%v want 2,true", got, ok)
	}
	if err := os.Mkdir(filepath.Join(dir, "dir.db"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, ok := userEngramReadHeader(filepath.Join(dir, "dir.db")); ok {
		t.Fatal("a directory must have no header")
	}
}

func TestUserEngramForwardedCarriesOnlyEngramLaunchKeys(t *testing.T) {
	env := map[string]string{"ENGRAM_URL": "http://127.0.0.1:7437", "ENGRAM_STARTUP_POLICY": "allow", "ENGRAM_STARTUP_NOTICE": "n", "ENGRAM_HTTP_TOKEN": "secret", "PATH": "/caller", "HOME": "/caller/home", "ENGRAM_PORT": ""}
	got := userEngramForwarded(func(key string) (string, bool) { value, ok := env[key]; return value, ok })
	if want := []string{"ENGRAM_URL=http://127.0.0.1:7437", "ENGRAM_STARTUP_POLICY=allow", "ENGRAM_STARTUP_NOTICE=n"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("forwarded %q, want %q", got, want)
	}
}

// userEngramObserveDecisions replaces the platform's launch decision with a
// recorder of the roots it ran for.
func userEngramObserveDecisions(t *testing.T) *[]string {
	t.Helper()
	original := userEngramLaunchEnv
	t.Cleanup(func() { userEngramLaunchEnv = original })
	roots := &[]string{}
	userEngramLaunchEnv = func(_ context.Context, root string) []string {
		*roots = append(*roots, root)
		return []string{"ENGRAM_URL=decided"}
	}
	return roots
}

// Pi arguments of a version or help run, and of runs that still decide.
var (
	userEngramQuietArgs    = [][]string{{"--version"}, {"-v"}, {"--help"}, {"-h"}, {"--model", "x", "-v"}, {"-p", "hi", "--help"}, {"--continue", "-h"}}
	userEngramDecidingArgs = [][]string{nil, {"--model", "-v"}, {"--", "--version"}, {"-e", "--help"}, {"-p", "hi"}, {"--versions"}, {"--version=1"}}
)

// Shell's piVersionOrHelpRequested (lib/gentle-shell-bundled.ts): a version
// or help token in flag position, before Pi's own "--", and not the value of
// one of Pi 1.1.0's value options.
func TestUserEngramVersionOrHelpPortsShellsTokenRule(t *testing.T) {
	for _, args := range userEngramQuietArgs {
		if !userEngramVersionOrHelp(args) {
			t.Errorf("%q is a version or help run", args)
		}
	}
	for _, args := range userEngramDecidingArgs {
		if userEngramVersionOrHelp(args) {
			t.Errorf("%q is not a version or help run", args)
		}
	}
	for _, option := range []string{"--provider", "--model", "--api-key", "--system-prompt", "--append-system-prompt", "--name", "-n", "--session",
		"--session-id", "--fork", "--session-dir", "--models", "--tools", "-t", "--exclude-tools", "-xt", "--thinking", "--export", "--extension", "-e",
		"--skill", "--prompt-template", "--theme"} {
		if userEngramVersionOrHelp([]string{option, "--version"}) || !userEngramVersionOrHelp([]string{option, "value", "--version"}) {
			t.Errorf("%s must take exactly the next token as its value", option)
		}
	}
	for _, option := range []string{"--print", "-p", "--continue", "-c", "--resume", "-r", "--no-tools", "--offline"} {
		if !userEngramVersionOrHelp([]string{option, "-h"}) {
			t.Errorf("%s takes no value, so a following -h is still a help run", option)
		}
	}
}

func TestUserEngramLaunchContext(t *testing.T) {
	if got := userEngramContextEnv(context.Background()); got != nil {
		t.Fatalf("a context without a launch decision forwarded %q", got)
	}
	calls := 0
	ctx := userEngramWithLaunch(context.Background(), func() []string { calls++; return []string{"ENGRAM_URL=x"} })
	if got := userEngramContextEnv(ctx); !reflect.DeepEqual(got, []string{"ENGRAM_URL=x"}) || calls != 1 {
		t.Fatalf("context env %q after %d calls", got, calls)
	}
}
