/**
 * model-variants
 * Exports per-model variant (effort level) data for gentle-ai.
 *
 * On OpenCode startup, fetches the provider list via the in-process SDK client,
 * extracts variant keys per model, and writes a minimal JSON cache to
 * ~/.gentle-ai/cache/model-variants.json. gentle-ai reads this file
 * to populate the effort level picker without needing a live API connection.
 */

import type { Plugin } from "@opencode-ai/plugin"
import { writeFile, mkdir, rename } from "fs/promises"
import { homedir } from "os"
import path from "path"

export const ModelVariantsPlugin: Plugin = async (input) => {
  async function refreshVariantsCache() {
    try {
      const result = await input.client.provider.list()
      const data = (result as any).data ?? result
      const providerList: any[] = data?.all ?? data?.providers ?? (Array.isArray(data) ? data : [])

      const variants: Record<string, Record<string, string[]>> = {}
      for (const prov of providerList) {
        for (const [modelId, model] of Object.entries(prov.models ?? {})) {
          const m = model as any
          if (m.variants && Object.keys(m.variants).length > 0) {
            variants[prov.id] = variants[prov.id] || {}
            variants[prov.id][modelId] = Object.keys(m.variants).sort()
          }
        }
      }

      const cacheDir = path.join(homedir(), ".gentle-ai", "cache")
      await mkdir(cacheDir, { recursive: true })

      // Atomic write: write to .tmp then rename. rename() is atomic on POSIX,
      // so concurrent readers (e.g. `gentle-ai sync`) never see a partial JSON.
      // Always write — even when empty — to avoid leaving a stale cache from
      // a previous run alive after providers stop reporting variants.
      const finalPath = path.join(cacheDir, "model-variants.json")
      const tmpPath = finalPath + ".tmp"
      await writeFile(tmpPath, JSON.stringify(variants, null, 2))
      await rename(tmpPath, finalPath)
    } catch (err) {
      console.error("[model-variants] cache refresh failed:", err)
    }
  }

  // Don't await — server isn't ready during plugin init. Fire and forget.
  refreshVariantsCache().catch((err) => {
    console.error("[model-variants] unexpected refresh error:", err)
  })

  return {}
}

export default ModelVariantsPlugin
