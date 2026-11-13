package reviewtransaction

import (
	"os"
	"time"
)

// GetRelayTimeout returns the duration for relay timeout, configurable via
// GENTLE_AI_RELAY_TIMEOUT. If unset or invalid, returns the default of 10 minutes.
func GetRelayTimeout() time.Duration {
	if v := os.Getenv("GENTLE_AI_RELAY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	// Default: 10 minutes (calibrated from L6 evidence)
	return 10 * time.Minute
}
