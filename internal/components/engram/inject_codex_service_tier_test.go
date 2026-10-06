package engram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func injectCodexServiceTier(t *testing.T, seed string, opts InjectOptions) string {
	t.Helper()
	return injectCodexServiceTierAt(t, t.TempDir(), seed, opts)
}

func injectCodexServiceTierAt(t *testing.T, home, seed string, opts InjectOptions) string {
	t.Helper()
	validCodexRuntime(t)
	path := filepath.Join(home, ".codex", "config.toml")
	if seed != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := InjectWithOptions(home, codexAdapter(), opts); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestInjectCodexServiceTierWritesSelectedTier(t *testing.T) {
	got := injectCodexServiceTier(t, "service_tier = \"flex\"\n", InjectOptions{CodexServiceTier: "priority"})
	if strings.Count(got, "service_tier =") != 1 || !strings.Contains(got, `service_tier = "priority"`) {
		t.Fatalf("selected tier not written as the single top-level service_tier:\n%s", got)
	}
}

func TestInjectCodexStandardRetiresOnlyTheTierGentleWrote(t *testing.T) {
	const tail = "\n[profiles.fast]\nservice_tier = \"priority\"\n"
	tests := []struct {
		name, seed, managed string
		wantTier            string // "" = top-level key absent
	}{
		{"managed value still present is removed", "model = \"m\"\nservice_tier = \"priority\"\n" + tail, "priority", ""},
		{"user changed the value after Gentle wrote it", "model = \"m\"\nservice_tier = \"flex\"\n" + tail, "priority", `service_tier = "flex"`},
		{"user tier Gentle never managed", "model = \"m\"\nservice_tier = \"priority\"\n" + tail, "", `service_tier = "priority"`},
		{"user tier with inline comment is not exact", "model = \"m\"\nservice_tier = \"priority\" # mine\n" + tail, "priority", `service_tier = "priority" # mine`},
		// An unchanged selection (selected == managed) never rewrites a user edit.
		{"unchanged selection keeps a user edit", "model = \"m\"\nservice_tier = \"flex\"\n" + tail, "priority", `service_tier = "flex"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := InjectOptions{CodexManagedServiceTier: tt.managed}
			if strings.HasPrefix(tt.name, "unchanged") {
				opts.CodexServiceTier = tt.managed
			}
			got := injectCodexServiceTier(t, tt.seed, opts)
			top, _, _ := strings.Cut(got, "[")
			if tt.wantTier == "" && strings.Contains(top, "service_tier") {
				t.Fatalf("managed top-level service_tier was not retired:\n%s", got)
			}
			if tt.wantTier != "" && !strings.Contains(top, tt.wantTier) {
				t.Fatalf("user-owned service_tier %q was changed:\n%s", tt.wantTier, got)
			}
			if !strings.Contains(got, tail) || !strings.Contains(got, `model = "m"`) {
				t.Fatalf("unrelated config changed:\n%s", got)
			}
		})
	}
}

func TestInjectCodexWithoutTierKeepsConfigByteIdentical(t *testing.T) {
	home := t.TempDir()
	first := injectCodexServiceTierAt(t, home, "model = \"m\"\n", InjectOptions{})
	if strings.Contains(first, "service_tier") {
		t.Fatalf("service_tier written without a selection:\n%s", first)
	}
	if again := injectCodexServiceTierAt(t, home, first, InjectOptions{}); again != first {
		t.Fatalf("no-tier inject changed config:\n--- first\n%s\n--- again\n%s", first, again)
	}
	withUserTier := strings.Replace(first, "model = \"m\"\n", "model = \"m\"\nservice_tier = \"flex\"\n", 1)
	if got := injectCodexServiceTierAt(t, home, withUserTier, InjectOptions{}); got != withUserTier {
		t.Fatalf("no-tier inject changed a user tier:\n--- want\n%s\n--- got\n%s", withUserTier, got)
	}
}
