package reviewerprovider

import "github.com/gentleman-programming/gentle-ai/v2/internal/model"

// registeredRuntimeIdentities is deliberately closed. A runtime appears here
// only after the compiled review boundary admits it: Claude's prompt-carried
// generated reviewer, Codex's advisory scratch process, and the host-mediated
// relays owned by OpenCode and gentle-pi. Consumers of the published contract
// bundle verify this list offline before trusting a runtime identity; prompt
// prose never expands it.
var registeredRuntimeIdentities = []string{
	"claude-code",
	"codex",
	"opencode",
	"pi",
}

// RegisteredRuntimeIdentities returns a copy of every runtime identity the
// provider contract admits, in stable lexical order.
func RegisteredRuntimeIdentities() []string {
	return append([]string(nil), registeredRuntimeIdentities...)
}

// RegisteredRuntime reports whether the agent is one of the closed runtime
// identities above, so consumers can gate runtime-wide contract prose on the
// same list the compiled review boundary admits instead of restating it.
func RegisteredRuntime(agent model.AgentID) bool {
	for _, identity := range registeredRuntimeIdentities {
		if identity == string(agent) {
			return true
		}
	}
	return false
}
