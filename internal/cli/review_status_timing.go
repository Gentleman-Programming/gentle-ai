package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

var reviewStatusPhaseTimingsWriter io.Writer = os.Stderr

const reviewStatusPhaseTimingsEnvVar = "GENTLE_AI_STATUS_PHASE_TIMINGS"

func reviewStatusPhaseTimingsEnabled(args []string) bool {
	enabled := reviewStatusPhaseTimingsEnvEnabled()
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--phase-timings" {
			enabled = true
		} else if strings.HasPrefix(arg, "--phase-timings=") {
			parsed, err := strconv.ParseBool(strings.TrimPrefix(arg, "--phase-timings="))
			if err == nil {
				enabled = parsed
			}
		}
	}
	return enabled
}

func reviewStatusPhaseTimingsEnvEnabled() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(reviewStatusPhaseTimingsEnvVar)))
	return err == nil && enabled
}

func emitReviewStatusPhaseTimings(timings *reviewtransaction.StatusPhaseTimings) {
	if timings == nil || reviewStatusPhaseTimingsWriter == nil {
		return
	}
	_, _ = fmt.Fprintln(reviewStatusPhaseTimingsWriter, timings.Snapshot().FormatSummary())
}
