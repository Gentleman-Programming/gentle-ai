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
	if reviewStatusPhaseTimingsEnvEnabled() {
		return true
	}
	for _, arg := range args {
		if arg == "--phase-timings" || strings.HasPrefix(arg, "--phase-timings=") {
			if strings.HasPrefix(arg, "--phase-timings=") {
				enabled, err := strconv.ParseBool(strings.TrimPrefix(arg, "--phase-timings="))
				return err == nil && enabled
			}
			return true
		}
	}
	return false
}

func reviewStatusPhaseTimingsEnvEnabled() bool {
	val := strings.TrimSpace(os.Getenv(reviewStatusPhaseTimingsEnvVar))
	if val == "" {
		return false
	}
	enabled, err := strconv.ParseBool(val)
	return err == nil && enabled
}

func emitReviewStatusPhaseTimings(timings *reviewtransaction.StatusPhaseTimings) {
	if timings == nil || reviewStatusPhaseTimingsWriter == nil {
		return
	}
	_, _ = fmt.Fprintln(reviewStatusPhaseTimingsWriter, timings.Snapshot().FormatSummary())
}
