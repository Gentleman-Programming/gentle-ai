# Pi's built-in MCP consumer: offline characterization

This is a tagged Linux check of the **published Pi CLI**, not a new MCP client,
SDK-session test, installer acceptance test, or connection to the Context7 service.
The ordinary Go suite has no new Node dependency.

Run `TestUserContext7FrozenBuiltinMCPCLI` with `shell_context7_canonical` only in
an asserted offline Guest. Supply the committed Pi 1.0.0 archive at
`/frozen-pi.tgz`, its existing read-only stock dependency closure at
`/node_modules/@earendil-works/pi-coding-agent/node_modules`, and authenticated
Node 24.18 at `/opt/node/bin/node`. No install scripts, npm, or network acquisition
are needed. The test verifies the archive against the committed lock before
private extraction, then executes its actual `dist/bundle/cli.js mcp list --json`.

A local HTTP fixture proves initialization and discovery of `fixture_docs`.
Disabled and malformed entries must cause **zero HTTP requests**; disconnected
and malformed entries must return exit 1. Each canonical `agent/mcp.json` must
retain its bytes, inode, permissions and modification time. Private HOME, Pi
agent directory and Gentle config home are deliberately distinct.

**Health is not session readiness.** The CLI still connects with
`extensions: ["-builtin:mcp"]`, because shell MCP commands do not load extensions.
An installer must separately establish that its session's built-in MCP is active
and that the actual Context7 endpoint works. Tool discovery here does not prove
runtime tool execution, project-override behavior, full dependency provenance,
packed lifecycle, or Linux/Darwin/Windows installed qualification.

Commands use a test-local Linux process group, group cancellation and bounded
pipe draining, with best-effort owned-group cleanup. Ordinary Linux regression
controls cover normal output, an inherited pipe and cancellation without Node.
These new assertions have not been executed during recovery; they do not prove
escaped-descendant containment, hostile-same-UID custody or full-tree reaping.
