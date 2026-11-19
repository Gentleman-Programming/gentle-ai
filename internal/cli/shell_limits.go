package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// runShellLimits shows or changes the Gentle Shell unit budget (#6055). The
// setting is per user and applies from the next launch; flags left out keep
// their saved value.
func runShellLimits(args []string, stdout io.Writer) error {
	if runtime.GOOS != "linux" {
		return errors.New("gentle-ai shell limits applies to the Linux transient unit only")
	}
	flags := flag.NewFlagSet("gentle-ai shell limits", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cpus := flags.String("cpus", "", "CPU count")
	memory := flags.String("memory", "", "memory limit, for example 12G")
	swap := flags.String("swap", "", "swap limit, for example 8G or 0")
	tasks := flags.String("tasks", "", "process and thread limit")
	reset := flags.Bool("reset", false, "restore the defaults")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w; run gentle-ai shell --help", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q; run gentle-ai shell --help", flags.Arg(0))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	changed := *cpus != "" || *memory != "" || *swap != "" || *tasks != ""
	if *reset && changed {
		return errors.New("--reset cannot be combined with other flags; run gentle-ai shell limits --reset alone")
	}
	if *reset {
		if err := shellinstaller.ResetUserUnitLimits(home); err != nil {
			return err
		}
	} else if changed {
		limits, err := shellinstaller.LoadUserUnitLimits(home)
		if err != nil {
			return err
		}
		if *cpus != "" {
			n, err := strconv.ParseUint(*cpus, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid --cpus %q; run gentle-ai shell limits --cpus N with a whole number", *cpus)
			}
			if host := uint64(runtime.NumCPU()); n > host {
				return fmt.Errorf("--cpus %d: this machine has %d CPUs; run gentle-ai shell limits --cpus %d or fewer", n, host, host)
			}
			limits.CPUs = n
		}
		if *memory != "" {
			if limits.MemoryMaxBytes, err = shellinstaller.ParseUserUnitSize(*memory); err != nil {
				return fmt.Errorf("--memory: %w", err)
			}
		}
		if *swap != "" {
			if limits.SwapMaxBytes, err = shellinstaller.ParseUserUnitSize(*swap); err != nil {
				return fmt.Errorf("--swap: %w", err)
			}
		}
		if *tasks != "" {
			if limits.TasksMax, err = strconv.ParseUint(*tasks, 10, 64); err != nil {
				return fmt.Errorf("invalid --tasks %q; run gentle-ai shell limits --tasks N with a whole number", *tasks)
			}
		}
		if err := shellinstaller.SaveUserUnitLimits(home, limits); err != nil {
			return err
		}
	}
	limits, err := shellinstaller.LoadUserUnitLimits(home)
	if err != nil {
		return err
	}
	return writeShellLimits(stdout, limits, home, changed || *reset)
}

func writeShellLimits(stdout io.Writer, limits shellinstaller.UserUnitLimits, home string, changed bool) error {
	source := shellinstaller.UserUnitLimitsPath(home)
	if limits == shellinstaller.DefaultUserUnitLimits() {
		source = "(defaults)"
	}
	text := fmt.Sprintf("Gentle Shell unit limits %s\n  cpus     %d\n  memory   %s\n  swap     %s\n  tasks    %d\n",
		source, limits.CPUs, formatShellSize(limits.MemoryMaxBytes), formatShellSize(limits.SwapMaxBytes), limits.TasksMax)
	if changed {
		text += "Applies from the next Gentle Shell launch.\n"
	}
	_, err := io.WriteString(stdout, text)
	return err
}

func formatShellSize(bytes uint64) string {
	for _, unit := range []struct {
		suffix string
		size   uint64
	}{{"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}} {
		if bytes != 0 && bytes%unit.size == 0 {
			return strconv.FormatUint(bytes/unit.size, 10) + unit.suffix
		}
	}
	return strconv.FormatUint(bytes, 10)
}
