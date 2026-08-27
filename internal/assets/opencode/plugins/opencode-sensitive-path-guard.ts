import type { Plugin } from "@opencode-ai/plugin"
import { lstat } from "node:fs/promises"
import { resolve } from "node:path"
const REFUSAL = "opencode_sensitive_path_guard_refused"
const PATH_FIELDS = ["path", "include"] as const
function pathStrings(value: unknown): string[] {
  return typeof value === "string" ? [value] : Array.isArray(value) ? value.flatMap(pathStrings) : []
}
function normalize(value: string): string {
  const absolute = value.startsWith("/") || /^[A-Za-z]:[\\/]/.test(value)
  const parts: string[] = []
  for (const part of value.replaceAll("\\", "/").split("/")) {
    if (part === "" || part === ".") continue
    if (part === "..") {
      if (parts.length > 0 && parts[parts.length - 1] !== "..") parts.pop()
      else if (!absolute) parts.push(part)
      continue
    }
    parts.push(part)
  }
  return (absolute ? "/" : "") + parts.join("/")
}
function sensitivePath(value: string): boolean {
  const path = normalize(value)
  return /(^|\/)\.env(?:\..*)?$/i.test(path) || /(^|\/)\*\.env\*?$/i.test(path) || /(^|\/)secrets(?:\/|$)/i.test(path) ||
    /(^|\/)credentials\.json$/i.test(path) || /(^|\/)\.ssh(?:\/|$)/i.test(path) ||
    /(^|\/)\.credentials(?:\/|$)/i.test(path) || /(^|\/)Library\/Keychains(?:\/|$)/i.test(path) ||
    /(^|\/)\.aws\/credentials$/i.test(path) || /(^|\/)\.config\/gh\/hosts\.yml$/i.test(path) || /\.(pem|key)$/i.test(path)
}
async function broadTarget(value: string, root: string): Promise<boolean> {
  const path = normalize(value)
  if (path === "" || path === "." || path === "/" || path.endsWith("/")) return true
  if (!path.includes("*")) {
    try { const info = await lstat(resolve(root, path)); return info.isDirectory() || info.isSymbolicLink() } catch { return true }
  }
  const basename = path.slice(path.lastIndexOf("/") + 1)
  return path.includes("**") || !/^\*[^/]*\.[A-Za-z0-9]+$/.test(basename)
}
function grepTargets(args: unknown): string[] {
  return args !== null && typeof args === "object" && !Array.isArray(args)
    ? PATH_FIELDS.flatMap((field) => pathStrings((args as Record<string, unknown>)[field])) : []
}
export const OpenCodeSensitivePathGuard: Plugin = async (plugin) => ({
  "tool.execute.before": async (input, output) => {
    if (input.tool !== "grep") return
    const targets = grepTargets(output.args)
    const root = plugin.directory || plugin.worktree
    const broad = await Promise.all(targets.map((target) => broadTarget(target, root)))
    if (targets.length === 0 || targets.some((target, index) => sensitivePath(target) || broad[index])) {
      throw new Error(REFUSAL)
    }
  },
})
export default OpenCodeSensitivePathGuard
