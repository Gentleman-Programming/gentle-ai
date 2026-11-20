//go:build darwin

package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func init() { userPlatformPreflight = userDarwinPreflight }

// userDarwinPreflight moves the darwin refusals that used to surface only
// after private staging or the downloads into inspection, so the operator sees
// them before confirming. Both checks stay authoritative at action time too:
// installation repeats the supervisor provenance check when it copies the
// supervisor (a fresh install only), and every write still requires F_FULLFSYNC and RENAME_EXCL.
// Verifying an existing installation stages nothing and skips both; a
// reconfiguration skips the supervisor check and keeps the volume probe.
func userDarwinPreflight(req UserInstallRequest) error {
	_, statErr := os.Lstat(req.Destination)
	exists := statErr == nil
	if exists && userReconfigureTarget() != req.Destination {
		return nil
	}
	// Reconfiguring an existing install (including the SDK channel overlay)
	// never copies the supervisor, so provenance is a fresh-install gate only.
	if !exists {
		self, err := userExecutable()
		if err != nil {
			return privateError("source", err)
		}
		// The same policy as action time (#5814): no exception for Homebrew; the
		// refusal names a remediation that leaves Homebrew permissions alone.
		if _, err := userSupervisorSHA(context.Background(), self); err != nil {
			return fmt.Errorf("supervisor provenance: %w", err)
		}
	}
	return userVolumePreflight(filepath.Dir(req.Destination))
}

// userVolumePreflight proves on a probe file inside parent that its volume
// supports F_FULLFSYNC and RENAME_EXCL, which every installer write and the
// stage publication need. exFAT, SMB and NFS volumes often lack one of them;
// without this probe they refused only after the downloads. The probe files
// are removed on every path.
func userVolumePreflight(parent string) (err error) {
	probe, err := os.CreateTemp(parent, ".gentle-volume-probe.*")
	if err != nil {
		return privateError("filesystem", err)
	}
	name, renamed := probe.Name(), probe.Name()+".excl"
	created := []string{name}
	defer func() {
		// Only entries this probe created are removed: a failed no-replace
		// rename never claims an existing renamed path.
		for _, path := range created {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
				err = errors.Join(err, privateError("filesystem", removeErr))
			}
		}
	}()
	syncErr := userFullSync(probe.Fd())
	if closeErr := probe.Close(); closeErr != nil && syncErr == nil {
		return privateError("filesystem", closeErr)
	}
	if syncErr != nil {
		return userVolumeRefusal(parent, "F_FULLFSYNC", syncErr)
	}
	if err := userRenameNoReplace(name, renamed); err != nil {
		return userVolumeRefusal(parent, "RENAME_EXCL", err)
	}
	created = []string{renamed}
	return nil
}

func userVolumeRefusal(parent, capability string, cause error) error {
	// refusal:by-design world-action: the operator must choose a target on a volume with durable flush and no-replace rename
	return privateError("refused", fmt.Errorf("destination volume refused: the filesystem holding %q does not support %s, which the installer needs for durable, no-replace publication (exFAT, SMB and NFS volumes often lack it); choose a target parent on an APFS or HFS+ volume: %w", parent, capability, cause))
}
