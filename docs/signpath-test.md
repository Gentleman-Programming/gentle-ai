# Run the SignPath TEST signing pilot

Maintainers can use this manual CI pilot to check SignPath setup. Its self-signed artifacts are **TEST only**, not releases or publicly trusted Windows binaries. The existing Windows distribution hold remains in force.

The SignPath scripts and configuration live in the nested module under `.github/signpath/`. Go excludes this directory from the published Gentle AI module ZIP.

## Verify before merge

The pull request's secret-free Windows fixture job builds both binaries and exercises the verifier without submitting a signing request or publishing artifacts. Require its successful result before considering a separately authorized merge. Production signing remains disabled.

## Provision once

1. Create your SignPath account and accept the organization invitation **before** confirming the CI user's email address.
2. In the Gentle AI SignPath organization, keep the existing single-PE configuration. Add `github-test-zip` using the canonical [ZIP configuration](../.github/signpath/github-test-zip.xml), for project `gentle-ai` and policy `test-signing`.
3. Configure the GitHub `signpath-test` environment. A human provisions `SIGNPATH_API_TOKEN` as an environment secret without sharing it in logs or messages. Set the environment variable `SIGNPATH_TEST_CERTIFICATE_SHA256` to the independently obtained SHA256 of the test signer's DER certificate: exactly 64 hexadecimal digits.
4. Confirm the GitHub trusted-build binding and any environment approvals. After the premerge fixtures pass, obtain separate authorization to merge the reviewed pilot to main. Only then may an authorized manual dispatch of **SignPath TEST signing pilot** on reviewed main submit a test request. No other ref or input is supported.

## Verify and send evidence

The job builds two source binaries, exercises synthetic verifier fixtures, sends the GitHub artifact ID to SignPath, and verifies the returned files before uploading the `signpath-test-verified-NOT-FOR-RELEASE` artifact. A failed step must not yield a verified artifact. Preserve the run URL, reviewed commit, signing request identifier/status and verification result; send those and the configuration reference to the Foundation for setup review. Do not send secrets.

The verifier never opens or modifies any certificate store (store writes can block on a Windows trust prompt in a non-interactive session). It requires the signer to match the pinned SHA256, rejects `HashMismatch`, builds the signer chain in memory with only that pinned public certificate as custom root (revocation disabled for the disposable self-signed chain), and cryptographically checks the embedded PKCS#7 signature. This limited, in-process test trust does **not** establish public Windows trust. The fixture harness bounds each verifier child (120 s), kills its process tree on timeout and prints `SIGNPATH-STAGE` markers (enabled by `SIGNPATH_VERIFY_TRACE=1`) to locate any hang.

## Next step

Production certificate issuance remains CSR pending until Foundation review. Public code-signing policy/privacy requirements and enforced PE product/version metadata remain later prerequisites; this pilot does not claim production compliance or restore publication. See [release signing](release-signing.md) for the existing release boundary.
