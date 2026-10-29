//go:build linux || darwin

package shellinstaller

import (
	"context"
	"path/filepath"
	"strings"
)

// userMainSDKAction is the one boundary through which Go drives the owned
// provisioner's Main actions. Tests substitute it to observe the exact action
// sequence and its filesystem effects without executing Node.
var userMainSDKAction = userVerifyMainBindingsAction

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
	if err := userMaterializeMainSDK(ctx, root, finalRoot, prefix, agent, artifact); err != nil {
		return err
	}
	if err := userMainSDKAction(ctx, root, finalRoot, prefix, agent, artifact, "select-main-sdk-package"); err != nil {
		return err
	}
	return userMainSDKAction(ctx, root, finalRoot, prefix, agent, artifact, userMainSDKSelectedAction)
}

// userMainSDKSelectedAction verifies a runtime-materialized composition whose
// agent declares the composed package. It is the only verification that holds
// for an installed Main tree: the non-runtime verify-main-sdk-composition
// treats the materialized stock native member as extra.
const userMainSDKSelectedAction = "verify-main-sdk-selected"

// userMaterializeMainSDK writes the composed Main SDK under root: the source
// composition, then the pinned stock native member and the runtime/node_modules
// dependency link, each with its exact provisioner readback. The fresh install
// and the Unix channel overlay share it; only the selection step differs
// (the provisioner selects an unpublished stage, the overlay flips the
// published settings itself), and both verify with userMainSDKSelectedAction.
func userMaterializeMainSDK(ctx context.Context, root, finalRoot, prefix, agent string, artifact userSnapshotArtifact) error {
	for _, action := range []string{"materialize-main-sdk-source", "materialize-main-sdk-runtime"} {
		if err := userMainSDKAction(ctx, root, finalRoot, prefix, agent, artifact, action); err != nil {
			return err
		}
	}
	return nil
}

// Historical nil metadata keeps the exact ordinary stock verifier. A selected
// SDK never bypasses the supplier/graph checks or repairs settings drift. Its
// manifest shape is shared with the reuse check, which applies it without
// driving the provisioner.
func userVerifyInstalledGlobal(ctx context.Context, root string, manifest userManifest) error {
	if manifest.MainSDK == nil {
		return userVerifyGlobal(ctx, root, manifest.Prefix, manifest.Agent, manifest.Prefix, root, manifest.Mode)
	}
	if err := userMainBindingShape(root, manifest); err != nil {
		return err
	}
	return userMainSDKAction(ctx, root, root, manifest.Prefix, manifest.Agent, *manifest.MainSDK, userMainSDKSelectedAction)
}

// userMainBindingShape holds the manifest shape every Main SDK verification
// requires, so a reuse check can apply it without driving the provisioner.
func userMainBindingShape(root string, manifest userManifest) error {
	if !userMainPlatform() || (manifest.Mode != "separate" && manifest.Mode != "agent") || manifest.Destination != root || manifest.Prefix != filepath.Join(root, "prefix") {
		return experienceError("Main SDK manifest requires qualified Linux amd64 or macOS arm64 bindings for On my existing Pi or Separate")
	}
	if manifest.Mode == "separate" && manifest.Agent != filepath.Join(root, "agent") {
		return experienceError("Main SDK manifest requires qualified Linux amd64 or macOS arm64 bindings for On my existing Pi or Separate")
	}
	if manifest.Mode == "agent" && (manifest.Agent == "" || !privateHierarchyPath(manifest.Agent) || strings.HasPrefix(manifest.Agent, root+"/")) {
		return experienceError("Main SDK manifest requires a qualified external agent")
	}
	return nil
}
