# Delta for pi-model-routing

## ADDED Requirements

### Requirement: Executable Resolution Order

The system MUST resolve `gentle-pi-models` PATH-first then via package sources. It MUST probe each candidate with a bounded `capabilities` request before acceptance and MUST NOT invoke `pi install`, `npm`, or network.

#### Scenario: PATH hit accepted
- GIVEN `gentle-pi-models` on PATH with successful probe
- WHEN `ResolveModelRoutingExecutable` runs
- THEN it returns the PATH binary without checking package sources

#### Scenario: PATH miss falls through
- GIVEN PATH has no valid `gentle-pi-models`
- WHEN resolution continues
- THEN it probes package candidates in precedence order

#### Scenario: No candidate passes
- GIVEN no candidate passes probe
- WHEN resolution ends
- THEN it returns typed `missing` with no fallback edit

### Requirement: Configuration Precedence and Directory Overrides

The system MUST prefer project `settings.json` over user, `PI_CODING_AGENT_DIR` over `~/.pi/agent`, and explicit `AgentDir` over env. Path computation MUST be injectable.

#### Scenario: Project overrides user
- GIVEN project and user select different sources
- WHEN selecting source
- THEN project source wins

#### Scenario: PI_CODING_AGENT_DIR overrides default
- GIVEN `PI_CODING_AGENT_DIR=/tmp/pi-agent` set
- WHEN resolving user config/roots
- THEN it uses `/tmp/pi-agent`

#### Scenario: Explicit AgentDir wins
- GIVEN `AgentDir=/custom` with env set
- WHEN resolving
- THEN it uses `/custom`

### Requirement: Package Mapping and Manifest Bin Verification

The system MUST map `npm:`/`git:`/`local:` via `packageRootForSource` per `docs/packages.md` and MUST verify bin via `ResolvePackageBin` (64 KiB, duplicate-key reject, `absent-bin`/`unsafe-bin`/`missing-bin-target`/`non-regular`/`non-executable`, symlink containment).

#### Scenario: All package kinds map
- GIVEN `npm:`, `git:`, `local:` sources
- WHEN mapping roots
- THEN each resolves to its documented layout

#### Scenario: Unsafe manifest rejected
- GIVEN `package.json` bin has `../escape` or missing target
- WHEN verifying
- THEN it returns `unsafe-bin`/`missing-bin-target` and rejects

### Requirement: Bounded Capability Probe

The system MUST probe every candidate with a bounded read-only `capabilities` request (timeout + bounded stdout) and MUST NOT do network or writes.

#### Scenario: Probe timeout
- GIVEN candidate exceeds timeout
- WHEN probing
- THEN it kills child and returns `timeout`

#### Scenario: No network/no-write
- GIVEN any probe
- WHEN observed via injected runner and FS snapshot
- THEN no `npm`/`pi install`/network and FS unchanged

### Requirement: Versioned JSON Contract Client

The client MUST provide typed `Capabilities`/`Inspect`/`Validate`/`Apply`. Each call MUST write one JSON request with `contract:"gentle-pi.model-routing/v1"` to stdin, read one bounded JSON response, validate contract/schema, preserve unknown fields, and map exit classes to typed errors.

#### Scenario: Success round-trip
- GIVEN valid `Validate` draft
- WHEN `Validate` called on compliant binary
- THEN it writes one request, reads bounded matching response, returns result on exit 0

#### Scenario: Contract mismatch rejected
- GIVEN binary returns wrong contract, invalid JSON, or oversized output
- WHEN any method called
- THEN it returns `unsupported-contract`/`invalid-json` without retry

#### Scenario: Exit class mapping
- GIVEN binary exits with documented non-zero class
- WHEN processing response
- THEN it maps to the distinct typed error

### Requirement: Typed Errors, Cancellation, and Read-Only Invariants

The system MUST expose distinct errors (`missing`/`malformed`/`timeout`/`invalid-json`/`unsupported-contract`/`probe-failed`), MUST cancel by killing child on `context.Context` cancellation, MUST stay read-only, and MUST NOT regress `Adapter`/`opencode`/`cli`.

#### Scenario: Cancellation kills child
- GIVEN `Capabilities` with cancellable context
- WHEN context cancelled
- THEN it kills child and returns cancellation promptly

#### Scenario: No adapter regression
- GIVEN existing `ProvisionEngramMCP` flow
- WHEN resolver/client present but not used
- THEN flow is unchanged (no `mergePiSettingsFile` effect)

#### Scenario: Distinct taxonomy
- GIVEN malformed JSON, missing binary, probe failure
- WHEN triggered
- THEN it returns `malformed`, `missing`, `probe-failed` distinctly
