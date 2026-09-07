package telemetry

import "strings"

// Source names which input decided whether telemetry is enabled, in the
// exact precedence order the issue specifies.
type Source string

const (
	SourceDoNotTrack   Source = "DO_NOT_TRACK"
	SourceEnvOptOut    Source = "GENTLE_AI_TELEMETRY"
	SourceCI           Source = "CI"
	SourceStateDisable Source = "state"
	SourceDefault      Source = "default"
)

// Decision reports whether sending is allowed and which source decided it.
type Decision struct {
	Enabled bool
	Source  Source
}

// Getenv is the shape telemetry needs from the environment; production
// callers pass os.Getenv, tests pass a map lookup.
type Getenv func(key string) string

// Decide evaluates the kill switches in their documented precedence:
// DO_NOT_TRACK set to anything but empty, "0", or "false", then
// GENTLE_AI_TELEMETRY=0, then CI=true, then the persisted state's enabled
// flag. The first one that opts out wins; with none present, telemetry is
// enabled by default.
func Decide(getenv Getenv, persisted State) Decision {
	if doNotTrack(getenv("DO_NOT_TRACK")) {
		return Decision{Enabled: false, Source: SourceDoNotTrack}
	}
	if getenv("GENTLE_AI_TELEMETRY") == "0" {
		return Decision{Enabled: false, Source: SourceEnvOptOut}
	}
	if strings.EqualFold(strings.TrimSpace(getenv("CI")), "true") {
		return Decision{Enabled: false, Source: SourceCI}
	}
	if !persisted.Enabled {
		return Decision{Enabled: false, Source: SourceStateDisable}
	}
	return Decision{Enabled: true, Source: SourceDefault}
}

// doNotTrack follows the console DO_NOT_TRACK convention: opted out for any
// value other than empty, "0", or "false" (case-insensitive, trimmed).
func doNotTrack(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v != "" && v != "0" && v != "false"
}

// Endpoint resolves the collector URL: GENTLE_AI_TELEMETRY_ENDPOINT when set
// and non-empty, otherwise DefaultEndpoint.
func Endpoint(getenv Getenv) string {
	if v := strings.TrimSpace(getenv(EndpointEnvVar)); v != "" {
		return v
	}
	return DefaultEndpoint
}
