//go:build !windows

package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// Platform help and TUI title live in shell_install_help_{darwin,other}.go.

// The flag is registered only so an explicit selector is refused by name
// instead of being dropped: the Linux request has no channel to bind.
func shellInstallPlatformFlags(flags *flag.FlagSet, _ *shellinstaller.UserInstallRequest) func() error {
	channel := flags.String("channel", "", "Windows-only channel selection")
	return func() error {
		explicit := false
		flags.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "channel" })
		if explicit {
			return fmt.Errorf("--channel %q refused: channel selection is Windows-only; rerun `gentle-ai shell install` without --channel", *channel)
		}
		return nil
	}
}

func shellEntryValues(req shellinstaller.UserInstallRequest) []string {
	return []string{req.Destination, req.Mode, req.SharedPrefix, req.SharedAgent, req.Confirmation}
}

func shellInstallInspected(stdout io.Writer, req shellinstaller.UserInstallRequest, token string) error {
	preview, err := shellinstaller.PreviewUserInstall(req, token)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Confirmation: %s\nCommands: %s/bin/gentle-shell, %s/bin/pi\n%s", token, req.Destination, req.Destination, preview)
	return err
}

func runShellInstallTUI(ctx context.Context, cancel context.CancelFunc, self string, stdout io.Writer) error {
	model := shellInstallModel{cancel: cancel, req: shellinstaller.UserInstallRequest{Mode: "separate"}}
	final, err := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(stdout)).Run()
	if err != nil {
		return err
	}
	selection := final.(shellInstallModel)
	if !selection.confirmed {
		return selection.err
	}
	// Run only after Bubble Tea has restored the terminal and released stdin.
	return shellinstaller.RunUserEntry(ctx, self, append([]string{"install"}, shellEntryValues(selection.req)...), os.Stdin, stdout, os.Stderr)
}
