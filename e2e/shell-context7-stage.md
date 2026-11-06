# Internal POSIX Separate canonical MCP stage

> Integration note: statements about later layers being absent describe the original incremental source checkpoint. Current cohesive source includes the051–065 Main worker and066 Configure/Ready consumer; base profiles and Linux amd64 SDK Main + stock may reach fresh Separate review. Explicit CodeGraph/Context7/GGA/RDD/Strict-TDD public choices remain refused. Historical Guest results do not establish current-candidate functional or installed acceptance; see `shell-main-integration.md`.

`userPrepareInstallStage` now composes the existing config writer and
`userStageContext7` to produce `agent/mcp.json` for a private, unpublished stage.
On/off use the fixed official endpoint `https://mcp.context7.com/mcp`, explicit
`enabled` and `codemode` exposure. An absent choice is an effect-free no-op.

The writer requires physical private stage/agent directories and an empty agent
home. It refuses Shared, existing files/links and unsafe identities/modes. It
reuses rooted exclusive writes, file/directory synchronization, native checksum
custody and directory identity readback. It does not inspect config/persona or
projects. The caller owns cleanup; this is not hostile same-UID atomicity.

## Evidence boundary

- Exact official on/off bytes and mode0600 are tested without network access.
- Frozen Pi1.0.0 CLI consumes the preparer's **unchanged off file** and reports
  the official URL, source, global scope, disabled state, codemode and zero tools.
- Other CLI transport fixtures use the existing exclusive-write primitive with
  a local URL. They are not fixed-endpoint Context7 producer output, and they do
  not prove actual Context7 health. No endpoint override or interception exists.

The actual POSIX flow passes `req.Mode` into stage preparation before acquisition
or Shared provisioning. Full immutable profile/mode validation precedes root
creation; explicit Shared Context7 on/off and native RDD remain refused. No
selection field is cleared: config and agent policy have distinct consumers.

Profiles without Context7, Strict TDD, GGA or CodeGraph selections retain the
complete generic validator and native config writer. Explicit selections use the
validated POSIX composition; CodeGraph remains private Linux amd64 Separate only.

Public nonnil profiles and the generic/Windows experience validator remain
refused. This is internal POSIX stage integration, not a Windows implementation
or a public install qualification. Provisioner source preserves `mcp.json` while
publishing `settings.json`; authenticated full provisioning/lifecycle execution
with this profile, whole-product distribution and installed Ready remain pending.
Trusted project config can override global entries; file-only `mcp list` health
is not an active session's permission-qualified tool execution.

This reconciliation does not execute the retained tests or CLI. The complete
165-line composition matrix is now included: all18 persona/background/MCP
combinations, immutable selection/encoding, precreation mode/RDD refusals,
Shared absence and occupied-root preimages. Its explicit legacy fixture is not
default-API proof. No current parent GREEN, installed effect or public profile
acceptance is implied.
