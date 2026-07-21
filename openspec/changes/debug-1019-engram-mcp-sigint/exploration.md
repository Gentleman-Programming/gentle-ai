# Exploration: debug-1019-engram-mcp-sigint

**Linked issue:** Gentleman-Programming/gentle-ai#1019 (GinoL221, 2026-07-05)

---

## TL;DR — Root Cause Hypothesis

**Confidence: Medium-Low** — insufficient direct evidence in this repo; fix hypothesis requires upstream verification.

**Hypothesis:** The Engram MCP server binary (not gentle-ai's registration) fails to meet Claude Code's MCP protocol lifecycle expectations — specifically, not sending `notifications/initialized` after the `initialize` handshake and/or not responding to periodic ping messages. Claude Code's stdio MCP client has a hard timeout (2-5s) for these lifecycle acknowledgements; when they're missing, it sends SIGINT, clears the connection cache, and never reconnects.

**Key discrepancy:** The issue states registration is via `enabledPlugins["engram@engram"]` in `settings.json`, but gentle-ai's Claude Code adapter writes to `~/.claude/mcp/engram.json` (StrategySeparateMCPFiles). These are structurally different registration paths. The `enabledPlugins` mechanism is not touched by any gentle-ai code in this repo.

---

## Where the Fix Must Live

**Classification: (B) engram binary — with a possible (D) combination**

The fix likely lives in the **engram binary's MCP server implementation** (upstream: `gentleman-programming/engram`). The binary passes the issue reporter's manual JSON-RPC test (`initialize` + `tools/list` over stdio), but that test bypasses Claude Code's MCP client behavior. Claude Code's stdio MCP client likely:

1. Sends `initialize` request
2. **Expects the server to send `notifications/initialized` back** (this is a required notification per MCP spec after initialization)
3. Sends periodic `ping` requests
4. Expects the server to respond to pings

If the Engram binary doesn't implement steps 2 or 3, Claude Code's client will timeout and SIGINT the process.

**Evidence for (B):** The issue reporter explicitly verified the binary is healthy with raw JSON-RPC. The gap is between "binary works in isolation" and "binary works when managed by Claude Code's stdio transport."

**Evidence for (A) as a contributor:** If the `~/.claude/mcp/engram.json` registration shape differs from what Claude Code's native `enabledPlugins` mechanism produces, there could be a difference in how Claude Code's client manages the process lifecycle (e.g., timeout values, stdio buffering, signal handling). gentle-ai could potentially work around this by registering via the same `enabledPlugins` mechanism that works for the issue reporter.

---

## Evidence Map — 9 Questions

### Q1: Where does gentle-ai register the Engram plugin?

**Finding: `~/.claude/mcp/engram.json` (NOT `enabledPlugins`)**

- `internal/agents/claude/adapter.go:107-108` — Claude adapter returns `StrategySeparateMCPFiles`
- `internal/components/engram/inject.go:318-332` — For StrategySeparateMCPFiles, writes to `~/.claude/mcp/engram.json` via `buildSeparateMCPContent`
- `internal/components/engram/inject.go:85-91` — `engramServerJSONWithCmd` produces: `{"command": cmd, "args": ["mcp", "--tools=agent"]}`
- **gentle-ai does NOT write to `enabledPlugins` anywhere in the codebase** — grep confirms zero matches for `enabledPlugins` or `engram@engram`

**Conclusion:** The issue's mention of `enabledPlugins["engram@engram"]` suggests either (a) the issue reporter has a different registration mechanism active, or (b) Claude Code has a native plugin system that consumes the same `~/.claude/mcp/engram.json` and exposes it under `enabledPlugins` in its UI.

---

### Q2: What MCP config does Engram emit?

**Finding: Config written to `~/.claude/mcp/engram.json` for Claude Code**

```json
{
  "command": "engram",
  "args": ["mcp", "--tools=agent"]
}
```

- `internal/components/engram/inject.go:85-91` — `engramServerJSONWithCmd`
- `internal/components/engram/inject.go:798-828` — `buildSeparateMCPContent` preserves absolute paths from `engram setup` while canonicalizing args to `["mcp", "--tools=agent"]`

**No other keys written.** The config is minimal — no `env`, no `metadata`, no lifecycle/keepalive options.

---

### Q3: What is the lifecycle Claude Code expects from a healthy stdio MCP server?

**Finding: `initialize` → `notifications/initialized` → `tools/list` → `ping` loop**

The canonical MCP lifecycle sequence (observed in `internal/components/communitytool/pi_codegraph.go:550-574`):

```go
// Step 1: Client sends initialize
encoder.Encode({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {...}})
// Step 2: Server responds with result
initializeResponse := readResponse(id=1)
// Step 3 (REQUIRED): Server sends notifications/initialized
encoder.Encode({"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}})
// Step 4: Client sends tools/list
encoder.Encode({"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}})
// Step 5: Server responds with tools
```

**Missing step 3 (`notifications/initialized`) is the most likely SIGINT trigger.** Per MCP protocol spec, the server MUST send this notification after processing `initialize`. If the Engram binary doesn't send it, Claude Code's client will consider the handshake incomplete and terminate the server.

---

### Q4: What does the Engram binary actually implement today?

**Finding: Binary is external to this repo; gentle-ai does not ship or bind the Engram binary**

- `internal/components/engram/download.go` — resolves/enumerates versions but does not embed the binary
- `internal/components/engram/verify.go` — runs `engram --version` for version checking only
- `internal/components/engram/setup.go:42-49` — runs `engram setup --help` as a capability probe (5s timeout)

**The Engram binary is a separate Go module** (`github.com/gentleman-programming/engram`). gentle-ai depends on it being pre-installed or installed via `go install`.

---

### Q5: What changed since ~2026-07-03 (symptom onset)?

**Finding: 3 relevant commits in the July 1-10 window**

| Commit | Date | Change | Relevance to SIGINT |
|--------|------|--------|---------------------|
| `aba1adc` | Jul 3 | `fix(release): address user-blocking setup issues` — changed MCP injector to route Claude Code with StrategySeparateMCPFiles through `injectMergeIntoSettings` for Context7 | **Low** — affects Context7 (not Engram) injection |
| `d9c1c89` | Jul 8 | `feat(engram): consolidate protocol assets and slim Claude Code section` — added `protocolFor`, slim section gating, `--protocol` probe | **Medium** — changed Claude Code's injected protocol content |
| `e6d7728` | Jul 9 | `fix(engram): ensure saving to memory never replaces the user reply` — added delivery guarantee clause to protocol | **Low** — protocol text only |

**Key change:** `d9c1c89` introduced `protocolFor()` and `IsVerifiedSlimAdapter()` (`internal/components/engram/protocol.go`), gating the 11-line slim section on engram binary version ≥1.4.0. If the user's binary is below v1.4.0 and the gating is incorrectly applied, the slim section might be sent instead of full — but this affects prompt content, not MCP lifecycle.

**The `aba1adc` change to `mcp/inject.go` does NOT affect Engram** — it's a Context7-only change (`internal/components/mcp/inject.go:26-27`).

---

### Q6: Is there a competing MCP registration?

**Finding: NO — gentle-ai only registers via `~/.claude/mcp/engram.json`**

- grep confirms: `enabledPlugins` and `engram@engram` have zero matches in Go source
- The issue's `enabledPlugins` reference is likely Claude Code's native plugin UI showing the MCP server, not a separate registration path
- No evidence of duplicate registrations from `gentle-ai doctor` or `gentle-ai init` re-running

---

### Q7: What existing tests cover the Engram registration path?

**Finding: Yes — `inject_test.go` covers the Claude Code registration shape**

- `internal/components/engram/inject_test.go:57-96` — `TestInjectClaudeWritesMCPConfig` validates `~/.claude/mcp/engram.json` contains `command` and `args` with `--tools=agent`
- `internal/components/engram/inject_test.go:98+` — `TestInjectClaudeWritesProtocolSection` validates CLAUDE.md injection
- `internal/components/engram/delivery_guarantee_installed_test.go` — semantic tests on installed outputs
- **No tests verify the MCP server lifecycle** — zero tests send `initialize`, `notifications/initialized`, or `ping` messages to a spawned Engram process

---

### Q8: What is the actual blast radius of a fix?

**Finding: High if registration shape changes; Low if binary behavior changes**

- If (A) gentle-ai changes the registration shape for Claude Code (e.g., adds `enabledPlugins` or changes the JSON structure):
  - Affected: every agent using StrategySeparateMCPFiles (`internal/model/types.go:125-127`)
  - Risk: older config files would need migration

- If (B) engram binary is fixed upstream:
  - gentle-ai no-op — nothing changes in registration
  - Risk: users with older engram binaries would still have the issue

---

### Q9: Where does the fix actually live?

**Classification: (B) engram binary primary, (A) gentle-ai workaround possible**

**B (engram binary):** The Engram MCP server implementation needs to send `notifications/initialized` after the `initialize` handshake and respond to ping messages. This is upstream (`gentleman-programming/engram`), not this repo.

**A (gentle-ai):** As a workaround, gentle-ai could investigate whether registering Engram via a different mechanism (e.g., `enabledPlugins`-compatible config, or adding lifecycle options like `timeout` or `ping` to the server config) makes Claude Code's client behave differently. However, there is no code evidence that the current `~/.claude/mcp/engram.json` shape is wrong.

**C (Claude Code):** If Claude Code's MCP client has a bug where it sends SIGINT prematurely even for a correctly-behaving server, that would be an Anthropic issue — outside both gentle-ai and engram's control.

---

## Healthy vs Broken Logs Comparison

| Aspect | Healthy (pre-~Jul 3) | Broken (post-~Jul 3) |
|--------|----------------------|----------------------|
| Connection | Single connect, stays alive | SIGINT after 2-5s |
| Reconnection | None needed | "Cleared connection cache for reconnection", no reconnect |
| `notifications/initialized` | Presumably sent | Presumably missing |
| Protocol section | Full (~65 lines) | Slim (~11 lines) if binary ≥1.4.0 |
| Registration path | `~/.claude/mcp/engram.json` | Same |

**What changed:** The protocol section injected into `CLAUDE.md` changed from full to slim (for binaries ≥v1.4.0) via `d9c1c89`. The slim section is 11 lines vs ~65 lines. However, this only affects the system prompt content — it should NOT affect the MCP server lifecycle.

**The SIGINT pattern (2-5s, clear cache, no reconnect) points to a lifecycle timeout, NOT a content issue.**

---

## Reproduction Plan

### Step 1: Instrument the Engram binary's MCP server
Add temporary debug logging to the Engram binary (upstream) to trace:
- [ ] Is `notifications/initialized` being sent after `initialize`?
- [ ] Is the server responding to `ping` messages?
- [ ] What is the actual timeline from startup to SIGINT?

### Step 2: Compare Claude Code's MCP client behavior
Trace Claude Code's MCP client logs (enable with `CLAUDE_TRACE=1` or equivalent) to confirm:
- [ ] Is `initialize` being sent?
- [ ] Is `notifications/initialized` being received?
- [ ] Is `ping` being sent? If so, is there a response?

### Step 3: Test with different registration shapes
If Step 1-2 confirm the lifecycle gap, test whether different registration shapes work:
- [ ] Register via `enabledPlugins` in `settings.json` instead of `~/.claude/mcp/engram.json`
- [ ] Add `timeout` or `ping` fields to the server config (if Claude Code supports them)
- [ ] Use a wrapper script that filters/logs stdio traffic

### Step 4: Verify fix in engram binary
Once the missing lifecycle message is identified, patch the engram binary and re-test.

---

## Risks

1. **Wrong root cause hypothesis** — If the issue is actually a Claude Code behavior change (not engram binary), the investigation in (B) will not resolve it.
2. **Cannot reproduce without Claude Code** — this repo has no Claude Code binary; reproduction requires the issue reporter's environment.
3. **Registration path discrepancy** — the issue says `enabledPlugins`, gentle-ai writes `~/.claude/mcp/engram.json`. If the issue reporter has a different mechanism, the fix may not apply to their setup.
4. **Version gating uncertainty** — `IsVerifiedSlimAdapter` gates on engram ≥v1.4.0. If the issue reporter has v1.4.0+ but something else is wrong, the gating logic is not the culprit.
5. **The `aba1adc` change to MCP injector** — while it only affects Context7 (not Engram), it shows that Claude Code registration behavior changed on July 3. If Context7 changed, Engram might also be affected indirectly.

---

## Open Questions

1. **UNKLIKELY — needs data:** Does the Engram binary send `notifications/initialized` after `initialize`? The issue reporter's manual test (`initialize` + `tools/list`) does NOT test this — the binary could be failing this step without the manual test detecting it.

2. **UNKNOWN — needs data:** Does Claude Code send `ping` messages to MCP servers? If so, does the Engram binary respond to them?

3. **UNKNOWN — needs data:** What is the exact content of the issue reporter's `~/.claude/settings.json` and `~/.claude/mcp/engram.json`? Are there competing registrations?

4. **UNKNOWN — needs data:** What engram binary version does the issue reporter have? (`engram --version`)

5. **UNKNOWN — needs data:** Does the issue reproduce with `~/.claude/mcp/engram.json` deleted and gentle-ai re-run, forcing a fresh registration?

6. **UNKNOWN — needs data:** What happens if the user runs `engram mcp --tools=agent` manually under `claude` (not via Claude Code's MCP manager) — does it stay alive?

7. **UNKLIKELY — needs data:** Could the `--tools=agent` flag (which limits exposed tools) cause Claude Code's client to fail differently than `--tools=all` or no flag?

8. **UNKNOWN — needs data:** Is there a Claude Code version that changed behavior around July 3, 2026?

---

## Next Phase Recommendation

**Phase: sdd-propose** is NOT yet warranted. **More investigation needed first.**

The primary blocker is the **upstream engram binary's MCP server implementation** — we cannot fix that from this repo. Before proposing, we need:

1. **Engram binary MCP lifecycle trace** — add temporary logging to `gentleman-programming/engram` to confirm whether `notifications/initialized` is being sent.
2. **Claude Code MCP client trace** — confirm whether `notifications/initialized` is being received and whether `ping` messages are being sent.
3. **Issue reporter's config dump** — `~/.claude/settings.json`, `~/.claude/mcp/engram.json`, and `engram --version`.

If the investigation confirms (B) engram binary is the fix location, the proposal should be:
- **For gentle-ai (A):** Document the working registration shape, add a test that verifies the lifecycle, and potentially add a warning if the binary version doesn't support `notifications/initialized`.
- **For engram binary (upstream):** File an issue against `gentleman-programming/engram` with the reproduction steps and lifecycle trace.

**If the investigation reveals the issue is actually in gentle-ai's registration shape**, then sdd-propose can proceed with option (A) or (D) as the primary fix.
