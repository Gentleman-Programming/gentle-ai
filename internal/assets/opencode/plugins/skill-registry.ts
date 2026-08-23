/**
 * skill-registry
 * Refreshes Gentle AI's project skill registry when OpenCode starts.
 *
 * Codex and Claude Code use native startup hooks for the same command. OpenCode
 * loads plugins at startup, so this plugin provides the equivalent behavior
 * without depending on shell interpolation or command-file parse-time cwd.
 */

import type { Plugin } from "@opencode-ai/plugin"
import { execFile } from "child_process"
import { access } from "fs/promises"
import { homedir } from "os"
import { join, parse } from "path"
import { promisify } from "util"

const execFileAsync = promisify(execFile)

// Mirrors the CLI guard's markers (.git, .atl, and ProjectSkillDirs in internal/skillregistry/registry.go); a Go parity test pins this list.
const PROJECT_MARKERS = [".git", ".atl", "skills", ".opencode/skills", ".claude/skills", ".gemini/skills", ".cursor/skills", ".github/skills", ".codex/skills", ".qwen/skills", ".kiro/skills", ".openclaw/skills", ".pi/skills", ".agent/skills", ".agents/skills", ".atl/skills", ".hermes/skills"]

async function pathExists(path: string): Promise<boolean> {
  try {
    await access(path)
    return true
  } catch {
    return false
  }
}

/**
 * OpenCode started in a brand-new non-project directory can resolve the
 * working directory to "/", the user's home directory, or a markerless
 * scratch folder. Refreshing there would initialize a stray .atl registry
 * (or fail loudly on a read-only root) at every startup. The CLI refuses
 * those locations too; this guard skips the spawn entirely.
 */
async function isProjectRoot(cwd: string): Promise<boolean> {
  if (!cwd) return false
  if (cwd === parse(cwd).root) return false
  if (cwd === homedir()) return false
  for (const marker of PROJECT_MARKERS) if (await pathExists(join(cwd, ...marker.split("/")))) return true
  return false
}

export const SkillRegistryPlugin: Plugin = async (input) => {
  async function refreshSkillRegistry() {
    const cwd = input.worktree || input.directory || process.cwd()

    if (!(await isProjectRoot(cwd))) {
      // Startup hooks must not scream: a non-project directory is a normal
      // situation, not an error.
      console.info("[skill-registry] skipping refresh: not a project root:", cwd)
      return
    }

    try {
      await execFileAsync(
        "gentle-ai",
        ["skill-registry", "refresh", "--quiet", "--no-gitignore", "--cwd", cwd],
        { timeout: 30_000 },
      )
    } catch (err) {
      console.error("[skill-registry] refresh failed:", err)
    }
  }

  // Don't await — keep OpenCode startup responsive. The command is
  // fingerprint-cached, so normal startup stays cheap.
  refreshSkillRegistry().catch((err) => {
    console.error("[skill-registry] unexpected refresh error:", err)
  })

  return {}
}

export default SkillRegistryPlugin
