package cli

import "github.com/gentleman-programming/gentle-ai/v4/internal/system"

// RunInstall keeps argument-based setup in tests; production reuses a prepared plan.
func RunInstall(args []string, detection system.DetectionResult) (InstallResult, error) {
	prepared, err := PrepareInstall(args, detection)
	if err != nil {
		return InstallResult{}, err
	}
	return RunPreparedInstall(prepared, detection)
}
