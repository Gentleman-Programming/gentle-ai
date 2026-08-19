package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// sddVerifyGateVerdict is the CI-facing shape of a `sdd-verify-gate` run. It
// carries no artifact-store internals — only what a consumer repository's
// pipeline needs to decide whether the SDD verify gate cleared.
type sddVerifyGateVerdict struct {
	ChangeName      string   `json:"changeName"`
	Passing         bool     `json:"passing"`
	Advisory        bool     `json:"advisory"`
	NextRecommended string   `json:"nextRecommended"`
	BlockedReasons  []string `json:"blockedReasons"`
}

// RunSDDVerifyGate is the CLI entry point for
// `gentle-ai sdd-verify-gate <change> [--cwd <repo>] [--json] [--advisory]`.
//
// It is designed to be invoked from a CONSUMER repository's CI pipeline (not
// gentle-ai's own), gating on the exact same strict verify-report evaluator
// sdd-status/sdd-continue already use via sddstatus.Resolve — no report
// parsing is re-implemented here. A change is passing exactly when
// Dependencies.Verify == DependencyAllDone, which sddstatus already computes
// only when the verify report is current, non-stale, and its evaluator
// reports zero blockers, zero critical findings, a non-"fail" verdict, and
// complete requirements/scenarios (verification.go's parseVerifyResult).
//
// Exit code semantics reuse main.go's existing os.Exit(1)-on-error path
// (the same mechanism sdd-verify-validate relies on): a non-nil error here
// is exit 1, nil is exit 0. --advisory always returns nil so the job can
// ship informationally without blocking a consumer's pipeline.
func RunSDDVerifyGate(args []string, stdout io.Writer) error {
	if hasSDDVerifyGateHelp(args) {
		return renderSDDVerifyGateHelp(stdout)
	}
	advisory := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--advisory" {
			advisory = true
			continue
		}
		filtered = append(filtered, arg)
	}

	parsed, err := sddstatus.ParseCommandArgs(filtered)
	if err != nil {
		return err
	}
	if strings.TrimSpace(parsed.ChangeName) == "" {
		return errors.New("sdd-verify-gate requires <change>")
	}

	status, err := sddstatus.Resolve(sddstatus.ResolveOptions{
		CWD:                        parsed.CWD,
		ChangeName:                 parsed.ChangeName,
		IncludeInstructions:        false,
		ReviewDisabledForWorkspace: sddReviewDisabledForWorkspace,
	})
	if err != nil {
		return fmt.Errorf("resolve sdd status: %w", err)
	}

	verdict := sddVerifyGateVerdict{
		ChangeName:      parsed.ChangeName,
		Passing:         status.Dependencies.Verify == sddstatus.DependencyAllDone,
		Advisory:        advisory,
		NextRecommended: status.NextRecommended,
		BlockedReasons:  status.BlockedReasons,
	}

	if parsed.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if encodeErr := encoder.Encode(verdict); encodeErr != nil {
			return encodeErr
		}
	} else {
		_, _ = fmt.Fprintln(stdout, renderSDDVerifyGateText(verdict))
	}

	if advisory || verdict.Passing {
		return nil
	}
	return fmt.Errorf("sdd-verify-gate: %s is not clean (nextRecommended=%s): %s",
		verdict.ChangeName, verdict.NextRecommended, sddVerifyGateReasonsSummary(verdict.BlockedReasons))
}

func sddVerifyGateReasonsSummary(reasons []string) string {
	if len(reasons) == 0 {
		return "verify has not reached all_done"
	}
	return strings.Join(reasons, "; ")
}

func renderSDDVerifyGateText(verdict sddVerifyGateVerdict) string {
	label := "BLOCKED"
	if verdict.Passing {
		label = "OK"
	}
	suffix := ""
	if verdict.Advisory {
		suffix = " (advisory)"
	}
	line := fmt.Sprintf("%s%s: sdd-verify-gate %s (nextRecommended=%s)", label, suffix, verdict.ChangeName, verdict.NextRecommended)
	if !verdict.Passing && len(verdict.BlockedReasons) > 0 {
		line += "\n  - " + strings.Join(verdict.BlockedReasons, "\n  - ")
	}
	return line
}

func hasSDDVerifyGateHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func renderSDDVerifyGateHelp(stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai sdd-verify-gate <change> [--cwd <repo>] [--json] [--advisory]")
	_, _ = fmt.Fprintln(stdout, "\nRuns the same strict SDD verify-report evaluator sdd-status uses and reports")
	_, _ = fmt.Fprintln(stdout, "whether <change> is clean. Intended for a CONSUMER repository's CI pipeline.")
	_, _ = fmt.Fprintln(stdout, "\nFlags:")
	_, _ = fmt.Fprintln(stdout, "  --cwd <repo>   Workspace root to resolve the change against (default: current directory)")
	_, _ = fmt.Fprintln(stdout, "  --json         Emit the verdict as JSON instead of a text summary")
	_, _ = fmt.Fprintln(stdout, "  --advisory     Always exit 0; report the verdict without blocking the pipeline")
	_, _ = fmt.Fprintln(stdout, "\nExit codes:")
	_, _ = fmt.Fprintln(stdout, "  0  clean (or any verdict when --advisory is set)")
	_, _ = fmt.Fprintln(stdout, "  1  critical findings, blockers, a non-passing verdict, or verify dependency not all_done")
	return nil
}
