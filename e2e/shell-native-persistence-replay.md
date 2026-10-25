# Authenticate held persistence responses over private TLS

This unit qualifies the closed replay fixture and ordinary system-CA trust,
not PNPM persistence, installation, publication or launcher acceptance.
The original installer, pins and plan are unchanged. `NODE_USE_SYSTEM_CA=1`
aligns the native environment with the existing private worker; it trusts
administrator-installed system CAs without disabling TLS or overriding a CA.

## Guest proof

`TestUserNativePersistenceMirror`, under `shell_native_runtime`, uses the
existing source/Node custody setup and owned-process primitives. Real private
Node HTTPS requests must read all seven retained responses with matching
size/SHA and reject a non-inventoried Node URL with404. The helper must shut
down normally through its owner pipe, join and show an empty owned group.
Source, Node, copied bootstrap tools and private project are read back before
success-gated cleanup; failed workspaces stay available.

The Guest baseline failed the three Node routes with502 while the four npm
registry routes and unknown-request refusal passed. The known-byte handler now
serves Node without the old24.18 specialization; that replay proof passed.
Unknown routes now use permanent404 rather than retryable502, so PNPM can
report unavailable held packages without spending its retries on the fixture.
A fixture `upstream:false` report is not independent zero-egress evidence:
the separate private-network/kernel boundary supplies that constraint.
This first slice supplies the shared mirror helper for the later original
Prepare/Run persistent-runtime qualification; it does not replace that proof.
