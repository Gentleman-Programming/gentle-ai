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
import { writeFile, mkdir } from "fs/promises"
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

      // Write directly. On Windows, rename() is fragile with OneDrive-synced
      // directories — the .tmp file can vanish between write and rename.
      // The Go-side reader already handles a missing/incomplete cache gracefully.
      await writeFile(path.join(cacheDir, "model-variants.json"), JSON.stringify(variants, null, 2))
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
