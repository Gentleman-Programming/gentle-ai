# Release signing and key rotation

Gentle AI releases only when the protected `release` environment provides a real Minisign credential whose public key matches the trust anchors embedded in the binary. An unset, malformed, placeholder, or isolated test key stops both the updater and release workflow.

## User verification

1. Obtain the production Minisign public-key payload and its SHA256 fingerprint from a maintainer-controlled channel independent of the GitHub release assets.
2. Download `checksums.txt`, `checksums.txt.minisig`, and the archive from one exact `vMAJOR.MINOR.PATCH` release.
3. Verify the signed identity before trusting any checksum:

   ```bash
   minisign -VQm checksums.txt \
     -x checksums.txt.minisig \
     -P "$GENTLE_AI_MINISIGN_PUBLIC_KEY"
   # Must print exactly:
   # repo=Gentleman-Programming/gentle-ai;tag=vMAJOR.MINOR.PATCH

   sha256sum --check --strict --ignore-missing checksums.txt
   ```

The public key is not secret, but its provenance is security-critical. A key fetched only from the release page cannot authenticate that same page.

## Provision the first signed release

> Maintainer-only boundary: this repository intentionally contains no production private key and no production public-key value.

1. Generate the production pair on a controlled maintainer system. The CI key must be unencrypted because the runner is non-interactive; GitHub's protected environment secret provides encryption at rest and access control.

   ```bash
   minisign -G -W -p gentle-ai-release.pub -s gentle-ai-release.key
   ```

2. Extract the base64 payload from line 2 of `gentle-ai-release.pub`. Publish that payload and a separately computed fingerprint through the project website or another maintainer-authenticated channel **before** publishing the first signed release.
3. Create or protect the GitHub Actions environment named `release`. Require appropriate reviewers and restrict it to protected stable-version tags.
4. Configure the public trust anchor as a repository Actions variable so the read-only preflight job can validate it. Configure the private key only inside the protected `release` environment:

   | Name | Kind | Exact value |
   |---|---|---|
   | `MINISIGN_PUBLIC_KEYS` | Repository Actions variable | One canonical Minisign base64 public-key payload; during rotation, exactly two distinct payloads separated by one comma, with no whitespace or trailing separator |
   | `MINISIGN_SECRET_KEY_BASE64` | Protected `release` environment secret | Base64 of the complete `gentle-ai-release.key` file |

5. Keep the existing `HOMEBREW_TAP_TOKEN` environment secret. Do not add the Minisign private key to repository variables, files, logs, artifacts, caches, or command-line arguments.
6. Run no release until `scripts/release-signing-preflight.sh` proves that the private key derives one configured public key, rejects the isolated test key, and signs/verifies an exact repository/tag canary.

The workflow validates the complete repository-variable value, exports a separate canonical value, and permits GoReleaser to inject only that validated output through this exact linker variable:

```text
github.com/gentleman-programming/gentle-ai/v2/internal/update/upgrade.releaseMinisignPublicKeys
```

Source/test builds retain `UNSET`; their binary self-updater refuses network replacement. There is no grace version and no unsigned fallback.

The updater caps a release archive at **128 MiB**. It rejects both oversized `Content-Length` declarations and chunked or otherwise unknown-length responses that cross the same ceiling, deleting partial downloads without changing the installed binary.

### Bootstrap existing installations

Binaries released before authenticated manifests cannot retroactively authenticate their first upgrade. For the first signed version, instruct existing users to download manually, verify the out-of-band public key fingerprint, verify the signed manifest and checksum, and then install. All later built-in upgrades are authenticated by the embedded trust anchor.

## Rotate without a trust gap

The runtime accepts at most two distinct keys to provide a bounded overlap:

1. Generate and publish the new public key and fingerprint out of band.
2. Publish a bridge release signed by the **old** private key with `MINISIGN_PUBLIC_KEYS=old,new`. Clients authenticate the bridge with the old key and receive both trust anchors.
3. Change the protected signing secret to the new private key. Publish subsequent releases with the same `old,new` overlap; signing preflight requires the new derived public key to be in that set.
4. After the supported upgrade window has passed, publish a release embedding only `new` and remove access to the old private key.
5. Revoke and securely destroy the old private key according to the maintainer incident process. Never reuse the isolated key under `internal/update/upgrade/testdata/`.

If a key may be compromised, stop releases. Do not silently replace a trust anchor or bypass the matching preflight; use an independently authenticated incident channel and require manual bootstrap when the old key can no longer sign a safe bridge.

## Release gates

### Windows distribution restoration gate

Windows source compatibility, CI, and runtime tests remain supported. Official
Windows executable/archive assets and Scoop publication are temporarily omitted.
Because there is no signed Windows asset to download, Windows never downloads an
unsigned executable and never executes a remote update script. Instead:

- With Go 1.25.10+ on `PATH`, the built-in upgrader runs
  `go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@vX.Y.Z`,
  pinned to the exact release tag. This is verified — just against a different
  trust anchor: the module is checked against the Go checksum database
  (`sum.golang.org`) rather than our minisign release signature. The upgrader
  does not disable the checksum database on this path.
- Without Go on `PATH`, the upgrader fails closed to `go install` guidance and
  performs no download or execution at all.

Linux and macOS are unaffected by this: they continue to download the signed
release archive and verify it with minisign, and they never route through
`go install`.

Restore Windows distribution only after all of these conditions are enforced:

1. Provision publicly trusted RSA Authenticode signing, preferably managed OIDC
   with Azure Artifact Signing; self-signed or mock credentials are not acceptable.
2. Sign both amd64 and arm64 executables before archive and checksum generation.
3. Add pre-publication and remote-release verification that fails if either
   executable is unsigned or its signature is not publicly trusted.
4. Restore the Windows GoReleaser targets and Scoop publisher together, with
   regression coverage proving no unsigned artifact can be emitted.

Until then, `scripts/verify-release-distribution-policy.sh` blocks any Windows or
Scoop entry in `.goreleaser.yaml`, and the exact remote asset check accepts only
the four macOS/Linux archives plus the signed checksum manifest.

The tag workflow fails unless all of these hold:

- the event is an annotated exact `vMAJOR.MINOR.PATCH` tag;
- the event SHA, tag target, checkout, remote tag, and current `origin/main` are identical;
- the worktree and module graph remain immutable (`go mod tidy -diff`);
- tests, vet, and format checks pass under read-only token permissions;
- the protected signing key matches a non-test injected trust anchor;
- the Windows/Scoop omission policy passes before any publication;
- GoReleaser signs the full `${artifact}` path with the exact trusted comment;
- the published GitHub asset set is exact, the remote signature is valid, and every remote checksum verifies.
