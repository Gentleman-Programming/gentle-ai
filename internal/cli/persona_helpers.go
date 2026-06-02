package cli

import "github.com/gentleman-programming/gentle-ai/internal/model"

func isGentlemanConversationPersona(persona model.PersonaID) bool {
	return persona == model.PersonaGentleman || persona == model.PersonaGentlemanNeutralArtifacts
}
