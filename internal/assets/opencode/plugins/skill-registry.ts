/**
 * skill-registry
 * Refreshes Gentle AI's project skill registry when OpenCode starts.
 *
 * Codex and Claude Code use native startup hooks for the same command. OpenCode
 * loads plugins at startup, so this plugin provides the equivalent behavior
 * without depending on shell interpolation or command-file parse-time cwd.
 *
 * Failure policy (issue #2971): the plugin is best-effort and must never
 * block OpenCode startup. When `gentle-ai` is not on the OpenCode process PATH
 * we emit one actionable line that names the missing binary and the manual
 * continuation, instead of a raw Node `ENOENT` stack. Other failures keep a
 * concise single line with the error code so the cause is still diagnosable.
 */

import type { Plugin } from "@opencode-ai/plugin"
import { execFile } from "child_process"
import { access } from "fs/promises"
import { homedir } from "os"
import { join, parse } from "path"
import { promisify } from "util"

const execFileAsync = promisify(execFile)

/**
 * Classify an execFileAsync failure into a single actionable log line.
 *
 * - `ENOENT` (libuv spawn failure): gentle-ai was not on PATH. Emit one line
 *   that names the missing binary and the manual continuation command, and
 *   stay silent about the Node stack trace.
 * - Anything else: emit one line with the error code (when available) and
 *   message, without printing the Node stack object.
 *
 * Exported so future CI coverage (or manual smoke scripts) can exercise the
 * string-generation logic without going through OpenCode's plugin loader.
 */
export function describeRefreshFailure(err: unknown, cwd: string): string {
  const code = (err as NodeJS.ErrnoException | undefined)?.code
  if (code === "ENOENT") {
    return (
      `[skill-registry] gentle-ai executable was not found on the PATH inherited by the OpenCode process; ` +
      `skipping the skill-registry refresh for "${cwd}". ` +
      `Run \`gentle-ai skill-registry refresh --cwd "${cwd}"\` from a shell where gentle-ai is installed, ` +
      `then re-launch OpenCode in a session that inherits that PATH. ` +
      `Plugin stays best-effort and does not block startup.`
    )
  }
  const safeMessage = err instanceof Error ? err.message : String(err)
  const codeTag = code ? ` code=${code}` : ""
  return `[skill-registry] refresh failed for "${cwd}"${codeTag}: ${safeMessage}`
}

export const SkillRegistryPlugin: Plugin = async (input) => {
  async function refreshSkillRegistry() {
    const cwd = input.worktree || input.directory || process.cwd()

    if (!(await isProjectRoot(cwd))) {
      // Startup hooks must not scream: a non-project directory is a normal
      // situation, not an error. Log to stderr — stdout belongs to commands
      // like `opencode models --verbose`, whose output gentle-ai parses.
      console.error("[skill-registry] skipping refresh: not a project root:", cwd)
      return
    }

    try {
      await execFileAsync(
        "gentle-ai",
        ["skill-registry", "refresh", "--quiet", "--no-gitignore", "--cwd", cwd],
        { timeout: 30_000 },
      )
    } catch (err) {
      console.error(describeRefreshFailure(err, cwd))
    }
  }

  // Don't await — keep OpenCode startup responsive. The command is
  // fingerprint-cached, so normal startup stays cheap.
  refreshSkillRegistry().catch((err) => {
    const message = err instanceof Error ? err.message : String(err)
    console.error(`[skill-registry] unexpected refresh error for "${cwd}": ${message}`)
  })

  return {}
}

export default SkillRegistryPlugin
