# OpenCode compatibility

Axiom selects integrations from the detected OpenCode major version. It does
not silently migrate an existing installation. Unknown, unsupported or ambiguous
version evidence refuses incompatible writes rather than assuming V2.

| Surface | V1 | V2 (tested release: 2.0.4) |
| --- | --- | --- |
| Config, model references and permissions | Existing behavior retained | Native config/profile/MCP handling and permission-preserving merges tested |
| Managed plugins | Existing assets retained | Separate telemetry, model catalog, skill registry, SDD and staged review assets |
| Gentle logo | Existing placement unchanged | Explicitly skipped; no equivalent `home_logo` slot, no relocation or config writes |
| Community TUI plugins | Existing integration retained | Compatibility unproven; installation/update refuses, not silently omitted |
| Native review | Existing V1 capability path retained | Unavailable; no positive review admission advertised |

## What has actually been checked

All five V2 assets typecheck against released `@opencode/plugin@2.0.4` declarations.
Isolated actual-host fixtures activated them in separate global and project scopes.
A scripted loopback provider proved foreground subagent dispatch, hook ordering,
structured completed output and exact raw child bytes. It also proved inherited
project/child-agent instructions and an empty deny-all child tool inventory.

That ordinary child is **not an isolated reviewer**. Scripted output is transport
proof, not model quality, authority, consent or native review admission. Actual
native review attempts remain refused before a review child is launched. The
negative shell declaration also prevents a V2 host from inheriting V1 capability
merely because `opencode` on PATH points to V1.

V1 regression coverage and V2 fixture checks are recorded in
[`odd/tasks/opencode-v2-support.md`](../odd/tasks/opencode-v2-support.md). Integrated functional checks passed through the scoped follow-ups recorded there;
the initial full-suite command failed on its CLI timeout and legacy Bash. Native
review and complete runtime certification remain separate pending gates.

## Installation and verification boundaries

New CLI installation advice uses `@opencode/cli`; its required install scripts must
not be disabled. Plugin dependencies differ: V1 uses `@opencode-ai/plugin`, while
V2 uses `@opencode/plugin`. Existing user-owned incompatible assets are preserved.
The accepted temporary logo omission does not authorize dropping other features.

The local conformance harness uses disposable configuration, an allowlisted
environment, fixture authentication and process cleanup. Its loopback mode requires
a supported per-process network guard and never uses an external model. CI checks
released SDK types and fixture unit tests; it does not imply that the Darwin-only
organic host fixture ran on every supported operating system. Positive V2 native
review conformance and the broader runtime matrix remain incomplete.
