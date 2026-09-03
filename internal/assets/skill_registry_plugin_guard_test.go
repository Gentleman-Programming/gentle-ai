package assets

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/skillregistry"
)

// TestSkillRegistryPluginSkipsNonProjectDirectories runs the real plugin
// under node with a fake `gentle-ai` on PATH that records its argv. OpenCode
// resolves a brand-new non-project directory to "/" (or another markerless
// location); the plugin must skip those without spawning, and spawn exactly
// once for a real project root.
func TestSkillRegistryPluginSkipsNonProjectDirectories(t *testing.T) {
	source, err := Read("opencode/plugins/skill-registry.ts")
	if err != nil {
		t.Fatal(err)
	}
	const harness = `import { existsSync, mkdirSync, mkdtempSync, readFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import plugin from "./plugin.mts"

const projectDir = mkdtempSync(join(tmpdir(), "skill-registry-project-"))
mkdirSync(join(projectDir, ".git"))
const bareDir = mkdtempSync(join(tmpdir(), "skill-registry-bare-"))
const skillsDir = mkdtempSync(join(tmpdir(), "skill-registry-skills-")); mkdirSync(join(skillsDir, ".claude", "skills"), { recursive: true })

await plugin({ worktree: "/", directory: "" })
await plugin({ worktree: "", directory: "" }) // falls back to process.cwd(): the markerless harness dir
await plugin({ worktree: bareDir, directory: "" })
await plugin({ worktree: projectDir, directory: "" })
await plugin({ worktree: skillsDir, directory: "" }) // no .git/.atl: a skills dir alone is a project marker

const logPath = process.env.GENTLE_AI_RELAY_LOG
const readLines = () => (existsSync(logPath) ? readFileSync(logPath, "utf8").split("\n").filter(Boolean) : [])
const deadline = Date.now() + 5000
while (Date.now() < deadline && readLines().length < 2) {
  await new Promise((resolve) => setTimeout(resolve, 50))
}
// Grace period so a wrongly-spawned refresh for a skipped directory can land.
await new Promise((resolve) => setTimeout(resolve, 300))
console.log(JSON.stringify({ project: projectDir, skills: skillsDir, lines: readLines() }))
`
	const relay = `#!/usr/bin/env node
import { appendFileSync } from "node:fs"
appendFileSync(process.env.GENTLE_AI_RELAY_LOG, JSON.stringify(process.argv.slice(2)) + "\n")
`
	output, _ := runOpenCodeTransportPluginHarness(t, map[string]string{"plugin.mts": string(source)}, harness, relay)
	// The plugin's stderr skip notices share the combined harness output with
	// the stdout result; the result is the final line. Skip notices must stay
	// off stdout so `opencode models --verbose` parsing never sees them, and
	// must stay calm notices, never refresh errors.
	trimmed := strings.Split(strings.TrimSpace(output), "\n")
	last := trimmed[len(trimmed)-1]
	for _, line := range trimmed[:len(trimmed)-1] {
		if !strings.HasPrefix(line, "[skill-registry] skipping refresh:") {
			t.Fatalf("unexpected plugin output line %q (skips must be calm notices, never errors)", line)
		}
	}
	var result struct {
		Project string   `json:"project"`
		Skills  string   `json:"skills"`
		Lines   []string `json:"lines"`
	}
	if err := json.Unmarshal([]byte(last), &result); err != nil {
		t.Fatalf("decode guard harness output %q: %v", output, err)
	}
	if len(result.Lines) != 2 {
		t.Fatalf("plugin must spawn exactly twice (.git project root and skills-dir-only workspace), got %d spawns: %v", len(result.Lines), result.Lines)
	}
	sort.Strings(result.Lines) // deterministic: "...-project-*" sorts before "...-skills-*"
	want := fmt.Sprintf(`["skill-registry","refresh","--quiet","--no-gitignore","--cwd",%q]`, result.Project)
	if result.Lines[0] != want {
		t.Fatalf("spawn argv = %s, want %s", result.Lines[0], want)
	}
	wantSkills := fmt.Sprintf(`["skill-registry","refresh","--quiet","--no-gitignore","--cwd",%q]`, result.Skills)
	if result.Lines[1] != wantSkills {
		t.Fatalf("skills-dir-only workspace spawn argv = %s, want %s", result.Lines[1], wantSkills)
	}
}

// TestSkillRegistryPluginMarkersMatchCLIGuard pins the plugin's PROJECT_MARKERS to the CLI guard's set: .git, .atl, and every ProjectSkillDirs workspace skill directory.
func TestSkillRegistryPluginMarkersMatchCLIGuard(t *testing.T) {
	list := regexp.MustCompile(`const PROJECT_MARKERS = \[[^\]]*\]`).FindString(MustRead("opencode/plugins/skill-registry.ts"))
	got := regexp.MustCompile(`"[^"]*"`).FindAllString(list, -1)
	want := []string{`".git"`, `".atl"`}
	for _, dir := range skillregistry.ProjectSkillDirs("") {
		want = append(want, `"`+filepath.ToSlash(dir)+`"`)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("plugin PROJECT_MARKERS = %v, want the CLI guard's markers %v", got, want)
	}
}
