//go:build !linux

package cli

import (
	"context"
	"errors"
)

// ShellCandidateNotAuthorized is returned when the gate refuses to
// authorize a shell installation candidate. This is the fail-closed
// default for all production/host contexts. Kept message-identical to
// the linux variant in shell_install_gate.go.
var ShellCandidateNotAuthorized = errors.New("shell candidate not authorized: explicit per-run consent and guest provisioning required")

// GateShellInstallCandidate is the non-linux variant of the gate. The
// protected provisioning/consent/verification seam (ProvisionedContext)
// exists on linux only; on every other platform the C6-protected sensitive
// modes ("gentle-shell", "pi") are strictly fail-closed for every context,
// while ordinary installation modes preserve the pre-gate behavior and are
// not gated.
func GateShellInstallCandidate(ctx context.Context, mode string) error {
	switch mode {
	case "gentle-shell", "pi":
		return ShellCandidateNotAuthorized
	default:
		return nil
	}
}
