package cli

import "github.com/gentleman-programming/gentle-ai/v2/internal/model"

// isGentlemanConversationPersona reports whether the persona keeps the voseo
// conversation tone. The gentleman-neutral-artifacts legacy alias is remapped
// to neutral (see normalizePersona) and is intentionally NOT gentleman here.
func isGentlemanConversationPersona(persona model.PersonaID) bool {
	return persona == model.PersonaGentleman
}
