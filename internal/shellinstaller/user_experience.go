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
	Context7            string `json:"context7,omitempty"`
	StrictTDD           string `json:"strict_tdd,omitempty"`
	GGA                 string `json:"gga,omitempty"`
	GGAProvider         string `json:"gga_provider,omitempty"`
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
	if p.Context7 != "" && p.Context7 != "on" && p.Context7 != "off" {
		return experienceError("choose on or off for Context7, or omit it to inherit")
	}
	if p.StrictTDD != "" && p.StrictTDD != "on" && p.StrictTDD != "off" {
		return experienceError("choose on or off for Strict TDD, or omit it to inherit")
	}
	return userValidateGGASelection(p.GGA, p.GGAProvider)
}

// Encode retains historical bytes when the new optional choices are omitted.
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
		"context7":             &profile.Context7,
		"strict_tdd":           &profile.StrictTDD,
		"gga":                  &profile.GGA,
		"gga_provider":         &profile.GGAProvider,
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
		if key == "context7" && *field == "" {
			return UserExperience{}, experienceError("an explicit Context7 choice must be on or off")
		}
		if key == "strict_tdd" && *field == "" {
			return UserExperience{}, experienceError("an explicit Strict TDD choice must be on or off")
		}
		if (key == "gga" || key == "gga_provider") && *field == "" {
			return UserExperience{}, experienceError("an explicit GGA choice/provider must not be empty")
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
