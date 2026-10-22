package shellinstaller

import (
	"encoding/json"
	"runtime"
)

// The optional named suffix keeps legacy positional fields and Windows channel
// selectors unambiguous. Decoding is inert; inspection still owns admission.
func userEntryExperience(args []string) ([]string, *UserExperience, error) {
	if len(args) < 2 || args[len(args)-2] != "--experience" {
		return args, nil, nil
	}
	profile, err := DecodeUserExperience(args[len(args)-1])
	if err != nil {
		return nil, nil, err
	}
	return args[:len(args)-2], &profile, nil
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
// legacy authenticated Main worker, including the CodeGraph runtime staged
// before the SDK overlay. GGA ON keeps the same refusal as the stock path.
func userQualifiedMainExperience(profile *UserExperience) bool {
	return profile != nil && runtime.GOOS == "linux" && runtime.GOARCH == "amd64" &&
		profile.SDKSource == "main-preview" && profile.GGA != "on"
}

// Both qualified distributions share the canonical private writers.
func userQualifiedPrivateChoices(profile *UserExperience) bool {
	return userQualifiedStockExperience(profile) || userQualifiedMainExperience(profile)
}

// The legacy authenticated worker acquires and installs GGA with its own setup
// instructions, so GGA ON keeps the same private writers on Linux amd64. The
// stock PNPM publication route still has no GGA producer and keeps refusing it.
func userQualifiedLegacyExperience(profile *UserExperience) bool {
	return profile != nil && runtime.GOOS == "linux" && runtime.GOARCH == "amd64"
}

func userValidateMainExperience(profile *UserExperience) error {
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		return nil
	}
	if runtime.GOOS == "windows" {
		return nil // The Windows native installer consumes the Main channel.
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
	if (profile.GlobalRDD != "" || profile.Context7 != "" || profile.StrictTDD != "") && runtime.GOOS != "windows" && !userQualifiedStockExperience(profile) && !userQualifiedLegacyExperience(profile) {
		return experienceError("selected additional experience preferences require a compatible installed runtime")
	}
	if profile.CodeGraph != "" && runtime.GOOS != "windows" && !userQualifiedStockExperience(profile) && !userQualifiedLegacyExperience(profile) {
		return experienceError("selected additional experience preferences require a compatible installed runtime")
	}
	if profile.GGA != "" && profile.GGA != "off" && !userQualifiedStockExperience(profile) && !userQualifiedLegacyExperience(profile) {
		return experienceError("selected additional experience preferences require a compatible installed runtime")
	}
	return nil
}
