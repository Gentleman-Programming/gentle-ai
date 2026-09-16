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
import { writeFile, mkdir, rename, rm } from "fs/promises"
import { randomBytes } from "crypto"
import { homedir } from "os"
import path from "path"

const MODEL_VARIANTS_CACHE_FILE = "model-variants.json"

function isIgnorableFileRace(err: unknown) {
  return typeof err === "object" && err !== null && "code" in err && (err as { code?: string }).code === "ENOENT"
}

async function removeOwnTempFile(tmpPath: string) {
  try {
    await rm(tmpPath, { force: true })
  } catch (err) {
    if (!isIgnorableFileRace(err)) {
      console.error("[model-variants] temp cleanup failed:", err)
    }
  }
}

export const ModelVariantsPlugin: Plugin = async (input) => {
  async function refreshVariantsCache() {
    let tmpPath: string | undefined
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

      const cacheDir = path.join(homedir(), ".axiom", "cache")
      await mkdir(cacheDir, { recursive: true })

      // Always write through a per-invocation tmp file before renaming, so
      // readers never see partial JSON and concurrent plugin loads do not
      // race over the same tmp path. See issues #766 and #786.
      const finalPath = path.join(cacheDir, MODEL_VARIANTS_CACHE_FILE)
      tmpPath = path.join(cacheDir, `${MODEL_VARIANTS_CACHE_FILE}.${randomBytes(3).toString("hex")}.tmp`)
      await writeFile(tmpPath, JSON.stringify(variants, null, 2))
      await rename(tmpPath, finalPath)
      tmpPath = undefined
    } catch (err) {
      console.error("[model-variants] cache refresh failed:", err)
    } finally {
      if (tmpPath) {
        await removeOwnTempFile(tmpPath)
      }
    }
  }

  // Don't await — server isn't ready during plugin init. Fire and forget.
  refreshVariantsCache().catch((err) => {
    console.error("[model-variants] unexpected refresh error:", err)
  })

  return {}
}

export default ModelVariantsPlugin
