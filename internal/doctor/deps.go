package doctor

import (
	"context"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

// managedBinaries lists the binaries that gentle-ai manages.
var managedBinaries = []string{"gentle-ai", "engram", "gga"}

// findAllBinaries is the function used to locate all copies of a binary in PATH.
// In production this is set to system.FindAllBinaryCopies by the CLI layer.
// Tests override it directly (same package).
var findAllBinaries func(string) []string

// SetFindAllBinaries wires the binary lookup function from the CLI layer.
// This must be called before RunChecks to enable dependency health checks.
func SetFindAllBinaries(fn func(string) []string) {
	findAllBinaries = fn
}

// NewDepsChecks creates the dependency health checks.
// For the MVP, this only includes duplicate binary detection.
// Future: version comparison (update.CheckFiltered), engram HTTP health, GGA assets.
func NewDepsChecks() []Check {
	checks := make([]Check, 0, len(managedBinaries))
	for _, bin := range managedBinaries {
		bin := bin // capture loop variable
		checks = append(checks, Check{
			ID:          "binary-" + bin,
			Category:    "deps",
			Description: bin + " binary on PATH",
			Run:         duplicateBinaryCheck(bin),
		})
	}
	return checks
}

// duplicateBinaryCheck returns a check function that detects whether a managed
// binary exists in PATH and whether multiple copies shadow each other.
func duplicateBinaryCheck(name string) func(context.Context) CheckResult {
	return func(ctx context.Context) CheckResult {
		if ctx.Err() != nil {
			return CheckResult{
				ID:       "binary-" + name,
				Category: "deps",
				Status:   verify.CheckStatusSkipped,
				Message:  "skipped: context cancelled",
			}
		}

		if findAllBinaries == nil {
			return CheckResult{
				ID:       "binary-" + name,
				Category: "deps",
				Status:   verify.CheckStatusSkipped,
				Message:  "skipped: binary lookup not configured",
			}
		}

		copies := findAllBinaries(name)

		switch len(copies) {
		case 0:
			return CheckResult{
				ID:       "binary-" + name,
				Category: "deps",
				Status:   verify.CheckStatusWarning,
				Message:  fmt.Sprintf("%s not found in PATH", name),
				Details:  []string{"Install with: gentle-ai install (or check your PATH)"},
			}

		case 1:
			return CheckResult{
				ID:       "binary-" + name,
				Category: "deps",
				Status:   verify.CheckStatusPassed,
				Message:  fmt.Sprintf("%s — single copy at %s", name, copies[0]),
			}

		default:
			details := make([]string, 0, len(copies)+2)
			details = append(details, fmt.Sprintf("found %d copies:", len(copies)))
			for i, p := range copies {
				if i == 0 {
					details = append(details, fmt.Sprintf("  %s (active — resolves first)", p))
				} else {
					details = append(details, fmt.Sprintf("  %s (shadowed)", p))
				}
			}
			details = append(details, "Fix: remove duplicates, keep only the one managed by your package manager")

			return CheckResult{
				ID:       "binary-" + name,
				Category: "deps",
				Status:   verify.CheckStatusWarning,
				Message:  fmt.Sprintf("%s has %d copies in PATH — possible shadowing", name, len(copies)),
				Details:  details,
			}
		}
	}
}

// AllChecks returns every check the doctor knows about.
// The CLI layer calls this to build the full check set.
func AllChecks() []Check {
	var all []Check
	all = append(all, NewDepsChecks()...)
	// Future phases:
	// all = append(all, NewPlatformChecks()...)
	// all = append(all, NewAgentChecks()...)
	// all = append(all, NewEnvChecks()...)
	return all
}
