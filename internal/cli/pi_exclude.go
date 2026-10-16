package cli

import (
	"fmt"
	"os"

	piagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// PiExcludePackagesEnv lists managed Pi package sources the install must not
// run `pi install` for (comma-separated). A non-empty --pi-exclude-package
// flag wins over it; the value is never persisted.
const PiExcludePackagesEnv = "GENTLE_AI_PI_EXCLUDE_PACKAGES"

const piExcludePackageFlag = "--pi-exclude-package"

// resolvePiExcludePackages applies non-empty flag > non-empty env. An empty or
// separator-only env value counts as unset.
func resolvePiExcludePackages(flagValues []string, envValue string, envSet bool) []string {
	if len(flagValues) > 0 {
		return flagValues
	}
	if !envSet {
		return nil
	}
	var values []string
	if err := (csvListFlag{values: &values}).Set(envValue); err != nil {
		return nil
	}
	return values
}

// validatePiExcludePackages refuses unknown or required sources and an
// exclusion without the pi agent, before any install write.
func validatePiExcludePackages(flagValues []string, agents []model.AgentID) ([]string, error) {
	envValue, envSet := os.LookupEnv(PiExcludePackagesEnv)
	excluded := resolvePiExcludePackages(flagValues, envValue, envSet)
	if len(excluded) == 0 {
		return nil, nil
	}
	source, remedy := piExcludePackageFlag, "remove "+piExcludePackageFlag
	if len(flagValues) == 0 {
		source, remedy = PiExcludePackagesEnv, "unset "+PiExcludePackagesEnv
	}
	if !containsAgent(agents, model.AgentPi) {
		return nil, fmt.Errorf("%s requires the pi agent in the resolved install plan; rerun gentle-ai install with pi added to your existing agent list, or %s", source, remedy)
	}
	if err := piagent.ValidateExcludedPackageSources(excluded); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	return excluded, nil
}
