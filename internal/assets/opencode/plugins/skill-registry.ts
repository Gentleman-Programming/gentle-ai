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
import { promisify } from "util"

const execFileAsync = promisify(execFile)

export const SkillRegistryPlugin: Plugin = async (input) => {
  async function refreshSkillRegistry() {
    const cwd = input.directory || input.worktree || process.cwd()

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
