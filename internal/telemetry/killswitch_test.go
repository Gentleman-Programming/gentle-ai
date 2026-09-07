package telemetry

import "testing"

func envMap(values map[string]string) Getenv {
	return func(key string) string { return values[key] }
}

func TestDecidePrecedence(t *testing.T) {
	enabledState := State{Enabled: true}
	disabledState := State{Enabled: false}

	cases := []struct {
		name       string
		env        map[string]string
		state      State
		wantSource Source
	}{
		{"do not track wins over everything", map[string]string{"DO_NOT_TRACK": "1", "GENTLE_AI_TELEMETRY": "1", "CI": "false"}, enabledState, SourceDoNotTrack},
		{"env opt-out wins over CI and state", map[string]string{"GENTLE_AI_TELEMETRY": "0", "CI": "false"}, enabledState, SourceEnvOptOut},
		{"CI true wins over state", map[string]string{"CI": "true"}, enabledState, SourceCI},
		{"CI is case-insensitive", map[string]string{"CI": "True"}, enabledState, SourceCI},
		{"state disable is the last resort", map[string]string{}, disabledState, SourceStateDisable},
		{"default enabled when nothing opts out", map[string]string{"DO_NOT_TRACK": "0", "GENTLE_AI_TELEMETRY": "1", "CI": "false"}, enabledState, SourceDefault},
		{"DO_NOT_TRACK opts out for any non-off value", map[string]string{"DO_NOT_TRACK": "yes"}, enabledState, SourceDoNotTrack},
		{"DO_NOT_TRACK true opts out", map[string]string{"DO_NOT_TRACK": "true"}, enabledState, SourceDoNotTrack},
		{"DO_NOT_TRACK 0 stays default", map[string]string{"DO_NOT_TRACK": "0"}, enabledState, SourceDefault},
		{"DO_NOT_TRACK false stays default", map[string]string{"DO_NOT_TRACK": "false"}, enabledState, SourceDefault},
		{"DO_NOT_TRACK empty stays default", map[string]string{"DO_NOT_TRACK": ""}, enabledState, SourceDefault},
		{"GENTLE_AI_TELEMETRY requires exactly 0", map[string]string{"GENTLE_AI_TELEMETRY": "false"}, enabledState, SourceDefault},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decision := Decide(envMap(c.env), c.state)
			if decision.Source != c.wantSource {
				t.Fatalf("source = %q, want %q", decision.Source, c.wantSource)
			}
			wantEnabled := c.wantSource == SourceDefault
			if decision.Enabled != wantEnabled {
				t.Fatalf("enabled = %v, want %v", decision.Enabled, wantEnabled)
			}
		})
	}
}

func TestEndpointDefaultAndOverride(t *testing.T) {
	if got := Endpoint(envMap(nil)); got != DefaultEndpoint {
		t.Fatalf("Endpoint() = %q, want default %q", got, DefaultEndpoint)
	}
	custom := "https://example.invalid/v1/events"
	if got := Endpoint(envMap(map[string]string{EndpointEnvVar: custom})); got != custom {
		t.Fatalf("Endpoint() = %q, want override %q", got, custom)
	}
	if got := Endpoint(envMap(map[string]string{EndpointEnvVar: "  "})); got != DefaultEndpoint {
		t.Fatalf("Endpoint() with blank override = %q, want default", got)
	}
}
