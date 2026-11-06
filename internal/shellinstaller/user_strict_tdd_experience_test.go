package shellinstaller

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestUserStrictTDDExperienceCodec(t *testing.T) {
	skipDarwinUnqualifiedMain(t)
	const fields = `"persona":"unmanaged","background_subagents":"off"`
	for _, choice := range []string{"on", "off"} {
		t.Run(choice, func(t *testing.T) {
			raw := `{` + fields + `,"context7":"off","strict_tdd":"` + choice + `"}`
			profile, err := DecodeUserExperience(`{"strict_tdd":"` + choice + `",` + fields + `,"context7":"off"}`)
			if err != nil {
				t.Fatal(err)
			}
			got, err := profile.Encode()
			if err != nil || got != raw {
				t.Fatalf("canonical encoding %q want %q: %v", got, raw, err)
			}
			data, err := userExperienceConfirmationData([]string{"target", "mode", "identity"}, &profile)
			var identity []string
			if err != nil || json.Unmarshal(data, &identity) != nil || len(identity) != 4 || identity[3] != raw {
				t.Fatalf("confirmation did not bind Strict TDD: %s: %v", data, err)
			}
			profile.SDKSource = "main-preview"
			if err := userExperienceSupported(&profile); err != nil {
				t.Fatalf("qualified Main + Strict TDD was refused: %v", err)
			}
		})
	}
	on, _ := DecodeUserExperience(`{` + fields + `,"strict_tdd":"on"}`)
	off, _ := DecodeUserExperience(`{` + fields + `,"strict_tdd":"off"}`)
	first, _ := userExperienceConfirmationData([]string{"same"}, &on)
	second, _ := userExperienceConfirmationData([]string{"same"}, &off)
	if string(first) == string(second) {
		t.Fatal("ON and OFF share confirmation bytes")
	}
}

func TestUserStrictTDDExperienceRejectsInvalid(t *testing.T) {
	const prefix = `{"persona":"unmanaged","background_subagents":"off",`
	for _, tail := range []string{
		`"strict_tdd":""}`, `"strict_tdd":null}`, `"strict_tdd":true}`, `"strict_tdd":17}`,
		`"strict_tdd":[]}`, `"strict_tdd":{}}`, `"strict_tdd":"ON"}`, `"strict_tdd":"inherit"}`,
		`"strict_tdd":"on","strict_tdd":"off"}`, `"strict_tdd":"on","\u0073trict_tdd":"off"}`,
	} {
		profile, err := DecodeUserExperience(prefix + tail)
		if err == nil || profile != (UserExperience{}) {
			t.Fatalf("invalid selection yielded %+v: %v", profile, err)
		}
	}
}

func TestUserStrictTDDExperienceHistoricalBytes(t *testing.T) {
	for _, raw := range []string{
		`{"persona":"gentleman","background_subagents":"on"}`,
		`{"persona":"unmanaged","background_subagents":"off","context7":"off"}`,
		`{"persona":"neutral","background_subagents":"on","global_rdd":"off","context7":"on"}`,
	} {
		profile, err := DecodeUserExperience(raw)
		if err != nil {
			t.Fatal(err)
		}
		got, err := profile.Encode()
		if err != nil || got != raw {
			t.Fatalf("historical bytes changed: %q: %v", got, err)
		}
	}
}

func TestUserStrictTDDExperienceGenericStageRefused(t *testing.T) {
	for _, choice := range []string{"on", "off"} {
		profile, err := DecodeUserExperience(`{"persona":"unmanaged","background_subagents":"off","strict_tdd":"` + choice + `"}`)
		if err != nil {
			t.Fatal(err)
		}
		profile.SDKSource = "main-preview" // Generic staging still cannot compose this SDK.
		files, err := userStageExperienceFiles(&profile)
		if runtime.GOOS == "windows" {
			// The Windows installer stages the Main channel natively (#5644),
			// including the explicit Strict TDD policy file.
			assertStagedExperienceFiles(t, files, err, "background-subagents.json", "strict-tdd.json")
			if !strings.Contains(string(files[1].data), `"policy": "`+choice+`"`) {
				t.Fatalf("Windows Strict TDD policy differs: %s", files[1].data)
			}
			continue
		}
		if err == nil || files != nil || !strings.Contains(err.Error(), "Main SDK") {
			t.Fatal("unqualified generic stage accepted choice", files, err)
		}
	}
}
