package app

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/pkg/doctor"
	"github.com/gentleman-programming/gentle-ai/pkg/doctor/checker"
)

// DoctorFlags holds parsed flags for the doctor subcommand.
// Parsed separately to keep app.go clean.
type DoctorFlags struct {
	JSONOutput  bool
	FixMode     bool
	Categories  []string
	Verbose     bool
	ConfigPaths []string
}

// parseDoctorFlags parses arguments for the doctor subcommand.
// Returns flags and any parsing error.
func parseDoctorFlags(args []string) (DoctorFlags, error) {
	var flags DoctorFlags

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json" || arg == "-j":
			flags.JSONOutput = true
		case arg == "--fix" || arg == "-f":
			flags.FixMode = true
		case arg == "--verbose" || arg == "-v":
			flags.Verbose = true
		case arg == "--category" || arg == "-c":
			if i+1 >= len(args) {
				return flags, fmt.Errorf("--category requires a value")
			}
			flags.Categories = append(flags.Categories, args[i+1])
			i++
		case arg == "--config-path":
			if i+1 >= len(args) {
				return flags, fmt.Errorf("--config-path requires a value")
			}
			flags.ConfigPaths = append(flags.ConfigPaths, args[i+1])
			i++
		case arg == "--help" || arg == "-h":
			// Caller handles help display
			return flags, nil
		case strings.HasPrefix(arg, "-"):
			return flags, fmt.Errorf("unknown doctor flag: %s", arg)
		}
	}

	return flags, nil
}

// toPkgDoctorFlags converts internal DoctorFlags to pkg/doctor.DoctorFlags.
func (f DoctorFlags) toPkgDoctorFlags() doctor.DoctorFlags {
	return doctor.DoctorFlags{
		JSONOutput:  f.JSONOutput,
		FixMode:     f.FixMode,
		Categories:  f.Categories,
		Verbose:     f.Verbose,
		ConfigPaths: f.ConfigPaths,
	}
}

// createDoctorCheckers creates checkers for the given categories.
func createDoctorCheckers(categories []string) []doctor.Checker {
	// Parse categories the same way pkg/doctor does
	catMap := make(map[doctor.Category]bool)
	for _, c := range categories {
		lower := strings.ToLower(strings.TrimSpace(c))
		switch lower {
		case "hw", "hardware":
			catMap[doctor.CategoryHardware] = true
		case "sw", "software":
			catMap[doctor.CategorySoftware] = true
		case "cfg", "config":
			catMap[doctor.CategoryConfig] = true
		default:
			// Unknown category - ignore
		}
	}

	// Default to all if none specified
	if len(catMap) == 0 {
		catMap[doctor.CategoryHardware] = true
		catMap[doctor.CategorySoftware] = true
		catMap[doctor.CategoryConfig] = true
	}

	var checkers []doctor.Checker
	for cat := range catMap {
		switch cat {
		case doctor.CategoryHardware:
			checkers = append(checkers, checker.NewHardwareChecker())
		case doctor.CategorySoftware:
			checkers = append(checkers, checker.NewSoftwareChecker())
		case doctor.CategoryConfig:
			checkers = append(checkers, checker.NewConfigChecker())
		}
	}
	return checkers
}