//go:build windows

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func userWindowsVerifyArchive(ctx context.Context, root string, artifact userWindowsArtifact, destination, member string) error {
	data, err := userWindowsRead(filepath.Join(root, "runtime/archives", artifact.Archive), artifact.Bound)
	if err != nil {
		return err
	}
	selected, err := userWindowsZIP(ctx, data, artifact, "", artifact.Prefix+"/"+member)
	if err != nil {
		return err
	}
	if artifact.Archive == "fd.zip" || artifact.Archive == "rg.zip" {
		actual, err := userWindowsRead(filepath.Join(destination, member), 32<<20)
		if err != nil || !bytes.Equal(actual, selected) {
			return errors.Join(err, errors.New("Windows stock helper differs from retained supplier"))
		}
		return nil
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		stream, err := file.Open()
		if err != nil {
			return err
		}
		original, readErr := io.ReadAll(io.LimitReader(stream, int64(file.UncompressedSize64)+1))
		closeErr := stream.Close()
		relative := strings.TrimPrefix(file.Name, artifact.Prefix+"/")
		actual, actualErr := userWindowsRead(filepath.Join(destination, filepath.FromSlash(relative)), int64(file.UncompressedSize64))
		if cause := errors.Join(readErr, closeErr, actualErr); cause != nil || !bytes.Equal(original, actual) {
			return errors.Join(cause, errors.New("Windows private toolchain differs from retained supplier"))
		}
	}
	return nil
}

func userWindowsVerify(ctx context.Context, root string, stdout, stderr io.Writer) error {
	manifest, err := userWindowsManifestRead(root)
	if err != nil {
		return err
	}
	image, err := userWindowsRead(filepath.Join(root, "supervisor.exe"), 256<<20)
	if err != nil || userWindowsSHA(image) != manifest.SupervisorSHA {
		return errors.Join(err, errors.New("owned Windows supervisor differs"))
	}
	prefix, err := userWindowsIdentity(filepath.Join(root, "prefix"), true)
	if err != nil || prefix != manifest.PrefixIdentity {
		return errors.Join(err, errors.New("owned Windows prefix identity differs"))
	}
	agent, err := userWindowsIdentity(filepath.Join(root, "agent"), true)
	if err != nil || agent != manifest.AgentIdentity {
		return errors.Join(err, errors.New("owned Windows agent identity differs"))
	}
	if err := userWindowsInventory(root, userWindowsInventoryLaunch); err != nil {
		return err
	}
	for _, product := range []string{"pi", "gentle-shell"} {
		binding, err := userWindowsRead(filepath.Join(root, "bin", product+".cmd"), 8192)
		if err != nil || !bytes.Equal(binding, userWindowsBinding(root, product)) {
			return errors.Join(err, errors.New("owned Windows command binding differs"))
		}
	}
	for i, artifact := range userWindowsArtifacts {
		destination, member := filepath.Join(root, "agent/bin"), strings.TrimSuffix(artifact.Archive, ".zip")+".exe"
		if i < 2 {
			directory := "node"
			if i == 1 {
				directory = "go"
			}
			destination = filepath.Join(root, "runtime", directory)
			member = "node.exe"
			if i == 1 {
				member = "bin/go.exe"
			}
		}
		if err := userWindowsVerifyArchive(ctx, root, artifact, destination, member); err != nil {
			return err
		}
	}
	if manifest.MainCommit != "" {
		artifact, err := userWindowsMainRead(root, manifest)
		if err != nil {
			return err
		}
		data, err := userWindowsRead(filepath.Join(root, "runtime/archives/main.zip"), artifact.Bound)
		if err != nil {
			return err
		}
		if err := userWindowsCompareMain(ctx, data, artifact, filepath.Join(root, "prefix/node_modules/gentle-pi")); err != nil {
			return err
		}
	}
	return userWindowsProvision(ctx, root, "verify", stdout, stderr)
}

func userWindowsConsoleSave() func() error {
	type console struct {
		handle windows.Handle
		mode   uint32
	}
	var saved []console
	for _, file := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		handle := windows.Handle(file.Fd())
		var mode uint32
		if windows.GetConsoleMode(handle, &mode) == nil {
			saved = append(saved, console{handle, mode})
		}
	}
	return func() error {
		var cause error
		for _, item := range saved {
			cause = errors.Join(cause, windows.SetConsoleMode(item.handle, item.mode))
		}
		return cause
	}
}

func userWindowsEntryContext(ctx context.Context, action string) (context.Context, context.CancelFunc) {
	if action == "launch" {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, 900*time.Second)
}

func userWindowsLaunchCLI(root, product string) string {
	if product == "gentle-shell" {
		return filepath.Join(root, "prefix/node_modules/gentle-pi/bin/gentle-shell.mjs")
	}
	return filepath.Join(root, "prefix/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js")
}

func RunUserEntry(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer) (err error) {
	if ctx == nil || ctx.Err() != nil || len(args) == 0 {
		return errors.New("missing or canceled Windows shell entry")
	}
	if err := UserKernelCheck(); err != nil {
		return err
	}
	if !strings.HasPrefix(args[0], "internal-") {
		ctx, cancel := userWindowsEntryContext(ctx, args[0])
		defer cancel()
		env, envErr := userWindowsEnvironment("")
		if envErr != nil {
			return envErr
		}
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return cwdErr
		}
		restore := userWindowsConsoleSave()
		defer func() {
			err = errors.Join(err, restore())
			after, cause := os.Getwd()
			if cause != nil || after != cwd {
				err = errors.Join(err, cause, errors.New("Windows caller CWD differs after owned entry"))
			}
		}()
		forward := append([]string{"shell", "internal-" + args[0]}, args[1:]...)
		command := exec.CommandContext(ctx, self, forward...)
		command.Env, command.Stdin, command.Stdout, command.Stderr = env, stdin, stdout, stderr
		grace := 2 * time.Second
		if args[0] == "install" {
			grace = 120 * time.Second // Quiesce owned descendants and remove the owned stage.
		}
		closeChannel, channelErr := userWindowsCooperative(command, grace)
		if channelErr != nil {
			return channelErr
		}
		release, startErr := userWindowsStart(command)
		if startErr != nil {
			return errors.Join(startErr, closeChannel())
		}
		return errors.Join(command.Wait(), release(), closeChannel(), ctx.Err())
	}
	if err := userWindowsWorkerCheck(); err != nil {
		return err
	}
	ctx, stop, watchErr := userWindowsCancelWatch(ctx, os.Getenv(userWindowsCancelEnv))
	if watchErr != nil {
		return watchErr
	}
	defer stop()
	switch strings.TrimPrefix(args[0], "internal-") {
	case "check":
		return nil
	case "install":
		req, err := UserInstallFromEntry(args[1:])
		if err != nil {
			return err
		}
		result, err := RunUserInstall(ctx, req)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "Installed %s (%s); owned commands %s and %s (personal PATH unchanged)\nCurrent CMD only: set \"PATH=%s;%%PATH%%\"\n", result.Destination, req.Channel, filepath.Join(result.Destination, "bin/gentle-shell.cmd"), filepath.Join(result.Destination, "bin/pi.cmd"), filepath.Join(result.Destination, "bin"))
		return err
	case "launch":
		if len(args) < 3 || (args[2] != "pi" && args[2] != "gentle-shell") {
			return errors.New("missing owned Windows product/root")
		}
		root := args[1]
		verification, cancel := context.WithTimeout(ctx, 900*time.Second)
		cause := userWindowsVerify(verification, root, io.Discard, io.Discard)
		cancel()
		if cause != nil {
			return cause
		}
		env, err := userWindowsEnvironment(root)
		if err != nil {
			return err
		}
		cli := userWindowsLaunchCLI(root, args[2])
		forward := []string{cli}
		if args[2] == "gentle-shell" {
			forward = append(forward, "--isolated", "--") // Caller flags cannot redirect the Shell home/package.
		}
		command := exec.CommandContext(ctx, filepath.Join(root, "runtime/node/node.exe"), append(forward, args[3:]...)...)
		command.Env, command.Stdin, command.Stdout, command.Stderr, command.WaitDelay = env, stdin, stdout, stderr, 2*time.Second
		return command.Run() // Inherits the caller CWD and the bounded owned Job Object.
	default:
		return errors.New("Windows MVP supports check, Separate install and owned launch; no Shared/update/recovery")
	}
}
