package sdd

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

const engramProjectIdentityContractAsset = "skills/_shared/engram-project-identity.md"

func engramProjectIdentityContract() string {
	return strings.TrimSpace(assets.MustRead(engramProjectIdentityContractAsset))
}

// injectEngramProjectIdentityContract gives every rendered SDD entry point the
// same workspace-versus-Engram identity contract without duplicating it across
// provider-specific assets.
func injectEngramProjectIdentityContract(prompt string) string {
	return filemerge.InjectMarkdownSection(prompt, "engram-project-identity", engramProjectIdentityContract())
}
