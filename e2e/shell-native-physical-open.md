# Descriptor-bound native payload inspection

Native file hashing now opens with nofollow/nonblocking flags and compares the
descriptor's identity and stamp to the selected physical file before reading.
A same-byte replacement is still a different selection; a symlink or FIFO cannot
turn that later open into an aliased or indefinitely waiting source read.
Directory inventory likewise requires a nofollow, nonblocking directory open.
The existing allowed modes, owner checks, byte/hash limits and17-entry bound stay.

The new owned-fixture controls replace a selected file before the real opener
with identical bytes, a symlink or FIFO, and exercise stable native-file hashing.
Directory controls preserve physical empty-directory behavior and reject aliases,
FIFOs and regular files without changing their selection metadata. Existing
native, installer and GGA integrity assertions remain separate and unchanged.

This closes later generic reopens too; a safe first GGA archive read does not
protect a subsequent unsafe open. No tests, build, Guest, CLI, SDK or installation
were executed during this recovery. These are source controls, not a recorded
GREEN, imported native approval, installed acceptance or a full-tree custody claim.
