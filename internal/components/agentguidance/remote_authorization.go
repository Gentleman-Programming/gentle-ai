package agentguidance

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

// InjectRemoteAuthorization projects the canonical behavioral boundary into a
// prompt without changing native permissions. Repeated projection is a no-op.
// Executor integrations can reuse it without importing coordinator routing.
func InjectRemoteAuthorization(prompt string) string {
	contract := assets.MustRead("generic/remote-authorization-contract.md")
	return filemerge.InjectMarkdownSection(prompt, "remote-authorization", strings.TrimSpace(contract))
}
