package shellinstaller

import "encoding/json"

func userValidateStageExperience(profile *UserExperience) error {
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
