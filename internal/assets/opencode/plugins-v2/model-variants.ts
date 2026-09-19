// gentle-ai:managed model-variants/v2
import { Plugin } from "@opencode/plugin"
import { mkdir, writeFile, rename, rm } from "node:fs/promises"
import { createHash, randomBytes } from "node:crypto"
import { homedir } from "node:os"
import path from "node:path"

export default Plugin.define({
  id: "gentle-ai.model-variants",
  setup(ctx) {
    const abort = new AbortController()
    // V2 locations cannot overwrite the V1 global catalog or each other.
    const key = createHash("sha256").update(JSON.stringify([ctx.location.directory, ctx.location.workspaceID ?? ""])).digest("hex")
    const directory = path.join(homedir(), ".gentle-ai", "cache", "opencode-v2")
    const destination = path.join(directory, `${key}.json`)
    const refresh = async () => {
      let temporary: string | undefined
      try {
        const result = await ctx.model.list()
        if (abort.signal.aborted || result.location.directory !== ctx.location.directory) return
        const variants: Record<string, Record<string, string[]>> = Object.create(null)
        for (const model of result.data) {
          if (!model.variants?.length) continue
          const provider = variants[model.providerID] ??= Object.create(null)
          provider[model.id] = model.variants.map(variant => variant.id).sort()
        }
        await mkdir(directory, { recursive: true })
        temporary = `${destination}.${randomBytes(6).toString("hex")}.tmp`
        await writeFile(temporary, JSON.stringify(variants), { mode: 0o600 })
        if (!abort.signal.aborted) await rename(temporary, destination)
      } catch { /* Catalog failure is unavailable coverage, never startup failure. */ }
      finally { if (temporary) await rm(temporary, { force: true }).catch(() => {}) }
    }
    // Subscribe before initial snapshot so startup/reload updates cannot be lost.
    const events = ctx.event.subscribe({ signal: abort.signal })[Symbol.asyncIterator]()
    // SharedEvents starts only on next(), not subscribe(). Handle rejection now
    // so a failed stream cannot become unhandled while the snapshot is pending.
    const next = () => events.next().then(value => ({ value }), error => ({ error }))
    const running = (async () => {
      try {
        let pending = next()
        await refresh()
        while (!abort.signal.aborted) {
          const result = await pending
          if ("error" in result || result.value.done) break
          const event = result.value.value
          pending = next()
          if (abort.signal.aborted) break
          if ((event.type === "model.updated" || event.type === "provider.updated")
            && event.location?.directory === ctx.location.directory
            && event.location?.workspaceID === ctx.location.workspaceID) await refresh()
        }
      } catch { /* No retry loop or global fallback cache. */ }
      finally { await events.return?.() }
    })()
    return async () => { abort.abort(); await running }
  },
})
