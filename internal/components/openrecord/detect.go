package openrecord

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// BinaryName is the command name installed by both routes of InstallMethod
// option (B): `go install .../cmd/openrecord` and openrecord's own script.
const BinaryName = "openrecord"

func defaultDetector() communitytool.Detector {
	return communitytool.DetectorFunc(exec.LookPath)
}

// FallbackPaths returns the two destinations InstallMethod option (B) can
// write to: the install script's default (~/.local/bin) and go install's
// default GOBIN (~/go/bin). Both are checked because a machine with Go on
// PATH installs to the second even when a prior install.sh run left a copy at
// the first — this is the same shape registered on the update.ToolInfo entry,
// so install and upgrade detection never disagree about where to look.
func FallbackPaths(homeDir, localAppData string) []string {
	if homeDir == "" {
		return nil
	}
	binary := BinaryName
	if localAppData != "" {
		binary += ".exe"
	}
	return []string{
		filepath.Join(homeDir, ".local", "bin", BinaryName),
		filepath.Join(homeDir, "go", "bin", binary),
	}
}

// goEnvValue is a package-level var so tests can fake `go env` without a real
// Go toolchain on the machine running them.
var goEnvValue = func(key string) (string, error) {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// goInstallDestinationDir resolves where `go install` writes: GOBIN when set,
// otherwise GOPATH's first entry plus "bin" — the same resolution
// internal/update/upgrade's goInstallDestinationDir performs. It is
// duplicated here rather than imported because that one is unexported and
// package upgrade must not gain a dependency on community-tool concerns.
func goInstallDestinationDir() (string, error) {
	if gobin, err := goEnvValue("GOBIN"); err == nil && gobin != "" {
		return gobin, nil
	}
	gopath, err := goEnvValue("GOPATH")
	if err != nil {
		return "", err
	}
	entries := filepath.SplitList(gopath)
	if len(entries) == 0 || strings.TrimSpace(entries[0]) == "" {
		return "", fmt.Errorf("go env reported neither GOBIN nor GOPATH")
	}
	return filepath.Join(entries[0], "bin"), nil
}

// resolveBinaryPath locates an already-installed openrecord: PATH, then the
// two FallbackPaths, then a custom GOBIN/GOPATH — a plain go-install default
// mismatch is covered by FallbackPaths, but a user-set GOBIN or a non-default
// GOPATH matches neither hardcoded path and would otherwise be missed.
func resolveBinaryPath(homeDir string, detector communitytool.Detector) (string, bool) {
	if detector != nil {
		if path, err := detector.LookPath(BinaryName); err == nil && strings.TrimSpace(path) != "" {
			return path, true
		}
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	for _, candidate := range FallbackPaths(homeDir, localAppData) {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	if dir, err := goInstallDestinationDir(); err == nil {
		binary := BinaryName
		if localAppData != "" {
			binary += ".exe"
		}
		candidate := filepath.Join(dir, binary)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

// DetectStatus reports openrecord's installed/absent state. Presence alone is
// not enough — a shadowed or broken binary would still resolve on PATH — so
// this actually runs `<path> version` and reports installed only when it
// succeeds, the same two-question shape openrecord's own `qmd status` uses.
func DetectStatus(homeDir string, detector communitytool.Detector, runner communitytool.Runner) communitytool.Status {
	status := communitytool.Status{Tool: model.CommunityToolOpenRecord, CLI: communitytool.AvailabilityMissing}
	path, found := resolveBinaryPath(homeDir, detector)
	if !found || runner == nil {
		return status
	}
	if err := runner.Run(path, "version"); err != nil {
		return status
	}
	status.CLI = communitytool.AvailabilityAvailable
	status.CLIPath = path
	return status
}
