package cli_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
)

// TestCursorJudgmentDayCLISmoke exercises the shipped executable, not CLI test
// seams. It proves packaging and refresh, not Cursor's actual agent execution.
func TestCursorJudgmentDayCLISmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the real CLI in an isolated home")
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("Go compiler unavailable")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	sandbox := t.TempDir()
	binary := filepath.Join(sandbox, "gentle-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// This test binary needs no release VCS stamp. TestMain isolates HOME,
	// including Git configuration, so do not require repository trust metadata.
	build := exec.CommandContext(ctx, goBinary, "build", "-buildvcs=false", "-o", binary, "./cmd/gentle-ai")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	home := filepath.Join(sandbox, "home")
	cwd := filepath.Join(sandbox, "workspace")
	emptyPath := filepath.Join(sandbox, "empty-bin")
	temp := filepath.Join(sandbox, "tmp")
	for _, dir := range []string{home, cwd, emptyPath, temp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS == "linux" {
		// Linux startup requires package-manager discovery even for asset-only
		// commands. Keep PATH isolated and reject any actual package operation.
		apt := []byte("#!/bin/sh\necho 'unexpected package-manager execution' >&2\nexit 1\n")
		if err := os.WriteFile(filepath.Join(emptyPath, "apt"), apt, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Do not inherit host config selectors, credentials, runtime identities, or
	// executable discovery. Disable telemetry and self-update explicitly.
	env := []string{
		"HOME=" + home, "USERPROFILE=" + home,
		"APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TEMP=" + temp, "TMP=" + temp, "TMPDIR=" + temp,
		"PATH=" + emptyPath, "CI=1", "DO_NOT_TRACK=1",
		"GENTLE_AI_TELEMETRY=0", "GENTLE_AI_NO_SELF_UPDATE=1",
	}
	for _, key := range []string{"SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir, cmd.Env = cwd, env
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("CLI %v: %v\nstdout:\n%s\nstderr:\n%s", args, err, &stdout, &stderr)
		}
		t.Logf("CLI %v: exit=0\nstdout:\n%s\nstderr:\n%s", args, &stdout, &stderr)
		if args[0] == "sync" && stderr.Len() != 0 {
			t.Fatalf("sync stderr must be empty: %s", &stderr)
		}
		return stdout.String()
	}
	agentsDir := filepath.Join(home, ".cursor", "agents")
	names := []string{"jd-judge-a.md", "jd-judge-b.md", "jd-fix-agent.md"}
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	checkAgents := func() map[string]string {
		t.Helper()
		got := make(map[string]string)
		for _, name := range names {
			body := read(filepath.Join(agentsDir, name))
			got[name] = body
			wants := []string{"name: " + strings.TrimSuffix(name, ".md"), "model: inherit", "background: false", "Maximum 2 fix rounds", "Only the parent"}
			if strings.HasPrefix(name, "jd-judge-") {
				wants = append(wants, "readonly: true", reviewassets.NativeReviewerResultSchema, "frozen ledger plus immutable fix delta", "asks the user to approve", "no `review-refuter`", "do not persist or modify the ledger")
			} else {
				wants = append(wants, "readonly: false", "Fix ONLY the confirmed issues", "parent owns user approval and ledger persistence")
			}
			for _, want := range wants {
				if !strings.Contains(body, want) {
					t.Errorf("%s missing rendered contract %q", name, want)
				}
			}
			if strings.Contains(body, "{{") {
				t.Errorf("%s contains unresolved placeholders", name)
			}
		}
		entries, err := os.ReadDir(agentsDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "review-") {
				t.Errorf("CLI installed RDD agent %s", entry.Name())
			}
		}
		return got
	}

	installOutput := run("install", "--agent", "cursor", "--component", "skills", "--skill", "judgment-day")
	if !strings.Contains(installOutput, "Verification checks:") || !strings.Contains(installOutput, "0 failed") {
		t.Fatalf("install did not report successful verification: %s", installOutput)
	}
	original := checkAgents()
	skillPath := filepath.Join(home, ".cursor", "skills", "judgment-day", "SKILL.md")
	if got := read(skillPath); got != assets.MustRead("skills/judgment-day/SKILL.md") {
		t.Fatal("CLI did not install the current Judgment Day skill")
	}

	// Sync must repair missing managed files left by an incomplete installation.
	for _, path := range []string{filepath.Join(agentsDir, "jd-judge-b.md"), skillPath} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	if output := run("sync", "--agent", "cursor"); !strings.Contains(output, "Agents synced: cursor") {
		t.Fatalf("repair sync did not report Cursor: %s", output)
	}
	for name, body := range checkAgents() {
		if body != original[name] {
			t.Errorf("%s changed after repair sync", name)
		}
	}
	if got := read(skillPath); got != assets.MustRead("skills/judgment-day/SKILL.md") {
		t.Fatal("sync did not restore the current Judgment Day skill")
	}
	wantNoOp := "gentle-ai sync — no managed sync actions needed\nAgents: cursor\nAll managed assets are already up to date. No files changed.\n"
	if output := run("sync", "--agent", "cursor"); output != wantNoOp {
		t.Fatalf("idempotent stdout = %q, want %q", output, wantNoOp)
	}
	for name, body := range checkAgents() {
		if body != original[name] {
			t.Errorf("idempotent sync changed %s", name)
		}
	}

	userJudge := filepath.Join(agentsDir, "jd-judge-a.md")
	userAgent := filepath.Join(agentsDir, "my-agent.md")
	for _, path := range []string{userJudge, userAgent} {
		if err := os.WriteFile(path, []byte("user-owned agent\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if output := run("sync", "--agent", "cursor"); !strings.Contains(output, "preserved native agents were not updated") {
		t.Fatalf("sync did not report preserved native agent: %s", output)
	}
	for _, path := range []string{userJudge, userAgent} {
		if got := read(path); got != "user-owned agent\n" {
			t.Fatalf("sync overwrote user file %s: %q", path, got)
		}
	}
}
