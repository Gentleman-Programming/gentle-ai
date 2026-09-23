package skillregistry

// Engram mirror constants (D-11, REQ-22.11). The unified index persists to a
// single stable topic so two workspaces cannot clobber each other's history.
const (
	// MirrorTopicKey is the stable Engram upsert key for the skills index.
	MirrorTopicKey = "skill-registry"
	// MirrorType is the Engram observation type of the mirrored index.
	MirrorType = "config"
)

// MirrorRequest is the observation the unified skills index persists to the
// Engram topic `skill-registry` (REQ-22.11, D-11). It is the engine-side shape;
// the wiring layer translates it into an engram MCP `mem_save` call.
type MirrorRequest struct {
	// Project scopes the observation to one workspace so two projects cannot
	// overwrite each other's topic_key. The engine leaves it empty: the wiring
	// layer resolves it at one point (open decision O-5).
	Project string
	// TopicKey is the stable upsert key (always MirrorTopicKey).
	TopicKey string
	// Type is the observation type (always MirrorType).
	Type string
	// Title is the observation title, e.g. "Skill registry — my-project".
	Title string
	// Content is the managed `## Skills` body preceded by one provenance line.
	Content string
	// CapturePrompt is always false: an automated artifact must never fabricate
	// prompt capture for a write a human did not author.
	CapturePrompt bool
}

// MirrorFunc persists one observation. It is the only doorway out of this
// package toward Engram: internal/skillregistry never imports
// internal/components/engram (the import-boundary guard enforces it).
//
// A nil MirrorFunc reports MirrorFailed with the reason "mirror not
// configured": the binary cannot claim `mirror ok` for a write it never
// attempted (T-8).
type MirrorFunc func(req MirrorRequest) error
