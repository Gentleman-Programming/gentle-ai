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
// exists on linux only; on every other platform the gate is strictly
// fail-closed for every context, test or otherwise.
func GateShellInstallCandidate(ctx context.Context, mode string) error {
	return ShellCandidateNotAuthorized
}
