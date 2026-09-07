package telemetry

import (
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// versionPattern mirrors event.schema.json's `version` pattern: a numeric
// release version, optionally with a pre-release suffix. Nothing else is
// ever sent — see sanitizeVersion.
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.+-]{1,64})?$`)

// devVersion is what an unrecognized (untagged, "dev", or otherwise
// non-numeric) build reports instead of its raw version string, so `version`
// is always closed to the contract's pattern.
const devVersion = "0.0.0-dev"

// sanitizeVersion returns v unchanged when it already matches the
// contract's numeric pattern, and devVersion otherwise. This is what lets
// the schema enforce a pattern on `version` instead of leaving it free text.
func sanitizeVersion(v string) string {
	if versionPattern.MatchString(v) {
		return v
	}
	return devVersion
}

// knownAgents and knownComponents mirror event.schema.json's closed enums.
// They are derived from internal/model's own AgentID/ComponentID constants
// rather than a second hand-maintained list, though the schema itself still
// needs updating by hand when a new agent or component ships (a schema
// cannot import Go constants).
var knownAgents = map[string]bool{
	string(model.AgentClaudeCode): true, string(model.AgentOpenCode): true, string(model.AgentKilocode): true,
	string(model.AgentGeminiCLI): true, string(model.AgentCursor): true, string(model.AgentVSCodeCopilot): true,
	string(model.AgentCodex): true, string(model.AgentAntigravity): true, string(model.AgentWindsurf): true,
	string(model.AgentKimi): true, string(model.AgentQwenCode): true, string(model.AgentKiroIDE): true,
	string(model.AgentOpenClaw): true, string(model.AgentPi): true, string(model.AgentTrae): true,
	string(model.AgentHermes): true,
}

var knownComponents = map[string]bool{
	string(model.ComponentEngram): true, string(model.ComponentSDD): true, string(model.ComponentSkills): true,
	string(model.ComponentContext7): true, string(model.ComponentPersona): true, string(model.ComponentPermission): true,
	string(model.ComponentGGA): true, string(model.ComponentTheme): true, string(model.ComponentClaudeTheme): true,
	string(model.ComponentOpenCodeGentleLogo): true,
}

// filterKnown drops any value not present in known, rather than sending an
// unrecognized identifier the closed schema would reject wholesale. A
// persisted selection can only ever contain values that were valid at
// install time, but this stays defensive against a future rename or a
// state file written by a newer build.
func filterKnown(values []string, known map[string]bool) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if known[v] {
			out = append(out, v)
		}
	}
	return out
}

// Event is the exact JSON POST body sent to the collector
// (gentle-ai.telemetry-event/v1). Every field is either a fixed enum, a
// random identifier, or a count — never a path, hostname, username, prompt,
// or diff.
type Event struct {
	Schema     string    `json:"schema"`
	Event      string    `json:"event"`
	InstallID  string    `json:"install_id"`
	SentAt     string    `json:"sent_at"`
	Version    string    `json:"version"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	Agents     []string  `json:"agents"`
	Components []string  `json:"components"`
	RDDEnabled bool      `json:"rdd_enabled"`
	Counters   *Counters `json:"counters,omitempty"`
}

// BuildInput is every input the event builder needs. Kind selects the event
// shape: EventInstall never carries counters, EventHeartbeat always does
// (even when every counter is zero, so the collector can distinguish "no
// activity" from "no report").
type BuildInput struct {
	Kind       string
	InstallID  string
	Now        time.Time
	Version    string
	Agents     []string
	Components []string
	RDDEnabled bool
	Counters   Counters
}

// Build assembles one Event from already-resolved inputs. It never reads the
// filesystem, the environment, or another package's state: every value it
// needs is a parameter, which is what keeps this leaf package free of the
// internal/cli and internal/state dependency that would otherwise cycle.
func Build(in BuildInput) Event {
	ev := Event{
		Schema:     EventSchema,
		Event:      in.Kind,
		InstallID:  in.InstallID,
		SentAt:     in.Now.UTC().Format(time.RFC3339),
		Version:    sanitizeVersion(in.Version),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Agents:     nonNilStrings(filterKnown(in.Agents, knownAgents)),
		Components: nonNilStrings(filterKnown(in.Components, knownComponents)),
		RDDEnabled: in.RDDEnabled,
	}
	if in.Kind == EventHeartbeat {
		counters := in.Counters
		ev.Counters = &counters
	}
	return ev
}

// Marshal encodes the event and enforces the contract's 4 KiB body limit.
func Marshal(ev Event) ([]byte, error) {
	data, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxPayloadBytes {
		return nil, fmt.Errorf("telemetry event exceeds %d bytes (got %d)", MaxPayloadBytes, len(data))
	}
	return data, nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
