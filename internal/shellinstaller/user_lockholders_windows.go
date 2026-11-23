//go:build windows

package shellinstaller

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// When a rename or removal under an installation stays busy, the installer asks
// the Windows Restart Manager which processes hold files under the root so the
// error can name them. This is diagnostic only: nothing here stops, signals or
// kills a process. The walk and the registration are bounded so a large tree
// (tens of thousands of files) never makes the final failure slow, and the
// whole enumeration runs once, on the final failure, never on a retry.
type userWindowsLockHolder struct {
	PID  uint32
	Name string
}

var (
	// userWindowsLockHolders is the enumerator the busy retry consults on its
	// final failure; tests replace it.
	userWindowsLockHolders = userWindowsRestartManagerHolders

	userWindowsLockHolderFileLimit  = 4096
	userWindowsLockHolderEntryLimit = 16384
)

const userWindowsLockHolderChunk = 256

var (
	userWindowsRstrtmgr             = windows.NewLazySystemDLL("rstrtmgr.dll")
	userWindowsRmStartSession       = userWindowsRstrtmgr.NewProc("RmStartSession")
	userWindowsRmRegisterResources  = userWindowsRstrtmgr.NewProc("RmRegisterResources")
	userWindowsRmGetList            = userWindowsRstrtmgr.NewProc("RmGetList")
	userWindowsRmEndSession         = userWindowsRstrtmgr.NewProc("RmEndSession")
	userWindowsRmSessionKeyChars    = 32 + 1 // CCH_RM_SESSION_KEY + terminator
	userWindowsRmGetListMaxAttempts = 4
)

// userWindowsRmUniqueProcess mirrors RM_UNIQUE_PROCESS.
type userWindowsRmUniqueProcess struct {
	ProcessID        uint32
	ProcessStartTime windows.Filetime
}

// userWindowsRmProcessInfo mirrors RM_PROCESS_INFO (668 bytes, 4-byte aligned).
type userWindowsRmProcessInfo struct {
	Process          userWindowsRmUniqueProcess
	AppName          [256]uint16 // CCH_RM_MAX_APP_NAME + 1
	ServiceShortName [64]uint16  // CCH_RM_MAX_SVC_NAME + 1
	ApplicationType  uint32
	AppStatus        uint32
	TSSessionID      uint32
	Restartable      int32
}

// userWindowsBusyError builds the final busy error. Holder enumeration only
// appends to the existing message: when it fails the message is unchanged, and
// when it finds nothing the message adds the working-directory case Restart
// Manager cannot see.
func userWindowsBusyError(root string, cause error) error {
	message := fmt.Sprintf("a file under %s is still in use by another program (for example an antivirus scan or an open editor or terminal); close it and retry", root)
	holders, err := userWindowsLockHolders(root)
	switch {
	case err != nil:
	case len(holders) == 0:
		message += fmt.Sprintf("; a program may also be running with its working directory inside %s (for example a terminal or a WSL session)", root)
	default:
		names := make([]string, 0, len(holders))
		for _, holder := range holders {
			names = append(names, fmt.Sprintf("%s (PID %d)", holder.Name, holder.PID))
		}
		message += fmt.Sprintf("; processes holding files under %s: %s", root, strings.Join(names, ", "))
	}
	return fmt.Errorf("%w; %s", cause, message)
}

// userWindowsLockHolderCandidates lists up to fileLimit regular files under
// root, visiting at most entryLimit entries. Unreadable entries are skipped;
// symbolic links and junctions are not followed.
func userWindowsLockHolderCandidates(root string, fileLimit, entryLimit int) []string {
	var files []string
	visited := 0
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		visited++
		if visited > entryLimit || len(files) >= fileLimit {
			return fs.SkipAll
		}
		if err != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	return files
}

// userWindowsRestartManagerHolders asks Restart Manager which processes hold
// the bounded candidate files under root. It returns the holders sorted by PID
// with duplicates removed, or an error when Restart Manager is unavailable or
// fails.
func userWindowsRestartManagerHolders(root string) ([]userWindowsLockHolder, error) {
	files := userWindowsLockHolderCandidates(root, userWindowsLockHolderFileLimit, userWindowsLockHolderEntryLimit)
	if len(files) == 0 {
		return nil, nil
	}
	if err := userWindowsRstrtmgr.Load(); err != nil {
		return nil, err
	}
	var session uint32
	key := make([]uint16, userWindowsRmSessionKeyChars)
	if code, _, _ := userWindowsRmStartSession.Call(uintptr(unsafe.Pointer(&session)), 0, uintptr(unsafe.Pointer(&key[0]))); code != 0 {
		return nil, fmt.Errorf("RmStartSession: %w", windows.Errno(code))
	}
	defer userWindowsRmEndSession.Call(uintptr(session))

	for start := 0; start < len(files); start += userWindowsLockHolderChunk {
		end := min(start+userWindowsLockHolderChunk, len(files))
		names := make([]*uint16, 0, end-start)
		for _, file := range files[start:end] {
			name, err := windows.UTF16PtrFromString(file)
			if err != nil {
				continue
			}
			names = append(names, name)
		}
		if len(names) == 0 {
			continue
		}
		code, _, _ := userWindowsRmRegisterResources.Call(uintptr(session), uintptr(len(names)), uintptr(unsafe.Pointer(&names[0])), 0, 0, 0, 0)
		if code != 0 {
			return nil, fmt.Errorf("RmRegisterResources: %w", windows.Errno(code))
		}
	}

	infos, err := userWindowsRmList(session)
	if err != nil {
		return nil, err
	}
	seen := make(map[uint32]bool, len(infos))
	holders := make([]userWindowsLockHolder, 0, len(infos))
	for _, info := range infos {
		pid := info.Process.ProcessID
		if pid == 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		name := userWindowsProcessImageName(pid)
		if name == "" {
			name = windows.UTF16ToString(info.AppName[:])
		}
		if name == "" {
			name = "unknown process"
		}
		holders = append(holders, userWindowsLockHolder{PID: pid, Name: name})
	}
	sort.Slice(holders, func(i, j int) bool { return holders[i].PID < holders[j].PID })
	return holders, nil
}

// userWindowsRmList calls RmGetList, growing the buffer while Restart Manager
// reports ERROR_MORE_DATA.
func userWindowsRmList(session uint32) ([]userWindowsRmProcessInfo, error) {
	var needed, count uint32
	var reasons uint32
	infos := make([]userWindowsRmProcessInfo, 0)
	for attempt := 0; attempt < userWindowsRmGetListMaxAttempts; attempt++ {
		count = uint32(len(infos))
		var buffer uintptr
		if count > 0 {
			buffer = uintptr(unsafe.Pointer(&infos[0]))
		}
		code, _, _ := userWindowsRmGetList.Call(uintptr(session), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)), buffer, uintptr(unsafe.Pointer(&reasons)))
		switch windows.Errno(code) {
		case 0:
			return infos[:min(count, uint32(len(infos)))], nil
		case windows.ERROR_MORE_DATA:
			// A process can start holding a file between calls; leave headroom.
			infos = make([]userWindowsRmProcessInfo, needed+4)
		default:
			return nil, fmt.Errorf("RmGetList: %w", windows.Errno(code))
		}
	}
	return nil, errors.New("RmGetList: holder list kept growing")
}

// userWindowsProcessImageName returns the executable base name of pid, or ""
// when the process cannot be queried.
func userWindowsProcessImageName(pid uint32) string {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buffer[:size]))
}
