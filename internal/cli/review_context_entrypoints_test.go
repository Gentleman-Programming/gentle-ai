package cli

import (
	"context"
	"io"
)

type reviewContextCommandForTest func(context.Context, []string, io.Writer) error

func reviewContextEntrypointForTest(command reviewContextCommandForTest) func([]string, io.Writer) error {
	return func(args []string, stdout io.Writer) error {
		return command(context.Background(), args, stdout)
	}
}

var (
	RunReviewFacadeStart    = reviewContextEntrypointForTest(runReviewFacadeStart)
	RunReviewFacadeValidate = reviewContextEntrypointForTest(runReviewFacadeValidateNonDeciding)
	RunReviewStatus         = reviewContextEntrypointForTest(runReviewStatus)
	RunReviewRepair         = reviewContextEntrypointForTest(runReviewRepair)
)
