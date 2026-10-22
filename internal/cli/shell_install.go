package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// Platform contracts live in shell_install_platform_{windows,other}.go:
// shellInstallHelp, shellInstallPlatformFlags, shellEntryValues,
// shellInstallInspected and runShellInstallTUI.

func parseShellInstall(args []string, stdout io.Writer) (shellinstaller.UserInstallRequest, bool, error) {
	var req shellinstaller.UserInstallRequest
	flags := flag.NewFlagSet("shell install", flag.ContinueOnError)
	flags.SetOutput(stdout)
	flags.StringVar(&req.Destination, "target", "", "owned installation target")
	flags.StringVar(&req.Mode, "mode", "separate", "separate or shared")
	flags.StringVar(&req.SharedPrefix, "prefix", "", "selected existing global Pi prefix")
	flags.StringVar(&req.SharedAgent, "agent", "", "selected existing Pi configuration")
	flags.StringVar(&req.Confirmation, "confirm", "", "physical selection SHA256")
	inspect := flags.Bool("inspect", false, "inspect without installation")
	finish := shellInstallPlatformFlags(flags, &req)
	flags.Usage = func() { _, _ = io.WriteString(stdout, shellInstallHelp) }
	if err := flags.Parse(args); err != nil {
		return shellinstaller.UserInstallRequest{}, false, err
	}
	if flags.NArg() != 0 {
		return shellinstaller.UserInstallRequest{}, false, errors.New("unexpected shell install positional arguments; run gentle-ai shell install --help for supported flags")
	}
	if err := finish(); err != nil {
		return shellinstaller.UserInstallRequest{}, false, err
	}
	return req, *inspect, nil
}

// Dedicated early route, deliberately independent of the generic installer,
// profile detector, self-update, gate and ordinary Gentle AI startup TUI.
func RunShell(args []string, stdout io.Writer) (resultErr error) {
	defer func() {
		var failure *shellinstaller.PrivateRuntimeError
		if errors.As(resultErr, &failure) && (failure.Workspace != "" || failure.Destination != "") {
			resultErr = fmt.Errorf("%w\nPreserve evidence: workspace=%q destination/unit=%q\nFor shared installation recovery, inspect ROOT=workspace/installed or published destination with gentle-ai shell recover ROOT inspect", resultErr, failure.Workspace, failure.Destination)
		}
	}()
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := io.WriteString(stdout, shellInstallHelp)
		return err
	}
	switch args[0] {
	case "install", "launch", "recover", "internal-check", "internal-install", "internal-launch", "internal-recover":
		// The delegated supervisor or worker re-enters here; internal selectors
		// still repeat kernel qualification and never grant execution authority.
		// Each backend refuses the operations its platform does not provide.
	default:
		return fmt.Errorf("unknown shell command %q; run gentle-ai shell --help", args[0])
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if args[0] != "install" {
		return shellinstaller.RunUserEntry(ctx, self, args, os.Stdin, stdout, os.Stderr)
	}
	if len(args) == 1 {
		return runShellInstallTUI(ctx, cancel, self, stdout)
	}
	req, inspect, err := parseShellInstall(args[1:], stdout)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	token, err := shellinstaller.InspectUserInstall(req)
	if err != nil {
		return err
	}
	if inspect {
		return shellInstallInspected(stdout, req, token)
	}
	// guard:population windows-separate-confirmation fail-closed: legitimate explicit installations carry the current token from the owned physical selection; missing or mismatched confirmations remain excluded without starting a worker
	if req.Confirmation != token {
		return errors.New("inspect the physical selection first with --inspect, then pass its --confirm SHA256; run gentle-ai shell install --help for selection flags or gentle-ai shell install for interactive review")
	}
	return shellinstaller.RunUserEntry(ctx, self, append([]string{"install"}, shellEntryValues(req)...), os.Stdin, stdout, os.Stderr)
}
