//go:build windows

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// userWindowsSystemGoCandidateEnv carries the single caller context item the
// bounded worker needs for system-Go reuse: the absolute go.exe path resolved
// on the original process PATH by the outer entry process. The sanitized
// worker environment deliberately forwards no caller PATH, credentials or
// configuration; the worker revalidates the candidate completely before any
// trust decision.
const userWindowsSystemGoCandidateEnv = "GENTLE_SHELL_WINDOWS_SYSTEM_GO"

// userWindowsSystemGoMarker is the canonical in-target record of a reused
// system Go toolchain. Its presence and validity is the only reason the
// pinned go.zip artifact and the extracted runtime/go toolchain may be
// absent from a published installation. It always lives inside the target,
// never in a global location.
const userWindowsSystemGoMarker = "runtime/system-go.json"

// userWindowsSystemGoMinimum is the oldest toolchain accepted for the owned
// native builds; older, prerelease or unparsable versions fall back to the
// pinned go.zip artifact.
var userWindowsSystemGoMinimum = [3]int{1, 25, 10}

type userWindowsSystemGo struct {
	Go      string `json:"Go"`
	Version string `json:"Version"`
}

var userWindowsSystemGoVersionPattern = regexp.MustCompile(`^go(0|[1-9]\d*)\.(0|[1-9]\d*)(?:\.(0|[1-9]\d*))?$`)

// userWindowsSystemGoVersion parses the exact stable toolchain token of a
// `go version` output line and reports whether it is at or above the minimum.
// Prerelease, devel and unparsable outputs are refused.
func userWindowsSystemGoVersion(output string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 4 || fields[0] != "go" || fields[1] != "version" {
		return "", false
	}
	match := userWindowsSystemGoVersionPattern.FindStringSubmatch(fields[2])
	if match == nil {
		return "", false
	}
	var numbers [3]int
	for i := range numbers {
		numbers[i], _ = strconv.Atoi(match[1+i])
	}
	for i, number := range numbers {
		if number != userWindowsSystemGoMinimum[i] {
			if number < userWindowsSystemGoMinimum[i] {
				return "", false
			}
			break
		}
	}
	return fields[2], true
}

// userWindowsSystemGoProbe runs the foreign `go version` probe under a
// deadline. It is a seam so focused tests never execute a toolchain.
var userWindowsSystemGoProbe = func(ctx context.Context, goExe string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, goExe, "version")
	command.WaitDelay = 2 * time.Second
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	return output.Bytes(), err
}

// userWindowsSystemGoCandidate resolves a caller-installed go.exe by exact
// name on the original process PATH of the outer entry process. It performs
// no execution and no trust decision.
func userWindowsSystemGoCandidate() string {
	candidate, err := exec.LookPath("go.exe")
	if err != nil || !filepath.IsAbs(candidate) {
		return ""
	}
	return filepath.Clean(candidate)
}

// userWindowsSystemGoValidate applies every static reuse requirement to a
// candidate toolchain: an absolute canonical regular go.exe executable
// without reparse points or aliases anywhere on its path, a stable canonical
// NTFS directory, and a supported stable version. The owned-identity check
// walks every ancestor, so junctioned or aliased install directories and
// foreign-owned or mutually writable locations are refused.
func userWindowsSystemGoValidate(candidate, version string) error {
	if !filepath.IsAbs(candidate) || filepath.Clean(candidate) != candidate || !strings.EqualFold(filepath.Base(candidate), "go.exe") {
		return errors.New("Windows system Go path is not an absolute canonical go.exe")
	}
	info, err := os.Lstat(candidate)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Windows system Go executable is missing or not a regular file")
	}
	if _, err := userWindowsIdentity(candidate, false); err != nil {
		return errors.Join(err, errors.New("Windows system Go directory custody differs"))
	}
	if _, ok := userWindowsSystemGoVersion("go version " + version + " windows/amd64"); !ok {
		return errors.New("Windows system Go version is unsupported")
	}
	return nil
}

// userWindowsSystemGoDetect resolves and fully validates the caller
// candidate. Any absence, incompatibility or ambiguity is a silent fallback
// to the pinned go.zip artifact, never an install failure.
func userWindowsSystemGoDetect(ctx context.Context) (userWindowsSystemGo, bool) {
	candidate := os.Getenv(userWindowsSystemGoCandidateEnv)
	if candidate == "" || ctx.Err() != nil {
		fmt.Fprintln(os.Stderr, "Windows install phase: system-go unavailable; pinned go.zip artifact retained")
		return userWindowsSystemGo{}, false
	}
	output, err := userWindowsSystemGoProbe(ctx, candidate)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Windows install phase: system-go probe failed; pinned go.zip artifact retained")
		return userWindowsSystemGo{}, false
	}
	version, ok := userWindowsSystemGoVersion(string(output))
	if !ok {
		fmt.Fprintln(os.Stderr, "Windows install phase: system-go version unsupported; pinned go.zip artifact retained")
		return userWindowsSystemGo{}, false
	}
	if err := userWindowsSystemGoValidate(candidate, version); err != nil {
		fmt.Fprintf(os.Stderr, "Windows install phase: system-go candidate refused (%v); pinned go.zip artifact retained\n", err)
		return userWindowsSystemGo{}, false
	}
	return userWindowsSystemGo{Go: candidate, Version: version}, true
}

// userWindowsSystemGoStage records the accepted reuse inside the staged
// target itself as canonical complete JSON with exclusive owned custody.
func userWindowsSystemGoStage(stage string, marker userWindowsSystemGo) error {
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return userWindowsWrite(filepath.Join(stage, filepath.FromSlash(userWindowsSystemGoMarker)), data)
}

// userWindowsSystemGoMarkerRead reads and fully revalidates the in-target
// reuse marker. A missing marker is exactly os.ErrNotExist; a present but
// malformed, aliased, foreign or unsupported marker is a distinct error that
// verification refuses outright instead of falling back.
func userWindowsSystemGoMarkerRead(root string) (userWindowsSystemGo, error) {
	var marker userWindowsSystemGo
	path := filepath.Join(root, filepath.FromSlash(userWindowsSystemGoMarker))
	if _, err := os.Lstat(path); err != nil {
		return marker, err // Absence stays a plain os.ErrNotExist for callers.
	}
	data, err := userWindowsRead(path, 4096)
	if err != nil {
		return marker, err
	}
	if err := json.Unmarshal(data, &marker); err != nil {
		return marker, errors.Join(err, errors.New("Windows system Go marker is not canonical JSON"))
	}
	canonical, _ := json.Marshal(marker)
	if string(data) != string(canonical) {
		return marker, errors.New("Windows system Go marker fields differ")
	}
	if err := userWindowsSystemGoValidate(marker.Go, marker.Version); err != nil {
		return marker, err
	}
	return marker, nil
}

// userWindowsSystemGoBin is the owned PATH entry for the Go toolchain of an
// installed root: the system Go directory when a valid reuse marker exists,
// the pinned runtime directory otherwise. It is best-effort on purpose:
// launch never depends on the marker, and any doubt keeps the pinned entry.
func userWindowsSystemGoBin(root string) string {
	if marker, err := userWindowsSystemGoMarkerRead(root); err == nil {
		return filepath.Dir(marker.Go)
	}
	return filepath.Join(root, "runtime/go/bin")
}

// userWindowsGoZipSkipped reports whether the pinned go.zip artifact is not
// acquired or unpacked because an accepted system Go replaces it. Only the
// pinned Go artifact is ever skipped; every other artifact keeps its path.
func userWindowsGoZipSkipped(systemGoAccepted bool, archive string) bool {
	return systemGoAccepted && archive == "go.zip"
}

// userWindowsVerifyGoZipRequired reports whether the pinned go.zip artifact
// must still be verified for root. A valid canonical reuse marker makes the
// pinned Go artifact intentionally absent; a present but invalid marker is
// refused outright; an absent marker keeps the pinned archive contract.
func userWindowsVerifyGoZipRequired(root string) (bool, error) {
	_, err := userWindowsSystemGoMarkerRead(root)
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, os.ErrNotExist):
		return true, nil
	default:
		return false, err
	}
}
