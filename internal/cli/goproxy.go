package cli

import (
	"os"
	"strings"
)

// GoProxyBypassEnv returns a copy of base (or the current environment when base is
// nil) with the given module prepended to GONOSUMDB, GOPRIVATE and GONOPROXY. This
// makes `go install <module>...@main` fetch the true HEAD directly from the source
// instead of a proxy/sumdb-cached pseudo-version. It is shared by the channel install
// flow and the self-upgrade flow so both build beta identically.
func GoProxyBypassEnv(base []string, module string) []string {
	if base == nil {
		base = os.Environ()
	}
	env := append([]string{}, base...)
	for _, key := range []string{"GONOSUMDB", "GOPRIVATE", "GONOPROXY"} {
		env = setGoEnvValue(env, key, prependGoPattern(getGoEnvValue(env, key), module))
	}
	return env
}

func getGoEnvValue(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}

func setGoEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func prependGoPattern(existing, pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return existing
	}
	parts := strings.Split(existing, ",")
	for _, part := range parts {
		if strings.TrimSpace(part) == pattern {
			return existing
		}
	}
	if strings.TrimSpace(existing) == "" {
		return pattern
	}
	return pattern + "," + existing
}
