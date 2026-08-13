/**
 * reviewer-shim
 * Native OpenCode reviewer shim dispatcher (change #3138, slice 4).
 *
 * This plugin is the managed seam that will dispatch the opaque native
 * reviewer binding to the Go shim (internal/advisoryreview/opencode_shim.go).
 * During the migration window it carried ZERO logic while the legacy
 * review-result-artifacts.ts review half was still installed and remained the
 * sole injection source (SEN-RPC-17 / B7). Change #3138 slice 6 retired that
 * legacy plugin: its SDD half is native Go and its review half is gone, so
 * re-sync scrubs stale copies (single injection source). This plugin STILL
 * registers no hooks and takes over nothing -- the Go shim owns all dispatch
 * decisioning (provenance admission #3049, legacy deferral, and the one
 * binding route parse) and the glue never parses, never renders, and never
 * rewrites a prompt. A later slice activates dispatch here; the file must
 * stay hook-free until then.
 *
 * The plugin shape mirrors skill-registry.ts (@opencode-ai/plugin, pinned to
 * versions.OpenCode by the organic reviewer e2e harness): an async Plugin
 * factory that yields its hooks object. Here that object is empty, which is
 * the deferral: an empty hook set cannot double-inject by construction.
 */

import type { Plugin } from "@opencode-ai/plugin"

export const ReviewerShimPlugin: Plugin = async () => {
  return {}
}

export default ReviewerShimPlugin