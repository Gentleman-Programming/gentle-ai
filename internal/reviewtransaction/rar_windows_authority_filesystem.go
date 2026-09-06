package reviewtransaction

import "fmt"

// windowsDriveTypeFixed mirrors Win32 GetDriveType's DRIVE_FIXED (3). Defined
// locally (not imported from golang.org/x/sys/windows) so this file, and its
// table test, build on every GOOS.
const windowsDriveTypeFixed uint32 = 3

// rarWindowsACLCapableFilesystems are the filesystem names
// GetVolumeInformationW reports for a volume that can host owner-only,
// ACL-protected RAR authority state.
var rarWindowsACLCapableFilesystems = map[string]struct{}{"NTFS": {}, "ReFS": {}}

// rarWindowsUnsupportedFilesystems are filesystem names known NOT to support
// the persistent Windows ownership semantics RAR authority requires.
var rarWindowsUnsupportedFilesystems = map[string]struct{}{"exFAT": {}, "FAT32": {}}

// windowsAuthorityFilesystemClass is the outcome of classifying one resolved
// Windows volume for RAR authority hosting.
type windowsAuthorityFilesystemClass int

const (
	// windowsAuthorityFilesystemUnknown is the fail-closed default.
	windowsAuthorityFilesystemUnknown windowsAuthorityFilesystemClass = iota
	// windowsAuthorityFilesystemSupported is a local fixed NTFS/ReFS volume.
	windowsAuthorityFilesystemSupported
	// windowsAuthorityFilesystemUnsupported is a local fixed volume on
	// exFAT/FAT32, which lack persistent Windows ownership semantics.
	windowsAuthorityFilesystemUnsupported
)

// classifyWindowsAuthorityFilesystem is the pure decision behind the Windows
// RAR authority filesystem gate (#4048). The Windows-only probe resolves the
// volume root of the path under validation, calls GetDriveType and
// GetVolumeInformationW against that root, and hands the three observed facts
// here so the decision is a pure function, table-tested on every platform.
//
// probeErr is kept separate from an empty/unrecognized fstype: a failed probe
// is unknown-but-named, carrying the real Win32 error, never guessed to be
// "remote" the way returning "" on every failure used to collapse it. A drive
// type other than DRIVE_FIXED is also unknown: #4048's defect was calling
// GetVolumeInformationW on an arbitrary directory instead of the resolved
// volume root, which produced "" for a genuinely local fixed NTFS volume;
// requiring DRIVE_FIXED tells a local volume apart from a same-named remote
// share without trusting the caller's claim about where the path lives.
func classifyWindowsAuthorityFilesystem(fstype string, driveType uint32, probeErr error) (windowsAuthorityFilesystemClass, string) {
	if probeErr != nil {
		return windowsAuthorityFilesystemUnknown, fmt.Sprintf("the volume information probe failed: %v", probeErr)
	}
	if driveType != windowsDriveTypeFixed {
		return windowsAuthorityFilesystemUnknown, fmt.Sprintf("drive type %d is not a local fixed drive", driveType)
	}
	if _, ok := rarWindowsACLCapableFilesystems[fstype]; ok {
		return windowsAuthorityFilesystemSupported, fstype
	}
	if _, ok := rarWindowsUnsupportedFilesystems[fstype]; ok {
		return windowsAuthorityFilesystemUnsupported, fstype
	}
	return windowsAuthorityFilesystemUnknown, fmt.Sprintf("unrecognized filesystem %q", fstype)
}
