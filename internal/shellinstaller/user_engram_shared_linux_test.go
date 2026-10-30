//go:build linux

package shellinstaller

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUserEngramLaunchContextOnlyForLaunch(t *testing.T) {
	original := userEngramLaunchEnv
	t.Cleanup(func() { userEngramLaunchEnv = original })
	var roots []string
	userEngramLaunchEnv = func(_ context.Context, root string) []string {
		roots = append(roots, root)
		return []string{"ENGRAM_URL=decided"}
	}
	for _, args := range [][]string{{"install", "/owned"}, {"internal-install", "/owned"}, {"check"}, {"recover", "/owned"}, {"launch-pnpm", "/owned", "pi"}, {"native-coordinate", "/owned"}, {"launch"}, {"internal-launch", "/owned"}} {
		if env := userEngramContextEnv(userEngramLaunchContext(context.Background(), args)); env != nil || len(roots) != 0 {
			t.Fatalf("%q ran the decision: env %q roots %q", args, env, roots)
		}
	}
	env := userEngramContextEnv(userEngramLaunchContext(context.Background(), []string{"launch", "/owned", "--flag"}))
	if !reflect.DeepEqual(env, []string{"ENGRAM_URL=decided"}) || !reflect.DeepEqual(roots, []string{"/owned"}) {
		t.Fatalf("launch forwarded %q for roots %q", env, roots)
	}
}

// Inside the unit the worker forwards exactly the Engram entries the outer
// entry put in the unit environment, and nothing else from it.
func TestUserEngramLinuxWorkerForwardsOnlyEngramEntries(t *testing.T) {
	userEngramClearEnv(t)
	t.Setenv("ENGRAM_STARTUP_POLICY", "allow")
	t.Setenv("ENGRAM_URL", userEngramDefaultURL)
	t.Setenv("ENGRAM_HTTP_TOKEN", "secret")
	t.Setenv("GENTLE_AI_MAIN_FALLBACK", "1")
	root := filepath.Join(t.TempDir(), "shell")
	manifest := userManifest{Mode: "separate", Prefix: filepath.Join(root, "prefix"), Agent: filepath.Join(root, "agent")}
	baseline := append(userEnvironment(root, manifest.Prefix, manifest.Agent), "GIT_OPTIONAL_LOCKS=0")
	got := userLaunchCommand(userEngramLaunchScope(context.Background(), root), root, manifest, "/not-executed/cli.js", nil).Env
	if want := append(append([]string{}, baseline...), "ENGRAM_URL="+userEngramDefaultURL, "ENGRAM_STARTUP_POLICY=allow"); !reflect.DeepEqual(got, want) {
		t.Fatalf("worker launch env = %q, want %q", got, want)
	}
	for _, entry := range got {
		if strings.HasPrefix(entry, "ENGRAM_HTTP_TOKEN=") || strings.HasPrefix(entry, "GENTLE_AI_MAIN_FALLBACK=") {
			t.Fatalf("worker forwarded %q", entry)
		}
	}
}

// The unit receives its environment on the systemd-run command line, where
// any local process can read it: a credential-bearing Engram selection never
// crosses it, and that launch runs with memory disabled.
func TestUserEngramLinuxUnitArgvCarriesNoEngramCredentials(t *testing.T) {
	tests := []struct {
		name, key, value string
		disabled         bool
	}{
		{"database URL", "ENGRAM_DATABASE_URL", "postgres://user:hunter2@db.example/engram", true},
		{"database URL without userinfo", "ENGRAM_DATABASE_URL", "postgres://db.example/engram?password=hunter2", true},
		{"server URL with userinfo", "ENGRAM_URL", "http://user:hunter2@127.0.0.1:7437", true},
		{"server URL with user only", "ENGRAM_URL", "http://hunter2@127.0.0.1:7437", true},
		{"unparseable server URL", "ENGRAM_URL", "http://user:hunter2@[::1", true},
		{"plain server URL", "ENGRAM_URL", "http://127.0.0.1:17437", false},
		{"data directory", "ENGRAM_DATA_DIR", "/home/user/engram-hunter2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userEngramClearEnv(t)
			home := userEngramPrivateDir(t)
			t.Setenv("HOME", home)
			t.Setenv(tt.key, tt.value)
			root, _ := userEngramSeparateFixture(t, "separate")
			userEngramUseProbeBase(t, "http://"+userEngramFreePort(t))
			env := userEngramContextEnv(userEngramLaunchContext(context.Background(), []string{"launch", root}))
			argv := userServiceArgs("gentle-shell-0123456789abcdef", false, "/owned/supervisor", "/project", []string{"internal-launch", root}, env)
			if !tt.disabled {
				if want := []string{tt.key + "=" + tt.value}; !reflect.DeepEqual(env, want) {
					t.Fatalf("unit env = %q, want the selection %q forwarded", env, want)
				}
				return
			}
			for _, entry := range append(append([]string{}, env...), argv...) {
				if strings.Contains(entry, "hunter2") {
					t.Fatalf("credential-bearing %s reached the unit: %q", tt.key, entry)
				}
			}
			if want := userEngramDisabled(userEngramArgvNotice(tt.key)); !reflect.DeepEqual(env, want) {
				t.Fatalf("unit env = %q, want %q", env, want)
			}
			if _, started := userEngramWaitPID(t, home, 200*time.Millisecond); started {
				t.Fatal("a server was started")
			}
		})
	}
}

// The root-risk system unit (and any path that bypasses the outer decision)
// carries no Engram entries: the Separate worker disables memory instead of
// letting the companion resolve an unverified engram on PATH.
func TestUserEngramLinuxWorkerWithoutDecisionDisablesSeparateMemory(t *testing.T) {
	userEngramClearEnv(t)
	root := filepath.Join(t.TempDir(), "shell")
	manifest := userManifest{Mode: "separate", Prefix: filepath.Join(root, "prefix"), Agent: filepath.Join(root, "agent")}
	env := userLaunchCommand(userEngramLaunchScope(context.Background(), root), root, manifest, "/not-executed/cli.js", nil).Env
	if userEngramPolicy(env) != "disabled" {
		t.Fatalf("worker env without a decision = %q, want policy disabled", env)
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, "ENGRAM_BIN=") || strings.HasPrefix(entry, "ENGRAM_URL=") {
			t.Fatalf("worker env without a decision carried %q", entry)
		}
	}
}

// launch-pnpm builds its own environment outside userLaunchCommand and is
// observable only with a published PNPM fixture, so this pins both the exact
// entries its call adds per mode and that userLaunchPNPM appends that call.
func TestUserEngramPNPMLaunchFailsClosedOnlyForSeparate(t *testing.T) {
	for mode, want := range map[string][]string{"separate": userEngramDisabled(userEngramNoDecisionNotice), "agent": nil, "shared": nil} {
		if got := userEngramSeparateLaunch(mode, nil); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s launch-pnpm Engram entries = %q, want %q", mode, got, want)
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "user_pnpm_launch_linux.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "userLaunchPNPM" {
			return true
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 || types.ExprString(assign.Lhs[0]) != "cmd.Env" {
				return true
			}
			if types.ExprString(assign.Rhs[0]) == "append(cmd.Env, userEngramSeparateLaunch(manifest.Mode, nil)...)" {
				found = true
			}
			return true
		})
		return false
	})
	if !found {
		t.Fatal("userLaunchPNPM does not append userEngramSeparateLaunch(manifest.Mode, nil) to cmd.Env")
	}
}
