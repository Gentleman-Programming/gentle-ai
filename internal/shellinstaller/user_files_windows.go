//go:build windows

package shellinstaller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func userWindowsSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

// Administrators and SYSTEM are trusted OS custodians, not hostile-user guards.
// Selection itself must belong to the current unelevated user.
func userWindowsSecurity(path string, selected bool) error {
	sid, err := userWindowsSID()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return errors.Join(err, errors.New("missing Windows owner"))
	}
	trusted := func(s string) bool {
		return s == sid || s == "S-1-5-18" || s == "S-1-5-32-544" || (!selected && s == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464") // Windows Modules Installer, OS ancestors only.
	}
	if (selected && owner.String() != sid) || !trusted(owner.String()) {
		return errors.New("foreign Windows owner; select a directory owned by your account")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.Join(err, errors.New("missing private Windows DACL"))
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE || ace.Header.AceFlags&8 != 0 { // INHERIT_ONLY does not authorize this object.
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("unsupported Windows access rule; select a private local directory")
		}
		// Ancestors may permit creation of unrelated children. They may not
		// permit deleting/replacing an existing ancestor or its access policy.
		mutation := uint32(0x500D0150)
		if selected {
			mutation |= 6 // No foreign file/subdirectory creation in our root.
		}
		who := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if uint32(ace.Mask)&mutation != 0 && !trusted(who) {
			return errors.New("Windows directory grants another account mutation access")
		}
	}
	return nil
}

func userWindowsIdentity(path string, selected bool) (string, error) {
	return userWindowsIdentityBelow(path, selected, "")
}

// Names strictly below data are owned file data (Go's bang-escaped module
// cache, Pi sessions named after the caller CWD), never a command-binding
// selection: they may contain ! % & ^. Data itself, its ancestors and every
// owner/DACL/reparse/volume/hard-link check stay unchanged.
func userWindowsIdentityBelow(path string, selected bool, data string) (string, error) {
	strict, relaxed := path, ""
	if data != "" && strings.HasPrefix(path, data+string(filepath.Separator)) {
		strict, relaxed = data, path[len(data):]
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(filepath.VolumeName(path)) != 2 || strings.ContainsAny(strict, "\x00\r\n\"%&|<>^") || strings.ContainsAny(relaxed, "\x00\r\n\"|<>") {
		return "", errors.New("select a canonical local drive path without shell metacharacters")
	}
	for current := path; ; current = filepath.Dir(current) {
		name, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return "", err
		}
		attributes, err := windows.GetFileAttributes(name)
		if err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return "", errors.Join(err, errors.New("Windows aliases/reparse points are not an owned selection"))
		}
		// Official Go archives contain bang-escaped module fixture filenames.
		// Permit only a regular leaf file; directories and binding selections
		// still reject bang characters (including every ancestor directory)
		// unless they lie strictly below an explicit owned data root.
		below := data != "" && strings.HasPrefix(current, data+string(filepath.Separator))
		if strings.Contains(current, "!") && !below && (current != path || attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
			return "", errors.New("select a canonical local drive path without shell metacharacters")
		}
		if err := userWindowsSecurity(current, selected && current == path); err != nil {
			return "", err
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	name, _ := windows.UTF16PtrFromString(path)
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return "", err
	}
	var filesystem [32]uint16
	if err := windows.GetVolumeInformationByHandle(handle, nil, 0, nil, nil, nil, &filesystem[0], uint32(len(filesystem))); err != nil || windows.UTF16ToString(filesystem[:]) != "NTFS" {
		return "", errors.Join(err, errors.New("Windows Separate requires local NTFS"))
	}
	var canonical [32768]uint16
	length, err := windows.GetFinalPathNameByHandle(handle, &canonical[0], uint32(len(canonical)), 0)
	if err != nil || length == 0 || length >= uint32(len(canonical)) || !strings.EqualFold(strings.TrimPrefix(windows.UTF16ToString(canonical[:length]), `\\?\`), path) {
		return "", errors.Join(err, errors.New("Windows drive/physical path alias differs"))
	}
	if selected && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 && info.NumberOfLinks != 1 {
		return "", errors.New("Windows selected file has unexpected hard links")
	}
	return fmt.Sprintf("%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}

func userWindowsPrivate(path string) error {
	sid, err := userWindowsSID()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return err
	}
	_, err = userWindowsIdentity(path, true)
	return err
}

func userWindowsWrite(path string, data []byte) error {
	if _, err := userWindowsIdentity(filepath.Dir(path), true); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	actual, err := userWindowsRead(path, int64(len(data)))
	if err != nil || string(actual) != string(data) {
		return errors.Join(err, errors.New("Windows exclusive write readback differs"))
	}
	return nil
}

func userWindowsRead(path string, bound int64) ([]byte, error) {
	return userWindowsReadTrusted(path, bound, true)
}

// The currently invoked installer image may be installed by an OS custodian.
// Owned installation data always requires the stricter current-SID selection.
func userWindowsReadTrusted(path string, bound int64, selected bool) ([]byte, error) {
	before, err := userWindowsIdentity(path, selected)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, bound+1))
	closeErr := file.Close()
	after, identityErr := userWindowsIdentity(path, selected)
	if err := errors.Join(readErr, closeErr, identityErr); err != nil {
		return nil, err
	}
	if before != after || int64(len(data)) > bound {
		return nil, errors.New("Windows read identity/byte bound differs")
	}
	return data, nil
}

func userWindowsSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
