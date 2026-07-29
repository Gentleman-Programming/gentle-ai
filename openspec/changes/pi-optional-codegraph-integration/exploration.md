## Exploration: pi-optional-codegraph-integration

### Current State
CodeGraph is an optional community tool in Gentle AI. Its installer installs `@colbymchenry/codegraph`, runs `codegraph install --yes`, injects managed lazy-init guidance into each detected supported agent's system prompt, and validates a CLI plus per-agent marker contract. Pi is listed as supported, but its integration is only `APPEND_SYSTEM.md`: it neither configures Pi's MCP/extension surface nor touches Pi child assets.

Pi installation deliberately installs `gentle-pi`, `gentle-engram`, `pi-mcp-adapter`, and `pi-subagents-j0k3r`; `gentle-pi` owns Pi persona, SDD assets, chains, and child agents. The Pi adapter reports `SupportsSubAgents() == false`, so generic SDD child injection does not apply. Current CodeGraph status therefore treats an `APPEND_SYSTEM.md` marker as a configured Pi runtime even though actual children may have neither CodeGraph tools nor lazy-init guidance.

`gentle-ai sync` re-injects selected components and refreshes global CodeGraph guidance when its marker/configuration is detected, but it does not reinstall/configure the optional Pi CodeGraph extension or reapply configuration after gentle-pi refreshes its assets. Uninstall removes managed component sections/files, but has no CodeGraph community-tool cleanup path.

### Affected Areas
- `internal/components/communitytool/tool.go` — CodeGraph install, supported-agent detection, and marker-based status validation; Pi needs an actual capability contract rather than an APPEND_SYSTEM marker.
- `internal/components/communitytool/codegraph_guidance.go` — global guidance injection and sync cleanup; Pi-specific child guidance/configuration must be managed separately and reversibly.
- `internal/agents/pi/adapter.go` — owns Pi package/settings paths and package merging; the optional integration belongs here or in a Pi-specific community-tool provisioner, not in gentle-pi.
- `internal/cli/run.go` — orders Pi package installation, community-tool installation, component injection, backup targets, and post-install verification.
- `internal/cli/sync.go` — plans CodeGraph refresh but does not reapply optional Pi integration after gentle-pi asset sync.
- `internal/components/uninstall/service.go` — must include only Gentle-AI-owned Pi CodeGraph files/settings in backup and cleanup operations.
- `internal/components/sdd/inject.go`, `internal/components/sdd/prompts.go` — show the existing pattern for injecting guidance into every generated child prompt; Pi cannot use it because its child runtime is externally owned.
- `internal/components/communitytool/tool_test.go`, `internal/cli/run_integration_test.go`, `internal/cli/sync_test.go`, `internal/components/uninstall/service_test.go` — focused contract, lifecycle, rollback, sync, and removal coverage.
- `docs/pi.md` — documents that gentle-pi owns its assets; any documentation change must preserve that CodeGraph remains optional and external to gentle-pi.

### Approaches
1. **Pi-specific optional integration managed by Gentle AI** — Add a narrowly scoped CodeGraph-for-Pi provisioner that installs the CodeGraph CLI/MCP/extension, writes only Gentle-AI-owned Pi configuration/guidance, discovers compatible child assets, injects tools only where supported, injects lazy-init guidance into every child, verifies those children directly, reapplies after Pi asset sync, and removes its owned artifacts on uninstall.
   - Pros: Meets the isolation requirement; validates real child capability; preserves gentle-pi as CodeGraph-agnostic; can be idempotent and reversible.
   - Cons: Requires an explicit ownership manifest/markers and careful handling of package-managed versus project-local child assets.
   - Effort: High.

2. **Extend the generic APPEND_SYSTEM guidance path** — Keep Pi under generic agent guidance and add more marker checks.
   - Pros: Small localized diff.
   - Cons: Cannot configure extension/MCP tools or inspect each actual child; repeats the current false-positive contract; cannot reliably survive gentle-pi asset synchronization.
   - Effort: Low, but insufficient.

### Recommendation
Choose the Pi-specific optional integration. Model it as a Gentle-AI-owned reconciliation contract: selection installs and configures CodeGraph without changing gentle-pi; every compatible child is discovered and verified for both tool availability and lazy-init guidance; unsupported children receive guidance only; sync runs the same reconciliation after gentle-pi assets are refreshed; uninstall removes only the managed extension/configuration/marker blocks. Define the exact Pi extension and MCP schema from the installed Pi runtime or CodeGraph CLI output before implementation rather than guessing it.

### Risks
- Pi child-agent discovery spans package assets and project-local overrides; modifying package-owned files would be overwritten or violate ownership, while modifying only `APPEND_SYSTEM.md` is insufficient.
- The CodeGraph CLI's Pi extension/MCP installation contract must be observed from current upstream output; hardcoding an unverified schema risks a nonfunctional integration.
- Existing install verification checks file existence and a parent marker only. New verification must check each detected compatible child and distinguish unavailable, unsupported, and misconfigured states without making CodeGraph mandatory for normal Pi installs.
- Uninstall must avoid removing user-created MCP entries, extensions, or child content; marker-bounded edits and owned-path tracking are required.
- This likely exceeds the 800-line review budget once lifecycle tests are included; task planning should forecast a chained delivery unless the implementation surface is demonstrably smaller.

### Ready for Proposal
Yes — provided the proposal explicitly states that gentle-pi receives no dependency, tool, prompt, or behavior change, and the design phase first records the verified Pi/CodeGraph extension schema plus the ownership boundary for child assets.
