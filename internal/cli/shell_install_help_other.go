//go:build !windows && !darwin

package cli

// Linux contract. Other non-Windows hosts reuse it only to reach the
// backend's unsupported-platform refusal; it grants them no support.
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
Requires Linux amd64 and qualified cgroup limits or an existing delegated
systemd user manager >=254. No sudo, delegation creation or container fallback.
No --channel: channel selection is Windows-only.
`
const shellInstallTitle = "Gentle Shell Linux user installer"
