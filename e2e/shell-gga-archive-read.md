# Bounded retained GGA archive read

Retained readback now opens a nofollow, nonblocking descriptor, compares its
identity/stamp to the validated pathname, then reads at most 81842 bytes. Exact
81841-byte size, private mode/owner/single link and fixed SHA remain mandatory.
Cancellation and Close errors are joined; no data escapes a failed read.

The controlled owned-fixture opener replaces the path after validation with a
same-byte different inode, oversized file, symlink or FIFO. Each is refused
without blocking or additional tree mutation; cancellation retains its cause.
The authentic unchanged archive succeeds. Existing payload/manifest controls
still run in the offline Guest, not a fabricated installed environment.

This separately addresses approved `R3-archive-read-race`; its original receipt
stands. No PATH/binding/provider command or public profile activation. Rollback
removes only this bounded reader and restores the prior retained read path.

The rollback sentence above describes the original source034 correction. In
this recovered chain the bounded production reader already belongs to the
preceding integrity unit. Rolling back this characterization removes these
replacement fixtures and this note, never restores an unlimited pathname read.
No new Guest execution, native receipt, installed acceptance or Ready is claimed.
