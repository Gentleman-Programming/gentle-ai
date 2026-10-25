//go:build linux || darwin

package shellinstaller

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
)

// The real worker owns this action; qualification aliases remain in tests.
func userVerifyMainBindingsAction(ctx context.Context, root, finalRoot, prefix, agent string, artifact userSnapshotArtifact, action string) error {
	if err := userCompareMainSource(ctx, root, artifact); err != nil {
		return err
	}
	mode := "separate"
	if agent != filepath.Join(root, "agent") {
		mode = "agent"
	}
	if err := userVerifyGlobalAction(ctx, root, prefix, agent,
		filepath.Join(finalRoot, "prefix"), finalRoot, mode, action); err != nil {
		return err
	}
	return userCompareMainSource(ctx, root, artifact)
}

// The confirmed worker owns acquisition and partial-stage cleanup. Stock stays
// installed: only the private SDK declaration changes after authenticated DATA.
func userInstallMainSDK(ctx context.Context, root, finalRoot, prefix, agent string, profile *UserExperience) (*userSnapshotArtifact, error) {
	if profile == nil || profile.SDKSource != "main-preview" {
		return nil, nil
	}
	artifact, data, err := userMainSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
		return nil, err
	}
	if err := userStageMainSource(ctx, root, artifact); err != nil {
		return nil, err
	}
	if err := userComposeMainSDK(ctx, root, finalRoot, prefix, agent, artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func userComposeMainSDK(ctx context.Context, root, finalRoot, prefix, agent string, artifact userSnapshotArtifact) error {
	for _, action := range []string{"materialize-main-sdk-source", "materialize-main-sdk-runtime", "select-main-sdk-package"} {
		if err := userVerifyMainBindingsAction(ctx, root, finalRoot, prefix, agent, artifact, action); err != nil {
			return err
		}
	}
	return userVerifyMainBindingsAction(ctx, root, finalRoot, prefix, agent, artifact, "verify-main-sdk-selected")
}

// Historical nil metadata keeps the exact ordinary stock verifier. A selected
// SDK never bypasses the supplier/graph checks or repairs settings drift.
func userVerifyInstalledGlobal(ctx context.Context, root string, manifest userManifest) error {
	if manifest.MainSDK == nil {
		return userVerifyGlobal(ctx, root, manifest.Prefix, manifest.Agent, manifest.Prefix, root, manifest.Mode)
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" || (manifest.Mode != "separate" && manifest.Mode != "agent") || manifest.Destination != root || manifest.Prefix != filepath.Join(root, "prefix") {
		return experienceError("Main SDK manifest requires qualified Linux amd64 bindings for On my existing Pi or Separate")
	}
	if manifest.Mode == "separate" && manifest.Agent != filepath.Join(root, "agent") {
		return experienceError("Main SDK manifest requires qualified Linux amd64 bindings for On my existing Pi or Separate")
	}
	if manifest.Mode == "agent" && (manifest.Agent == "" || !privateHierarchyPath(manifest.Agent) || strings.HasPrefix(manifest.Agent, root+"/")) {
		return experienceError("Main SDK manifest requires a qualified external agent")
	}
	return userVerifyMainBindingsAction(ctx, root, root, manifest.Prefix, manifest.Agent, *manifest.MainSDK, "verify-main-sdk-selected")
}
