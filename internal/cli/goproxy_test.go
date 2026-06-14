package cli

import "testing"

func TestGoProxyBypassEnvPreservesExistingPatterns(t *testing.T) {
	module := "github.com/gentleman-programming/gentle-ai"
	env := GoProxyBypassEnv([]string{
		"PATH=/usr/bin",
		"GONOSUMDB=example.com/private",
		"GOPRIVATE=github.com/acme/*",
		"GONOPROXY=github.com/gentleman-programming/gentle-ai",
	}, module)

	for _, want := range []string{
		"PATH=/usr/bin",
		"GONOSUMDB=github.com/gentleman-programming/gentle-ai,example.com/private",
		"GOPRIVATE=github.com/gentleman-programming/gentle-ai,github.com/acme/*",
		// Already present: must not be duplicated.
		"GONOPROXY=github.com/gentleman-programming/gentle-ai",
	} {
		if !goEnvContains(env, want) {
			t.Fatalf("env missing %q in %v", want, env)
		}
	}
}

func goEnvContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
