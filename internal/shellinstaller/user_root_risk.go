package shellinstaller

import (
	"errors"
	"os"
	"strings"
)

// The private installer's guarantees — the capability drop, NoNewPrivileges
// and the delegated systemd unit with memory, CPU and task limits — are
// meaningful only for a non-root operator. A root installation can still be
// performed, but only when the operator explicitly accepts that those
// guarantees are disabled and that every command the agent runs executes
// with root powers. The acceptance is one exact environment binding set by
// the installer front ends after an explicit human action; it is an informed
// consent record, never a security boundary.
const (
	// RootRiskEnv carries the operator's explicit root acceptance into the
	// delegated worker. Front ends set it only after typed or flagged consent.
	RootRiskEnv = "GENTLE_SHELL_ROOT_RISK"
	// RootRiskBinding is the only accepted value of RootRiskEnv.
	RootRiskBinding = "accepted-i-understand-the-private-sandbox-is-disabled"
	// RootRiskPhrase is the exact text the installer TUI requires the operator
	// to type before a root installation may proceed.
	RootRiskPhrase = "I accept the risk"
)

// RootRiskAccepted reports whether the current process carries the exact
// root acceptance binding.
func RootRiskAccepted() bool {
	return os.Getenv(RootRiskEnv) == RootRiskBinding
}

// AcceptRootRiskEnvironment returns the binding as one environment entry for
// delegated workers.
func AcceptRootRiskEnvironment() string {
	return RootRiskEnv + "=" + RootRiskBinding
}

// RootRiskRequiredError is returned when root runs the installer without the
// explicit acceptance binding. It names the consequence and both remedies
// instead of the generic kernel refusal.
type RootRiskRequiredError struct{}

func (e *RootRiskRequiredError) Error() string {
	return strings.Join([]string{
		"gentle-shell refuses to install as root by default: the private sandbox",
		"(capability drop, NoNewPrivileges, delegated systemd unit with memory, CPU",
		"and task limits) cannot contain a root installation, and every command the",
		"agent runs would run with root powers.",
		"Recommended: create a regular user and install from it (adduser <name> && su - <name>).",
		"To accept the risk explicitly, rerun with --accept-root-risk or type the",
		"acceptance phrase on the installer warning screen.",
	}, " ")
}

// AsRootRiskRequired reports whether err is the informed root refusal.
func AsRootRiskRequired(err error) bool {
	var required *RootRiskRequiredError
	return errors.As(err, &required)
}
