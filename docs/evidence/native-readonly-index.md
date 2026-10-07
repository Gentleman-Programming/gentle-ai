# Repeated Linux inspection keeps Git-directory metadata

Refs #5326. Snapshot indexes prefer an absolute, qualified process-temp base and an owned private directory/file. Every Unix ancestor must be root/caller-owned, nonsymlink, and nonwritable by other users or sticky. Unsafe/unusable storage retains the established Git-local fallback; Windows retains that fallback pending equivalent ancestor-ACL proof.

## Candidate-local evidence

Base: `631cf9295f1393f857ce2f0f7088a30dd08b70f0`. Run compilation/tests only inside a physically bounded, credentialless Guest; no operator-host SDK execution.

- `go test ./internal/reviewtransaction -run '^TestSnapshot'`: 42 top-level tests pass. New controls cover workspace/staged/HEAD identity, evidence, real-index bytes, mtime/ctime, relative process-temp paths, unsafe ancestors, ownership/modes, cleanup, and fallback/refusal.
- `go test ./internal/cli -run '^TestNegotiatedStatusUnderUnavailableProcessTemp'`: three tests pass, preserving #2044/#2132 compatibility.
- Actual compiled CLI: seed then repeat `review status --cwd=<repo> --contract=gentle-ai.review-integration/v2 --next-transition=true` on an empty repository. Original main changes `.git` mtime/ctime; the correction preserves them. Response remains 2914 bytes, SHA-256 `8b32e4b35ea3ac66158eff09b47db70db0e7e6f8f4ef975bebccfcff9c07465e`. Independent verification reproduced the correction.
- Darwin/Windows test-binary compilation passes; this is not runtime proof. An earlier broad candidate-package run encounters nine pre-existing failing test names, also reproduced on pristine main under the same restrictive Guest umask; no final full-package-green claim.

The core bench's capability gate represents CLI verbs/flags, not a Linux physical-metadata prerequisite. This flow therefore uses actual Linux runtime E2E rather than fabricated capability/unsupported outcomes or new runner policy. No model requests, proxy binary, or SDK substitution are involved.

Rollback: remove the new snapshot temp helpers/tests and this note, and restore only the two original index-allocation blocks in `snapshot.go`. No receipt schema, lifecycle, pin, or release changes belong to this unit. Dirty-workspace object publication is not claimed side-effect-free. Local proof does not qualify #5242's pinned native v4.0.0; native closure and authentic distribution/pin decisions remain separate gates.
