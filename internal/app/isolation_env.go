package app

import (
	"os"

	piagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// validateIsolationEnv rejects an invalid GENTLE_AI_HOME,
// GENTLE_AI_PI_CONFIG_DIR or GENTLE_AI_ENGRAM_BIN before detection,
// self-update or any write, then applies GENTLE_AI_HOME to backup checks.
func validateIsolationEnv() error {
	for _, validate := range []func() error{state.ValidateHomeEnv, piagent.ValidateGentlePiConfigDirEnv, engram.ValidateCallerBinary} {
		if err := validate(); err != nil {
			return err
		}
	}
	applyStateHome()
	return nil
}

// applyStateHome points backup deletion checks at GENTLE_AI_HOME/backups,
// where install and sync write backups when that variable is set.
func applyStateHome() {
	if os.Getenv(state.HomeEnv) == "" {
		return
	}
	backup.BackupRootFn = func() (string, error) { return state.BackupsDir(""), nil }
}
