# GGA selection without implicit activation

GGA is optional. This unit transports an explicit `gga: on|off` selection
and `gga_provider` in the existing experience/confirmation contract. It does
not install GGA, authenticate a provider, configure a project, or install hooks.
Public non-nil profiles and generic/Windows/Shared GGA stages remain refused.
Private Separate preparation can write the canonical configuration; this does not
admit installation or certify provider availability.

## Selection contract

- Absence preserves historical bytes and makes no GGA selection.
- OFF carries no provider. ON requires an explicit bounded provider token;
  never derive Claude from the selected Pi runtime.
- Provider syntax follows [GGA v2.10.1 documentation](https://github.com/Gentleman-Programming/gentleman-guardian-angel/tree/v2.10.1):
  Ollama/GitHub require a model; Kiro does not accept an inline model.
- Tokens are at most 64 ASCII letters/digits or `_./:-`; the complete input
  keeps its existing 256-byte bound. This is not provider availability/model health.
- Confirmation binds both the opt-in and provider. JSON null, duplicate/escaped
  duplicate keys, shell syntax and unsupported Pi provider bindings are refused.

## Proof and next boundary

`go test -run '^TestUserGGA' ./internal/shellinstaller` covers canonical encoding,
confirmation differences, malformed input, historical bytes and unchanged
filesystems on unsupported Shared/generic stage requests.
Run Go only inside the existing offline credentialless verification Guest.
Rollback removes these optional fields, validation and tests together.

The [private stage proof](shell-gga-stage.md) checks canonical configuration and
actual fixed-source CLI readback. Next: qualify CLI acquisition/provisioning and
operational provider setup before admitting installation. `gga init`/`gga install` require
explicit per-project consent; configuration readback is not an AI review receipt.

This recovery retains source assertions without a new local suite/build/probe.
No provider service, project hooks, native runtime or installed Ready was tested.
