# Engram MCP Lifecycle

Reference for the lifecycle that the `engram` binary must satisfy when
Claude Code registers it as a plugin. This page is the canonical runbook
for the gentle-ai side of the integration; the actual MCP server
implementation lives in
[`gentleman-programming/engram`](https://github.com/Gentleman-Programming/engram).

> **Status (2026-07-21):** the open defect is tracked in
> [Gentleman-Programming/gentle-ai#1019](https://github.com/Gentleman-Programming/gentle-ai/issues/1019).
> This runbook documents what gentle-ai emits and what Claude Code
> expects. The actual fix is upstream in the engram binary.

## What gentle-ai writes

`gentle-ai` registers `engram` with Claude Code by writing a single MCP
config file. The shape and path depend on the strategy chosen by the
Claude Code adapter:

| Adapter strategy | File path (Linux/macOS)                              | Equivalent on Windows          |
|------------------|------------------------------------------------------|-------------------------------|
| `StrategySeparateMCPFiles` | `~/.claude/mcp/engram.json`            | `%USERPROFILE%\.claude\mcp\engram.json` |
| Other strategies            | merged into `~/.claude/settings.json` | merged into the Windows settings.json |

For the common case (`StrategySeparateMCPFiles`), the file contents are:

```json
{
  "command": "engram",
  "args": ["mcp", "--tools=agent"]
}
```

The absolute path to the binary is resolved at write time and preserved
when the file already exists (see
`internal/components/engram/inject.go:798-828`).

## Expected MCP lifecycle

Claude Code's stdio MCP client drives the binary through the canonical
MCP protocol sequence:

1. Client sends `initialize` (JSON-RPC request).
2. Server responds with server info and capabilities.
3. **Server sends `notifications/initialized`** (required notification,
   per the MCP spec).
4. Client sends `tools/list`.
5. Server responds with the list of tools.
6. Client sends periodic `ping` requests.
7. Server responds to each ping with an empty result.

A reference implementation of this sequence lives at
`internal/components/communitytool/pi_codegraph.go:550-574` (read-only —
do not modify from this PR).

## Known-good version

| engram binary version | Status     |
|-----------------------|------------|
| `TBD`                 | Known good (will be set once a release ships the `notifications/initialized` notification). |

Until this constant is set, `gentle-ai doctor` does NOT emit a WARN for
old versions — the check is dormant. The dormant default is defined in
`internal/components/engram/doctor.go` as
`MinEngramVersionForHealthyLifecycle = "0.0.0"`. A `TODO` comment in
the same file marks the action required to flip the switch.

## Verifying the lifecycle locally

A regression test lives at
`internal/components/engram/lifecycle_test.go` and is guarded by the
`engram_lifecycle` build tag so it does NOT run under the default
`go test ./...` workflow. To run it explicitly:

```bash
go test -count=1 -tags engram_lifecycle ./internal/components/engram/...
```

The test:

1. Spawns the `engram` binary on `$PATH` via `exec.Cmd`.
2. Sends `initialize`, `notifications/initialized`, `tools/list`, and
   `ping` over stdio.
3. Asserts the binary stays alive for 10 seconds after the ping.
4. If the binary is missing, calls `t.Skip` with a clear message.
5. If the binary dies mid-test, fails with a diagnostic that names
   Gentleman-Programming/gentle-ai#1019 and includes the partial
   JSON-RPC exchange.

## See also

- [Gentleman-Programming/gentle-ai#1019](https://github.com/Gentleman-Programming/gentle-ai/issues/1019) — the open defect.
- `internal/components/engram/inject.go` — the registration writer.
- `internal/components/engram/verify.go:31-37` — the `engram version`
  seam reused by the doctor check.
- `internal/components/communitytool/pi_codegraph.go:550-574` —
  reference MCP lifecycle implementation (read-only).
