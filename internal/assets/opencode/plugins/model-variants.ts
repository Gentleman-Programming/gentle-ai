/**
 * model-variants
 * Exports per-model variant (effort level) data for gentle-ai.
 *
 * On OpenCode startup, fetches the provider list via the in-process SDK client,
 * extracts variant keys per model, and writes a minimal JSON cache to
 * ~/.cache/gentle-ai/model-variants.json. gentle-ai reads this file
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

      if (Object.keys(variants).length === 0) return

      const cacheDir = path.join(homedir(), ".cache", "gentle-ai")
      await mkdir(cacheDir, { recursive: true })
      await writeFile(
        path.join(cacheDir, "model-variants.json"),
        JSON.stringify(variants, null, 2),
      )
    } catch {
      // Silent failure — file won't be created/updated
    }
  }

  // Don't await — server isn't ready during plugin init. Fire and forget.
  refreshVariantsCache().catch(() => {})

  return {}
}

export default ModelVariantsPlugin
