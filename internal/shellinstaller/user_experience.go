package shellinstaller

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
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
	CodeGraph           string `json:"codegraph,omitempty"`
	SDKSource           string `json:"sdk_source,omitempty"`
}

// DefaultUserExperience starts a new independent, inert base selection.
func DefaultUserExperience() UserExperience {
	return UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
}

// userForceGGAInert retires GGA on every shell install: the retained struct
// fields are forced to OFF with an empty provider so an old profile, manifest
// bit or environment can never silently enable GGA again.
// Canonical per-install update channels. The SDK source preference is an
// update-channel preference, not a component: Main preview routes update
// checks at channel main, Stable at channel stable. Installing or replacing
// components stays outside the preference apply path.
const (
	userChannelStable = "stable"
	userChannelMain   = "main"
)

// userUpdateChannelRelPath is the target-relative canonical file that records
// one installation's SDK update channel. It is a small regular preference
// file inside config, so the installed-tree inventory rules accept it on every
// platform without a special name grant.
const userUpdateChannelRelPath = "config/update-channel.json"

func userUpdateChannelPath(target string) string {
	return filepath.Join(target, filepath.FromSlash(userUpdateChannelRelPath))
}

// userUpdateChannelBytes are the exact canonical bytes persisted for a
// channel; single-key marshalling keeps the encoding deterministic.
func userUpdateChannelBytes(channel string) []byte {
	data, _ := json.Marshal(map[string]string{"channel": channel})
	return data
}

// userUpdateChannelFromBytes accepts only the two canonical channel values.
func userUpdateChannelFromBytes(data []byte) (string, bool) {
	if len(data) > 4096 {
		return "", false
	}
	var value struct {
		Channel string `json:"channel"`
	}
	if json.Unmarshal(data, &value) != nil {
		return "", false
	}
	if value.Channel != userChannelStable && value.Channel != userChannelMain {
		return "", false
	}
	return value.Channel, true
}

// userReadUpdateChannel returns the persisted channel and whether a canonical
// preference was found. Missing, oversized or non-canonical bytes mean no
// recorded preference; readers fall back to the installed channel authority.
func userReadUpdateChannel(target string) (string, bool) {
	data, err := os.ReadFile(userUpdateChannelPath(target))
	if err != nil {
		return "", false
	}
	return userUpdateChannelFromBytes(data)
}

// userSDKSourceChannel maps an SDK source selection onto its update channel.
func userSDKSourceChannel(source string) (string, error) {
	switch source {
	case "main-preview":
		return userChannelMain, nil
	case "stable":
		return userChannelStable, nil
	}
	return "", experienceError("choose stable or main-preview for SDK source, or omit it for stock")
}

// userChannelSDKSource maps a persisted channel back onto the SDK source.
func userChannelSDKSource(channel string) (string, bool) {
	switch channel {
	case userChannelMain:
		return "main-preview", true
	case userChannelStable:
		return "stable", true
	}
	return "", false
}

func userForceGGAInert(profile *UserExperience) {
	if profile == nil {
		return
	}
	profile.GGA, profile.GGAProvider = "off", ""
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
	if p.CodeGraph != "" && p.CodeGraph != "on" && p.CodeGraph != "off" {
		return experienceError("choose on or off for CodeGraph, or omit it to inherit")
	}
	if p.SDKSource != "" && p.SDKSource != "stable" && p.SDKSource != "main-preview" {
		return experienceError("choose stable or main-preview for SDK source, or omit it for stock")
	}
	// GGA is retired from the shell experience. The retained struct fields stay
	// inert for transport compatibility; provisioning and recording are gone.
	return nil
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
		"codegraph":            &profile.CodeGraph,
		"sdk_source":           &profile.SDKSource,
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
		if key == "sdk_source" && *field == "" {
			return UserExperience{}, experienceError("an explicit SDK source must be stable or main-preview")
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
		if key == "codegraph" && *field == "" {
			return UserExperience{}, experienceError("an explicit CodeGraph choice must be on or off")
		}
		// Retired "gga"/"gga_provider" keys still decode inertly so an older
		// profile keeps loading; the install flow forces them off before staging.
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
