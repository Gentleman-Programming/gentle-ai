//go:build darwin

package cli

// macOS contract: Separate mode only until Shared and recovery are enabled on
// darwin; the backend refuses both before any change.
const shellInstallHelp = `gentle-ai shell --help      print this help without starting a supervisor
gentle-ai shell install --target /owned/private-parent/shell --mode separate
  --inspect                 print physical-selection confirmation without effects
  --confirm SHA256          approve that exact inspected selection
No flags: dedicated installer TUI. Commands live in TARGET/bin, outside npm's bin.
Installation also writes pinned fd and rg helpers to AGENT/bin of the private
agent, which stock Pi prefers over PATH.
gentle-ai shell launch ROOT [PI_ARGS...]
  Launch the selected stock Pi; normal use is through TARGET/bin/pi or gentle-shell.
Requires macOS 14 or newer on Apple silicon (arm64), run as the target user
without sudo. Separate mode only: Shared mode (--mode shared) and gentle-ai shell recover
are not yet available on macOS and refuse before any change.
Containment is one process group plus per-process rlimits, not a cgroup:
a descendant that calls setsid escapes the group and is not reaped;
there is no memory cap; the process limit counts every process of your user.
Durable writes use F_FULLFSYNC; filesystems without it refuse installation.
No --channel: channel selection is Windows-only.
`

const shellInstallTitle = "Gentle Shell macOS user installer"
