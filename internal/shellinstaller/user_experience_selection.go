package shellinstaller

import (
	"encoding/json"
	"errors"
	"runtime"
)

// userEntryScopeBound bounds the canonical scope string: the widest known
// scope is far below this, so the bound only refuses crafted input.
const userEntryScopeBound = 128

// The optional named suffix keeps legacy positional fields and Windows channel
// selectors unambiguous. Decoding is inert; inspection still owns admission.
// The --scope suffix carries the component scope the inspected confirmation
// bound: without it the worker recomputes a different token and refuses the
// exact selection the operator confirmed.
func userEntryExperience(args []string) ([]string, *UserExperience, string, error) {
	var profile *UserExperience
	scope := ""
	seen := map[string]bool{}
	for len(args) >= 2 {
		flag, value := args[len(args)-2], args[len(args)-1]
		switch flag {
		case "--experience":
			if seen[flag] {
				return nil, nil, "", experienceError("duplicate experience choice")
			}
			seen[flag] = true
			decoded, err := DecodeUserExperience(value)
			if err != nil {
				return nil, nil, "", err
			}
			profile = &decoded
		case "--scope":
			if seen[flag] {
				return nil, nil, "", experienceError("duplicate update scope")
			}
			seen[flag] = true
			if value == "" || len(value) > userEntryScopeBound || !userValidRequestScope(value) {
				return nil, nil, "", experienceError("invalid update component scope")
			}
			scope = value
		default:
			return args, profile, scope, nil
		}
		args = args[:len(args)-2]
	}
	return args, profile, scope, nil
}

func userExperienceConfirmationData(fields []string, profile *UserExperience) ([]byte, error) {
	if profile != nil {
		encoded, err := profile.Encode()
		if err != nil {
			return nil, err
		}
		fields = append(fields, encoded)
	}
	return json.Marshal(fields)
}

// Only the stock Linux amd64 path has the qualified private consumers below.
// Request validation still owns fresh Separate paths and physical confirmation;
// this predicate never grants execution or changes a reviewed preference.
func userQualifiedStockExperience(profile *UserExperience) bool {
	return profile != nil && runtime.GOOS == "linux" && runtime.GOARCH == "amd64" &&
		profile.SDKSource != "main-preview" && profile.GGA != "on"
}

// Main preview delivers the same private preference consumers through the
// authenticated Main worker, including the CodeGraph runtime staged before the
// SDK overlay. The POSIX flow composes Main identically on Linux amd64 and
// macOS arm64 (pinned stock native member for each release). GGA ON keeps the
// same refusal as the stock path.
func userQualifiedMainExperience(profile *UserExperience) bool {
	return profile != nil && userMainPlatform() &&
		profile.SDKSource == "main-preview" && profile.GGA != "on"
}

// userMainPlatform names the POSIX platforms whose shared install flow and
// owned provisioner compose the Main SDK.
func userMainPlatform() bool {
	return (runtime.GOOS == "linux" && runtime.GOARCH == "amd64") || (runtime.GOOS == "darwin" && runtime.GOARCH == "arm64")
}

// macOS arm64 Stable has no native runtime, but the darwin flow stages every
// base and private preference through the same canonical writers as Linux,
// including the pinned darwin-arm64 CodeGraph library. Main preview is the
// separate userQualifiedMainExperience qualification.
func userQualifiedDarwinExperience(profile *UserExperience) bool {
	return profile != nil && runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" &&
		profile.SDKSource != "main-preview" && profile.GGA != "on"
}

// Every qualified distribution shares the canonical private writers.
func userQualifiedPrivateChoices(profile *UserExperience) bool {
	return userQualifiedStockExperience(profile) || userQualifiedMainExperience(profile) || userQualifiedDarwinExperience(profile)
}

// The TUI names this refusal verbatim; the CLI has no flag that selects it.
var errUserDarwinMain = errors.New("Main preview is qualified on macOS arm64 only; choose Stable to install")

// The legacy authenticated worker acquires and installs GGA with its own setup
// instructions, so GGA ON keeps the same private writers on Linux amd64. The
// stock PNPM publication route still has no GGA producer and keeps refusing it.
func userQualifiedLegacyExperience(profile *UserExperience) bool {
	return profile != nil && runtime.GOOS == "linux" && runtime.GOARCH == "amd64"
}

func userValidateMainExperience(profile *UserExperience) error {
	if userMainPlatform() {
		return nil
	}
	if runtime.GOOS == "windows" {
		return nil // The Windows native installer consumes the Main channel.
	}
	if runtime.GOOS == "darwin" {
		return errUserDarwinMain
	}
	return experienceError("SDK Main + stock provider is qualified on Linux amd64 today; macOS support is being completed")
}

// A valid DTO alone is not admission: unsupported distributions/combinations
// retain their refusal, while the qualified route materializes every selection.
func userExperienceSupported(profile *UserExperience) error {
	if profile == nil {
		return nil
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	if profile.SDKSource == "main-preview" {
		return userValidateMainExperience(profile)
	}
	qualified := userQualifiedStockExperience(profile) || userQualifiedLegacyExperience(profile) || userQualifiedDarwinExperience(profile)
	if (profile.GlobalRDD != "" || profile.Context7 != "" || profile.StrictTDD != "" || profile.CodeGraph != "") && runtime.GOOS != "windows" && !qualified {
		return experienceError("selected additional experience preferences require a compatible installed runtime")
	}
	if profile.GGA != "" && profile.GGA != "off" && !userQualifiedStockExperience(profile) && !userQualifiedLegacyExperience(profile) {
		return experienceError("selected additional experience preferences require a compatible installed runtime")
	}
	return nil
}
