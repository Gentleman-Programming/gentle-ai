package qastage

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// VocabularyID is the stable identifier for the QA Orchestrator V2 vocabulary.
const VocabularyID = "gentle-ai.qa-orchestrator/v2"

// VocabularyV1 returns the standard QA Orchestrator V2 stage vocabulary.
// It enforces the exact sequence: explore -> spec -> apply -> verify -> docs.
func VocabularyV1() sddstatus.StageVocabulary {
	return sddstatus.StageVocabulary{
		ID: VocabularyID,
		Stages: []sddstatus.Stage{
			{Label: "explore"},
			{Label: "spec"},
			{Label: "apply"},
			{Label: "verify"},
			{Label: "docs"},
		},
	}
}

// LedgerChangeName prefixes the user's target change name with "qa--"
// to ensure perfect isolation from the primary SDD runtime ledger.
// This prevents cross-ledger pollution since QA Orchestrator uses
// the exact same underlying filecoord persistence mechanism.
func LedgerChangeName(change string) string {
	change = strings.TrimSpace(change)
	if change == "" {
		return ""
	}
	return fmt.Sprintf("qa--%s", change)
}
