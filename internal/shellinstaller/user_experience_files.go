package shellinstaller

import (
	"encoding/json"
	"runtime"
)

func userValidateStageExperience(profile *UserExperience) error {
	if err := userValidateStagePreferences(profile); err != nil {
		return err
	}
	if userQualifiedStockExperience(profile) || userQualifiedDarwinExperience(profile) {
		return nil
	}
	if profile != nil && profile.SDKSource == "main-preview" && runtime.GOOS != "windows" {
		return experienceError("Main SDK requires its authenticated runtime producer")
	}
	if profile != nil && profile.CodeGraph != "" && runtime.GOOS != "windows" {
		return experienceError("explicit CodeGraph requires a qualified private library/runtime stage")
	}
	if profile != nil && profile.GGA != "" && !(runtime.GOOS == "windows" && profile.GGA == "off") {
		return experienceError("explicit GGA requires a qualified optional private CLI/config stage")
	}
	if profile != nil && profile.StrictTDD != "" && runtime.GOOS != "windows" {
		return experienceError("explicit Strict TDD requires a supported private SDK stage")
	}
	if profile != nil && profile.Context7 != "" && runtime.GOOS != "windows" {
		return experienceError("explicit Context7 requires a supported private MCP stage")
	}
	return nil
}

// Typed preferences and distribution qualification are shared. Request/stage
// callers still validate fresh Separate mode; other platforms retain refusals.
func userValidateStagePreferences(profile *UserExperience) error {
	if profile == nil {
		return nil
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	if profile.GlobalRDD != "" && runtime.GOOS != "windows" && !userQualifiedPrivateChoices(profile) && !userQualifiedLegacyExperience(profile) {
		return experienceError("explicit global RDD requires a supported native distribution")
	}
	return nil
}

type userExperienceStageFile struct {
	name string
	data []byte
}

// userStageExperienceFiles has no filesystem effects. Native writers consume
// these canonical bytes; custody and publication stay with the native writer.
// Unmanaged omits persona entirely, not an entry to inspect or replace.
func userStageExperienceFiles(profile *UserExperience) ([]userExperienceStageFile, error) {
	if err := userValidateStageExperience(profile); err != nil {
		return nil, err
	}
	return userExperienceConfigFiles(profile)
}

// Serialize only the config-home preferences of an already validated profile.
// Context7 belongs to agent/mcp.json, not a dropped selection or config key.
func userExperienceConfigFiles(profile *UserExperience) ([]userExperienceStageFile, error) {
	if profile == nil {
		return nil, nil
	}
	values := []struct {
		name  string
		value any
	}{
		{"background-subagents.json", struct {
			Schema string `json:"schema"`
			Policy string `json:"policy"`
		}{"gentle-pi.background-subagents/v1", profile.BackgroundSubagents}},
	}
	if profile.Persona != "unmanaged" {
		values = append(values, struct {
			name  string
			value any
		}{"persona.json", struct {
			Mode string `json:"mode"`
		}{profile.Persona}})
	}
	if profile.StrictTDD != "" {
		values = append(values, struct {
			name  string
			value any
		}{"strict-tdd.json", struct {
			Schema string `json:"schema"`
			Policy string `json:"policy"`
		}{"gentle-pi.strict-tdd/v1", profile.StrictTDD}})
	}
	files := make([]userExperienceStageFile, 0, len(values))
	for _, value := range values {
		data, err := json.MarshalIndent(value.value, "", "  ")
		if err != nil {
			return nil, err
		}
		files = append(files, userExperienceStageFile{value.name, append(data, '\n')})
	}
	return files, nil
}

// UserKernelGate is the kernel-qualification seam for the shell installer
// entry points, mirroring the darwin userKernelGate pattern: tests stub it to
// exercise flow logic on hosts the real guard refuses (Windows Server
// runners) and must restore the previous value. It is nil in production, so
// the Windows check in UserKernelCheck always runs its real guard; the Linux
// and darwin checks keep their own platforms' guards.
var UserKernelGate func() error
