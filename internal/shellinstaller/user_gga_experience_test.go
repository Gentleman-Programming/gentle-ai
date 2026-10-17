package shellinstaller

import (
	"strings"
	"testing"
)

func TestUserGGAExperienceExplicitSelection(t *testing.T) {
	const base = `"persona":"unmanaged","background_subagents":"off"`
	for _, provider := range []string{"claude", "gemini", "codex", "opencode", "cursor", "kilo", "kiro", "ollama:llama3.2", "lmstudio", "github:gpt-4o", "minimax"} {
		t.Run(provider, func(t *testing.T) {
			raw := `{` + base + `,"gga":"on","gga_provider":"` + provider + `"}`
			profile, err := DecodeUserExperience(raw)
			if err != nil {
				t.Fatal(err)
			}
			if profile.GGA != "on" || profile.GGAProvider != provider {
				t.Fatalf("selection changed: %+v", profile)
			}
			got, err := profile.Encode()
			if err != nil || got != raw {
				t.Fatalf("canonical selection %q: %v", got, err)
			}
			on, err := userExperienceConfirmationData([]string{"same"}, &profile)
			if err != nil {
				t.Fatal(err)
			}
			off := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", GGA: "off"}
			other, err := userExperienceConfirmationData([]string{"same"}, &off)
			if err != nil || string(on) == string(other) {
				t.Fatal("confirmation did not bind explicit GGA configuration", err)
			}
			profile.GGAProvider = "codex:other-model"
			changed, err := userExperienceConfirmationData([]string{"same"}, &profile)
			if err != nil || string(changed) == string(on) {
				t.Fatal("confirmation did not bind provider", err)
			}
		})
	}
}

func TestUserGGAExperienceInvalidSelection(t *testing.T) {
	const prefix = `{"persona":"unmanaged","background_subagents":"off",`
	for _, tail := range []string{
		`"gga":"on"}`, `"gga":"on","gga_provider":"pi"}`, `"gga":"off","gga_provider":"codex"}`,
		`"gga_provider":"codex"}`, `"gga":""}`, `"gga":null}`, `"gga":true}`, `"gga":17}`,
		`"gga":[]}`, `"gga":{}}`, `"gga":"ON"}`, `"gga":"inherit"}`,
		`"gga":"on","gga":"off","gga_provider":"codex"}`,
		`"gga":"on","gga_provider":"codex","\u0067ga_provider":"claude"}`,
		`"gga":"on","gga_provider":null}`, `"gga":"on","gga_provider":""}`,
		`"gga":"on","gga_provider":true}`, `"gga":"on","gga_provider":{}}`,
		`"gga":"on","gga_provider":"kiro:model"}`, `"gga":"on","gga_provider":"ollama"}`,
		`"gga":"on","gga_provider":"github"}`, `"gga":"on","gga_provider":"codex:"}`,
		`"gga":"on","gga_provider":"codex:model\nPROVIDER=claude"}`,
		`"gga":"on","gga_provider":"codex:$(touch /tmp/injected)"}`,
		`"gga":"on","gga_provider":"codex:` + strings.Repeat("a", 64) + `"}`,
	} {
		t.Run(tail, func(t *testing.T) {
			profile, err := DecodeUserExperience(prefix + tail)
			if err == nil || profile != (UserExperience{}) {
				t.Fatalf("invalid configuration yielded %+v: %v", profile, err)
			}
		})
	}
}

func TestUserGGAExperienceHistoricalAndRefusal(t *testing.T) {
	for _, raw := range []string{
		`{"persona":"gentleman","background_subagents":"on"}`,
		`{"persona":"unmanaged","background_subagents":"off","context7":"off","strict_tdd":"on"}`,
		`{"persona":"unmanaged","background_subagents":"off","gga":"off"}`,
	} {
		profile, err := DecodeUserExperience(raw)
		if err != nil {
			t.Fatal(err)
		}
		got, err := profile.Encode()
		if err != nil || got != raw {
			t.Fatalf("historical/OFF bytes changed %q: %v", got, err)
		}
	}
	for _, profile := range []UserExperience{
		{Persona: "unmanaged", BackgroundSubagents: "off", GGA: "off"},
		{Persona: "unmanaged", BackgroundSubagents: "off", GGA: "on", GGAProvider: "codex"},
	} {
		if err := userExperienceSupported(&profile); err == nil {
			t.Fatal("transport granted public installation")
		}
		if files, err := userStageExperienceFiles(&profile); err == nil || files != nil || !strings.Contains(err.Error(), "GGA") {
			t.Fatal("unqualified generic/Windows stage accepted GGA", files, err)
		}
	}
	if err := userValidateStagePreferences(nil); err != nil {
		t.Fatal("nil profile changed", err)
	}
}
