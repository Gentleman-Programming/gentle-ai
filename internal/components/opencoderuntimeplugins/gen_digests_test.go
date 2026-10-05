package opencoderuntimeplugins

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The registry only grows: regenerating it from an incomplete local tag set
// must refuse instead of silently dropping released digests.
func TestGenerateDigestsRefusesIncompleteTags(t *testing.T) {
	for _, tool := range []string{"bash", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable: %v", tool, err)
		}
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "scripts", "gen-opencode-plugin-digests.sh"))
	if err != nil {
		t.Fatal(err)
	}
	committed := "package opencoderuntimeplugins\n\nvar releasedPluginDigests = map[string][]string{\n" +
		"\t\"skill-registry.ts\": {\n\t\t\"" + strings.Repeat("ab", 32) + "\", // plugins/ v1.39.1\n\t},\n}\n"
	for _, tc := range []struct {
		name string
		tags []string
		want string
	}{
		{"missing anchor tag", []string{"v9.9.9"}, "v1.7.19"},
		{"fewer digests than committed", []string{"v1.7.19", "v4.0.0"}, "skill-registry.ts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			git := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
				cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
			}
			git("init", "-q")
			registry := filepath.Join(repo, "internal", "components", "opencoderuntimeplugins", "released_digests.go")
			if err := os.MkdirAll(filepath.Dir(registry), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(registry, []byte(committed), 0644); err != nil {
				t.Fatal(err)
			}
			git("add", ".")
			git("commit", "-q", "-m", "registry")
			for _, tag := range tc.tags {
				git("tag", tag)
			}
			cmd := exec.Command("bash", script)
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err == nil {
				t.Fatalf("generator accepted incomplete tags:\n%s", out)
			}
			if msg := stderr.String(); !strings.Contains(msg, tc.want) || !strings.Contains(msg, "git fetch --tags") {
				t.Fatalf("refusal lacks %q and remedy: %s", tc.want, msg)
			}
		})
	}
}
