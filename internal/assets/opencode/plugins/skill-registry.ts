/**
 * skill-registry
 * Refreshes Gentle AI's project skill registry when OpenCode starts.
 *
 * Codex and Claude Code use native startup hooks for the same command. OpenCode
 * loads plugins at startup, so this plugin provides the equivalent behavior
 * without depending on shell interpolation or command-file parse-time cwd.
 *
 * Failure policy (issue #2971): the plugin is best-effort and must never
 * block OpenCode startup. When the refresh command fails, we emit one
 * actionable single line that distinguishes a missing binary from an
 * unreachable working directory, instead of a raw Node `ENOENT` stack.
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

/**
 * Sanitize a working-directory string for inclusion in a shell example and
 * for safe single-line logging. `JSON.stringify` produces a POSIX-safe
 * double-quoted form (escapes control characters and shell metacharacters)
 * and never embeds a literal newline.
 */
function quoteCwd(cwd: string): string {
  return JSON.stringify(cwd)
}

function singleLine(s: string): string {
  return s.replace(/[\r\n]+/g, " ")
}

/**
 * Classify an `execFileAsync` failure for `gentle-ai skill-registry refresh`
 * into a single actionable log line.
 *
 * - `ENOENT` from a `spawn gentle-ai` syscall: the binary was not on the
 *   OpenCode process PATH. Emit one line that names the missing binary and
 *   the manual continuation command.
 * - `ENOENT` from any other syscall (typically `access`/`stat` on the
 *   working directory): the working directory itself is invalid. Emit one
 *   line that names the cwd rather than falsely blaming the binary.
 * - Anything else: emit one line that includes the error code (when
 *   available) and message, without printing the Node stack object.
 *
 * Exported so the Go-side test fixture can drive the string-generation
 * logic through a Node subprocess without going through OpenCode's plugin
 * loader.
 */
export function describeRefreshFailure(err: unknown, cwd: string): string {
  const code = (err as NodeJS.ErrnoException | undefined)?.code
  const syscall = (err as NodeJS.ErrnoException | undefined)?.syscall
  const cwdExample = quoteCwd(cwd)
  if (code === "ENOENT" && (!syscall || syscall.startsWith("spawn"))) {
    return singleLine(
      `[skill-registry] gentle-ai executable was not found on the PATH inherited by the OpenCode process; ` +
      `skipping the skill-registry refresh for ${cwdExample}. ` +
      `Run \`gentle-ai skill-registry refresh --cwd ${cwdExample}\` from a shell where gentle-ai is installed, ` +
      `then re-launch OpenCode in a session that inherits that PATH. ` +
      `Plugin stays best-effort and does not block startup.`,
    )
  }
  if (code === "ENOENT") {
    return singleLine(
      `[skill-registry] gentle-ai skill-registry refresh could not access the working directory ${cwdExample}: ` +
      `ENOENT reached execFile (syscall=${syscall ?? "unknown"}). ` +
      `Verify the directory exists and is reachable from the OpenCode process. ` +
      `Plugin stays best-effort and does not block startup.`,
    )
  }
  const safeMessage = err instanceof Error ? err.message : String(err)
  const codeTag = code ? ` code=${code}` : ""
  return singleLine(
    `[skill-registry] refresh failed for ${cwdExample}${codeTag}: ${safeMessage}`,
  )
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
    console.error(`[skill-registry] unexpected refresh error for "${cwd}": ${singleLine(message)}`)
  })

  return {}
}

export default SkillRegistryPlugin
