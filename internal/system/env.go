package system

import (
	"os"
	"strings"
)

// Well-known environment variable names with Axiom canonical and Gentle AI legacy keys.
const (
	EnvChannelAxiom                      = "AXIOM_CHANNEL"
	EnvChannelGentle                     = "GENTLE_AI_CHANNEL"
	EnvOpenCodeBackgroundSubagentsAxiom  = "AXIOM_OPENCODE_BACKGROUND_SUBAGENTS"
	EnvOpenCodeBackgroundSubagentsGentle = "GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS"
	EnvStateDirAxiom                     = "AXIOM_STATE_DIR"
	EnvStateDirGentle                    = "GENTLE_AI_STATE_DIR"
)

// LookupEnv returns the value of the first present environment variable from the provided keys.
// If a single key starting with "AXIOM_" is passed, it automatically evaluates the legacy
// "GENTLE_AI_" counterpart as fallback when the primary key is unset.
func LookupEnv(keys ...string) (string, bool) {
	if len(keys) == 0 {
		return "", false
	}
	for _, key := range keys {
		if key != "" {
			if val, ok := os.LookupEnv(key); ok {
				return val, true
			}
		}
	}
	if len(keys) == 1 && strings.HasPrefix(keys[0], "AXIOM_") {
		fallbackKey := "GENTLE_AI_" + strings.TrimPrefix(keys[0], "AXIOM_")
		return os.LookupEnv(fallbackKey)
	}
	return "", false
}

// Getenv returns the first non-empty value of the environment variables from the provided keys.
// If a single key starting with "AXIOM_" is passed, it automatically evaluates the legacy
// "GENTLE_AI_" counterpart as fallback when the primary key is unset or empty.
// If none is set, it returns the empty string.
func Getenv(keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	for _, key := range keys {
		if key != "" {
			if val := os.Getenv(key); val != "" {
				return val
			}
		}
	}
	if len(keys) == 1 && strings.HasPrefix(keys[0], "AXIOM_") {
		fallbackKey := "GENTLE_AI_" + strings.TrimPrefix(keys[0], "AXIOM_")
		return os.Getenv(fallbackKey)
	}
	return ""
}
