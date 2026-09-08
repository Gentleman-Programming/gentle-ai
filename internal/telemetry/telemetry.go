// Package telemetry implements Gentle AI's anonymous, opt-out usage
// telemetry (issue #4309). It knows how many installs stay alive and how
// they use the review pipeline, without collecting anything about the user,
// their code, or their machine identity.
//
// The package is intentionally a leaf: it must not import internal/cli or
// internal/app, both of which import it (internal/app also imports
// internal/cli), so any input it needs that would otherwise come from those
// packages (the build version, the RDD kill-switch reading) is passed in by
// the caller instead of resolved here.
package telemetry

import "time"

// EventSchema identifies the JSON POST body sent to the collector.
const EventSchema = "gentle-ai.telemetry-event/v1"

// StatusSchema identifies the `gentle-ai telemetry status` projection.
const StatusSchema = "gentle-ai.telemetry-status/v1"

// EventInstall and EventHeartbeat are the two event kinds the contract
// defines. No other value is ever sent.
const (
	EventInstall   = "install"
	EventHeartbeat = "heartbeat"
)

// DefaultEndpoint is the collector URL used when GENTLE_AI_TELEMETRY_ENDPOINT
// is not set.
const DefaultEndpoint = "https://telemetry.gentlemanprogramming.com/v1/events"

// EndpointEnvVar overrides DefaultEndpoint.
const EndpointEnvVar = "GENTLE_AI_TELEMETRY_ENDPOINT"

// NoticeLine is printed to stderr exactly once: by Opportunistic's
// enrollment step, before any event is ever built or sent. That one
// enrollment run sends nothing at all; the first real send only happens on
// a later trigger. It is never printed by the sender itself.
const NoticeLine = "Gentle AI sends anonymous usage metrics (version, OS, agents, counters); run gentle-ai telemetry disable to opt out."

// MaxPayloadBytes bounds the JSON POST body per the issue's contract.
const MaxPayloadBytes = 4096

// ConnectTimeout and TotalTimeout bound the fire-and-forget send. They are
// enforced by the detached `telemetry send` child process, never by the
// command that triggered it.
const (
	ConnectTimeout = 2 * time.Second
	TotalTimeout   = 3 * time.Second
)

// HeartbeatInterval is the minimum spacing between two heartbeat events for
// the same install_id.
const HeartbeatInterval = 24 * time.Hour

// FailureBackoff is how long Opportunistic waits after a failed send before
// spawning another one, so a blocked or unreachable endpoint does not
// respawn a failing child on every install/update/sync/review/sdd-attempt.
const FailureBackoff = 6 * time.Hour
