// gentle-ai:managed skill-registry/v2
import { Plugin } from "@opencode/plugin"
import { execFile } from "node:child_process"
import { access } from "node:fs/promises"
import { homedir } from "node:os"
import { join, parse } from "node:path"

const PROJECT_MARKERS = [".git", ".atl", "skills", ".opencode/skills", ".claude/skills", ".gemini/skills", ".cursor/skills", ".github/skills", ".codex/skills", ".qwen/skills", ".kiro/skills", ".openclaw/skills", ".pi/skills", ".agent/skills", ".agents/skills", ".atl/skills", ".hermes/skills"]

export default Plugin.define({
  id: "gentle-ai.skill-registry",
  async setup(ctx) {
    // The loaded location, never process.cwd or the containing project's root.
    const cwd = ctx.location.directory
    if (!cwd || cwd === parse(cwd).root || cwd === homedir()) return
    let project = false
    for (const marker of PROJECT_MARKERS) {
      if (await access(join(cwd, marker)).then(() => true, () => false)) { project = true; break }
    }
    if (!project) return
    try {
      let child: ReturnType<typeof execFile> | undefined
      child = execFile("gentle-ai", ["skill-registry", "refresh", "--quiet", "--no-gitignore", "--cwd", cwd],
        { cwd, timeout: 30_000, maxBuffer: 1024, windowsHide: true }, () => { child = undefined })
      return () => { child?.kill("SIGKILL"); child = undefined }
    } catch { /* Missing executable is not a startup failure. */ }
  },
})
