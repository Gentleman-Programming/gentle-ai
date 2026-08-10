//go:build windows

package reviewtransaction

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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
	err = windows.CreateDirectory(name, &attributes)
	runtime.KeepAlive(descriptor)
	created := err == nil
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return false, err
	}
	if err := validatePrivateRARDirectory(path); err != nil {
		if created {
			_ = os.Remove(path)
		}
		return false, err
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
	handle, err := openWindowsRARObject(ntPath(path), directory)
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

func openWindowsRARObject(objectPath string, directory bool) (windows.Handle, error) {
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
			windows.FILE_GENERIC_READ|windows.READ_CONTROL,
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
	if descriptor == nil || !descriptor.IsValid() {
		return false
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PRESENT == 0 ||
		control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	if !rarSecurityDescriptorOwnedByCurrentUser(descriptor) {
		return false
	}
	currentUser, err := currentRARWindowsUserSID()
	if err != nil {
		return false
	}
	dacl, defaulted, err := descriptor.DACL()
	if err != nil || dacl == nil || defaulted || dacl.AceCount != 1 {
		return false
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil || ace == nil ||
		ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		return false
	}
	wantFlags := uint8(0)
	if directory {
		wantFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	if ace.Header.AceFlags != wantFlags || !ownerOnlyRARWindowsAccessMask(ace.Mask) {
		return false
	}
	const sidOffset = unsafe.Offsetof(windows.ACCESS_ALLOWED_ACE{}.SidStart)
	if uintptr(ace.Header.AceSize) < sidOffset+unsafe.Sizeof(ace.SidStart) {
		return false
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	return aceSID.IsValid() &&
		uintptr(ace.Header.AceSize) >= sidOffset+uintptr(aceSID.Len()) &&
		aceSID.Equals(currentUser)
}

func rarSecurityDescriptorOwnedByCurrentUser(
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
	return err == nil && owner.Equals(currentUser)
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
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + sid + "D:P(A;" + inheritance + ";GA;;;" + sid + ")",
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
