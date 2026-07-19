## Exploration: reject-unknown-upgrade-flags

### Current State
`RunArgs` dispatches `upgrade` to `runUpgrade` after system detection. `runUpgrade` manually recognizes `--dry-run`/`-n` and `--no-backup`; non-flag arguments become tool filters, while any other flag-shaped argument is silently ignored. Validation happens before home-directory resolution, update checks, spinner output, upgrade execution, report rendering, and possible restart guidance, so an early parser error can prevent all upgrade side effects.

`update.CheckFiltered` filters against the registered `update.Tools` list but explicitly ignores unknown tool names. Existing seams (`updateCheckFiltered`, `upgradeExecuteWithOptions`, and related legacy `upgradeExecute`) allow focused tests without network or real upgrade commands. Existing tests cover failed update checks stopping execution, flags, positional `engram`, rendering, and the absence of install/sync behavior, but do not cover unknown flags or unknown positional filters with strict side-effect assertions.

The approved issue expects a clear non-zero CLI error for `--bad-flag` and `-x` before checking or upgrading tools. It also raises unknown positional tool filters as a policy decision: they should either be rejected clearly or deliberately remain a no-op. The current behavior makes both unknown flags silently accepted and unknown filters silently ignored.

### Affected Areas
- `internal/app/app.go` — `runUpgrade` argument loop is the validation boundary and must reject unsupported flag-shaped arguments before filesystem, update, spinner, or upgrade work.
- `internal/app/update_test.go` — existing injected seams prove update-check failure and execution prevention; suitable home for focused `runUpgrade` parser/side-effect tests.
- `internal/app/upgrade_test.go` — existing CLI-level upgrade coverage, including positional `engram`, can be extended for public error behavior if desired.
- `internal/update/check.go` — `CheckFiltered` currently ignores unknown tool names; changing positional-filter policy here would broaden scope beyond flag parsing.
- `openspec/config.yaml` — strict TDD and Go test/vet verification rules apply; no code changes are proposed in exploration.

### Approaches
1. **Validate unknown flags only at `runUpgrade`** — reject every unsupported argument beginning with `-`, preserve recognized flags and positional filters, and return a stable error such as `unknown upgrade argument "--bad-flag"` before any side effect.
   - Pros: smallest issue-aligned change; preserves existing positional filter behavior; keeps validation close to the manual parser; easy table-driven unit coverage.
   - Cons: unknown tool names remain silently ignored unless separately addressed.
   - Effort: Low

2. **Validate flags and positional tool filters at `runUpgrade`** — additionally compare filters with the registered tool names and reject unknown names before execution.
   - Pros: prevents misleading no-op commands and gives users one consistent validation boundary.
   - Cons: expands behavior beyond the issue's explicit unknown-flag regression; couples `internal/app` to update-tool registry semantics; requires deciding duplicate/empty/filter naming behavior and may break users relying on ignored filters.
   - Effort: Medium

3. **Replace manual parsing with `flag.FlagSet`** — define supported flags and parse the remaining tool filters through the standard Go flag package.
   - Pros: standard unknown-flag handling and less custom switch logic.
   - Cons: likely changes error wording/usage formatting and positional semantics; requires careful handling of interspersed filters and existing `-n` alias; larger surface than necessary for this bounded fix.
   - Effort: Medium

### Recommendation
Choose Approach 1 for issue #535: add explicit unknown-flag rejection in `runUpgrade`, retaining positional tool filters as currently supported. Add table-driven tests for `--bad-flag` and `-x`, recognized flags, and positional `engram`; inject `updateCheckFiltered` and `upgradeExecuteWithOptions` (or equivalent seams) to assert invalid input calls neither check nor execution and produces no spinner/upgrade output. Treat unknown positional filters as a follow-up decision unless the proposal explicitly expands scope; if included, validate them in `runUpgrade` rather than changing the generic `CheckFiltered` contract.

### Risks
- Error text is user-visible and may be asserted by tests or scripts; settle on one stable format and non-zero propagation through `RunArgs`/`cmd/gentle-ai/main.go`.
- Validation must occur before `os.UserHomeDir`, `updateCheckFiltered`, spinner creation, upgrade execution, report rendering, and restart guidance to guarantee side-effect prevention.
- A naive `strings.HasPrefix(arg, "-")` rejection could mishandle future flag values or `--` conventions; document whether `--` is unsupported or terminates flag parsing.
- Positional filter validation is intentionally unresolved; silently ignored unknown tool names remain a separate UX gap.

### Ready for Proposal
Yes — the bounded recommendation, affected seams, expected CLI error behavior, and test strategy are clear. The proposal should explicitly record whether unknown positional tool filters remain out of scope or are included as a separate requirement.
