// claude_opencode_adapter_test.go — change #3138, slice 3, RED-first
// (task 3.2): the thin Claude/OpenCode transport adapters must implement the
// Invoker seam (REQ-RPC-4/5) and return untouched raw bytes or a transport
// error, never fabricated, parsed, or validated bytes (SEN-RPC-5), strand
// nothing and consume no budget or correction state (SEN-RPC-19/20). The
// adapter types were missing until task 3.3, so this file failed to compile
// by construction.
package advisoryreview

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// providerAdapterInvoker is the Request carrying the Invoker seam the tests
// below pin. The compile assertion lives with the tests so a future adapter
// that stops conforming fails here first.
var providerAdapterInvoker = []Invoker{(*ClaudeAdapter)(nil), (*OpenCodeAdapter)(nil)}

func TestProviderAdaptersReturnTypedUnavailableWhenBinaryMissing(t *testing.T) {
	tests := []struct {
		name       string
		adapter    Invoker
		wantPrefix string
	}{
		{name: "claude", adapter: &ClaudeAdapter{LookPath: func(string) (string, error) {
			return "", errors.New("no claude on PATH")
		}, PromptFor: passThroughPrompt}, wantPrefix: "claude advisory transport unavailable"},
		{name: "opencode", adapter: &OpenCodeAdapter{LookPath: func(string) (string, error) {
			return "", errors.New("no opencode on PATH")
		}, PromptFor: passThroughPrompt}, wantPrefix: "opencode advisory transport unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := test.adapter.Invoke(context.Background(), testRequest(t))
			if err == nil {
				t.Fatalf("Invoke() = %q, nil, want a typed unavailable transport error", raw)
			}
			if !strings.Contains(err.Error(), test.wantPrefix) {
				t.Fatalf("Invoke() error = %v, want a %s message", err, test.wantPrefix)
			}
			if raw != nil {
				t.Fatalf("Invoke() returned bytes alongside a transport error: %q", raw)
			}
		})
	}
}

// TestProviderAdaptersReturnTransportErrorWhenPromptRenderingFails proves an
// adapter that cannot obtain the provider-rendered prompt never fabricates
// bytes: it reports the failure as a transport-style error and returns nil,
// matching CodexAdapter's contract that every failure is a transport error.
func TestProviderAdaptersReturnTransportErrorWhenPromptRenderingFails(t *testing.T) {
	tests := []struct {
		name       string
		adapter    Invoker
		wantPrefix string
	}{
		{name: "claude", adapter: &ClaudeAdapter{LookPath: func(string) (string, error) {
			return "/irrelevant/claude", nil
		}, PromptFor: failingPrompt}, wantPrefix: "claude advisory transport unavailable"},
		{name: "opencode", adapter: &OpenCodeAdapter{LookPath: func(string) (string, error) {
			return "/irrelevant/opencode", nil
		}, PromptFor: failingPrompt}, wantPrefix: "opencode advisory transport unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := test.adapter.Invoke(context.Background(), testRequest(t))
			if err == nil || !strings.Contains(err.Error(), test.wantPrefix) {
				t.Fatalf("Invoke() = %q, %v; want the prompt-rendering failure surfaced as a %s transport error", raw, err, test.wantPrefix)
			}
			if raw != nil {
				t.Fatalf("Invoke() returned fabricated bytes alongside a transport error: %q", raw)
			}
		})
	}
}

// fakeRawReviewerScript writes a POSIX shell script standing in for a real
// runtime binary that prints its raw final message on stdout (the shape both
// `claude --print` and `opencode run` use). It records the directory it was
// launched from, that directory's entire listing AT LAUNCH TIME (before the
// adapter's own deferred cleanup can remove it), every argument it received,
// and everything it read on stdin.
func fakeRawReviewerScript(t *testing.T, fixedOutput string) (path string, invocation func() (dir string, entriesAtLaunch, args []string, stdin string)) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime script targets POSIX shells")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "invocation.log")
	entriesPath := filepath.Join(dir, "entries.log")
	stdinPath := filepath.Join(dir, "stdin.log")
	script := filepath.Join(dir, "fake-runtime")
	contents := "#!/bin/sh\n" +
		"pwd > " + shellQuote(logPath) + "\n" +
		"printf '%s\\n' \"$@\" >> " + shellQuote(logPath) + "\n" +
		"cat > " + shellQuote(stdinPath) + "\n" +
		"ls -A . > " + shellQuote(entriesPath) + "\n" +
		"printf '%s' " + shellQuote(fixedOutput) + "\n"
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, func() (string, []string, []string, string) {
		logPayload, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read fake runtime invocation log: %v", err)
		}
		lines := strings.Split(strings.TrimRight(string(logPayload), "\n"), "\n")
		var invocationDir string
		var args []string
		if len(lines) > 0 {
			invocationDir, args = lines[0], lines[1:]
		}
		entriesPayload, err := os.ReadFile(entriesPath)
		if err != nil {
			t.Fatalf("read fake runtime directory listing: %v", err)
		}
		var entries []string
		for _, entry := range strings.Split(strings.TrimRight(string(entriesPayload), "\n"), "\n") {
			if entry != "" {
				entries = append(entries, entry)
			}
		}
		stdinPayload, err := os.ReadFile(stdinPath)
		if err != nil {
			t.Fatalf("read fake runtime stdin capture: %v", err)
		}
		return invocationDir, entries, args, string(stdinPayload)
	}
}

// passThroughPrompt and failingPrompt are the PromptFor seam stubs the
// adapter tests inject, mirroring how tests override CodexAdapter.LookPath.
func passThroughPrompt(_ context.Context, _ Request) ([]byte, error) {
	return []byte("the canonical advisory prompt"), nil
}

func failingPrompt(_ context.Context, _ Request) ([]byte, error) {
	return nil, errors.New("render failed")
}

// TestAdapterInvokesInAnEmptyScratchDirectoryAndReturnsRawBytes proves both
// adapter invocation shapes without network access or a real account: each
// test launches its own fake runtime binary in place of the real one and
// asserts the shared advisory contract -- a working directory the adapter
// itself created and never named by the caller, that directory empty at
// launch and gone afterwards, the prompt delivered exactly once via the same
// channel the real CLI uses (stdin for claude --print, trailing argv for
// opencode run), raw output returned unmodified, and no git/repository
// selector anywhere in the child argv (threat matrix, Git repository
// selection).
func TestAdapterInvokesInAnEmptyScratchDirectoryAndReturnsRawBytes(t *testing.T) {
	rawOutput := `{"subject_hash":"` + strings.Repeat("c", 64) + `","findings":[]}`
	script, invocation := fakeRawReviewerScript(t, rawOutput)
	request := testRequest(t)

	t.Run("claude", func(t *testing.T) {
		var prompt string
		adapter := &ClaudeAdapter{
			LookPath: func(name string) (string, error) {
				if name != "claude" {
					t.Fatalf("LookPath(%q), want LookPath(\"claude\")", name)
				}
				return script, nil
			},
			PromptFor: func(_ context.Context, _ Request) ([]byte, error) {
				prompt = "the canonical advisory prompt"
				return []byte(prompt), nil
			},
		}
		raw, err := adapter.Invoke(context.Background(), request)
		if err != nil {
			t.Fatalf("Invoke() error = %v", err)
		}
		if string(raw) != rawOutput {
			t.Fatalf("Invoke() = %q, want the fake runtime's fixed raw output unmodified", raw)
		}

		dir, entriesAtLaunch, args, stdin := invocation()
		if dir == "" {
			t.Fatal("fake runtime recorded no working directory")
		}
		if len(entriesAtLaunch) != 0 {
			t.Fatalf("scratch directory %s was not empty at launch: %v", dir, entriesAtLaunch)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("Invoke() did not remove its scratch directory %s: err=%v", dir, err)
		}
		if stdin != prompt {
			t.Fatalf("claude child stdin = %q, want the provider-rendered prompt %q", stdin, prompt)
		}
		joined := strings.Join(args, "\n")
		for _, want := range []string{"--print", "--bare", "--permission-mode", "dontAsk"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("claude invocation args = %q, missing %q", args, want)
			}
		}
		for _, forbidden := range []string{"-C", "--dir", "--git", "rev-parse"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("claude invocation args = %q, must not carry git/repository selector %q", args, forbidden)
			}
		}
	})

	t.Run("opencode", func(t *testing.T) {
		const prompt = "the canonical advisory prompt"
		adapter := &OpenCodeAdapter{
			LookPath: func(name string) (string, error) {
				if name != "opencode" {
					t.Fatalf("LookPath(%q), want LookPath(\"opencode\")", name)
				}
				return script, nil
			},
			PromptFor: func(_ context.Context, _ Request) ([]byte, error) {
				return []byte(prompt), nil
			},
		}
		raw, err := adapter.Invoke(context.Background(), request)
		if err != nil {
			t.Fatalf("Invoke() error = %v", err)
		}
		if string(raw) != rawOutput {
			t.Fatalf("Invoke() = %q, want the fake runtime's fixed raw output unmodified", raw)
		}

		dir, entriesAtLaunch, args, stdin := invocation()
		if dir == "" {
			t.Fatal("fake runtime recorded no working directory")
		}
		if len(entriesAtLaunch) != 0 {
			t.Fatalf("scratch directory %s was not empty at launch: %v", dir, entriesAtLaunch)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("Invoke() did not remove its scratch directory %s: err=%v", dir, err)
		}
		if stdin != "" {
			t.Fatalf("opencode child stdin = %q, want empty (prompt travels as argv)", stdin)
		}
		if len(args) != 2 || args[0] != "run" || args[1] != prompt {
			t.Fatalf("opencode invocation args = %q, want [run %q]", args, prompt)
		}
		for _, forbidden := range []string{"-C", "--dir", "--git", "rev-parse"} {
			if strings.Contains(strings.Join(args, "\n"), forbidden) {
				t.Fatalf("opencode invocation args = %q, must not carry git/repository selector %q", args, forbidden)
			}
		}
	})
}

// buildRawEnvDumpingFakeRuntime compiles a tiny real binary standing in for a
// runtime, deliberately NOT a POSIX shell script: `sh` unconditionally
// overwrites PWD with its own getcwd() on every invocation, which would
// silently defeat the "PWD absent" assertion. A plain Go binary reports
// os.Environ() exactly as received.
func buildRawEnvDumpingFakeRuntime(t *testing.T, dumpPath, fixedOutput string) (path string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	program := "package main\n\n" +
		"import (\n\t\"os\"\n\t\"strings\"\n)\n\n" +
		"func main() {\n" +
		"\t_ = os.WriteFile(" + strconv.Quote(dumpPath) + ", []byte(strings.Join(os.Environ(), \"\\n\")), 0o644)\n" +
		"\t_, _ = os.Stdout.WriteString(" + strconv.Quote(fixedOutput) + ")\n" +
		"}\n"
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatalf("write fake runtime helper source: %v", err)
	}
	binary := filepath.Join(dir, "fake-runtime")
	build := exec.Command("go", "build", "-o", binary, source)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake runtime helper: %v\n%s", err, output)
	}
	return binary
}

// TestProviderAdaptersScrubChildEnvironmentToPathAndConfigHome proves both
// adapters hand their child process an explicit two-variable allowlist
// (PATH + the runtime's own config-home override), never the full ambient
// environment Invoke()'s own process happens to carry -- PWD is the
// motivating case, a sentinel variable proves the exclusion is general, and
// the config-home variable proves each runtime still receives its own
// auth/config location without inheriting the caller's.
func TestProviderAdaptersScrubChildEnvironmentToPathAndConfigHome(t *testing.T) {
	run := func(name, configVar string, makeAdapter func(binary string) Invoker) {
		t.Run(name, func(t *testing.T) {
			dumpDir := t.TempDir()
			dumpPath := filepath.Join(dumpDir, "env-dump.log")
			binary := buildRawEnvDumpingFakeRuntime(t, dumpPath, `{"ok":true}`)

			t.Setenv("PWD", "/leaked/real/worktree/should-never-reach-the-runtime")
			const sentinelName = "GENTLE_AI_PROVIDER_ADAPTER_TEST_SENTINEL"
			t.Setenv(sentinelName, "sentinel-value-must-not-leak")
			t.Setenv(configVar, filepath.Join(t.TempDir(), "config"))

			raw, err := makeAdapter(binary).Invoke(context.Background(), testRequest(t))
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			if string(raw) != `{"ok":true}` {
				t.Fatalf("Invoke() = %q, want the fake runtime's fixed raw output unmodified", raw)
			}

			dumped, err := os.ReadFile(dumpPath)
			if err != nil {
				t.Fatalf("read fake runtime environment dump: %v", err)
			}
			entries := map[string]string{}
			for _, line := range strings.Split(strings.TrimRight(string(dumped), "\n"), "\n") {
				if line == "" {
					continue
				}
				key, value, ok := strings.Cut(line, "=")
				if !ok {
					t.Fatalf("malformed dumped environment entry: %q", line)
				}
				entries[key] = value
			}

			if _, present := entries["PWD"]; present {
				t.Fatalf("child environment carried PWD=%q, want it absent", entries["PWD"])
			}
			if _, present := entries[sentinelName]; present {
				t.Fatalf("child environment carried the test sentinel %s, want it absent", sentinelName)
			}
			if value, present := entries["PATH"]; !present || value == "" {
				t.Fatalf("child environment PATH = %q, present=%v, want the allowlisted PATH", value, present)
			}
			if value, present := entries[configVar]; !present || value == "" {
				t.Fatalf("child environment %s = %q, present=%v, want the allowlisted config home", configVar, value, present)
			}
			// Windows processes cannot start without SYSTEMROOT, so os/exec
			// appends it to any Env that omits it (community report #2675).
			if runtime.GOOS == "windows" {
				for key := range entries {
					if strings.EqualFold(key, "SYSTEMROOT") {
						delete(entries, key)
					}
				}
			}
			if len(entries) != 2 {
				t.Fatalf("child environment = %v, want exactly PATH and %s", entries, configVar)
			}
		})
	}
	run("claude", "CLAUDE_CONFIG_DIR", func(binary string) Invoker {
		return &ClaudeAdapter{LookPath: func(name string) (string, error) {
			if name != "claude" {
				t.Fatalf("LookPath(%q), want LookPath(\"claude\")", name)
			}
			return binary, nil
		}, PromptFor: passThroughPrompt}
	})
	run("opencode", "OPENCODE_CONFIG_DIR", func(binary string) Invoker {
		return &OpenCodeAdapter{LookPath: func(name string) (string, error) {
			if name != "opencode" {
				t.Fatalf("LookPath(%q), want LookPath(\"opencode\")", name)
			}
			return binary, nil
		}, PromptFor: passThroughPrompt}
	})
}

// TestProviderAdaptersStrandNothingAndConsumeNoBudget proves the adapter is
// not an admission authority: a request carrying evidence past
// MaxEvidenceEntries -- which Provider.Validate refuses -- is still invoked
// unchanged (refusal belongs to Validate, never the transport), the raw
// output is returned untouched, and the scratch directory is removed so
// nothing is left behind (SEN-RPC-19/20: strand-nothing, no budget or
// correction consumption).
func TestProviderAdaptersStrandNothingAndConsumeNoBudget(t *testing.T) {
	overCount := freezeRequestWithEntries(t, MaxEvidenceEntries+1)
	rawOutput := `{"ok":true}`
	tests := []struct {
		name    string
		adapter Invoker
	}{
		{name: "claude", adapter: &ClaudeAdapter{LookPath: func(string) (string, error) {
			script, _ := fakeRawReviewerScript(t, rawOutput)
			return script, nil
		}, PromptFor: passThroughPrompt}},
		{name: "opencode", adapter: &OpenCodeAdapter{LookPath: func(string) (string, error) {
			script, _ := fakeRawReviewerScript(t, rawOutput)
			return script, nil
		}, PromptFor: passThroughPrompt}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			raw, err := test.adapter.Invoke(context.Background(), overCount)
			if err != nil {
				t.Fatalf("Invoke() error = %v; the adapter renders the request unchanged even past MaxEvidenceEntries (budget refusal belongs to Validate)", err)
			}
			if string(raw) != rawOutput {
				t.Fatalf("Invoke() = %q, want the fake runtime's fixed raw output unmodified", raw)
			}
		})
	}
}
