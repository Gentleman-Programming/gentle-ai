package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
)

// handleRolledBackPersonaTransition reports whether the pipeline failure is a
// fully rolled-back persona output-style removal. When the retired style file
// could not be removed after the new style and settings were written, the
// pipeline restores the pre-transition state; the CLI then converts that
// outcome into an explanatory warning plus exit 0 instead of a hard failure
// (REQ-EXIT-WARNING, D2). It returns true only for the typed removal error
// AND a successful rollback — a failed rollback, a generic error, or nil all
// fall through to the normal hard-failure path and print nothing.
func handleRolledBackPersonaTransition(exec pipeline.ExecutionResult) bool {
	var removalErr *persona.OutputStyleRemovalError
	if !errors.As(exec.Err, &removalErr) || !exec.Rollback.Success {
		return false
	}
	fmt.Fprintf(os.Stderr, "WARNING: %s\n", persona.MessageRolledBackOutputStyle)
	return true
}
