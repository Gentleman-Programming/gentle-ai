/**
 * Node.js preload script for PRoot/Termux platform override.
 *
 * Loaded via NODE_OPTIONS="--require /path/to/proot-override.js" when
 * opencode detects a PRoot environment. This script overrides the platform
 * properties that CodeGraph's npm-shim.js uses to select platform-specific
 * bundles, mapping "android" to "linux" so the correct bundle is downloaded.
 *
 * The script is non-destructive on non-PRoot systems — it only activates
 * when the CODEGRAPH_OVERRIDE_OS env var is set (which opencode sets
 * automatically during plugin installation when PRoot is detected).
 */

"use strict"

// Override process.platform if CODEGRAPH_OVERRIDE_OS is set
if (process.env.CODEGRAPH_OVERRIDE_OS) {
  Object.defineProperty(process, "platform", {
    value: process.env.CODEGRAPH_OVERRIDE_OS,
    writable: false,
    configurable: true,
  })
}

// Override process.arch if CODEGRAPH_OVERRIDE_ARCH is set
if (process.env.CODEGRAPH_OVERRIDE_ARCH) {
  Object.defineProperty(process, "arch", {
    value: process.env.CODEGRAPH_OVERRIDE_ARCH,
    writable: false,
    configurable: true,
  })
}
