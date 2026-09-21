package cli

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
)

// gateArtifactDigest resolves the digest a recorded GateRecord must carry
// for it to be judged consistently by resolveGateStatus (machine.go, D-08)
// against the SAME digest internal/sddstatus's own loadGovernance
// recomputes at every status read. The two computations must resolve the
// exact same file set for the exact same gate key, or a genuine rejection
// with no remediation would spuriously "reopen" to pending the moment
// status is re-read (the defect this function exists to close) — see
// sdd_gate_artifact_digest_test.go's own RED reproduction.
//
// It mirrors, file for file, the resolution
// internal/sddstatus/status.go's resolveArtifactPaths/findSpecFiles and
// internal/sddstatus/governance.go's governanceArtifactInputs already
// apply: design.md and tasks.md as single files, specs/**/spec.md
// (sorted, alphabetically) with a flat spec.md fallback for the "spec"
// gate, and a role-apply:<rol> gate's own sealed TasksFile. The
// "integration" gate is deliberately excluded: machine.go's EvaluateGates
// never judges it against any digest (it always passes "" for that gate),
// so this function must not invent one either — doing so would create the
// exact same kind of mismatch this fix is closing.
//
// An artifact that does not exist yet, or exists but is empty, resolves to
// "" (no digest) — the same "not done yet" contract
// kickoff.Inputs.Artifacts documents, and exactly what sddstatus's own
// setGovernanceArtifactDigest leaves absent from its map for the same
// reason. It is never an error for gate record to run against an artifact
// that has not been produced yet: the four fixed gates never required a
// sealed kickoff.yaml or an existing artifact before this fix, and that
// contract does not change here.
func gateArtifactDigest(changeRoot string, gate kickoff.GateKey, sealed *kickoff.Kickoff) (string, error) {
	switch gate {
	case kickoff.GateSpec:
		paths, err := specArtifactPathsForDigest(changeRoot)
		if err != nil {
			return "", err
		}
		return digestIfAllPresent(paths)
	case kickoff.GateDesign:
		return digestIfAllPresent([]string{filepath.Join(changeRoot, "design.md")})
	case kickoff.GateTasks:
		return digestIfAllPresent([]string{filepath.Join(changeRoot, "tasks.md")})
	case kickoff.GateIntegration:
		return "", nil
	default:
		return roleApplyArtifactDigestForRecord(changeRoot, gate, sealed)
	}
}

// roleApplyArtifactDigestForRecord resolves a role-apply:<rol> gate's
// digest against that role's own sealed TasksFile — the same artifact key
// ("tasks.<rol>") sddstatus.governanceArtifactInputs digests for the exact
// same gate. A gate that does not belong to the sealed roster (or a call
// with no sealed kickoff at all) resolves to "": the caller already
// refuses that combination before ever reaching kickoff.AppendGate.
func roleApplyArtifactDigestForRecord(changeRoot string, gate kickoff.GateKey, sealed *kickoff.Kickoff) (string, error) {
	if sealed == nil {
		return "", nil
	}
	for _, role := range sealed.Config.Roles {
		if kickoff.RoleApplyGate(role.Role) == gate {
			return digestIfAllPresent([]string{filepath.Join(changeRoot, role.TasksFile)})
		}
	}
	return "", nil
}

// digestIfAllPresent computes kickoff.ArtifactDigest over paths only when
// every one of them exists and is non-empty; otherwise it reports "" (no
// digest), matching sddstatus's own setGovernanceArtifactDigest contract
// exactly, so an artifact that has not reached "done" yet resolves to the
// same empty digest on both the write and the read side.
func digestIfAllPresent(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}
	for _, path := range paths {
		if !fileHasContent(path) {
			return "", nil
		}
	}
	return kickoff.ArtifactDigest(paths)
}

// specArtifactPathsForDigest mirrors internal/sddstatus/status.go's
// resolveArtifactPaths + findSpecFiles exactly: every specs/**/spec.md
// file under changeRoot, sorted, or the single flat spec.md when no
// per-capability file exists.
func specArtifactPathsForDigest(changeRoot string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(filepath.Join(changeRoot, "specs"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "spec.md" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		files = nil
	}
	sort.Strings(files)
	if len(files) == 0 {
		flat := filepath.Join(changeRoot, "spec.md")
		if fileHasContent(flat) {
			files = []string{flat}
		}
	}
	return files, nil
}
