package shellinstaller

import "encoding/json"

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

// A transport or valid DTO is not proof that the selected runtime applies it.
// Keep every non-nil profile refused until its canonical effects are supported.
func userExperienceSupported(profile *UserExperience) error {
	if profile == nil {
		return nil
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	return experienceError("experience preferences are not supported by this installer yet")
}
