package shellinstaller

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestUserContext7ExperienceCodec(t *testing.T) {
	for _, mode := range []string{"on", "off"} {
		for _, rdd := range []string{"", "on", "off"} {
			t.Run(mode+"-rdd-"+rdd, func(t *testing.T) {
				fields := `"persona":"neutral","background_subagents":"off"`
				if rdd != "" {
					fields += `,"global_rdd":"` + rdd + `"`
				}
				want := `{` + fields + `,"context7":"` + mode + `"}`
				// Input ordering must not determine canonical confirmation bytes.
				profile, err := DecodeUserExperience(`{"context7":"` + mode + `",` + fields + `}`)
				if err != nil {
					t.Fatal(err)
				}
				got, err := profile.Encode()
				if err != nil || got != want {
					t.Fatalf("encoded %q want %q: %v", got, want, err)
				}
				if _, err := DecodeUserExperience(got); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestUserContext7ExperienceInvalid(t *testing.T) {
	prefix := `{"persona":"neutral","background_subagents":"off",`
	for _, tail := range []string{
		`"context7":""}`, `"context7":null}`, `"context7":true}`, `"context7":17}`,
		`"context7":{}}`, `"context7":[]}`, `"context7":"ON"}`, `"context7":"inherit"}`,
		`"context7":"on","context7":"off"}`, `"context7":"on","\u0063ontext7":"off"}`,
		`"context7":"on","Context7":"off"}`,
	} {
		t.Run(tail, func(t *testing.T) {
			profile, err := DecodeUserExperience(prefix + tail)
			if err == nil || profile != (UserExperience{}) {
				t.Fatalf("invalid choice yielded %+v: %v", profile, err)
			}
		})
	}
}

func TestUserContext7ExperienceLegacyBytes(t *testing.T) {
	for _, raw := range []string{
		`{"persona":"gentleman","background_subagents":"on"}`,
		`{"persona":"unmanaged","background_subagents":"off"}`,
		`{"persona":"neutral","background_subagents":"off","global_rdd":"on"}`,
		`{"persona":"neutral","background_subagents":"off","global_rdd":"off"}`,
	} {
		profile, err := DecodeUserExperience(raw)
		if err != nil {
			t.Fatal(err)
		}
		got, err := profile.Encode()
		if err != nil || got != raw {
			t.Fatalf("legacy bytes changed: %q want %q: %v", got, raw, err)
		}
	}
	got, err := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}.Encode()
	if err != nil || got != `{"persona":"gentleman","background_subagents":"on"}` {
		t.Fatalf("legacy fixture now owns Context7: %q: %v", got, err)
	}
}

func TestUserContext7ExperienceRefusesUnsupportedStage(t *testing.T) {
	skipDarwinUnqualifiedMain(t)
	for _, mode := range []string{"on", "off"} {
		profile, err := DecodeUserExperience(`{"persona":"unmanaged","background_subagents":"on","context7":"` + mode + `"}`)
		if err != nil {
			t.Fatal(err)
		}
		profile.SDKSource = "main-preview" // MCP plus Main is served by its own worker, not the stock PNPM stage.
		if err := userExperienceSupported(&profile); err != nil {
			t.Fatalf("qualified Main Context7 was refused: %v", err)
		}
		files, err := userStageExperienceFiles(&profile)
		if runtime.GOOS == "windows" {
			// The Windows installer stages the Main channel natively (#5644);
			// Context7 belongs to agent/mcp.json, never to a config file.
			assertStagedExperienceFiles(t, files, err, "background-subagents.json")
		} else if err == nil || files != nil || !strings.Contains(err.Error(), "Main SDK") {
			t.Fatalf("unsupported MCP staging silently accepted: %v, %v", files, err)
		}
		identity := []string{"destination", "mode", "identity"}
		data, err := userExperienceConfirmationData(identity, &profile)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := json.Unmarshal(data, &got); err != nil || len(got) != 4 || got[2] != "identity" || !strings.Contains(got[3], `"context7":"`+mode+`"`) {
			t.Fatalf("selection did not bind new preference: %s, %v", data, err)
		}
	}
}
