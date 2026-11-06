package reviewtransaction

import "testing"

// TestValidationCheckInconclusiveRejectsUnknownStatus is the fail-closed
// proof for issue #4266's follow-up: a typed inspection.status of anything
// other than "completed" or "unavailable" -- empty, a typo, or wrong casing
// -- must be refused as a schema violation, never silently treated as a
// completed inspection.
func TestValidationCheckInconclusiveRejectsUnknownStatus(t *testing.T) {
	for _, status := range []ValidationInspectionStatus{"", "partial", "Completed"} {
		if _, err := ValidationCheckInconclusive(&ValidationInspection{Status: status}, nil); err == nil {
			t.Errorf("ValidationCheckInconclusive(status=%q) admitted an unrecognized status instead of refusing it", status)
		}
	}
}

// TestValidationCheckInconclusiveHonorsTheTwoAdmittedStatuses pairs the test
// above: the only two admitted values decide conclusiveness without error.
func TestValidationCheckInconclusiveHonorsTheTwoAdmittedStatuses(t *testing.T) {
	if inconclusive, err := ValidationCheckInconclusive(&ValidationInspection{Status: ValidationInspectionCompleted}, nil); err != nil || inconclusive {
		t.Fatalf("completed status = (%v, %v), want (false, nil)", inconclusive, err)
	}
	if inconclusive, err := ValidationCheckInconclusive(&ValidationInspection{Status: ValidationInspectionUnavailable, Reason: "no access"}, nil); err != nil || !inconclusive {
		t.Fatalf("unavailable status = (%v, %v), want (true, nil)", inconclusive, err)
	}
}
