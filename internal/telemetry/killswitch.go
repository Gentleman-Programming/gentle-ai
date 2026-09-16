package telemetry

import "strings"

// Source names which input decided whether telemetry is enabled, in the
// exact precedence order the issue specifies.
type Source string

const (
	SourceDoNotTrack     Source = "DO_NOT_TRACK"
	SourceAxiomEnvOptOut Source = "AXIOM_TELEMETRY"
	SourceEnvOptOut      Source = "GENTLE_AI_TELEMETRY"
	SourceCI             Source = "CI"
	SourceStateDisable   Source = "state"
	SourceDefault        Source = "default"
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
// AXIOM_TELEMETRY=0 (or GENTLE_AI_TELEMETRY=0 as fallback), then CI or
// GITHUB_ACTIONS set to anything but empty, "0", or "false", then the
// persisted state's enabled flag. The first one that opts out wins; with
// none present, telemetry is enabled by default.
func Decide(getenv Getenv, persisted State) Decision {
	if doNotTrack(getenv("DO_NOT_TRACK")) {
		return Decision{Enabled: false, Source: SourceDoNotTrack}
	}
	if v := getenv("AXIOM_TELEMETRY"); v != "" {
		if v == "0" {
			return Decision{Enabled: false, Source: SourceAxiomEnvOptOut}
		}
	} else if getenv("GENTLE_AI_TELEMETRY") == "0" {
		return Decision{Enabled: false, Source: SourceEnvOptOut}
	}
	if truthy(getenv("CI")) || truthy(getenv("GITHUB_ACTIONS")) {
		return Decision{Enabled: false, Source: SourceCI}
	}
	if !persisted.Enabled {
		return Decision{Enabled: false, Source: SourceStateDisable}
	}
	return Decision{Enabled: true, Source: SourceDefault}
}

// doNotTrack follows the console DO_NOT_TRACK convention: opted out for any
// value other than empty, "0", or "false" (case-insensitive, trimmed).
func doNotTrack(v string) bool { return truthy(v) }

// truthy reads a CI-style flag: set to anything but empty, "0", or "false".
// CI systems disagree on the value (GitHub Actions and most others export
// CI=true, some export CI=1), so equality with "true" is not enough.
func truthy(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v != "" && v != "0" && v != "false"
}

// Endpoint resolves the collector URL: AXIOM_TELEMETRY_ENDPOINT or
// GENTLE_AI_TELEMETRY_ENDPOINT when set and non-empty, otherwise DefaultEndpoint.
func Endpoint(getenv Getenv) string {
	if v := strings.TrimSpace(getenv(EndpointAxiomEnvVar)); v != "" {
		return v
	}
	if v := strings.TrimSpace(getenv(EndpointGentleEnvVar)); v != "" {
		return v
	}
	return DefaultEndpoint
}
