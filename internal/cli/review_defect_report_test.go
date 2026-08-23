package cli

import (
	"strings"
	"testing"
)

func TestDefectReportScrubsCaptureResultSlotConflict(t *testing.T) {
	home := "/home/agent/private-project"
	payload := reviewScrubDefectReportField("review capture-result " + reviewerResultSlotOccupiedCode + " " + home + "/result.json")
	if strings.Contains(payload, home) || !strings.Contains(payload, reviewerResultSlotOccupiedCode) {
		t.Fatalf("capture-result defect report field = %q", payload)
	}
}
