//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Prepared archives are acquired by the capped coordinator before production.
// Missing or unrequested library DATA never turns a valid DTO into admission.
func userPublishNativePNPMPrepared(ctx context.Context, source, stage, destination string, location NativeInstallationLocation, profile *UserExperience, archives [][]byte, modeAgent ...string) (result UserInstallResult, err error) {
	mode, agent := "separate", ""
	if len(modeAgent) == 2 {
		mode, agent = modeAgent[0], modeAgent[1]
	}
	if mode == "agent" && (agent == "" || !privateHierarchyPath(agent)) {
		return result, privateError("refused", errors.New("agent reuse requires one absolute existing agent"))
	}
	if err = userValidateStageExperience(profile); err != nil {
		return result, err
	}
	codegraph := profile != nil && profile.CodeGraph == "on"
	if codegraph && len(archives) != len(userCodeGraphSources) || !codegraph && len(archives) != 0 {
		return result, privateError("source", errors.New("prepared CodeGraph source set differs"))
	}
	self, err := userExecutable()
	if err != nil {
		return result, err
	}
	staged, err := StageNativePNPMInstallation(ctx, source, stage, destination, location)
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			// Rename consumes stage. Never remove a published destination, even
			// when its final readback or parent-directory synchronization fails.
			err = errors.Join(err, os.RemoveAll(stage))
		}
	}()
	if err = userStageExperience(ctx, stage, profile); err != nil {
		return result, err
	}
	if codegraph {
		if err = os.Mkdir(filepath.Join(stage, "runtime"), 0700); err != nil {
			return result, err
		}
		if err = userStageCodeGraphLibrary(ctx, stage, archives); err != nil {
			return result, err
		}
		if err = userStageCodeGraphExecutableWithNode(ctx, stage, destination, staged.Installation.Node); err != nil {
			return result, err
		}
	}
	supervisorSHA, err := userCopySupervisor(ctx, self, filepath.Join(stage, "supervisor"))
	if err != nil {
		return result, err
	}
	if err := os.Mkdir(filepath.Join(stage, "bin"), 0700); err != nil {
		return result, err
	}
	prefix, agent := filepath.Join(stage, "pnpm"), filepath.Join(stage, "agent")
	if mode == "agent" && profile != nil {
		if err = userStageOwnedAgentContext7(ctx, agent, profile.Context7, stage); err != nil {
			return result, privateError("refused", err)
		}
	}
	manifest := userManifest{
		Schema: userSchema, Destination: destination, Mode: mode,
		Prefix: filepath.Join(destination, "pnpm"), Agent: filepath.Join(destination, "agent"), SupervisorSHA: supervisorSHA, CodeGraph: codegraph,
		PNPM: &userPNPMManifest{PackageRoot: strings.TrimSuffix(staged.Installation.ShellCLI, "/bin/gentle-shell.mjs"), ShellVersion: location.ShellVersion, PeerVersion: location.PiVersion, CodeSeal: staged.CodeSeal},
	}
	if mode == "agent" {
		manifest.Agent = agent
		manifest.AgentIdentity, err = userOwnedIdentity(agent)
	} else {
		manifest.AgentIdentity, err = userIdentity(agent)
	}
	manifest.PrefixIdentity = ""
	if err == nil {
		manifest.PrefixIdentity, err = userIdentity(prefix)
	}
	if err != nil {
		return result, err
	}
	return userPublishManifest(ctx, stage, prefix, agent, manifest)
}
