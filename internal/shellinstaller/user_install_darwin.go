//go:build darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// Darwin entry points for Separate and Shared installation and Shared
// recovery. There is no user manager to re-enter: every entry repeats
// userDarwinKernelCheck, owned installer commands run through userSupervise
// and the launched Pi runs in one userLaunchGroup group.

// userKernelGate is the kernel qualification every darwin entry repeats; tests
// replace it to drive refusals.
var userKernelGate = userDarwinKernelCheck

func UserKernelCheck() error {
	return userKernelGate()
}

func ValidateUserInstall(req UserInstallRequest) error {
	return userValidateInstall(req)
}

func InspectUserInstall(req UserInstallRequest) (string, error) {
	return userInspectInstall(req)
}

func RunUserInstall(ctx context.Context, req UserInstallRequest) (UserInstallResult, error) {
	return userRunInstall(ctx, req)
}

// An internal selector names the same operation: darwin never forwards to a
// manager, so each entry repeats the kernel and physical selection checks.
func RunUserEntry(ctx context.Context, _ string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing shell entry")
	}
	if err := UserKernelCheck(); err != nil {
		return err
	}
	switch strings.TrimPrefix(args[0], "internal-") {
	case "check":
		return nil
	case "install":
		req, err := UserInstallFromEntry(args[1:])
		if err != nil {
			return err
		}
		result, err := RunUserInstall(ctx, req)
		if err == nil {
			_, err = fmt.Fprintf(stdout, "Installed %s; commands %s/bin/gentle-shell and %s/bin/pi\n", result.Destination, result.Destination, result.Destination)
		}
		return err
	case "recover":
		return userRecover(ctx, args[1:], stdout)
	case "launch":
		if len(args) < 2 {
			return errors.New("missing owned launch root")
		}
		return userLaunch(ctx, args[1], args[2:], stdin, stdout, stderr)
	default:
		return errors.New("unknown shell internal entry")
	}
}

// os.Executable keeps the exec spelling on darwin (a Homebrew link, /tmp); the
// supervisor checks need the physical file that spelling names.
func userExecutable() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(self)
}

// userOwnedRun runs one owned installer command through userSupervise: its own
// process group under the darwin rlimits, killed and reaped on cancellation, at
// the caller's deadline or when output exceeds the privateRun bound.
func userOwnedRun(ctx context.Context, cmd *exec.Cmd, cancel context.CancelFunc) (string, error) {
	if cmd.Err != nil || len(cmd.Args) == 0 {
		return "", privateError("start", errors.Join(cmd.Err, errors.New("owned command is unresolved")))
	}
	var deadline time.Duration
	if until, ok := ctx.Deadline(); ok {
		deadline = max(time.Until(until), time.Millisecond)
	}
	limits, err := userDarwinLimits(deadline)
	if err != nil {
		return "", err
	}
	output := &privateOutput{cancel: cancel}
	err = userSuperviseIn(ctx, limits, cmd.Dir, cmd.Env, nil, output, output, append([]string{cmd.Path}, cmd.Args[1:]...)...)
	if output.overflow || !utf8.Valid(output.data) {
		return "", privateError("output", err)
	}
	var failure *PrivateRuntimeError
	var exit *exec.ExitError
	switch {
	case err == nil:
		return string(output.data), nil
	case errors.As(err, &failure) && failure.Kind == "uncertain":
		return "", err // A group member outlived the bounded reap.
	case errors.As(err, &failure):
		return string(output.data), err
	case errors.As(err, &exit):
		return string(output.data), privateError("nonzero", err)
	}
	return string(output.data), privateError("start", err)
}

// userLaunchLimits applies the darwin rlimits to the launched Pi without a
// second process group: /bin/sh sets them on itself and execs Node in place,
// so the single userLaunchGroup group that owns the terminal stays Pi's and
// its kill reaches Pi. A launched session has no CPU deadline. The returned
// result classifies the wait status and must run once on every path after a
// successful call; with a nil error it only releases the attestation pipe.
func userLaunchLimits(cmd *exec.Cmd) (func(error) error, error) {
	if cmd.Err != nil || len(cmd.Args) == 0 || !filepath.IsAbs(cmd.Path) {
		return nil, privateError("refused", errors.Join(cmd.Err, errors.New("owned launch must name an absolute command")))
	}
	if err := userLimitsEnvironment(cmd.Env); err != nil {
		return nil, err
	}
	if err := userSealInheritedDescriptors(); err != nil {
		return nil, err
	}
	limits, err := userDarwinLimits(0)
	if err != nil {
		return nil, err
	}
	script, err := userLimitsScript(limits)
	if err != nil {
		return nil, err
	}
	witness, err := userLimitsAttach(cmd)
	if err != nil {
		return nil, err
	}
	cmd.Args = append([]string{userLimitsShell, "-c", script, userLimitsName, cmd.Path}, cmd.Args[1:]...)
	cmd.Path = userLimitsShell
	return func(waitErr error) error {
		defer witness.close()
		return witness.result(waitErr)
	}, nil
}

// The darwin bootstrap stops at a stage (BSD mv cannot rename without
// replacement); the installer publishes it.
func userBootstrapPublish(ctx context.Context, output, destination string) error {
	return userAdoptStage(ctx, output, destination, privateNativeNodeSize, privateNativeNodeSHA)
}

const (
	userStageReport    = "Node bootstrap staged: Node=24.18.0 npm=11.16.0 package-install=not-run launch=not-run Ready=false stage="
	userStagePrefix    = ".gentle-node-stage."
	userStageInventory = "BOOTSTRAP-SHA256SUMS"
	userStageEntries   = 65536
)

var userStageProvenance = "accepted-archive-sha256=" + privateColdSHA + "\nbytes=" + strconv.FormatInt(privateColdSize, 10) + "\nNode=24.18.0\nnpm=11.16.0\n"

var userStageListing = regexp.MustCompile(`^([0-9a-f]{64})  \./([A-Za-z0-9_@.+-]+(?:/[A-Za-z0-9_@.+-]+)*)$`)

// userStagePath accepts only the exact single line the bootstrap prints on
// success, naming <parent>/.gentle-node-stage.<8 mktemp characters>/node
// beside destination. The path is a claim, not authority; userAdoptStage
// verifies the tree it names.
func userStagePath(output, destination string) (string, error) {
	line, terminated := strings.CutSuffix(output, "\n")
	stage, reported := strings.CutPrefix(line, userStageReport)
	suffix, named := strings.CutPrefix(filepath.Base(filepath.Dir(stage)), userStagePrefix)
	alphanumeric := strings.IndexFunc(suffix, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) == -1
	if !terminated || !reported || !named || len(suffix) != 8 || !alphanumeric || stage != filepath.Dir(destination)+"/"+userStagePrefix+suffix+"/node" {
		return "", privateError("source", fmt.Errorf("darwin Node bootstrap stage report refused: %q", output))
	}
	return stage, nil
}

// userAdoptStage publishes the Node tree the darwin bootstrap staged. The stage
// container must be the only entry beside the absent destination and hold only
// node; the whole tree must match its inventory, including the independent
// Node pin, and keep its identity until RENAME_EXCL publishes it. The empty
// container is removed afterwards.
func userAdoptStage(ctx context.Context, output, destination string, nodeSize int64, nodeSHA string) error {
	stage, err := userStagePath(output, destination)
	if err != nil {
		return err
	}
	container, parent := filepath.Dir(stage), filepath.Dir(destination)
	occupancy := func() error {
		return errors.Join(userOnlyEntry(parent, filepath.Base(container)), userOnlyEntry(container, "node"))
	}
	containerInfo, containerErr := privateDirectory(container)
	stageInfo, stageErr := privateDirectory(stage)
	if err := errors.Join(containerErr, stageErr, occupancy()); err != nil {
		return err
	}
	if err := userStageVerify(ctx, stage, nodeSize, nodeSHA); err != nil {
		return err
	}
	freshContainer, containerErr := privateDirectory(container)
	freshStage, stageErr := privateDirectory(stage)
	if err := errors.Join(containerErr, stageErr, occupancy()); err != nil || !os.SameFile(containerInfo, freshContainer) || !os.SameFile(stageInfo, freshStage) {
		return privateError("preimage", errors.Join(err, errors.New("Node stage changed after verification")))
	}
	if err := userPublishStage(stage, destination); err != nil {
		return userPublishError(err)
	}
	published, err := privateDirectory(destination)
	if err != nil || !os.SameFile(stageInfo, published) {
		return privateError("uncertain", errors.Join(err, errors.New("published Node tree is not the verified stage")))
	}
	if err := os.Remove(container); err != nil {
		return privateError("filesystem", err)
	}
	if err := userDirectorySync(parent); err != nil {
		return privateError("uncertain", err)
	}
	return nil
}

// userPublishError classifies a stage publication failure: an occupied
// destination is a changed preimage, a failed parent sync after the rename
// stays uncertain, and any other rename refusal is a filesystem error.
func userPublishError(err error) error {
	var failure *PrivateRuntimeError
	switch {
	case errors.As(err, &failure):
		return err
	case errors.Is(err, unix.EEXIST):
		return privateError("preimage", err)
	}
	return privateError("filesystem", err)
}

func userOnlyEntry(directory, name string) error {
	entries, err := privateNativeEntries(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != name {
		return privateError("preimage", errors.Join(err, fmt.Errorf("%q must contain only %q", directory, name)))
	}
	return nil
}

// userStageVerify re-verifies every staged file against the bootstrap
// inventory, refuses unlisted or non-regular entries, and checks the fixed
// provenance and the independent Node pin.
func userStageVerify(ctx context.Context, stage string, nodeSize int64, nodeSHA string) error {
	inventory := filepath.Join(stage, userStageInventory)
	stamp, err := privateNativeFile(ctx, inventory, 0600, -1, "")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(inventory)
	if err != nil || len(data) > 4<<20 || !strings.HasSuffix(stamp, fmt.Sprintf(":%x", sha256.Sum256(data))) {
		return privateError("source", errors.Join(err, errors.New("stage inventory changed while read")))
	}
	listed := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		match := userStageListing.FindStringSubmatch(line)
		if match == nil || listed[match[2]] != "" || match[2] == userStageInventory || strings.Contains("/"+match[2]+"/", "/./") || strings.Contains("/"+match[2]+"/", "/../") {
			return privateError("source", fmt.Errorf("stage inventory line refused: %q", line))
		}
		listed[match[2]] = match[1]
	}
	if listed["bin/node"] != nodeSHA || listed["BOOTSTRAP-PROVENANCE"] != fmt.Sprintf("%x", sha256.Sum256([]byte(userStageProvenance))) {
		return privateError("source", errors.New("stage inventory lacks the pinned Node or the fixed provenance"))
	}
	if _, err := privateNativeFile(ctx, filepath.Join(stage, "bin/node"), 0700, nodeSize, nodeSHA); err != nil {
		return err
	}
	seen, visited := 0, 0
	err = filepath.WalkDir(stage, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if visited++; visited > userStageEntries {
			return privateError("source", errors.New("stage exceeds its entry bound"))
		}
		relative, err := filepath.Rel(stage, path)
		switch {
		case err != nil:
			return err
		case entry.IsDir():
			return privateNativeDirectory(path)
		case relative == userStageInventory:
			return nil
		case !entry.Type().IsRegular():
			return privateError("source", fmt.Errorf("stage entry is neither a regular file nor a directory: %q", relative))
		}
		pin, ok := listed[relative]
		if !ok {
			return privateError("source", fmt.Errorf("stage file absent from its inventory: %q", relative))
		}
		seen++
		_, err = privateNativeFile(ctx, path, 0, -1, pin)
		return err
	})
	if err == nil && seen != len(listed) {
		err = privateError("source", errors.New("stage inventory lists files the stage lacks"))
	}
	return err
}
