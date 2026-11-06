package cli

import (
	"context"
	"testing"
)

// Legacy resolution-test helpers stay out of the production call graph.
func checkToolBinaries(pathDirs []string, installedAgents []string) []CheckResult {
	required := requiredDoctorTools(installedAgents)
	results := make([]CheckResult, 0, len(required))
	for _, tool := range required {
		results = append(results, checkOneTool(tool, pathDirs))
	}
	return results
}

func checkOneTool(tool string, pathDirs []string) CheckResult {
	return checkOneToolContext(context.Background(), tool, pathDirs)
}

// Resolution/shadowing tests fake binaries, so explicitly fake execution too.
// The public execution regression leaves this seam real.
func stubDoctorToolProbe(t *testing.T) {
	t.Helper()
	original := doctorToolProbeFn
	t.Cleanup(func() { doctorToolProbeFn = original })
	doctorToolProbeFn = func(context.Context, string, string) error { return nil }
}
