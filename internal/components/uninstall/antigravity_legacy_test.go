package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func writeAntigravityLegacyFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readAntigravityMCPServers(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	return servers
}

const antigravityLegacyMCPConfig = `{"mcpServers":{"context7":{"serverUrl":"https://mcp.context7.com/mcp"},"engram":{"command":"engram","args":["mcp"]},"mine":{"command":"mine"}}}` + "\n"

// TestUninstallAntigravityCleansLegacyVariantRoots covers installs made
// before Gentle AI targeted agy's shared ~/.gemini/config root: their skills
// and MCP entries live under ~/.gemini/antigravity-{cli,desktop}. Uninstall
// must remove those managed entries and keep user-owned skills and servers.
func TestUninstallAntigravityCleansLegacyVariantRoots(t *testing.T) {
	for _, variant := range []string{"antigravity-cli", "antigravity-desktop"} {
		t.Run(variant, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			root := filepath.Join(home, ".gemini", variant)
			skillDir := filepath.Join(root, "skills")
			managedSkill := filepath.Join(skillDir, "go-testing", "SKILL.md")
			userSkill := filepath.Join(skillDir, "my-skill", "SKILL.md")
			writeAntigravityLegacyFile(t, managedSkill, "managed\n")
			writeAntigravityLegacyFile(t, userSkill, "mine\n")
			shared, err := skills.SharedReferencePaths(skillDir)
			if err != nil || len(shared) == 0 {
				t.Fatalf("SharedReferencePaths() = %v, %v", shared, err)
			}
			for _, path := range shared {
				writeAntigravityLegacyFile(t, path, "shared\n")
			}
			mcpPath := filepath.Join(root, "mcp_config.json")
			writeAntigravityLegacyFile(t, mcpPath, antigravityLegacyMCPConfig)

			svc, err := NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			svc.snapshotter = stubSnapshotter{}
			components := []model.ComponentID{model.ComponentSkills, model.ComponentContext7, model.ComponentEngram}
			if _, err := svc.PartialUninstall([]model.AgentID{model.AgentAntigravity}, components); err != nil {
				t.Fatalf("PartialUninstall(antigravity) error = %v", err)
			}

			for _, path := range append([]string{managedSkill}, shared...) {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Errorf("managed legacy skill file %q must be removed; lstat err = %v", path, err)
				}
			}
			if _, err := os.Stat(userSkill); err != nil {
				t.Errorf("user skill %q must survive uninstall: %v", userSkill, err)
			}
			servers := readAntigravityMCPServers(t, mcpPath)
			if servers["context7"] != nil || servers["engram"] != nil || servers["mine"] == nil {
				t.Errorf("legacy mcpServers = %v, want managed servers removed and user server kept", servers)
			}
		})
	}
}

// TestUninstallAntigravityLeavesLegacySymlinkIntoSharedRoot pins the migrated
// layout: the variant mcp_config.json and skills dir are symlinks into
// ~/.gemini/config. Uninstall cleans the shared files once and never
// rewrites or replaces the symlinks themselves.
func TestUninstallAntigravityLeavesLegacySymlinkIntoSharedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	shared := filepath.Join(home, ".gemini", "config")
	sharedMCP := filepath.Join(shared, "mcp_config.json")
	writeAntigravityLegacyFile(t, sharedMCP, antigravityLegacyMCPConfig)
	writeAntigravityLegacyFile(t, filepath.Join(shared, "skills", "my-skill", "SKILL.md"), "mine\n")
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		filepath.Join(cliDir, "mcp_config.json"): sharedMCP,
		filepath.Join(cliDir, "skills"):          filepath.Join(shared, "skills"),
	}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}

	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	components := []model.ComponentID{model.ComponentSkills, model.ComponentContext7, model.ComponentEngram}
	if _, err := svc.PartialUninstall([]model.AgentID{model.AgentAntigravity}, components); err != nil {
		t.Fatalf("PartialUninstall(antigravity) error = %v", err)
	}

	servers := readAntigravityMCPServers(t, sharedMCP)
	if servers["context7"] != nil || servers["engram"] != nil || servers["mine"] == nil {
		t.Errorf("shared mcpServers = %v, want managed servers removed and user server kept", servers)
	}
	for link := range links {
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("variant entry %q must remain a symlink into the shared root; lstat err = %v", link, err)
		}
	}
}

// TestUninstallGeminiCLILeavesAntigravityLegacyRoots proves the legacy cleanup
// is scoped to Antigravity: Gemini CLI shares ~/.gemini but never owned the
// Antigravity variant directories.
func TestUninstallGeminiCLILeavesAntigravityLegacyRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".gemini", "antigravity-cli")
	legacySkill := filepath.Join(root, "skills", "go-testing", "SKILL.md")
	writeAntigravityLegacyFile(t, legacySkill, "managed\n")
	mcpPath := filepath.Join(root, "mcp_config.json")
	writeAntigravityLegacyFile(t, mcpPath, antigravityLegacyMCPConfig)
	geminiSkill := filepath.Join(home, ".gemini", "skills", "go-testing", "SKILL.md")
	writeAntigravityLegacyFile(t, geminiSkill, "managed\n")

	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	components := []model.ComponentID{model.ComponentSkills, model.ComponentContext7, model.ComponentEngram}
	if _, err := svc.PartialUninstall([]model.AgentID{model.AgentGeminiCLI}, components); err != nil {
		t.Fatalf("PartialUninstall(gemini-cli) error = %v", err)
	}

	if _, err := os.Lstat(geminiSkill); !os.IsNotExist(err) {
		t.Errorf("Gemini CLI skill %q must be removed; lstat err = %v", geminiSkill, err)
	}
	if _, err := os.Stat(legacySkill); err != nil {
		t.Errorf("Antigravity legacy skill %q must survive a Gemini CLI uninstall: %v", legacySkill, err)
	}
	if got := readAntigravityMCPServers(t, mcpPath); len(got) != 3 {
		t.Errorf("Antigravity legacy mcpServers = %v, want untouched", got)
	}
}
