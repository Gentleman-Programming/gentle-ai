package reviewerprovider

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
