package reviewtransaction

import (
	"os"
	"testing"
	"time"
)

func TestGetRelayTimeoutDefault(t *testing.T) {
	// unset env -> default 10 minutes
	if v := os.Getenv("GENTLE_AI_RELAY_TIMEOUT"); v != "" {
		os.Unsetenv("GENTLE_AI_RELAY_TIMEOUT")
		defer os.Setenv("GENTLE_AI_RELAY_TIMEOUT", v)
	}
	if got := GetRelayTimeout(); got != 10*time.Minute {
		t.Errorf("GetRelayTimeout() = %v, want %v", got, 10*time.Minute)
	}
}

func TestGetRelayTimeoutValid(t *testing.T) {
	// set valid duration
	os.Setenv("GENTLE_AI_RELAY_TIMEOUT", "2m30s")
	if got := GetRelayTimeout(); got != 2*time.Minute+30*time.Second {
		t.Errorf("GetRelayTimeout() = %v, want %v", got, 2*time.Minute+30*time.Second)
	}
}

func TestGetRelayTimeoutInvalidFallback(t *testing.T) {
	// invalid duration falls back to default
	os.Setenv("GENTLE_AI_RELAY_TIMEOUT", "notaduration")
	if got := GetRelayTimeout(); got != 10*time.Minute {
		t.Errorf("GetRelayTimeout() = %v, want %v (fallback)", got, 10*time.Minute)
	}
}
