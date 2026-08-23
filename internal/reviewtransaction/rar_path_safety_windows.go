//go:build windows

package reviewtransaction

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func rarPathUnsafe(path string, info fs.FileInfo) bool {
	if path == "" || info == nil || info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attributes, err := windows.GetFileAttributes(name)
	return err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

// rarPrivateDirectoryCreate and rarPrivateDirectoryRepair mirror the POSIX
// seam: the only two primitives that establish an owner-only RAR directory,
// kept as variables so tests can reproduce a filesystem or token that ignores
// the descriptor it is handed.
var (
	rarPrivateDirectoryCreate = windows.CreateDirectory
	rarPrivateDirectoryRepair = repairPrivateRARDirectoryNoFollow
)

// FILE_LIST_DIRECTORY is requested because the repair below is allowed only on
// an empty directory, and a handle that cannot list one cannot establish that.
const rarDirectoryRepairAccess = windows.SYNCHRONIZE | windows.READ_CONTROL |
	windows.FILE_READ_ATTRIBUTES | windows.FILE_LIST_DIRECTORY |
	windows.WRITE_DAC | windows.WRITE_OWNER

// repairPrivateRARDirectoryNoFollow reapplies the owner-only protected DACL
// through a handle, never a name: SetNamedSecurityInfo resolves the name it is
// handed and has no no-follow mode, so a directory swapped for a junction
// between CreateDirectory and validation would take the repair on the
// attacker's target. The handle comes from the validator's own OBJ_DONT_REPARSE
// open, is refused when it is a reparse point, and carries both the write and
// the owner/DACL readback that clears it.
func repairPrivateRARDirectoryNoFollow(path string, _ fs.FileMode) error {
	descriptor, err := ownerOnlyRARSecurityDescriptor(true)
	if err != nil {
		return err
	}
	owner, _, ownerErr := descriptor.Owner()
	dacl, _, daclErr := descriptor.DACL()
	if ownerErr != nil || daclErr != nil || owner == nil || dacl == nil {
		return rarWindowsRepairRefusal(path,
			"the owner-only descriptor to apply is missing its owner or its DACL")
	}
	handle, err := openWindowsRARObject(ntPath(path), rarDirectoryRepairAccess, true)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), path)
	defer file.Close()
	if openWindowsRARFileUnsafe(file) {
		return rarWindowsRepairRefusal(path, "the repair handle resolved to a reparse point")
	}
	// What the object IS decides, never who created it. FILE_DIRECTORY_FILE and
	// OBJ_DONT_REPARSE already refused a non-directory and a junction, and this
	// same handle answers the owner before the write: only a principal this
	// process controls -- one this token could itself have stamped as an owner
	// -- repairs. See rarWindowsRepairOwnerControlled for why that is the
	// question, and why the narrower token-user comparison this replaced
	// refused directories the process had just created.
	existing, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	controlled, err := rarWindowsDescriptorOwnerControlled(existing)
	if err != nil {
		return err
	}
	if !controlled {
		return rarWindowsRepairRefusal(path,
			"the directory is owned by a principal this process does not control")
	}
	// Only an empty directory is repaired, read from this same handle. An
	// interrupted creation leaves nothing behind, so recovery works; a
	// populated authority directory whose ACL somebody widened stays refused,
	// because silently re-tightening it would hide the exposure.
	names, readErr := file.Readdirnames(-1)
	if readErr != nil {
		return rarWindowsRepairRefusal(path,
			fmt.Sprintf("the repair handle cannot list the directory (%v)", readErr))
	}
	if len(names) > 0 {
		return rarWindowsRepairRefusal(path,
			"the directory already holds state, so its widened ACL is not silently re-tightened")
	}
	err = windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|
			windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, dacl, nil)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return fmt.Errorf("apply the owner-only RAR directory descriptor: %w", err)
	}
	repaired, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read back the repaired RAR directory descriptor: %w", err)
	}
	if mismatch := privateRARSecurityDescriptorMismatch(repaired, true); mismatch != "" {
		return rarWindowsRepairRefusal(path,
			"the descriptor the repair wrote is still not owner-only: "+mismatch)
	}
	return nil
}

// rarWindowsRepairRefusal names which invariant refused.
//
// #3376 shipped a repair whose distinct refusals all produced one identical
// message, and a caller that then discarded the repair's error entirely, so
// the only thing a failure could say was that the path was unsafe. That made
// the Windows-only CI failure it produced impossible to attribute to a branch
// from its output alone, on a platform this repository cannot execute locally.
// The operator-facing cause and the typed *UnsafeRARPathError are both
// preserved; only the attribution is added.
func rarWindowsRepairRefusal(path, reason string) error {
	return fmt.Errorf("%s: %w", reason, unsafeRARPathError(path, true))
}

// rarWindowsDescriptorOwnerControlled answers the repair's ownership question
// for a descriptor read from the object itself. It is the only place the token
// is consulted; the rule applied to what it finds is
// rarWindowsRepairOwnerControlled, which is pure and executed by tests on
// every platform.
func rarWindowsDescriptorOwnerControlled(
	descriptor *windows.SECURITY_DESCRIPTOR,
) (bool, error) {
	if descriptor == nil || !descriptor.IsValid() {
		return false, nil
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return false, fmt.Errorf("read the RAR directory owner: %w", err)
	}
	if owner == nil || !owner.IsValid() {
		return false, nil
	}
	token, err := currentRARWindowsTokenPrincipals()
	if err != nil {
		return false, err
	}
	return rarWindowsRepairOwnerControlled(owner.String(), token), nil
}

// currentRARWindowsTokenPrincipals gathers every SID the current process could
// itself stamp as an owner.
func currentRARWindowsTokenPrincipals() (rarWindowsTokenPrincipals, error) {
	user, err := currentRARWindowsUserSID()
	if err != nil {
		return rarWindowsTokenPrincipals{}, err
	}
	defaultOwner, err := currentRARWindowsTokenOwnerSID()
	if err != nil {
		return rarWindowsTokenPrincipals{}, err
	}
	eligible, err := currentRARWindowsOwnerEligibleSIDs()
	if err != nil {
		return rarWindowsTokenPrincipals{}, err
	}
	return rarWindowsTokenPrincipals{
		User:          user.String(),
		DefaultOwner:  defaultOwner.String(),
		OwnerEligible: eligible,
	}, nil
}

// currentRARWindowsOwnerEligibleSIDs returns the token's SE_GROUP_OWNER
// groups: the exact set Windows will accept from this token as an owner SID.
// Plain membership is deliberately not enough -- a UAC-filtered administrator
// carries BUILTIN\Administrators as SE_GROUP_USE_FOR_DENY_ONLY and genuinely
// cannot take ownership, so it must not clear this bar.
func currentRARWindowsOwnerEligibleSIDs() ([]string, error) {
	groups, err := windows.GetCurrentProcessToken().GetTokenGroups()
	if err != nil {
		return nil, fmt.Errorf("resolve current Windows token groups: %w", err)
	}
	var eligible []string
	for _, group := range groups.AllGroups() {
		if group.Attributes&windows.SE_GROUP_OWNER == 0 ||
			group.Sid == nil || !group.Sid.IsValid() {
			continue
		}
		if sid := group.Sid.String(); sid != "" {
			eligible = append(eligible, sid)
		}
	}
	runtime.KeepAlive(groups)
	return eligible, nil
}

func createPrivateRARDirectory(path string) (bool, error) {
	descriptor, err := ownerOnlyRARSecurityDescriptor(true)
	if err != nil {
		return false, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attributes := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	err = rarPrivateDirectoryCreate(name, &attributes)
	runtime.KeepAlive(descriptor)
	created := err == nil
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return false, err
	}
	validateErr := validatePrivateRARDirectory(path)
	if validateErr != nil {
		// Same shape as the POSIX path: a directory at this path can
		// still fail its own owner-only validation when the filesystem or the
		// process token did not honour the descriptor handed to
		// CreateDirectory. Apply the owner-only, protected DACL explicitly and
		// revalidate before giving up. This is precisely the repair the CLI
		// prints, so anything it cannot fix here the operator still cannot fix
		// there, and the refusal below stays truthful.
		//
		// Do NOT gate this on `created`: CreateDirectory answers every later
		// attempt with ERROR_ALREADY_EXISTS, so that gate makes recovery from
		// an interrupted run once-only. The handle-verified object decides
		// instead: an empty directory this process's own principals own.
		if repairErr := rarPrivateDirectoryRepair(path, 0o700); repairErr == nil {
			validateErr = validatePrivateRARDirectory(path)
		} else {
			// #3376 discarded this. The refusal below then carried the same
			// generic unsafe-path message whichever invariant had actually
			// refused, which is exactly why the Windows-only failure it
			// shipped could not be attributed to a branch from CI output on a
			// platform this repository cannot execute locally. The typed
			// *UnsafeRARPathError of validateErr stays first in the chain, so
			// errors.As and errors.Is are unchanged.
			validateErr = fmt.Errorf(
				"%w (the automatic repair could not run: %v)",
				validateErr, repairErr,
			)
		}
	}
	if validateErr != nil {
		// The just-created directory deliberately stays on disk. Removing it
		// made the refusal name a path that no longer existed, so the repair
		// this product prints could not run at all and the operator had no way
		// out. Nothing is trusted by leaving it: every reader revalidates, and
		// the next attempt takes the already-exists branch and refuses again.
		return false, validateErr
	}
	return created, nil
}

func createPrivateRARFile(path string) (*os.File, error) {
	descriptor, err := ownerOnlyRARSecurityDescriptor(false)
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	attributes := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		&attributes,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	opened, statErr := file.Stat()
	current, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !opened.Mode().IsRegular() ||
		!os.SameFile(opened, current) || rarPathUnsafe(path, current) ||
		!privateOpenRARPathSafe(file, opened) {
		_ = file.Close()
		_ = os.Remove(path)
		if statErr != nil {
			return nil, statErr
		}
		if pathErr != nil {
			return nil, pathErr
		}
		return nil, errUnsafeRARAuthorityPath
	}
	return file, nil
}

func privateRARPathSafe(path string, info fs.FileInfo) bool {
	if path == "" || info == nil || rarPathUnsafe(path, info) {
		return false
	}
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	return err == nil && privateRARSecurityDescriptorSafe(descriptor, info.IsDir())
}

func privateOpenRARPathSafe(file *os.File, info fs.FileInfo) bool {
	if file == nil || info == nil || openWindowsRARFileUnsafe(file) {
		return false
	}
	descriptor, err := windows.GetSecurityInfo(
		windows.Handle(file.Fd()),
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	return err == nil && privateRARSecurityDescriptorSafe(descriptor, info.IsDir())
}

func rarRepositoryDirectorySafe(path string, info fs.FileInfo) bool {
	if path == "" || info == nil || !info.IsDir() || rarPathUnsafe(path, info) {
		return false
	}
	return rarWindowsAuthorityOwnerUnsafe(path, info)
}

var rarWindowsAuthorityOwnerUnsafe func(path string, info fs.FileInfo) bool = windowsRARAuthorityOwnerUnsafeDefault

func windowsRARAuthorityOwnerUnsafeDefault(path string, info fs.FileInfo) bool {
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		return false
	}
	return rarSharedSecurityDescriptorOwnedByCurrentProcess(descriptor)
}

func rarRepositoryOpenDirectorySafe(file *os.File, info fs.FileInfo) bool {
	if file == nil || info == nil || !info.IsDir() || openWindowsRARFileUnsafe(file) {
		return false
	}
	descriptor, err := windows.GetSecurityInfo(
		windows.Handle(file.Fd()),
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		return false
	}
	return rarSharedSecurityDescriptorOwnedByCurrentProcess(descriptor)
}

func openRARPathNoFollow(path string, directory bool) (*os.File, error) {
	handle, err := openWindowsRARObject(ntPath(path), windows.FILE_GENERIC_READ|windows.READ_CONTROL, directory)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if openWindowsRARFileUnsafe(file) {
		_ = file.Close()
		return nil, errUnsafeRARAuthorityPath
	}
	return file, nil
}

// OpenPhysicalPath opens a file or directory without traversing reparse points.
func OpenPhysicalPath(path string, directory bool) (*os.File, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return openRARPathNoFollow(absPath, directory)
}

func openWindowsRARObject(objectPath string, access uint32, directory bool) (windows.Handle, error) {
	open := func(path string) (windows.Handle, error) {
		objectName, err := windows.NewNTUnicodeString(path)
		if err != nil {
			return 0, err
		}
		attributes := &windows.OBJECT_ATTRIBUTES{
			Length:     uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
			ObjectName: objectName,
			Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
		}
		options := uint32(windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_OPEN_REPARSE_POINT)
		if directory {
			options |= windows.FILE_DIRECTORY_FILE
		} else {
			options |= windows.FILE_NON_DIRECTORY_FILE
		}
		var handle windows.Handle
		var status windows.IO_STATUS_BLOCK
		err = windows.NtCreateFile(
			&handle,
			access,
			attributes,
			&status,
			nil,
			windows.FILE_ATTRIBUTE_NORMAL,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			windows.FILE_OPEN,
			options,
			0,
			0,
		)
		return handle, err
	}
	handle, err := open(objectPath)
	if !errors.Is(err, windows.STATUS_REPARSE_POINT_ENCOUNTERED) {
		return handle, err
	}
	directPath, resolveErr := directLocalDriveObjectPath(objectPath, queryWindowsDosDevice)
	if resolveErr != nil {
		return 0, fmt.Errorf("resolve secure Windows RAR path after %w: %v", err, resolveErr)
	}
	return open(directPath)
}

func openWindowsRARFileUnsafe(file *os.File) bool {
	if file == nil {
		return true
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return true
	}
	return info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func privateRARSecurityDescriptorSafe(
	descriptor *windows.SECURITY_DESCRIPTOR,
	directory bool,
) bool {
	return privateRARSecurityDescriptorMismatch(descriptor, directory) == ""
}

// privateRARSecurityDescriptorMismatch reduces a live descriptor to the facts
// the owner-only rule decides from and returns what differs, or "" when it is
// exactly the descriptor gentle-ai writes. The unsafe pointer arithmetic and
// the bounds checks that make reading a raw ACE safe stay here; the rule
// applied to what they find is rarWindowsOwnerOnlyMismatch, which is pure and
// table-tested on every platform.
func privateRARSecurityDescriptorMismatch(
	descriptor *windows.SECURITY_DESCRIPTOR,
	directory bool,
) string {
	principal := ""
	if currentUser, err := currentRARWindowsUserSID(); err == nil {
		principal = currentUser.String()
	}
	return rarWindowsOwnerOnlyMismatch(
		observeRARWindowsDescriptor(descriptor), principal, directory)
}

// rarWindowsObservedACELimit bounds how much of a foreign ACL a refusal
// copies: the rule needs the entry count and the first entry, never an
// unbounded walk of somebody else's DACL.
const rarWindowsObservedACELimit = 4

func observeRARWindowsDescriptor(
	descriptor *windows.SECURITY_DESCRIPTOR,
) rarWindowsOwnerOnlyDescriptor {
	if descriptor == nil || !descriptor.IsValid() {
		return rarWindowsOwnerOnlyDescriptor{}
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return rarWindowsOwnerOnlyDescriptor{}
	}
	observed := rarWindowsOwnerOnlyDescriptor{
		Readable:      true,
		Control:       uint16(control),
		DACLPresent:   control&windows.SE_DACL_PRESENT != 0,
		DACLProtected: control&windows.SE_DACL_PROTECTED != 0,
	}
	if owner, _, ownerErr := descriptor.Owner(); ownerErr == nil &&
		owner != nil && owner.IsValid() {
		observed.Owner = owner.String()
	}
	dacl, defaulted, daclErr := descriptor.DACL()
	observed.DACLDefaulted = defaulted
	if daclErr != nil || dacl == nil {
		return observed
	}
	observed.DACLReadable = true
	observed.ACECount = int(dacl.AceCount)
	for index := 0; index < observed.ACECount && index < rarWindowsObservedACELimit; index++ {
		observed.ACEs = append(observed.ACEs, observeRARWindowsACE(dacl, uint32(index)))
	}
	return observed
}

func observeRARWindowsACE(dacl *windows.ACL, index uint32) rarWindowsOwnerOnlyACE {
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil {
		return rarWindowsOwnerOnlyACE{Malformed: "the entry could not be read"}
	}
	observed := rarWindowsOwnerOnlyACE{
		AccessAllowed:  ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE,
		Flags:          ace.Header.AceFlags,
		Mask:           uint32(ace.Mask),
		MaskAcceptable: ownerOnlyRARWindowsAccessMask(ace.Mask),
	}
	const sidOffset = unsafe.Offsetof(windows.ACCESS_ALLOWED_ACE{}.SidStart)
	if uintptr(ace.Header.AceSize) < sidOffset+unsafe.Sizeof(ace.SidStart) {
		observed.Malformed = "the entry is too small to carry a SID"
		return observed
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.IsValid() ||
		uintptr(ace.Header.AceSize) < sidOffset+uintptr(aceSID.Len()) {
		observed.Malformed = "the entry carries an invalid or oversized SID"
		return observed
	}
	observed.SID = aceSID.String()
	return observed
}

func rarSharedSecurityDescriptorOwnedByCurrentProcess(
	descriptor *windows.SECURITY_DESCRIPTOR,
) bool {
	if descriptor == nil || !descriptor.IsValid() {
		return false
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return false
	}
	currentUser, err := currentRARWindowsUserSID()
	if err == nil && owner.Equals(currentUser) {
		return true
	}
	tokenOwner, err := currentRARWindowsTokenOwnerSID()
	if err == nil && owner.Equals(tokenOwner) {
		return true
	}
	return rarTrustedWindowsAdministrativeOwner(owner)
}

// rarTrustedWindowsAdministrativeOwner reports whether a shared repository
// directory owner is a higher-privilege administrative principal that cannot
// be forged by a standard user.
//
// This check exists so an attacker-writable directory can never host review
// authority. Two well-known owners clear that bar without being the current
// token user:
//
//   - BUILTIN\Administrators (S-1-5-32-544): elevated shells, corporate
//     provisioning, and managed installs create directories owned by this
//     group rather than the individual user, because it is the default owner
//     of every elevated token. Assigning it as an owner requires a token
//     that already holds administrative rights (the SID must be
//     owner-eligible in the assigning token, or the caller must hold
//     SeRestorePrivilege/SeTakeOwnershipPrivilege), so it is at least as
//     trustworthy as the current user's own ownership. Note the token-owner
//     comparison above already accepts this exact owner whenever the current
//     process runs elevated; refusing the same owner from a non-elevated
//     token of the same user protected nothing and walled every repository
//     whose .git ancestry was ever touched from an elevated shell.
//   - NT AUTHORITY\SYSTEM (S-1-5-18): services, scheduled tasks, and CI
//     runners executing as LocalSystem own the directories they create.
//     Forging this owner requires SYSTEM itself.
//
// Deliberately NOT accepted: Everyone (S-1-1-0), Authenticated Users
// (S-1-5-11), or any other arbitrary owner. Any standard user can hold or
// squat such an owner, which is exactly the attacker-controlled
// authority-host threat this check refuses. The reparse-point rejection in
// rarPathUnsafe/openWindowsRARFileUnsafe is orthogonal and stays fully
// intact: a trusted owner never excuses a reparse point.
//
// Only the shared repository-parent checks consult this set. Private RAR
// authority state (privateRARSecurityDescriptorSafe) remains strictly
// current-token-user-only because gentle-ai creates it itself with an
// explicit owner-only descriptor.
func rarTrustedWindowsAdministrativeOwner(owner *windows.SID) bool {
	if owner == nil || !owner.IsValid() {
		return false
	}
	// guard:population shared-rar-owner too-tight: legitimate shared RAR owners are the current user, token owner, BUILTIN Administrators, or LocalSystem; arbitrary owners remain excluded
	return owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) ||
		owner.IsWellKnown(windows.WinLocalSystemSid)
}

// rarRepositoryOwnerDescription renders the refused directory's owner for
// operator-facing refusal messages. It is diagnostic only and never
// participates in the trust decision itself.
func rarRepositoryOwnerDescription(path string) string {
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		return "an unreadable owner"
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return "an unreadable owner"
	}
	description := owner.String()
	if account, domain, _, lookupErr := owner.LookupAccount(""); lookupErr == nil {
		if domain != "" {
			account = domain + `\` + account
		}
		description = account + " (" + description + ")"
	}
	return description
}

func ownerOnlyRARSecurityDescriptor(directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	currentUser, err := currentRARWindowsUserSID()
	if err != nil {
		return nil, err
	}
	sid := currentUser.String()
	if sid == "" {
		return nil, errors.New("current Windows user SID is unavailable")
	}
	inheritance := ""
	if directory {
		inheritance = "OICI"
	}
	// FA (FILE_ALL_ACCESS), not GA (GENERIC_ALL). The two grant the identical
	// access set on a file object -- GENERIC_ALL maps to exactly FILE_ALL_ACCESS
	// under the file generic mapping, which is why the readback rule accepts
	// both and why every directory already on disk stays valid. What changes is
	// that Windows no longer has to transform the ACL it is handed. Mapping
	// generic rights is the one transformation the kernel is documented to
	// apply to a supplied ACL, and it is applied by a different code path for
	// CreateDirectory's SeAssignSecurity than for SetSecurityInfo's
	// auto-inherit conversion. The create path demonstrably produces a DACL
	// this rule accepts; the repair path writes the same ACL through the other
	// one and its readback is what refuses. Handing both paths an already
	// specific mask removes that asymmetry from the question entirely.
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + sid + "D:P(A;" + inheritance + ";FA;;;" + sid + ")",
	)
	if err != nil || descriptor == nil || !descriptor.IsValid() {
		if err != nil {
			return nil, fmt.Errorf("build owner-only RAR DACL: %w", err)
		}
		return nil, errors.New("owner-only RAR DACL is invalid")
	}
	return descriptor, nil
}

func currentRARWindowsUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !user.User.Sid.IsValid() {
		if err != nil {
			return nil, fmt.Errorf("resolve current Windows user SID: %w", err)
		}
		return nil, errors.New("current Windows user SID is invalid")
	}
	sid, err := user.User.Sid.Copy()
	if err != nil {
		return nil, fmt.Errorf("copy current Windows user SID: %w", err)
	}
	return sid, nil
}

type rarWindowsTokenOwner struct {
	Owner *windows.SID
}

func currentRARWindowsTokenOwnerSID() (*windows.SID, error) {
	token := windows.GetCurrentProcessToken()
	var size uint32
	err := windows.GetTokenInformation(
		token,
		windows.TokenOwner,
		nil,
		0,
		&size,
	)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) ||
		size < uint32(unsafe.Sizeof(rarWindowsTokenOwner{})) {
		if err != nil {
			return nil, fmt.Errorf(
				"resolve current Windows token owner size: %w",
				err,
			)
		}
		return nil, errors.New(
			"current Windows token owner has an invalid size",
		)
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(
		token,
		windows.TokenOwner,
		&buffer[0],
		size,
		&size,
	); err != nil {
		return nil, fmt.Errorf(
			"resolve current Windows token owner: %w",
			err,
		)
	}
	value := (*rarWindowsTokenOwner)(unsafe.Pointer(&buffer[0]))
	if value.Owner == nil || !value.Owner.IsValid() {
		return nil, errors.New("current Windows token owner SID is invalid")
	}
	owner, err := value.Owner.Copy()
	runtime.KeepAlive(buffer)
	if err != nil {
		return nil, fmt.Errorf("copy current Windows token owner SID: %w", err)
	}
	return owner, nil
}

func ownerOnlyRARWindowsAccessMask(mask windows.ACCESS_MASK) bool {
	const fileAllAccess windows.ACCESS_MASK = windows.STANDARD_RIGHTS_REQUIRED |
		windows.SYNCHRONIZE | windows.ACCESS_MASK(0x1ff)
	return mask == windows.GENERIC_ALL || mask == fileAllAccess
}

// ---------------------------------------------------------------------------------------------------------------------
// Windows filesystem classifier — injectable for testing.
// ---------------------------------------------------------------------------------------------------------------------

var rarWindowsAuthorityFilesystemClassifier = windowsRARAuthorityFilesystemTypeDefault

var rarWindowsACLCapableFilesystems = map[string]struct{}{"NTFS": {}, "ReFS": {}}

var rarWindowsUnsupportedFilesystems = map[string]struct{}{"exFAT": {}, "FAT32": {}}

func windowsRARAuthorityFilesystemTypeDefault(path string) string {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	var volSerial, maxCompLen, fsFlags uint32
	var fsName [windows.MAX_PATH]uint16
	err = windows.GetVolumeInformation(name, &fsName[0], uint32(len(fsName))*2, &volSerial, &maxCompLen, &fsFlags, nil, 0)
	if err != nil {
		return ""
	}
	for i, c := range fsName {
		if c == 0 {
			return windows.UTF16ToString(fsName[:i])
		}
	}
	return windows.UTF16ToString(fsName[:])
}

func isWindowsAuthorityFilesystemSupported(fstype string) bool {
	_, ok := rarWindowsACLCapableFilesystems[fstype]
	return ok
}

func isUNCPath(path string) bool {
	return strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `\\?\UNC\`)
}

// ---------------------------------------------------------------------------------------------------------------------
// Typed sentinels for filesystem-specific errors.
// ---------------------------------------------------------------------------------------------------------------------

type errUnsupportedWindowsFilesystemType struct {
	path     string
	fstype   string
	innerErr error
}

func (e errUnsupportedWindowsFilesystemType) Error() string {
	return fmt.Sprintf(
		"RAR authority parent %q is on filesystem %q, which does not support persistent Windows ownership semantics; move the Git common directory to NTFS or ReFS",
		e.path, e.fstype,
	)
}
func (e errUnsupportedWindowsFilesystemType) Unwrap() error { return e.innerErr }
func (e errUnsupportedWindowsFilesystemType) Is(target error) bool {
	if t, ok := target.(errUnsupportedWindowsFilesystemType); ok {
		return e.innerErr == t.innerErr
	}
	return false
}

type errUnknownWindowsFilesystemType struct {
	path     string
	innerErr error
}

func (e errUnknownWindowsFilesystemType) Error() string {
	return fmt.Sprintf(
		"RAR authority parent %q is on an unknown or remote filesystem; cannot assume NTFS semantics; check the path and rerun",
		e.path,
	)
}
func (e errUnknownWindowsFilesystemType) Unwrap() error { return e.innerErr }
func (e errUnknownWindowsFilesystemType) Is(target error) bool {
	if t, ok := target.(errUnknownWindowsFilesystemType); ok {
		return e.innerErr == t.innerErr
	}
	return false
}

var errUnsupportedWindowsFilesystem = errUnsupportedWindowsFilesystemType{innerErr: errUnsafeRARAuthorityPath}
var errUnknownWindowsFilesystem = errUnknownWindowsFilesystemType{innerErr: errUnsafeRARAuthorityPath}

// ---------------------------------------------------------------------------------------------------------------------
// Refusal formatting.
// ---------------------------------------------------------------------------------------------------------------------

func formatWindowsRARAuthorityRefusal(path string) error {
	if isUNCPath(path) {
		return errUnknownWindowsFilesystemType{path: path, innerErr: errUnsafeRARAuthorityPath}
	}
	fstype := rarWindowsAuthorityFilesystemClassifier(path)
	if isWindowsAuthorityFilesystemSupported(fstype) {
		return fmt.Errorf(
			"RAR authority parent %q is owned by %s, which is neither the current user nor a trusted administrative authority: %w",
			path, rarRepositoryOwnerDescription(path), errUnsafeRARAuthorityPath,
		)
	}
	if _, unsupported := rarWindowsUnsupportedFilesystems[fstype]; unsupported {
		return errUnsupportedWindowsFilesystemType{path: path, fstype: fstype, innerErr: errUnsafeRARAuthorityPath}
	}
	return errUnknownWindowsFilesystemType{path: path, innerErr: errUnsafeRARAuthorityPath}
}

func formatRARAuthorityRefusal(path string) error { return formatWindowsRARAuthorityRefusal(path) }

// ---------------------------------------------------------------------------------------------------------------------
// Platform-specific validateRARRepositoryParent — adds UNC-path fail-closed handling.
// ---------------------------------------------------------------------------------------------------------------------

func validateRARRepositoryParent(path string) error {
	if isUNCPath(path) {
		return errUnknownWindowsFilesystemType{path: path, innerErr: errUnsafeRARAuthorityPath}
	}
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if rarPathUnsafe(path, before) || !before.IsDir() {
		return errUnsafeRARAuthorityPath
	}
	if !rarRepositoryDirectorySafe(path, before) {
		return formatRARAuthorityRefusal(path)
	}
	file, err := openRARPathNoFollow(path, true)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(before, opened) || !os.SameFile(opened, current) || !rarRepositoryOpenDirectorySafe(file, opened) {
		return errRARAuthorityPathReplaced
	}
	return nil
}
