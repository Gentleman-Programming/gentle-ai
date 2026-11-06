# Native coordinator and sealed-child checks

`userNativeCoordinatorCheck()` requires the live full UID/GID map shape
`0 0 4294967295`; this does not identify an initial user namespace.
`userNativeExecutionCheck(root, managerRuntime)` instead requires singleton own
UID/GID mappings of length one and `setgroups=deny` from fixed procfs files.
Both reuse the existing kernel limits and require matching four-field IDs, no
supplementary groups, five zero capability sets including bounding, and NNP.

The child requires physical owned source custody (`runtime/main` and
`runtime/main-source`) and canonical `/run/user/UID`, each read-only with no
writable descendant mount. The manager directory must be empty, with neither
`bus` nor `systemd/private` present. Mount evidence comes from live procfs;
caller-supplied namespace numbers and internal selectors confer no authority.

These guards do not compare the complete fixed source snapshot, authenticate
Node, establish physical confirmation, launch a service, or wire the TUI.
The caller must perform those checks and preserve custody throughout execution.
Parser tests and live refusal tests are not positive sealed-worker qualification.

Mount visibility follows parent IDs and same-point overmounts, not line order.
Hidden lower branches do not expose writable mounts; visible writable descendants
remain refused, as do duplicate IDs, ambiguous covers and parent cycles.
