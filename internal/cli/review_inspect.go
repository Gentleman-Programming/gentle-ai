package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func RunReviewInspectAuthority(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review inspect-authority", stdout, "Read every compact-v2 recovery edge, report invalid edges and load diagnostics, and never mutate authority.")
	cwd := flags.String("cwd", ".", "repository path")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review inspect-authority argument %q", flags.Arg(0))
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	report, err := reviewtransaction.InspectCompactAuthority(context.Background(), root)
	if err != nil {
		return fmt.Errorf("inspect compact authority: %w", err)
	}
	if err := encodeReviewJSON(stdout, report); err != nil {
		return err
	}
	if len(report.Diagnostics) != 0 {
		return errors.New("review inspect-authority found compact authority diagnostics")
	}
	return nil
}
