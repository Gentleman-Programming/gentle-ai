package shellinstaller

import "encoding/json"

func userValidateStageExperience(profile *UserExperience) error {
	if err := userValidateStagePreferences(profile); err != nil {
		return err
	}
	if profile != nil && profile.GGA != "" {
		return experienceError("explicit GGA requires a qualified optional private CLI/config stage")
	}
	if profile != nil && profile.StrictTDD != "" {
		return experienceError("explicit Strict TDD requires a supported private SDK stage")
	}
	if profile != nil && profile.Context7 != "" {
		return experienceError("explicit Context7 requires a supported private MCP stage")
	}
	return nil
}

// Typed preferences and native RDD refusal are shared. POSIX preparation also
// validates its actual mode; generic/Windows writers retain MCP/Strict TDD/GGA refusals.
func userValidateStagePreferences(profile *UserExperience) error {
	if profile == nil {
		return nil
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	if profile.GlobalRDD != "" {
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
