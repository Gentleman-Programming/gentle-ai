package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidateLocalFileReferenceAcceptsOrdinaryPathSpellings(t *testing.T) {
	// Classification is deliberately platform-independent; no Windows path is
	// opened on the host running this test.
	for _, target := range []string{`C:\Users\me\prompt.md`, "C:/Users/me/prompt.md", "/tmp/prompt.md", "../prompt.md", `..\prompt.md`, "~/prompt.md", "./??/x", "rel/x"} {
		t.Run(target, func(t *testing.T) {
			if err := ValidateLocalFileReference(target); err != nil {
				t.Fatalf("ordinary path %q rejected: %v", target, err)
			}
		})
	}
}

func TestSettingsFileReferencesRejectUnsafePathsBeforeProbe(t *testing.T) {
	for _, tc := range []struct{ name, target string }{
		{"slash UNC", "//attacker.example/share/leak"},
		{"backslash UNC", `\\attacker.example\share\leak`},
		{"mixed separators", `\/attacker.example\share/leak`},
		{"extended UNC", `\\?\UNC\attacker.example\share\leak`},
		{"normalized extended UNC", `//?/unc/attacker.example/share/leak`},
		{"device pipe", `\\.\pipe\example`},
		{"normalized device pipe", `//./pipe/example`},
		{"extended local device", `\\?\C:\prompts\example.md`},
		{"NT object UNC", `\??\UNC\host\share\x`},
		{"normalized NT object UNC", "/??/UNC/host/share/x"},
		{"NT object drive", `\??\C:\x`},
		{"mixed NT object UNC", `\/??/UNC/h/s`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			settings := filepath.Join(home, "opencode.json")
			original, err := json.Marshal(map[string]any{"agent": map[string]any{"custom": map[string]any{"prompt": "Before {file:" + tc.target + "} after"}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settings, original, 0o600); err != nil {
				t.Fatal(err)
			}
			var probes []string
			old := statSettingsFileReference
			statSettingsFileReference = func(target string) (os.FileInfo, error) {
				probes = append(probes, target)
				return nil, fmt.Errorf("probe blocked by test")
			}
			t.Cleanup(func() { statSettingsFileReference = old })
			want := fmt.Sprintf("invalid file reference in %s: %s: remote or device paths are not supported; restore the referenced file or correct its {file:...} reference before retrying", settings, tc.target)
			refs, err := SettingsFileReferences(settings, home)
			if err == nil || err.Error() != want || len(refs) != 0 {
				t.Errorf("references = %v, error = %v; want no references and %q", refs, err, want)
			}
			if err := ValidateSettingsFileReferences(settings, home); err == nil || err.Error() != want {
				t.Errorf("validation error = %v, want %q", err, want)
			}
			if len(probes) != 0 {
				t.Errorf("reference probes = %v, want none", probes)
			}
			if after, err := os.ReadFile(settings); err != nil || string(after) != string(original) {
				t.Errorf("settings changed: %s (%v)", after, err)
			}
		})
	}
}

func TestSettingsFileReferencesRejectUnsafeHomeBaseBeforeProbe(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "opencode.json")
	original := []byte(`{"prompt":"{file:~/prompt.md}"}`)
	if err := os.WriteFile(settings, original, 0o600); err != nil {
		t.Fatal(err)
	}
	remoteHome := "//attacker.example/share"
	var probes []string
	old := statSettingsFileReference
	statSettingsFileReference = func(target string) (os.FileInfo, error) {
		probes = append(probes, target)
		return nil, fmt.Errorf("probe blocked by test")
	}
	t.Cleanup(func() { statSettingsFileReference = old })
	want := fmt.Sprintf("invalid file reference in %s: %s: remote or device paths are not supported; restore the referenced file or correct its {file:...} reference before retrying", settings, remoteHome)
	refs, err := SettingsFileReferences(settings, remoteHome)
	if err == nil || err.Error() != want || len(refs) != 0 {
		t.Errorf("references = %v, error = %v; want no references and %q", refs, err, want)
	}
	if err := ValidateSettingsFileReferences(settings, remoteHome); err == nil || err.Error() != want {
		t.Errorf("validation error = %v, want %q", err, want)
	}
	if len(probes) != 0 {
		t.Errorf("reference probes = %v, want none", probes)
	}
	if after, err := os.ReadFile(settings); err != nil || string(after) != string(original) {
		t.Errorf("settings changed: %s (%v)", after, err)
	}
}

func TestValidateSettingsFileReferencesPreservesLocalForms(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "config")
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, "prompt.md")
	if err := os.WriteFile(file, []byte("local prompt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{file, "../prompt.md", "~/prompt.md"} {
		t.Run(target, func(t *testing.T) {
			settings := filepath.Join(config, "opencode.json")
			original, err := json.Marshal(map[string]any{"prompt": "{file:" + target + "}"})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settings, original, 0o600); err != nil {
				t.Fatal(err)
			}
			refs, err := SettingsFileReferences(settings, home)
			if err != nil || !slices.Equal(refs, []string{file}) {
				t.Fatalf("references = %v, error = %v", refs, err)
			}
			if err := ValidateSettingsFileReferences(settings, home); err != nil {
				t.Fatalf("local reference rejected: %v", err)
			}
			if after, err := os.ReadFile(settings); err != nil || string(after) != string(original) {
				t.Fatalf("settings changed: %s (%v)", after, err)
			}
		})
	}
}

func TestSettingsFileReferencesLeavesEmptyPlaceholderLiteral(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, "opencode.json")
	promptFile := filepath.Join(home, "prompt.md")
	if err := os.WriteFile(promptFile, []byte("Keep this prompt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, prompt string
		wantFile     bool
	}{
		{"empty", "{file:}", false},
		{"embedded empty", "Before {file:} after", false},
		{"empty alongside valid", "Before {file:} and {file:./prompt.md} after", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, err := json.Marshal(map[string]any{"agent": map[string]any{"custom": map[string]any{"prompt": tc.prompt}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settings, original, 0o600); err != nil {
				t.Fatal(err)
			}
			refs, err := SettingsFileReferences(settings, home)
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			if tc.wantFile {
				want = []string{promptFile}
			}
			if !slices.Equal(refs, want) {
				t.Fatalf("references = %v, want %v", refs, want)
			}
			if err := ValidateSettingsFileReferences(settings, home); err != nil {
				t.Fatalf("literal placeholder rejected: %v", err)
			}
			if after, err := os.ReadFile(settings); err != nil || string(after) != string(original) {
				t.Fatalf("settings changed: %s (%v)", after, err)
			}
		})
	}
}

func TestSettingsFileReferences(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	absolute := filepath.Join(home, "absolute.md")
	for _, relative := range []string{"relative.md", "../parent.md", "~/home.md", absolute} {
		t.Run(relative, func(t *testing.T) {
			settings := filepath.Join(dir, "opencode.jsonc")
			encoded, err := json.Marshal(map[string]any{"agent": map[string]any{"custom": map[string]any{"prompt": "Before {file:" + relative + "} after"}}, "instructions": []any{"{file:" + relative + "}"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settings, append([]byte("// {file:ignored-comment.md}\n"), encoded...), 0o600); err != nil {
				t.Fatal(err)
			}
			refs, err := SettingsFileReferences(settings, home)
			if err != nil {
				t.Fatal(err)
			}
			want := relative
			if strings.HasPrefix(want, "~/") {
				want = filepath.Join(home, want[2:])
			} else if !filepath.IsAbs(want) {
				want = filepath.Join(dir, want)
			}
			if !slices.Equal(refs, []string{filepath.Clean(want)}) {
				t.Fatalf("references = %v, want %s", refs, want)
			}
		})
	}
}
