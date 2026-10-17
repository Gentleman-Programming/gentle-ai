package shellinstaller

import (
	"encoding/json"
	"errors"
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

func experienceError(message string) error {
	return errors.New(message + "; run gentle-ai shell install --help")
}
