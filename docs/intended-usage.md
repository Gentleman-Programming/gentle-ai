# Intended Usage

← [Back to README](../README.md)

Gentle AI configures the AI agent you already use. After installation, open a project and describe the outcome; you do not need to invoke a planning command or hand-edit generated configuration.

## Everyday work: ODD

Organic Driven Development (ODD) starts by deciding whether your request authorizes a change. Explanations and investigations stay read-only. The agent explores existing code, asks only about decisions it cannot safely make, and implements authorized work with applicable checks.

Small, understood changes need no persistent task artifact. For substantial work, the agent creates one `odd/tasks/<feature-name>.md` before source changes, with scope, tasks, evidence, and the next step. An Engram copy under `odd/<feature-name>/tasks` helps resume across sessions. The parent reconciles both copies before resuming; missing memory is reported rather than treated as success. Accepted scope changes update affected tasks; findings do not silently expand authorization. Each task closes with a work-unit commit on the feature branch under the ODD protocol; push, pull request, and merge remain separate decisions.

Strict TDD follows the configured mode, source, and exact runner. When enabled, observe RED before implementation, GREEN after, then REFACTOR; when disabled, still run functional checks. The presence of tests alone does not enable TDD. See [ODD details](usage.md#organic-driven-development-odd).

## Review is a separate choice

Receipt-Driven Development (RDD) checks an exact change candidate when enabled. The user controls the switch with `gentle-ai review mode status`, `gentle-ai review mode enable`, and `gentle-ai review mode disable`. Review risk, consent, and authority come from the native review contract, not task checkboxes or model guesses. Disabling RDD leaves ordinary checks and repository delivery policy intact; a review result does not authorize commit, push, or release. See [Review](review-integration.md).

## Delegation and skills

The orchestrator can delegate broad exploration, multi-file writing, and independent verification to focused workers when the selected agent supports it. Workers receive a bounded mission and relevant exact skill paths; the parent verifies outcomes rather than treating self-report as proof. Native delegation capabilities differ by agent; see [Supported Agents](agents.md).

The skill registry catalogs project and installed skills. On supported clients, startup hooks refresh it with a cache; to refresh manually from a project use `gentle-ai skill-registry refresh --force`. Pi startup behavior belongs to the separately installed Gentle Shell package, not to this binary. See [Usage](usage.md#skill-registry-refresh) and [Pi integration](pi.md).

Engram™, when installed and active, saves project decisions and discoveries across sessions. The agent uses its memory tools automatically. To inspect or sync memories manually, use `engram tui` or `engram sync`; [Engram documentation](engram.md) explains the integration.

## Maintain your installation

Run `gentle-ai doctor` for read-only diagnostics. After replacing or upgrading the binary, preview with `gentle-ai sync --dry-run` and run `gentle-ai sync` to refresh managed agent assets. Sync targets agents recorded as installed unless you explicitly select others. Uninstall removes managed configuration, not unrelated user data or external packages; inspect its scope before confirming. See [install, update, and uninstall](usage.md#cli-commands).
