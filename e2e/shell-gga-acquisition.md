# Offline GGA acquisition controls

`TestUserGGAAcquisitionOfflineResponses` covers the same client acquisition
function called by private ON provisioning, using the authentic fixed archive.
Run it in the existing offline Guest with tag `shell_gga_canonical`.

- Success/omitted Content-Length produce all four private members and retain the
  authenticated 0600 archive; the actual CLI recipe test remains separate.
- Status, redirect, origin, TLS metadata, encoding and decompression rejection
  close the body exactly once without changing the native-prepared stage.
- Truncation, oversized/digest-altered data, read/close errors and cancellation
  preserve the stage. Oversized reads stop at exactly 81842 bytes.
- The copied client discards caller cookies and redirect policy. Every request
  remains credentialless GET to the fixed source commit, never another endpoint.
- Cancellation during a rejected response's Close preserves `context.Canceled`
  alongside the rejection/close errors; it is not converted to successful install.

Transport and TLS fields are explicit offline DATA fixtures, not a real TLS
handshake, live Go HTTPS proof, publisher signature or installed OS acceptance.
No provider/model command, credentials, hook, PATH or public profile admission.
Rollback removes the acquisition seam/controls without reverting the existing
private config or source recipe. Managed launch/integrity remain the next gate.
