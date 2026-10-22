# Main loads in the frozen SDK: bounded qualification

> Integration note: statements about later layers being absent describe the original incremental source checkpoint. Current cohesive source includes the051–065 Main worker and066 Configure/Ready consumer; base profiles and Linux amd64 SDK Main + stock may reach fresh Separate review. Explicit CodeGraph/Context7/GGA/RDD/Strict-TDD public choices remain refused. Historical Guest results do not establish current-candidate functional or installed acceptance; see `shell-main-integration.md`.

The retained official Main package loads its 20 extensions, skills, prompts and
themes through the actual Pi 1.0.0 SDK, then binds and shuts down a headless
session. This test-only unit does not change production admission or activation.

## Reproduce in the existing offline Guest

Use the credentialless, bounded Guest with the retained snapshot mounted at
`/frozen-main`, original SDK archive at `/frozen-pi.tgz`, read-only frozen
dependency layouts at `/node_modules` and the owned checkout at `/source`:

```sh
go test -tags=shell_main_sdk_consumer -run '^TestUserMainFrozenSDKConsumer$' -count=1 -v ./internal/shellinstaller
```

The test checks Main commit `43541a0d30dfafacfcb2e99ef4135ab8b1693e99` and
archive SHA `cbc39d46b0d5416435c22498515e5bdfef8b4368e6a8e9d591f265f2de99362e`.
Production custody/source consumers materialize and compare the complete source.
The SDK archive is authenticated against the committed modern npm SRI; test-only
ancestor links expose the original read-only dependency layouts, not an overlay
inside either source tree. SDK/model stores and package-owned agent assets use
fresh private directories; no invented zero-write storage contract is imposed.

## What the probe checks

- Actual public `DefaultResourceLoader`, `ModelRuntime`, `createAgentSession`,
  `SessionManager.inMemory`, binding and shutdown with SDK-required `reason:
  "quit"` before disposal; no factories replace Main code. Every loaded extension path belongs to retained Main.
- No loading warnings/errors or reported startup/shutdown handler errors;
  review, capture, CodeGraph and TODO definitions exist without invoking them.
- Main archive/source, SDK source, private Git and existing project entries stay
  exact. Separate personal Pi/project preservation fixtures also stay exact.
- Main creates only its existing private project skill-registry cache:
  `.atl/.gitignore`, `.atl/.skill-registry.cache.json` and `.atl/skill-registry.md`.
  This effect is disclosed and checked, not called project-wide immutability.

## Still closed

No model request, tool/CodeGraph execution, native bundle, installer invocation,
UI acceptance, remote health or installed launch is qualified. No pin changes or
Main supplier exception: the exact-byte gate still rejects its installer.
Behavioral RED is N/A for this tests/docs-only unit. Initial directory setup and
an over-strict zero-new-files assertion were fixture/observer failures, not an
SDK behavior fix. Rollback removes only this probe, tagged test and document.

Integration scope: source Guest observations above remain historical. This candidate retains the complete tests/negative controls but no local Go/Node/SDK/CLI/PTY/guest qualification, build or installation was executed. Foundation5585 CI does not verify this expanded candidate; fresh functional and independent verification remain pending.
