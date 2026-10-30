package cli

import (
	"errors"
	"os"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// restoreCodexServiceTier selects the tier state recorded as written, which is
// also the tier Gentle AI manages. Invalid persisted values are dropped and
// never used to edit config.toml.
func restoreCodexServiceTier(selection *model.Selection, persisted state.InstallState) {
	tier := persisted.CodexServiceTier
	if !model.ValidCodexServiceTier(tier) {
		tier = ""
	}
	selection.CodexServiceTier, selection.CodexManagedServiceTier = tier, tier
}

// noteCodexServiceTier keeps the service tier a global engram injection left
// in Codex's config.toml. Results that wrote no Codex config are ignored.
func (s *runtimeState) noteCodexServiceTier(result engram.InjectionResult) {
	if s != nil && result.CodexServiceTier != nil {
		s.codexServiceTier = result.CodexServiceTier
	}
}

// recordCodexServiceTier stores engram's write result as the managed tier.
// nil means Codex config.toml was not written, so the previous value stays.
func recordCodexServiceTier(homeDir string, tier *string) error {
	if tier == nil {
		return nil
	}
	return withInstallStateLock(homeDir, func() error {
		latest, err := state.Read(homeDir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if latest.CodexServiceTier == *tier {
			return nil
		}
		latest.CodexServiceTier = *tier
		return state.WriteReconciled(homeDir, latest)
	})
}
