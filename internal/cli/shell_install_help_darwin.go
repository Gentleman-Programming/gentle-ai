//go:build darwin

package cli

// macOS contract: Separate and Shared modes and Shared recovery, with the
// darwin platform, containment and durability disclosures.
const shellInstallHelp = `gentle-ai shell --help      print this help without starting a supervisor
gentle-ai shell install --target /owned/private-parent/shell --mode separate
  --mode shared --prefix /owned/selected-prefix --agent /owned/selected-agent
  --inspect                 print physical-selection confirmation without effects
  --confirm SHA256          approve that exact inspected selection
No flags: dedicated installer TUI. Commands live in TARGET/bin, outside npm's bin.
Installation also writes pinned fd and rg helpers to AGENT/bin, which stock Pi
prefers over PATH: the private agent (Separate) or the selected --agent (Shared).
Shared creates AGENT/bin/fd and AGENT/bin/rg; existing tools refuse before changes.
gentle-ai shell launch ROOT [PI_ARGS...]
  Launch the selected stock Pi; normal use is through TARGET/bin/pi or gentle-shell.
gentle-ai shell recover ROOT inspect
  Replace inspect with its printed confirmation to restore shared preimages.
Requires macOS 14 or newer on Apple silicon (arm64), run as the target user
without sudo.
Containment is one process group plus per-process rlimits, not a cgroup:
a descendant that calls setsid escapes the group and is not reaped;
there is no memory cap; the process limit counts every process of your user.
Installer writes use F_FULLFSYNC and refuse filesystems without it;
the Node helper falls back to a weaker fsync there.
No --channel: channel selection is Windows-only.
`

const shellInstallTitle = "Gentle Shell macOS user installer"
