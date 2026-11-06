package reviewtransaction

import (
	"errors"
	"strings"
	"testing"
)

// TestClassifyWindowsAuthorityFilesystem is the cross-platform table test for
// the pure decision behind #4048: a Git common directory on a healthy local
// fixed NTFS volume was classified as "unknown or remote" because the
// production probe called GetVolumeInformationW on the directory itself
// (never resolved to its volume root) and read the volume-label buffer as if
// it were the filesystem name. This test exercises only the decision, not the
// Win32 probe, so it runs on every platform.
func TestClassifyWindowsAuthorityFilesystem(t *testing.T) {
	const driveRemote = 4
	const driveRemovable = 2
	probeFailure := errors.New("access is denied")

	tests := []struct {
		name          string
		fstype        string
		driveType     uint32
		probeErr      error
		wantClass     windowsAuthorityFilesystemClass
		wantReasonHas string
	}{
		{name: "local fixed NTFS is supported", fstype: "NTFS", driveType: windowsDriveTypeFixed, wantClass: windowsAuthorityFilesystemSupported},
		{name: "local fixed ReFS is supported", fstype: "ReFS", driveType: windowsDriveTypeFixed, wantClass: windowsAuthorityFilesystemSupported},
		{name: "local fixed exFAT is explicitly unsupported", fstype: "exFAT", driveType: windowsDriveTypeFixed, wantClass: windowsAuthorityFilesystemUnsupported},
		{name: "local fixed FAT32 is explicitly unsupported", fstype: "FAT32", driveType: windowsDriveTypeFixed, wantClass: windowsAuthorityFilesystemUnsupported},
		{name: "local fixed but unrecognized filesystem is unknown", fstype: "FOOFS", driveType: windowsDriveTypeFixed, wantClass: windowsAuthorityFilesystemUnknown, wantReasonHas: `unrecognized filesystem "FOOFS"`},
		{name: "empty filesystem name is unknown", fstype: "", driveType: windowsDriveTypeFixed, wantClass: windowsAuthorityFilesystemUnknown, wantReasonHas: "unrecognized filesystem"},
		{name: "remote drive is unknown even with an NTFS-shaped name", fstype: "NTFS", driveType: driveRemote, wantClass: windowsAuthorityFilesystemUnknown, wantReasonHas: "drive type 4"},
		{name: "removable drive is unknown", fstype: "FAT32", driveType: driveRemovable, wantClass: windowsAuthorityFilesystemUnknown, wantReasonHas: "drive type 2"},
		{
			name: "a failed probe is unknown-but-named, never reported as remote",
			// #4048: returning "" on every GetVolumeInformationW failure made a
			// probe error indistinguishable from an unrecognized filesystem, and
			// both were folded into "unknown or remote". probeErr keeps the two
			// apart and the reason must carry the real cause, never the word
			// "remote".
			fstype: "", driveType: windowsDriveTypeFixed, probeErr: probeFailure,
			wantClass: windowsAuthorityFilesystemUnknown, wantReasonHas: "access is denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, reason := classifyWindowsAuthorityFilesystem(tt.fstype, tt.driveType, tt.probeErr)
			if class != tt.wantClass {
				t.Fatalf("classifyWindowsAuthorityFilesystem(%q, %d, %v) class = %v, want %v", tt.fstype, tt.driveType, tt.probeErr, class, tt.wantClass)
			}
			if tt.wantReasonHas != "" && !strings.Contains(reason, tt.wantReasonHas) {
				t.Fatalf("reason = %q, want it to contain %q", reason, tt.wantReasonHas)
			}
			if strings.Contains(strings.ToLower(reason), "remote") {
				t.Fatalf("reason %q must never claim the volume is remote: only a resolved DRIVE_FIXED root is ever inspected", reason)
			}
		})
	}
}
