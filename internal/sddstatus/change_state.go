package sddstatus

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// changeState is the subset of openspec/changes/<change>/state.yaml that
// status projects. The orchestrator owns that file as the change's DAG state
// (openspec-convention.md); status only reads it and never requires it.
type changeState struct {
	DependsOn []string `yaml:"dependsOn"`
}

// openSpecStateDependsOn projects the declared dependsOn list of a file-backed
// change (#3311). A missing file, a missing key, or an unreadable file keeps
// the empty list: the convention never makes state.yaml required, so it can
// never block status.
func openSpecStateDependsOn(store ArtifactStore, changeRoot *string) []string {
	if changeRoot == nil || (store != ArtifactStoreOpenSpec && store != ArtifactStoreHybrid) {
		return []string{}
	}
	data, err := os.ReadFile(filepath.Join(*changeRoot, "state.yaml"))
	if err != nil {
		return []string{}
	}
	var state changeState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return []string{}
	}
	dependsOn := make([]string, 0, len(state.DependsOn))
	for _, name := range state.DependsOn {
		if name = strings.TrimSpace(name); name != "" {
			dependsOn = append(dependsOn, name)
		}
	}
	return dependsOn
}
