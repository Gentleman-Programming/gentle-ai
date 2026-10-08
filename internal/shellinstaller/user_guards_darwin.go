//go:build darwin

package shellinstaller

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unicode"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Darwin guards behind the per-OS hooks the shared POSIX primitives call.
// Darwin entry points still refuse in user_install_unsupported.go.

func userDarwinKernelCheck() error {
	var name unix.Utsname
	if err := unix.Uname(&name); err != nil {
		return err
	}
	version, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return err
	}
	// Absent on Macs that cannot translate; 1 means this process runs under Rosetta.
	translated, err := unix.SysctlUint32("sysctl.proc_translated")
	if errors.Is(err, unix.ENOENT) {
		translated, err = 0, nil
	}
	if err != nil {
		return err
	}
	return userDarwinKernel(os.Getuid(), os.Geteuid(), runtime.GOARCH, unix.ByteSliceToString(name.Machine[:]), translated, version)
}

var userDarwinVersion = regexp.MustCompile(`^([0-9]{1,4})(\.[0-9]{1,4}){0,2}$`)

func userDarwinKernel(uid, euid int, goarch, machine string, translated uint32, version string) error {
	if uid == 0 || euid == 0 {
		return fmt.Errorf("Gentle Shell user installation on macOS refuses to run as root (uid=%d euid=%d); run it as the target user without sudo", uid, euid)
	}
	if goarch != "arm64" || machine != "arm64" || translated != 0 {
		return fmt.Errorf("Gentle Shell user installation on macOS requires native Apple silicon (arm64); found binary=%s kernel=%s translated=%d", goarch, machine, translated)
	}
	match := userDarwinVersion.FindStringSubmatch(version)
	if match == nil {
		return fmt.Errorf("Gentle Shell user installation requires macOS 14 or newer; found unreadable version %q", version)
	}
	if major, err := strconv.Atoi(match[1]); err != nil || major < 14 {
		return fmt.Errorf("Gentle Shell user installation requires macOS 14 or newer; found %s", version)
	}
	return nil
}

// userCanonicalPath resolves an operator path once to its physical spelling
// before the strict EvalSymlinks checks. Only root-owned links in root-owned,
// non-writable directories (/tmp, /var, /etc into /private) are followed; the
// final component is kept verbatim so the strict checks still refuse it if it
// is a link.
func userCanonicalPath(path string) (string, error) {
	if !privateHierarchyPath(path) {
		return "", privateError("refused", fmt.Errorf("canonical path requires a clean absolute path: %q", path))
	}
	if path == "/" {
		return path, nil
	}
	parent, err := userSystemResolve(filepath.Dir(path))
	if err != nil {
		return "", privateError("refused", err)
	}
	fd, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", privateError("refused", err)
	}
	var opened, named unix.Stat_t
	spelled, err := userDescriptorPath(fd)
	err = errors.Join(err, unix.Fstat(fd, &opened), unix.Close(fd))
	if err == nil {
		err = unix.Lstat(spelled, &named)
	}
	if err != nil || named.Dev != opened.Dev || named.Ino != opened.Ino {
		return "", privateError("refused", errors.Join(err, fmt.Errorf("canonical parent changed while resolving %q", path)))
	}
	return filepath.Join(spelled, filepath.Base(path)), nil
}

func userSystemResolve(path string) (string, error) {
	resolved, pending := "/", strings.Split(path, "/")
	for hops := 0; len(pending) > 0; {
		component := pending[0]
		pending = pending[1:]
		if component == "" || component == "." {
			continue
		}
		next := filepath.Join(resolved, component)
		info, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			resolved = next
			continue
		}
		container, err := os.Lstat(resolved)
		if hops++; err != nil || hops > 8 || info.Sys().(*syscall.Stat_t).Uid != 0 || container.Sys().(*syscall.Stat_t).Uid != 0 || container.Mode().Perm()&0022 != 0 {
			return "", errors.Join(err, fmt.Errorf("canonical path refuses a non-system symlink: %q", next))
		}
		target, err := os.Readlink(next)
		if err != nil {
			return "", err
		}
		if filepath.IsAbs(target) {
			resolved = "/"
		}
		pending = append(strings.Split(target, "/"), pending...)
	}
	return resolved, nil
}

// F_GETPATH returns the on-disk spelling (case, /private, firmlinks) of the
// object behind the descriptor.
func userDescriptorPath(fd int) (string, error) {
	buffer := make([]byte, unix.PathMax)
	// The pointer is converted inside the syscall.Syscall argument list so the
	// buffer stays live and unmoved for the kernel write (unsafe.Pointer rule 4).
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(unix.F_GETPATH), uintptr(unsafe.Pointer(&buffer[0]))); errno != 0 {
		return "", errno
	}
	return unix.ByteSliceToString(buffer), nil
}

// APFS is usually case- and normalization-insensitive, so lexical prefixes
// alone can call two spellings of one directory disjoint. Paths overlap when
// they do lexically (case-folded), by on-disk spelling, or by identity.
func userPathsOverlap(a, b string) bool {
	if userLexicalOverlap(strings.ToLower(a), strings.ToLower(b)) {
		return true
	}
	spelledA, errA := userSpelling(a)
	spelledB, errB := userSpelling(b)
	if errA != nil || errB != nil || userLexicalOverlap(strings.ToLower(spelledA), strings.ToLower(spelledB)) {
		return true
	}
	return userPhysicalWithin(a, b) || userPhysicalWithin(b, a)
}

func userLexicalOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// userSpelling returns the on-disk spelling of the deepest existing ancestor
// joined with the absent tail. A non-ASCII absent tail cannot be compared
// without normalization tables and fails closed.
func userSpelling(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("overlap requires absolute paths: %q", path)
	}
	existing, tail := filepath.Clean(path), ""
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !os.IsNotExist(err) || existing == "/" {
			return "", err
		}
		tail, existing = filepath.Join(filepath.Base(existing), tail), filepath.Dir(existing)
	}
	if strings.IndexFunc(tail, func(r rune) bool { return r > unicode.MaxASCII }) != -1 {
		return "", fmt.Errorf("absent non-ASCII path cannot be compared: %q", path)
	}
	fd, err := unix.Open(existing, unix.O_EVTONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", err
	}
	spelled, err := userDescriptorPath(fd)
	if err = errors.Join(err, unix.Close(fd)); err != nil {
		return "", err
	}
	return filepath.Join(spelled, tail), nil
}

func userPhysicalWithin(inner, outer string) bool {
	target, err := os.Stat(outer)
	if err != nil {
		return !os.IsNotExist(err)
	}
	for current := inner; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if (err == nil && os.SameFile(info, target)) || (err != nil && !os.IsNotExist(err)) {
			return true
		}
		if current == "/" || current == "." {
			return false
		}
	}
}

// Flags that neither restrict mutation nor hide content from the mode bits.
// UF_COMPRESSED is APFS transparent (decmpfs) compression of the same bytes.
const privateBenignFlags = unix.UF_NODUMP | unix.UF_HIDDEN | unix.UF_TRACKED | unix.UF_COMPRESSED

// privateExtendedMetadata refuses metadata the mode bits do not show: BSD
// flags (immutable, append-only, dataless, restricted, ...), ACLs with any
// non-deny entry, and com.apple.quarantine on owned entries. Deny-only ACLs
// (the default on ~), com.apple.provenance and com.apple.rootless cannot widen
// access and stay tolerated.
func privateExtendedMetadata(path string, info os.FileInfo, owned bool) error {
	if flags := info.Sys().(*syscall.Stat_t).Flags; flags&^privateBenignFlags != 0 {
		return fmt.Errorf("BSD file flags refused: path=%q flags=%#x", path, flags)
	}
	if err := privateACL(path); err != nil {
		return err
	}
	if owned {
		if _, err := unix.Lgetxattr(path, "com.apple.quarantine", nil); !errors.Is(err, unix.ENOATTR) {
			return fmt.Errorf("quarantined owned entry refused: path=%q (%v)", path, err)
		}
	}
	return nil
}

// The com.apple.system.Security xattr is unreadable even by the owner, so the
// kauth filesec comes from getattrlist(ATTR_CMN_EXTENDED_SECURITY).
func privateACL(path string) error {
	const filesecMagic, noACL, maxEntries, aceSize, aceDeny = 0x012cc16d, 0xffffffff, 128, 24, 2
	request := unix.Attrlist{Bitmapcount: unix.ATTR_BIT_MAP_COUNT, Commonattr: unix.ATTR_CMN_RETURNED_ATTRS | unix.ATTR_CMN_EXTENDED_SECURITY}
	buffer := make([]byte, 8192)
	name, err := unix.BytePtrFromString(path)
	if err != nil {
		return err
	}
	if _, _, errno := unix.Syscall6(unix.SYS_GETATTRLIST, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&request)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), unix.FSOPT_NOFOLLOW|unix.FSOPT_PACK_INVAL_ATTRS, 0); errno != 0 {
		return fmt.Errorf("extended security unreadable: path=%q: %w", path, errno)
	}
	order := binary.NativeEndian
	total := order.Uint32(buffer)
	if total < 32 || total > uint32(len(buffer)) {
		return fmt.Errorf("extended security malformed: path=%q", path)
	}
	if order.Uint32(buffer[4:])&unix.ATTR_CMN_EXTENDED_SECURITY == 0 {
		return nil
	}
	start, length := int64(24)+int64(int32(order.Uint32(buffer[24:]))), int64(order.Uint32(buffer[28:]))
	if length == 0 {
		return nil
	}
	if start < 32 || length < 44 || start+length > int64(total) {
		return fmt.Errorf("extended security malformed: path=%q", path)
	}
	filesec := buffer[start : start+length]
	count := order.Uint32(filesec[36:])
	if order.Uint32(filesec) != filesecMagic {
		return fmt.Errorf("extended security malformed: path=%q", path)
	}
	if count == noACL || count == 0 {
		return nil
	}
	if count > maxEntries || 44+int64(count)*aceSize > length {
		return fmt.Errorf("extended ACL malformed: path=%q entries=%d", path, count)
	}
	for entry := int64(0); entry < int64(count); entry++ {
		if kind := order.Uint32(filesec[44+entry*aceSize+16:]) & 0xf; kind != aceDeny {
			return fmt.Errorf("extended ACL grants or audits beyond the mode bits: path=%q entry=%d kind=%d", path, entry, kind)
		}
	}
	return nil
}

// userFullSync is the F_FULLFSYNC seam: darwin fsync stops at the drive cache.
var userFullSync = func(fd uintptr) error {
	_, err := unix.FcntlInt(fd, unix.F_FULLFSYNC, 0)
	return err
}

func userSync(file userSyncedFile) error {
	if descriptor, ok := file.(interface{ Fd() uintptr }); ok {
		return userFullSync(descriptor.Fd())
	}
	return file.Sync()
}
