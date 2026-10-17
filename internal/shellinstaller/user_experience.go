package shellinstaller

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// UserExperience is an inert selection of canonical preference values.
// Encoding does not install, write configuration or grant execution authority.
// Unmanaged means no persona write, not an SDK persona named custom.
type UserExperience struct {
	Persona             string `json:"persona"`
	BackgroundSubagents string `json:"background_subagents"`
	GlobalRDD           string `json:"global_rdd,omitempty"`
}

func (p UserExperience) Validate() error {
	if p.Persona != "gentleman" && p.Persona != "neutral" && p.Persona != "unmanaged" {
		return experienceError("choose gentleman, neutral or unmanaged persona")
	}
	if p.BackgroundSubagents != "on" && p.BackgroundSubagents != "off" {
		return experienceError("choose on or off for background subagents")
	}
	if p.GlobalRDD != "" && p.GlobalRDD != "on" && p.GlobalRDD != "off" {
		return experienceError("choose on or off for global RDD, or omit it to inherit")
	}
	return nil
}

// Encode retains the historical two-key bytes when global RDD is omitted.
func (p UserExperience) Encode() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(p)
	return string(data), err
}

func DecodeUserExperience(raw string) (UserExperience, error) {
	var profile UserExperience
	if len(raw) > 256 {
		return profile, experienceError("experience exceeds its bounded input size")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return profile, experienceError("experience must be a JSON object")
	}
	fields := map[string]*string{
		"persona":              &profile.Persona,
		"background_subagents": &profile.BackgroundSubagents,
		"global_rdd":           &profile.GlobalRDD,
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return UserExperience{}, err
		}
		key, ok := token.(string)
		field, supported := fields[key]
		if !ok || !supported || seen[key] {
			return UserExperience{}, experienceError("duplicate or unsupported experience choice")
		}
		seen[key] = true
		if err := decoder.Decode(field); err != nil {
			return UserExperience{}, err
		}
		if key == "global_rdd" && *field == "" {
			return UserExperience{}, experienceError("an explicit global RDD choice must be on or off")
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return UserExperience{}, experienceError("unterminated experience object")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return UserExperience{}, experienceError("trailing experience data")
	}
	if err := profile.Validate(); err != nil {
		return UserExperience{}, err
	}
	return profile, nil
}

func experienceError(message string) error {
	return errors.New(message + "; run gentle-ai shell install --help")
}
