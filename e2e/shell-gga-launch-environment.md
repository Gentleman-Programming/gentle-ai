# Optional GGA in the selected launch environment

The ordinary POSIX launch builder adds only `TARGET/runtime/gga/bin` to PATH for
an ON marker in a validated Separate manifest. OFF/absence and Shared do not
expose that private payload. Provisioning environments remain unchanged. Roots
containing a PATH separator refuse launch rather than selecting another directory.

## Functional proof

In the existing offline Guest, an authentic native-prepared GGA source recipe
runs under the existing `userLaunchGroup`, using the ordinary launch builder's
real environment. Actual `--version` and `config` consume the private executable
and XDG file; the original nonzero CLI status remains nonzero. The group is reaped
before retained-payload readback. Controlled payload mutation yields `uncertain`
with the private root retained; no repair, discard or successful result is invented.

The Node command is planned, not substituted or executed by this fixture. This
proves private environment lookup plus CLI/group/readback, not a complete installed
launcher, kernel/helper/Bash custody, SDK tool permission, provider authentication,
model health, AI-review receipt or three-OS acceptance. No new SDK GGA tool,
public binding, project hooks or personal activation. A system/project GGA outside
this managed payload is not a managed OFF guarantee. Public profiles stay refused.

Rollback removes the launch-only PATH selection and GGA post-exit check, preserving
the authenticated acquisition, configuration and retained-integrity units.

Recovery scope: complete source035/036 controls are retained without a new local
Go/CLI/group qualification. Historical source evidence remains historical; this
composition is not current-head CI or installed acceptance.
