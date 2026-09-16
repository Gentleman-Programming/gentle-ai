package system

import "os"

// LookupEnv returns the value of axiomKey if present in the environment.
// If axiomKey is unset, it falls back to looking up fallbackKey.
func LookupEnv(axiomKey, fallbackKey string) (string, bool) {
	if val, ok := os.LookupEnv(axiomKey); ok {
		return val, true
	}
	if fallbackKey != "" {
		return os.LookupEnv(fallbackKey)
	}
	return "", false
}

// Getenv returns the value of axiomKey if set and non-empty.
// If axiomKey is unset or empty, it returns the value of fallbackKey.
// If neither is set, it returns the empty string.
func Getenv(axiomKey, fallbackKey string) string {
	if val := os.Getenv(axiomKey); val != "" {
		return val
	}
	if fallbackKey != "" {
		return os.Getenv(fallbackKey)
	}
	return ""
}
