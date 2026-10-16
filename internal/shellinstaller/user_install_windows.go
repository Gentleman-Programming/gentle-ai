//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
	"golang.org/x/sys/windows"
)

const userWindowsSchema = "gentle-shell-windows-separate/v1"

type userWindowsManifest struct {
	Schema, Destination, SID, Identity, SupervisorSHA string
	PrefixIdentity, AgentIdentity                     string
	SelectionSHA                                      string
	MainCommit, MainArchiveSHA, LockSHA               string
}

func UserKernelCheck() error {
	version := windows.RtlGetVersion()
	var processMachine, nativeMachine uint16
	if err := windows.IsWow64Process2(windows.CurrentProcess(), &processMachine, &nativeMachine); err != nil {
		return err
	}
	if runtime.GOARCH != "amd64" || nativeMachine != 0x8664 || version.MajorVersion != 10 || version.BuildNumber < 22000 || version.ProductType != 1 {
		return errors.New("Gentle Shell Separate requires Windows 11 x64; Windows Server and ARM64 are not qualified targets")
	}
	if windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("run Gentle Shell from your normal Windows account, not an Administrator terminal")
	}
	return nil
}

func userWindowsWorkerCheck() error {
	if err := UserKernelCheck(); err != nil {
		return err
	}
	return userWindowsJobOwned()
}

// The current process runs in the exact job the owned supervisor created.
func userWindowsJobOwned() error {
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		return err
	}
	var cpu userWindowsCPU
	if err := windows.QueryInformationJobObject(0, windows.JobObjectCpuRateControlInformation, uintptr(unsafe.Pointer(&cpu)), uint32(unsafe.Sizeof(cpu)), nil); err != nil {
		return err
	}
	if limits.BasicLimitInformation.LimitFlags != userWindowsJobFlags || limits.BasicLimitInformation.ActiveProcessLimit != 64 || limits.JobMemoryLimit != 3221225472 || cpu != (userWindowsCPU{Flags: 5, Rate: uint32(10000 / runtime.NumCPU())}) {
		return errors.New("physical Windows worker Job Object limits differ")
	}
	return nil
}

func InspectUserInstall(req UserInstallRequest) (string, error) {
	channel, err := UserInstallChannel(req.Channel)
	if err != nil {
		return "", err
	}
	req.Channel = channel
	if err := UserKernelCheck(); err != nil {
		return "", err
	}
	if req.Mode != "separate" || req.SharedPrefix != "" || req.SharedAgent != "" || filepath.Base(req.Destination) == "." || filepath.Base(req.Destination) == string(filepath.Separator) {
		return "", errors.New("Windows MVP supports Separate only; existing Pi, configuration and bindings stay untouched")
	}
	parent, err := userWindowsIdentity(filepath.Dir(req.Destination), true)
	if err != nil || !filepath.IsAbs(req.Destination) || filepath.Clean(req.Destination) != req.Destination || strings.ContainsAny(req.Destination, "\x00\r\n\"%&|<>^!") {
		return "", errors.Join(err, errors.New("select an owned canonical Windows installation path"))
	}
	identity := "absent"
	if _, err := os.Lstat(req.Destination); err == nil {
		selected, err := userWindowsIdentity(req.Destination, true)
		if err != nil {
			return "", err
		}
		manifest, err := userWindowsManifestRead(req.Destination)
		if err != nil || manifest.Identity != selected {
			return "", errors.Join(err, errors.New("destination is not a completed owned Windows installation; no overwrite or recovery attempted"))
		}
		identity = selected
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	sid, err := userWindowsSID()
	if err != nil {
		return "", err
	}
	return userConfirmation(req, sid+"|"+parent+"|"+identity), nil
}

func userWindowsEnvironment(root string) ([]string, error) {
	system, err := windows.GetWindowsDirectory()
	if err != nil {
		return nil, err
	}
	if _, err := userWindowsIdentity(system, false); err != nil {
		return nil, err
	}
	env := []string{"SystemRoot=" + system, "WINDIR=" + system, "SystemDrive=" + filepath.VolumeName(system)}
	if root == "" {
		return append(env, "PATH="+filepath.Join(system, "System32")), nil
	}
	prefix, agent := filepath.Join(root, "prefix"), filepath.Join(root, "agent")
	return append(env,
		"HOME="+filepath.Join(root, "home"), "USERPROFILE="+filepath.Join(root, "home"),
		"APPDATA="+filepath.Join(root, "config"), "LOCALAPPDATA="+filepath.Join(root, "state"),
		"TEMP="+filepath.Join(root, "tmp"), "TMP="+filepath.Join(root, "tmp"),
		"GENTLE_PI_CONFIG_HOME="+filepath.Join(root, "config"), "PI_CODING_AGENT_DIR="+agent,
		"GENTLE_PI_AGENT_HOME="+agent, "GENTLE_PI_NO_SKILL_REGISTRY=1",
		"GENTLE_SHELL_HOME="+agent, "GENTLE_SHELL_CONFIG="+filepath.Join(root, "config/gentle-shell.json"),
		// Go's own defaults below the private HOME/LOCALAPPDATA, made explicit
		// because launch verification classifies them as mutable runtime state.
		"GOPATH="+filepath.Join(root, "home/go"), "GOMODCACHE="+filepath.Join(root, "home/go/pkg/mod"), "GOCACHE="+filepath.Join(root, "state/go-build"),
		"PATH="+strings.Join([]string{filepath.Join(root, "runtime/node"), filepath.Join(root, "runtime/go/bin"), filepath.Join(system, "System32")}, string(os.PathListSeparator)),
		"NPM_CONFIG_PREFIX="+prefix, "NPM_CONFIG_IGNORE_SCRIPTS=true", "NPM_CONFIG_AUDIT=false", "NPM_CONFIG_FUND=false",
		"NPM_CONFIG_USERCONFIG="+filepath.Join(root, "config/user.npmrc"), "NPM_CONFIG_GLOBALCONFIG="+filepath.Join(root, "config/global.npmrc"),
		"NPM_CONFIG_CACHE="+filepath.Join(root, "runtime/cache"), "NODE_USE_SYSTEM_CA=1"), nil
}

func userWindowsBinding(root, product string) []byte {
	return []byte("@echo off\r\n\"" + filepath.Join(root, "supervisor.exe") + "\" shell launch \"" + root + "\" " + product + " %*\r\n")
}

// Canonical complete JSON rejects missing, duplicate, aliased and unknown fields.
func userWindowsSelection(data []byte, root string) (UserInstallRequest, error) {
	var req UserInstallRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return UserInstallRequest{}, err
	}
	canonical, _ := json.Marshal(req)
	if string(data) != string(canonical) || req.Destination != root || req.Mode != "separate" || req.SharedPrefix != "" || req.SharedAgent != "" || (req.Channel != "stable" && req.Channel != "main") {
		return UserInstallRequest{}, errors.New("Windows retained selection fields/root differ")
	}
	return req, nil
}

func userWindowsManifestRead(root string) (userWindowsManifest, error) {
	var manifest userWindowsManifest
	data, err := userWindowsRead(filepath.Join(root, "manifest.json"), 4096)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	selection, selectionErr := userWindowsRead(filepath.Join(root, "selection.json"), 4096)
	if selectionErr != nil || userWindowsSHA(selection) != manifest.SelectionSHA {
		return manifest, errors.Join(selectionErr, errors.New("Windows selection binding differs"))
	}
	selected, err := userWindowsSelection(selection, root)
	if err != nil {
		return manifest, err
	}
	if selected.Channel == "main" {
		if _, err := userWindowsMainRead(root, manifest); err != nil {
			return manifest, err
		}
	} else if manifest.MainCommit != "" || manifest.MainArchiveSHA != "" {
		return manifest, errors.New("stable selection contains Main authority")
	}
	lock, err := userWindowsRead(filepath.Join(root, "source/package-lock.json"), 32<<20)
	if err != nil || userWindowsSHA(lock) != manifest.LockSHA {
		return manifest, errors.Join(err, errors.New("retained generated lock differs"))
	}
	canonical, _ := json.Marshal(manifest)
	identity, identityErr := userWindowsIdentity(root, true)
	sid, sidErr := userWindowsSID()
	if string(data) != string(canonical)+"\n" || manifest.Schema != userWindowsSchema || manifest.Destination != root || manifest.SID != sid || manifest.Identity != identity || identityErr != nil || sidErr != nil {
		return manifest, errors.Join(identityErr, sidErr, errors.New("Windows manifest/physical owner differs"))
	}
	return manifest, nil
}

// Keep whole small failures, never a plausible-looking prefix of a large one.
// The sanitized worker environment contains no caller credentials or config.
type userWindowsProvisionDiagnostic struct {
	data     []byte
	withheld bool
}

func (d *userWindowsProvisionDiagnostic) Write(p []byte) (int, error) {
	if d.withheld || len(p) > (16<<10)-len(d.data) {
		d.data, d.withheld = nil, true
	} else {
		d.data = append(d.data, p...)
	}
	return len(p), nil // Always drain the pipe, including wholly withheld output.
}

func (d *userWindowsProvisionDiagnostic) String() string {
	if d.withheld || !utf8.Valid(d.data) || strings.ContainsRune(string(d.data), 0) {
		return "whole diagnostic withheld (size, UTF-8, or NUL guard)"
	}
	return fmt.Sprintf("%q", d.data) // Escape terminal controls rather than emitting them.
}

func userWindowsProvision(ctx context.Context, root, action string, stdout, stderr io.Writer) error {
	helper, err := assets.ReadWindowsUserHelper()
	if err != nil {
		return err
	}
	actual, err := userWindowsRead(filepath.Join(root, "provision.mjs"), int64(len(helper)))
	if err != nil || string(actual) != string(helper) {
		return errors.Join(err, errors.New("Windows provision source differs"))
	}
	env, err := userWindowsEnvironment(root)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, filepath.Join(root, "runtime/node/node.exe"), filepath.Join(root, "provision.mjs"), root, action)
	command.Dir, command.Env = filepath.Join(root, "project"), env
	diagnostic := &userWindowsProvisionDiagnostic{}
	command.Stdout, command.Stderr, command.WaitDelay = stdout, io.MultiWriter(stderr, diagnostic), 2*time.Second
	if err := command.Run(); err != nil { // Already inside the read-back bounded Job Object.
		return fmt.Errorf("Windows provision %s failed: %w; stderr=%s", action, err, diagnostic)
	}
	return nil
}

// Runtime state written after publication by the owned Go, npm, Node and Pi
// processes: GOPATH/GOMODCACHE, GOCACHE, TEMP, the npm cache and Pi sessions
// named after the caller CWD. It is not an installed artefact.
var userWindowsMutableState = []string{"home", "state", "tmp", "runtime/cache", "agent/sessions"}

type userWindowsInventoryMode int

const (
	// Complete stage before publication: every name must stay selection-safe.
	userWindowsInventoryInstalled userWindowsInventoryMode = iota
	// Complete walk with every per-entry owner/DACL/reparse/hard-link/type and
	// resource check. Only names strictly below a fixed mutable state root may
	// be data names; the roots themselves and all artefacts stay strict.
	userWindowsInventoryLaunch
	// Complete owned stage before removal; data names permitted below the stage.
	userWindowsInventoryCleanup
)

func userWindowsInventory(root string, mode userWindowsInventoryMode) error {
	mutable := map[string]bool{}
	if mode == userWindowsInventoryLaunch {
		for _, relative := range userWindowsMutableState {
			mutable[filepath.Join(root, filepath.FromSlash(relative))] = true
		}
	}
	dataRoot := func(path string) string {
		if mode == userWindowsInventoryCleanup {
			return root
		}
		for current := filepath.Dir(path); len(current) > len(root); current = filepath.Dir(current) {
			if mutable[current] {
				return current
			}
		}
		return ""
	}
	var count int
	var total int64
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, cause error) error {
		if cause != nil {
			return cause
		}
		count++
		if count > 250000 {
			return errors.New("Windows inventory file-count bound")
		}
		if _, err := userWindowsIdentityBelow(path, true, dataRoot(path)); err != nil {
			return err
		}
		if mutable[path] && !entry.IsDir() {
			return errors.New("Windows mutable state root is not a private directory")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return errors.New("Windows inventory contains nonregular objects")
			}
			total += info.Size()
			bound := int64(32 << 20)
			if strings.HasPrefix(path, filepath.Join(root, "runtime/node")+string(filepath.Separator)) || strings.HasPrefix(path, filepath.Join(root, "runtime/go")+string(filepath.Separator)) || path == filepath.Join(root, "runtime/archives/go.zip") || path == filepath.Join(root, "runtime/archives/node.zip") || path == filepath.Join(root, "supervisor.exe") {
				bound = 256 << 20
			}
			if info.Size() > bound || total > 2<<30 {
				return errors.New("Windows inventory physical byte bound")
			}
		}
		return nil
	})
}

// Remove only the stage this worker created, with the same volume/file identity
// and a complete owned, alias-free inventory; anything uncertain is preserved.
func userWindowsStageRemove(stage, identity string) error {
	observed, err := userWindowsIdentity(stage, true)
	if err != nil || observed != identity {
		return errors.Join(err, fmt.Errorf("uncertain Windows stage identity; preserved %q", stage))
	}
	if err := userWindowsInventory(stage, userWindowsInventoryCleanup); err != nil {
		return errors.Join(err, fmt.Errorf("uncertain Windows stage inventory; preserved %q", stage))
	}
	return os.RemoveAll(stage)
}

func RunUserInstall(ctx context.Context, req UserInstallRequest) (result UserInstallResult, err error) {
	if ctx == nil || ctx.Err() != nil {
		return result, errors.New("Windows installation canceled")
	}
	channel, err := UserInstallChannel(req.Channel)
	if err != nil {
		return result, err
	}
	req.Channel = channel
	if err := userWindowsWorkerCheck(); err != nil {
		return result, err
	}
	confirmation, err := InspectUserInstall(req)
	if err != nil || confirmation != req.Confirmation {
		return result, errors.Join(err, errors.New("physical Windows selection confirmation differs"))
	}
	if _, err := os.Lstat(req.Destination); err == nil {
		if err := userWindowsVerify(ctx, req.Destination, io.Discard, io.Discard); err != nil {
			return result, err
		}
		selection, err := userWindowsRead(filepath.Join(req.Destination, "selection.json"), 4096)
		selected, selectionErr := userWindowsSelection(selection, req.Destination)
		if err != nil || selectionErr != nil || selected.Channel != channel {
			return result, errors.Join(err, selectionErr, errors.New("existing installation channel differs; no replacement"))
		}
		return UserInstallResult{req.Destination, filepath.Join(req.Destination, "prefix"), filepath.Join(req.Destination, "agent"), "already-installed"}, nil
	}
	var mainArtifact userWindowsArtifact
	var mainData []byte
	if channel == "main" {
		mainArtifact, mainData, err = userWindowsMainSnapshot(ctx) // Confirmed choice, before first filesystem effect.
		if err != nil {
			return result, err
		}
		if _, err := userWindowsMainFiles(ctx, mainData, mainArtifact); err != nil {
			return result, err
		}
		fmt.Fprintf(os.Stderr, "Frozen canonical Gentle Shell Main: %s archive SHA256 %s (TLS publisher, not signed binary)\n", mainArtifact.Prefix, mainArtifact.SHA)
	}
	parent := filepath.Dir(req.Destination)
	stage, err := os.MkdirTemp(parent, ".gentle-shell-windows-stage-")
	if err != nil {
		return result, err
	}
	stageIdentity, identityErr := userWindowsIdentity(stage, true)
	if identityErr != nil {
		return result, identityErr // Do not claim ownership or remove an unknown stage.
	}
	published := false
	defer func() {
		if published {
			return
		}
		// A canceled provision kills only Node; its npm/Go descendants may still write.
		if cause := userWindowsJobQuiesce(30 * time.Second); cause != nil {
			err = errors.Join(err, cause, fmt.Errorf("owned Windows processes may still write; preserved %q", stage))
			return
		}
		err = errors.Join(err, userWindowsStageRemove(stage, stageIdentity))
	}()
	if err := userWindowsPrivate(stage); err != nil {
		return result, err
	}
	for _, directory := range []string{"home", "config", "state", "agent/bin", "tmp", "project", "runtime/cache", "runtime/archives", "runtime/node", "runtime/go", "bin"} {
		if err := os.MkdirAll(filepath.Join(stage, filepath.FromSlash(directory)), 0700); err != nil {
			return result, err
		}
	}
	selection, _ := json.Marshal(req)
	if err := userWindowsWrite(filepath.Join(stage, "selection.json"), selection); err != nil {
		return result, err
	}
	started := time.Now()
	phase := func(step, artifact string) {
		fmt.Fprintf(os.Stderr, "Windows install phase: %s %s %dms\n", step, artifact, time.Since(started).Milliseconds())
	}
	for i, artifact := range userWindowsArtifacts {
		phase("acquire", artifact.Archive)
		data, err := userWindowsAcquire(ctx, artifact, filepath.Join(stage, "runtime/archives", artifact.Archive))
		if err != nil {
			return result, err
		}
		phase("unpack", artifact.Archive)
		if i < 2 {
			directory := "node"
			if i == 1 {
				directory = "go"
			}
			if _, err := userWindowsZIP(ctx, data, artifact, filepath.Join(stage, "runtime", directory), ""); err != nil {
				return result, err
			}
		} else {
			name := strings.TrimSuffix(artifact.Archive, ".zip") + ".exe"
			member, err := userWindowsZIP(ctx, data, artifact, "", artifact.Prefix+"/"+name)
			if err != nil {
				return result, err
			}
			if err := userWindowsWrite(filepath.Join(stage, "agent/bin", name), member); err != nil {
				return result, err
			}
		}
	}
	if channel == "main" {
		metadata, _ := json.Marshal(mainArtifact)
		if err := userWindowsWrite(filepath.Join(stage, "main.json"), metadata); err != nil {
			return result, err
		}
		if err := userWindowsWrite(filepath.Join(stage, "runtime/archives/main.zip"), mainData); err != nil {
			return result, err
		}
	}
	phase("runtimes-ready", "all")
	for _, name := range []string{"user.npmrc", "global.npmrc"} {
		if err := userWindowsWrite(filepath.Join(stage, "config", name), nil); err != nil {
			return result, err
		}
	}
	helper, err := assets.ReadWindowsUserHelper()
	if err != nil {
		return result, err
	}
	if err := userWindowsWrite(filepath.Join(stage, "provision.mjs"), helper); err != nil {
		return result, err
	}
	lockHelper, err := assets.ReadPrivateHelper("complete-generated-lock-sri.mjs")
	if err != nil {
		return result, err
	}
	if err := userWindowsWrite(filepath.Join(stage, "complete-generated-lock-sri.mjs"), lockHelper); err != nil {
		return result, err
	}
	self, err := os.Executable()
	if err != nil {
		return result, err
	}
	image, err := userWindowsReadTrusted(self, 256<<20, false)
	if err != nil {
		return result, err
	}
	if err := userWindowsWrite(filepath.Join(stage, "supervisor.exe"), image); err != nil {
		return result, err
	}
	phase("provision", "stock")
	if err := userWindowsProvision(ctx, stage, "install", io.Discard, io.Discard); err != nil {
		return result, err
	}
	phase("provision-ready", "stock")
	if channel == "main" {
		if err := userWindowsInventory(stage, userWindowsInventoryInstalled); err != nil {
			return result, err
		}
		gentle, stock := filepath.Join(stage, "prefix/node_modules/gentle-pi"), filepath.Join(stage, "stock-gentle-pi")
		if err := os.Rename(gentle, stock); err != nil {
			return result, err
		}
		if _, err := userWindowsZIP(ctx, mainData, mainArtifact, gentle, ""); err != nil {
			return result, err
		}
		for _, name := range []string{".gentle-ai", "node_modules"} {
			original := filepath.Join(stock, name)
			if _, err := os.Lstat(original); errors.Is(err, os.ErrNotExist) && name == "node_modules" {
				continue
			} else if err != nil {
				return result, err
			}
			if err := os.Rename(original, filepath.Join(gentle, name)); err != nil {
				return result, err
			}
		}
		if err := userWindowsCompareMain(ctx, mainData, mainArtifact, gentle); err != nil {
			return result, err
		}
		if err := userWindowsProvision(ctx, stage, "overlay", io.Discard, io.Discard); err != nil {
			return result, err
		}
	}
	for _, product := range []string{"pi", "gentle-shell"} {
		if err := userWindowsWrite(filepath.Join(stage, "bin", product+".cmd"), userWindowsBinding(req.Destination, product)); err != nil {
			return result, err
		}
	}
	if err := userWindowsInventory(stage, userWindowsInventoryInstalled); err != nil {
		return result, err
	}
	observed, err := InspectUserInstall(req)
	if err != nil || observed != req.Confirmation {
		return result, errors.Join(err, errors.New("Windows selection changed before publication"))
	}
	prefixID, err := userWindowsIdentity(filepath.Join(stage, "prefix"), true)
	if err != nil {
		return result, err
	}
	agentID, err := userWindowsIdentity(filepath.Join(stage, "agent"), true)
	if err != nil {
		return result, err
	}
	sid, err := userWindowsSID()
	if err != nil {
		return result, err
	}
	lock, err := userWindowsRead(filepath.Join(stage, "source/package-lock.json"), 32<<20)
	if err != nil {
		return result, err
	}
	manifest := userWindowsManifest{Schema: userWindowsSchema, Destination: req.Destination, SID: sid, Identity: stageIdentity, SupervisorSHA: userWindowsSHA(image), PrefixIdentity: prefixID, AgentIdentity: agentID, SelectionSHA: userWindowsSHA(selection), LockSHA: userWindowsSHA(lock)}
	if channel == "main" {
		manifest.MainCommit, manifest.MainArchiveSHA = strings.TrimPrefix(mainArtifact.Prefix, "gentle-shell-"), mainArtifact.SHA
	}
	encoded, _ := json.Marshal(manifest)
	if err := userWindowsWrite(filepath.Join(stage, "manifest.json"), append(encoded, '\n')); err != nil {
		return result, err
	}
	from, _ := windows.UTF16PtrFromString(stage)
	to, _ := windows.UTF16PtrFromString(req.Destination)
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return result, err
	} // No replace-existing flag.
	published = true
	if err := userWindowsVerify(ctx, req.Destination, io.Discard, io.Discard); err != nil {
		return result, errors.Join(err, errors.New("published Windows installation remains unqualified; preserved, no success reported"))
	}
	return UserInstallResult{req.Destination, filepath.Join(req.Destination, "prefix"), filepath.Join(req.Destination, "agent"), "installed"}, nil
}
