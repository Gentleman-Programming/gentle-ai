package verify

import "context"

type CheckStatus string

const (
	CheckStatusPassed  CheckStatus = "passed"
	CheckStatusFailed  CheckStatus = "failed"
	CheckStatusSkipped CheckStatus = "skipped"
	CheckStatusWarning CheckStatus = "warning"
)

type Check struct {
	ID          string
	Description string
	Run         func(context.Context) error
	// Soft marks this check as non-blocking: errors produce a warning instead of a failure.
	Soft bool
	// NoRollback marks this check as exempt from triggering a rollback on failure.
	// The zero value (false) preserves today's behavior: a non-soft check still triggers rollback.
	NoRollback bool
}

type CheckResult struct {
	ID          string
	Description string
	Status      CheckStatus
	Error       string
	NoRollback  bool
}

func RunChecks(ctx context.Context, checks []Check) []CheckResult {
	results := make([]CheckResult, 0, len(checks))
	for _, check := range checks {
		result := CheckResult{ID: check.ID, Description: check.Description, NoRollback: check.NoRollback}
		if check.Run == nil {
			result.Status = CheckStatusSkipped
			result.Error = "check not implemented"
			results = append(results, result)
			continue
		}

		if err := check.Run(ctx); err != nil {
			if check.Soft {
				result.Status = CheckStatusWarning
			} else {
				result.Status = CheckStatusFailed
			}
			result.Error = err.Error()
			results = append(results, result)
			continue
		}

		result.Status = CheckStatusPassed
		results = append(results, result)
	}

	return results
}
