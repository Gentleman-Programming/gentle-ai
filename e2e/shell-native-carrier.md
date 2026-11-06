# Selected local original installer carrier

`StageNativeInstallerCarrier(ctx, root)` stages the exact selected Gentle Shell
source and returns a snapshot-bound `NativeInstallerRuntime`. Prepare an owned
private worker and its `runtime` directory first. Materialization invokes no
supplier, bootstrap, network request, or live Main lookup and does not select
`SDKSource=main-preview`.

## Identity and custody

- Source commit: `8983b43bd52210a9242a056d291fb2bf30e73cc5`.
- ZIP SHA256: `f4b964eda29f2ff9b4fb6b8329a2f1eb60e7b5313984518cf6109159b18a71e9`.
- Embedded ZIP: `scripts/native-installer-source-8983.zip`, 11,837,894 bytes,
  generated with the canonical `gentle-shell-<commit>/` prefix.
- This is an explicitly selected local carrier, **not published-release provenance**.
  The existing descriptor's canonical URL is a format binding, not a fetch or
  evidence that this local commit/archive has been published at that URL.

Existing archive validation, retained custody, extraction and complete source
comparison are reused. Existing source/archive destinations and unsafe physical
worker paths are refused. Failures retain evidence for the caller; no existing
source is overwritten or removed.

Materialized files are still owned by the worker. Before any source execution,
the caller must enforce immutable custody for `runtime/main` and
`runtime/main-source`, authenticate temporary Node and establish the qualified
worker boundary. This helper supplies neither execution consent nor a sandbox.

## Verification

Run `go test -mod=readonly -p=1 -count=1 -timeout=110s -run '^TestUserNativeCarrier' ./internal/shellinstaller`
inside the qualified private offline worker. The tests verify the exact carrier,
complete extracted tree, canceled/unsafe/reused stages, tamper refusal and absent
supplier tooling. They do not prove ordinary TUI installation or native acceptance.
