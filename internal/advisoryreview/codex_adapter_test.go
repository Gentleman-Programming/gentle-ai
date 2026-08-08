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

func TestCodexAdapterReturnsTypedUnavailableWhenBinaryMissing(t *testing.T) {
	adapter := &CodexAdapter{LookPath: func(string) (string, error) {
		return "", errors.New("no codex on PATH")
	}}
	raw, err := adapter.Review(context.Background(), "irrelevant prompt")
	if err == nil {
		t.Fatalf("Review() = %q, nil, want a typed unavailable transport error", raw)
	}
	if !strings.Contains(err.Error(), "codex advisory transport unavailable") {
		t.Fatalf("Review() error = %v, want a codex advisory transport unavailable message", err)
	}
	if raw != nil {
		t.Fatalf("Review() returned bytes alongside a transport error: %q", raw)
	}
}

// fakeCodexScript writes a POSIX shell script standing in for the real codex
// binary. It records the directory it was launched from, that directory's
// entire listing AT LAUNCH TIME (before the adapter's own deferred cleanup
// can remove it), and every argument it received, then, matching the real
// CLI's --output-last-message contract, writes fixedOutput to the path
// following that flag.
func fakeCodexScript(t *testing.T, fixedOutput string) (path string, invocationLog func() (dir string, entriesAtLaunch []string, args []string)) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake codex script targets POSIX shells")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "invocation.log")
	entriesPath := filepath.Join(dir, "entries.log")
	script := filepath.Join(dir, "codex")
	contents := "#!/bin/sh\n" +
		"pwd > " + shellQuote(logPath) + "\n" +
		"printf '%s\\n' \"$@\" >> " + shellQuote(logPath) + "\n" +
		"ls -A . > " + shellQuote(entriesPath) + "\n" +
		"output=\"\"\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--output-last-message\" ]; then\n" +
		"    output=\"$2\"\n" +
		"  fi\n" +
		"  shift\n" +
		"done\n" +
		"printf '%s' " + shellQuote(fixedOutput) + " > \"$output\"\n"
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, func() (string, []string, []string) {
		payload, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read fake codex invocation log: %v", err)
		}
		lines := strings.Split(strings.TrimRight(string(payload), "\n"), "\n")
		var invocationDir string
		var args []string
		if len(lines) > 0 {
			invocationDir, args = lines[0], lines[1:]
		}
		entriesPayload, err := os.ReadFile(entriesPath)
		if err != nil {
			t.Fatalf("read fake codex directory listing: %v", err)
		}
		var entries []string
		for _, entry := range strings.Split(strings.TrimRight(string(entriesPayload), "\n"), "\n") {
			if entry != "" {
				entries = append(entries, entry)
			}
		}
		return invocationDir, entries, args
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// TestCodexAdapterInvokesNonInteractivelyInAnEmptyScratchDirectory proves the
// adapter's exact invocation shape without depending on network access or a
// real Codex account: it launches its own fake "codex" binary in place of the
// real one and asserts every argument the shared advisory contract requires
// (no-git-repo-check, ignore-user-config, read-only sandbox, a working
// directory the adapter itself created and never named by the caller) and
// that only that directory -- never the process's own working directory --
// is where codex was told to run.
func TestCodexAdapterInvokesNonInteractivelyInAnEmptyScratchDirectory(t *testing.T) {
	script, invocation := fakeCodexScript(t, `{"ok":true}`)
	adapter := &CodexAdapter{LookPath: func(name string) (string, error) {
		if name != "codex" {
			t.Fatalf("LookPath(%q), want LookPath(\"codex\")", name)
		}
		return script, nil
	}}

	raw, err := adapter.Review(context.Background(), "the canonical advisory prompt")
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("Review() = %q, want the fake codex's fixed raw output unmodified", raw)
	}

	dir, entriesAtLaunch, args := invocation()
	if dir == "" {
		t.Fatal("fake codex recorded no working directory")
	}
	// entriesAtLaunch is the scratch directory's listing captured by the fake
	// binary itself, at the moment codex actually ran and strictly before the
	// adapter's own deferred cleanup deletes it -- an empty directory has
	// nothing a reviewer could read beyond the supplied prompt.
	if len(entriesAtLaunch) != 0 {
		t.Fatalf("scratch directory %s was not empty at codex launch: %v", dir, entriesAtLaunch)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Review() did not remove its scratch directory %s: err=%v", dir, err)
	}

	joined := strings.Join(args, "\n")
	for _, want := range []string{"exec", "--skip-git-repo-check", "--ignore-user-config", "--sandbox", "read-only", "--output-last-message", "the canonical advisory prompt"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("codex invocation args = %q, missing %q", args, want)
		}
	}
	if strings.Contains(joined, "-C") {
		index := -1
		for position, arg := range args {
			if arg == "-C" {
				index = position
			}
		}
		if index < 0 || index+1 >= len(args) || args[index+1] != dir {
			t.Fatalf("codex was not launched with -C pointed at its own scratch directory: %v (ran in %s)", args, dir)
		}
	} else {
		t.Fatalf("codex invocation args = %q, missing -C", args)
	}
}

func TestCodexAdapterReturnsTransportErrorOnNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake codex script targets POSIX shells")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "codex")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho boom failure >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &CodexAdapter{LookPath: func(string) (string, error) { return script, nil }}
	raw, err := adapter.Review(context.Background(), "prompt")
	if err == nil {
		t.Fatalf("Review() = %q, nil, want a transport failure", raw)
	}
	if !strings.Contains(err.Error(), "codex advisory transport failed") || !strings.Contains(err.Error(), "boom failure") {
		t.Fatalf("Review() error = %v, want the transport failure to carry the process's stderr", err)
	}
}

// buildEnvDumpingFakeCodex compiles a tiny real binary standing in for codex,
// deliberately NOT a POSIX shell script like fakeCodexScript above: `sh`
// (dash on Debian/Ubuntu, this repo's CI base) unconditionally overwrites
// PWD with its own getcwd() result on every invocation, even when PWD is
// entirely absent from the environment it was launched with, which would
// silently defeat an assertion that PWD is absent. A plain Go binary just
// reports os.Environ() as received, with nothing rewritten in between. The
// dump path is baked into the generated source as a literal, exactly like
// fakeCodexScript bakes its own log paths into the shell script text, so no
// runtime coordination channel (env var or extra argument) is needed.
func buildEnvDumpingFakeCodex(t *testing.T, dumpPath string) (path string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	program := "package main\n\n" +
		"import (\n\t\"os\"\n\t\"strings\"\n)\n\n" +
		"func main() {\n" +
		"\t_ = os.WriteFile(" + strconv.Quote(dumpPath) + ", []byte(strings.Join(os.Environ(), \"\\n\")), 0o644)\n" +
		"\toutput := \"\"\n" +
		"\tfor index, arg := range os.Args {\n" +
		"\t\tif arg == \"--output-last-message\" && index+1 < len(os.Args) {\n" +
		"\t\t\toutput = os.Args[index+1]\n" +
		"\t\t}\n" +
		"\t}\n" +
		"\tif output != \"\" {\n" +
		"\t\t_ = os.WriteFile(output, []byte(`{\"ok\":true}`), 0o644)\n" +
		"\t}\n" +
		"}\n"
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatalf("write fake codex helper source: %v", err)
	}
	binaryName := "codex"
	if runtime.GOOS == "windows" {
		binaryName = "codex.exe"
	}
	binary := filepath.Join(dir, binaryName)
	build := exec.Command("go", "build", "-o", binary, source)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake codex helper: %v\n%s", err, output)
	}
	return binary
}

// TestCodexAdapterScrubsChildEnvironmentToPathAndCodexHome proves the
// hardened boundary this adapter now owns in addition to Dir/-C: the child
// process receives an explicit two-variable allowlist (PATH, CODEX_HOME),
// never the full ambient environment Review()'s own process happens to
// carry. PWD is the motivating case -- Dir and -C both point codex at the
// empty scratch directory, but without an explicit Env, the OS-level PWD the
// child inherits still names wherever this test process itself started,
// which for a real reviewer invocation is the caller's live worktree, not
// the empty scratch directory the rest of the boundary comment above
// promises. A sentinel variable set on the test process itself proves the
// exclusion is general, not special-cased to PWD alone.
func TestCodexAdapterScrubsChildEnvironmentToPathAndCodexHome(t *testing.T) {
	dumpDir := t.TempDir()
	dumpPath := filepath.Join(dumpDir, "env-dump.log")
	binary := buildEnvDumpingFakeCodex(t, dumpPath)

	t.Setenv("PWD", "/leaked/real/worktree/should-never-reach-codex")
	const sentinelName = "GENTLE_AI_CODEX_ADAPTER_TEST_SENTINEL"
	t.Setenv(sentinelName, "sentinel-value-must-not-leak")

	adapter := &CodexAdapter{LookPath: func(string) (string, error) { return binary, nil }}
	raw, err := adapter.Review(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("Review() = %q, want the fake codex's fixed raw output unmodified", raw)
	}

	dumped, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read fake codex environment dump: %v", err)
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
		t.Fatalf("codex child environment carried PWD=%q, want it absent", entries["PWD"])
	}
	if _, present := entries[sentinelName]; present {
		t.Fatalf("codex child environment carried the test sentinel %s, want it absent", sentinelName)
	}
	if value, present := entries["PATH"]; !present || value == "" {
		t.Fatalf("codex child environment PATH = %q, present=%v, want the allowlisted PATH", value, present)
	}
	if value, present := entries["CODEX_HOME"]; !present || value == "" {
		t.Fatalf("codex child environment CODEX_HOME = %q, present=%v, want the allowlisted CODEX_HOME", value, present)
	}
	// Windows processes cannot start without SYSTEMROOT, so os/exec appends it
	// to any Env that omits it. The hosted runner therefore yields a third
	// entry the allowlist never asked for and cannot refuse (community report
	// #2675). It is the platform's floor, not inherited ambient state: every
	// variable the scrub exists to keep out is still asserted absent above.
	if runtime.GOOS == "windows" {
		for key := range entries {
			if strings.EqualFold(key, "SYSTEMROOT") {
				delete(entries, key)
			}
		}
	}
	if len(entries) != 2 {
		t.Fatalf("codex child environment = %v, want exactly PATH and CODEX_HOME", entries)
	}
}

func TestCodexAdapterReturnsTransportErrorWhenContextExpires(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake codex script targets POSIX shells")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "codex")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := &CodexAdapter{LookPath: func(string) (string, error) { return script, nil }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := adapter.Review(ctx, "prompt")
	if err == nil {
		t.Fatalf("Review() = %q, nil, want a transport failure for an already-canceled context", raw)
	}
}
