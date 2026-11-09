package shellinstaller

import (
	"errors"
	"fmt"
)

// windowsQualifiedBuildFloor is the oldest Windows build the Separate installer
// qualifies: Windows 10 22H2 (build 19045) and every Windows 11 build (22000+)
// share the installer's full API surface (Job Objects with CPU rate control,
// IsWow64Process2, NTFS ACL walks via Get/SetNamedSecurityInfo); the floor was
// verified on a real Windows 10 22H2 host (issue #5927). Server (ProductType 2+)
// and ARM64 stay unqualified.
const windowsQualifiedBuildFloor = 19045

// windowsKernelQualificationError is the pure kernel-qualification predicate
// behind UserKernelCheck. It names the exact condition that failed, including
// the observed build, instead of folding every requirement into one message.
// The values come from the running host (GOARCH, IsWow64Process2 native
// machine, RtlGetVersion); the function itself is pure so the table test runs
// on every platform without skipping on the host.
func windowsKernelQualificationError(goarch string, nativeMachine uint16, majorVersion, buildNumber uint32, productType uint8) error {
	if goarch != "amd64" {
		return fmt.Errorf("this gentle-ai executable is %s, not amd64; Gentle Shell Separate is qualified on x64 only", goarch)
	}
	if nativeMachine != 0x8664 {
		return errors.New("the Windows host is not x64; ARM64 is not a qualified target")
	}
	if majorVersion != 10 {
		return fmt.Errorf("the Windows host reports major version %d; Gentle Shell Separate qualifies Windows 10 22H2 (build 19045) and Windows 11", majorVersion)
	}
	if buildNumber < windowsQualifiedBuildFloor {
		return fmt.Errorf("Windows build %d is below the qualified floor %d; use Windows 10 22H2 or Windows 11", buildNumber, windowsQualifiedBuildFloor)
	}
	if productType != 1 {
		return errors.New("Windows Server is not a qualified target; Gentle Shell Separate qualifies workstation editions")
	}
	return nil
}
