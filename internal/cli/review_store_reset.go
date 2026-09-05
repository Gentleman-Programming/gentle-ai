package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// ReviewStoreResetSchema identifies the user-facing store reset projection.
const ReviewStoreResetSchema = "gentle-ai.review-store-reset-result/v1"

// ErrReviewStoreResetRequiresGitRepository reports that review store-reset
// was attempted outside a Git repository.
// refusal:by-design operator-knowledge: review store-reset requires a Git repository; only the operator knows where the Git repository checkout is located
var ErrReviewStoreResetRequiresGitRepository = errors.New("review store-reset requires a Git repository")

// ReviewStoreResetRepositoryRequiredError represents a typed failure when
// store-reset is executed outside a Git repository, providing actionable guidance.
type ReviewStoreResetRepositoryRequiredError struct {
	Cause error
}

func (err *ReviewStoreResetRepositoryRequiredError) Unwrap() error { return err.Cause }

func (err *ReviewStoreResetRepositoryRequiredError) Error() string {
	// refusal:by-design operator-knowledge: running store-reset requires an existing Git repository; only the operator knows where the repository checkout is located
	return "review store-reset requires a Git repository; rerun the command from inside a Git repository, or rerun with --cwd pointing at one"
}

func (err *ReviewStoreResetRepositoryRequiredError) Is(target error) bool {
	return target == ErrReviewStoreResetRequiresGitRepository
}

func reviewStoreResetRepositoryRequiredRefusal(err error) error {
	if err == nil {
		return nil
	}
	var typed *reviewtransaction.StoreResetRepositoryRequiredError
	if errors.As(err, &typed) {
		return &ReviewStoreResetRepositoryRequiredError{Cause: err}
	}
	if reviewtransaction.ReviewRootResolutionReportsNoRepository(err) {
		return &ReviewStoreResetRepositoryRequiredError{Cause: err}
	}
	return nil
}

// ReviewStoreResetResult is the command's machine-readable output.
type ReviewStoreResetResult struct {
	Schema    string                             `json:"schema"`
	Operation string                             `json:"operation"`
	Report    reviewtransaction.StoreResetReport `json:"report"`
}

// RunReviewStoreReset removes this clone's accumulated review lineage state.
//
// It is user-initiated only, in the same sense `review mode disable` is: it
// carries no negotiated contract row, so no adapter can reach it on the machine
// route, and nothing in the product invokes it. A human types it or picks it in
// the TUI, or it does not happen.
//
// Preview is the default and removal needs --confirm. That asymmetry is
// deliberate. The operation is irreversible and clone-wide, so the bare
// invocation -- the one that gets typed from memory, pasted out of a chat log,
// or recalled from shell history -- has to be the one that only looks. Costing
// the user a second invocation is a much smaller price than the first mistake.
func RunReviewStoreReset(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review store-reset", stdout,
		"Remove this clone's accumulated review lineage state so reviews start from nothing. Previews by default and reports what it would remove per category; --confirm applies it. Removal is irreversible: nothing is copied aside. The receipt-driven-development kill switch, SDD runtime state, defect reports, the adapter-written reviews/ graph store, and any path this command does not recognize are preserved and reported. Reviews that have not reached a terminal state are refused unless --include-in-flight is given.")
	cwd := flags.String("cwd", ".", "repository path")
	confirm := flags.Bool("confirm", false, "apply the removal instead of previewing it")
	includeInFlight := flags.Bool("include-in-flight", false, "also remove reviews that have not reached a terminal state")
	includeAdapterReviews := flags.Bool("include-adapter-reviews", false, "also remove the adapter-written reviews/ graph store, which this command's maintenance lease and in-flight refusal do not cover")
	emitJSON := flags.Bool("json", false, "emit the machine-readable review store reset report")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		// refusal:by-design operator-knowledge: a stray positional argument is removed by whoever typed it; no command resolves a typo
		return fmt.Errorf("unexpected review store-reset argument %q", flags.Arg(0))
	}

	ctx := context.Background()
	// The preview is the preview of the run these same flags would make, so
	// the request is built once and both paths get it.
	request := reviewtransaction.StoreResetRequest{
		IncludeInFlight: *includeInFlight, IncludeAdapterReviews: *includeAdapterReviews,
	}
	var report reviewtransaction.StoreResetReport
	var err error
	if *confirm {
		report, err = reviewtransaction.ResetReviewStore(ctx, *cwd, request)
	} else {
		report, err = reviewtransaction.SurveyReviewStore(ctx, *cwd, request)
	}
	// A refusal is not an attempt. It returns before touching anything, so the
	// report has to be rendered in the same voice as a preview -- reporting it
	// as a removal that skipped everything would tell the user their store was
	// half-processed when nothing was opened at all.
	var refused *reviewtransaction.StoreResetInFlightError
	attempted := *confirm && !errors.As(err, &refused)

	// The report is emitted even on failure, and before the error is returned.
	// A caller that cannot see which categories went away cannot reconcile a
	// partially reset store, and a refusal is exactly when the list of open
	// reviews matters most.
	if report.Schema != "" {
		if emitErr := emitReviewStoreReset(stdout, report, attempted, *confirm, *emitJSON); emitErr != nil && err == nil {
			return emitErr
		}
	}
	if refusal := reviewStoreResetRepositoryRequiredRefusal(err); refusal != nil {
		return refusal
	}
	return err
}

func emitReviewStoreReset(stdout io.Writer, report reviewtransaction.StoreResetReport, attempted, confirmed, emitJSON bool) error {
	if emitJSON {
		payload, err := json.MarshalIndent(ReviewStoreResetResult{
			Schema: ReviewStoreResetSchema, Operation: "review/store-reset", Report: report,
		}, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "%s\n", payload)
		return err
	}
	return renderReviewStoreReset(stdout, report, attempted, confirmed)
}

func renderReviewStoreReset(stdout io.Writer, report reviewtransaction.StoreResetReport, attempted, confirmed bool) error {
	var out strings.Builder
	fmt.Fprintf(&out, "review store: %s\n", report.StoreRoot)

	heading := "would remove"
	if attempted {
		heading = "removed"
	}
	present := reviewStoreResetPresent(report.Removable)
	if len(present) == 0 {
		fmt.Fprintf(&out, "\nnothing to remove: this clone holds no review lineage state.\n")
	} else {
		fmt.Fprintf(&out, "\n%s:\n", heading)
		for _, entry := range present {
			status := ""
			switch {
			case entry.Skipped != "":
				status = "  SKIPPED: " + entry.Skipped
			case attempted && !entry.Removed:
				status = "  SKIPPED"
			}
			fmt.Fprintf(&out, "  %-34s %6d file(s)  %10s%s\n",
				entry.Name, entry.Files, reviewStoreResetBytes(entry.Bytes), status)
		}
	}

	if preserved := reviewStoreResetPresent(report.Preserved); len(preserved) > 0 {
		fmt.Fprintf(&out, "\npreserved:\n")
		for _, entry := range preserved {
			fmt.Fprintf(&out, "  %-34s %10s  %s\n", entry.Name, reviewStoreResetBytes(entry.Bytes), entry.Reason)
		}
	}
	if len(report.Unrecognized) > 0 {
		fmt.Fprintf(&out, "\nleft in place, not recognized by this command:\n")
		for _, entry := range report.Unrecognized {
			fmt.Fprintf(&out, "  %-34s %10s\n", entry.Name, reviewStoreResetBytes(entry.Bytes))
		}
	}

	fmt.Fprintf(&out, "\n%d settled review(s)", report.Settled)
	if len(report.InFlight) > 0 {
		verb := "still open"
		if attempted {
			verb = "removed while still open"
		}
		fmt.Fprintf(&out, ", %d %s: %s", len(report.InFlight), verb, reviewStoreResetLineages(report.InFlight))
	}
	out.WriteString("\n")

	switch {
	case attempted:
		fmt.Fprintf(&out, "freed %s across %d file(s).\n", reviewStoreResetBytes(report.RemovedBytes), report.RemovedFiles)
		if report.Residue != "" {
			fmt.Fprintf(&out, "staging directory %s still exists; the bytes it holds are not reclaimed.\n", report.Residue)
		}
		switch {
		case len(report.UnrestoredAdminDirs) > 0:
			// The usual line is withdrawn here, and it has to be. It promises
			// that SKIPPED means untouched, and for these views that promise
			// is already broken: the checkouts are where they were, but the
			// worktree registrations that make them usable are not.
			fmt.Fprintf(&out, "this reset was incomplete, and it did not leave everything it skipped untouched.\n")
			fmt.Fprintf(&out, "these worktree administrative directories were moved aside for a SKIPPED category and could not be put back:\n")
			for _, dir := range report.UnrestoredAdminDirs {
				fmt.Fprintf(&out, "  %s\n    now at %s\n", dir.Original, dir.Staged)
			}
			fmt.Fprintf(&out, "move each one back to its original path before using those candidate views again.\n")
		case !report.Complete:
			fmt.Fprintf(&out, "this reset was incomplete; nothing above marked SKIPPED was removed.\n")
		}
	case confirmed:
		// A refused --confirm. The error that follows names the exact
		// override, so repeating an instruction here would only compete
		// with it.
		fmt.Fprintf(&out, "nothing was removed.\n")
	case len(present) > 0:
		fmt.Fprintf(&out, "nothing was removed. Re-run with --confirm to apply this. Removal is irreversible: nothing is copied aside.\n")
	}

	_, err := io.WriteString(stdout, out.String())
	return err
}

func reviewStoreResetPresent(entries []reviewtransaction.StoreResetEntry) []reviewtransaction.StoreResetEntry {
	present := make([]reviewtransaction.StoreResetEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Present {
			present = append(present, entry)
		}
	}
	return present
}

func reviewStoreResetLineages(lineages []reviewtransaction.StoreResetLineage) string {
	names := make([]string, 0, len(lineages))
	for _, lineage := range lineages {
		if lineage.Unreadable != "" {
			names = append(names, lineage.LineageID+" (unreadable)")
			continue
		}
		names = append(names, fmt.Sprintf("%s (%s)", lineage.LineageID, lineage.State))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// reviewStoreResetBytes renders a size the way a person reading a terminal
// reads one. Exact byte counts stay available in the JSON projection.
func reviewStoreResetBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value, exponent := float64(bytes)/unit, 0
	for value >= unit && exponent < 3 {
		value /= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %sB", value, []string{"K", "M", "G", "T"}[exponent])
}
