package shellinstaller

import (
	"strings"
	"testing"
)

func TestUserCodeGraphExperienceExplicitSelection(t *testing.T) {
	const base = `{"persona":"unmanaged","background_subagents":"off"`
	var confirmations [][]byte
	for _, choice := range []string{"on", "off"} {
		t.Run(choice, func(t *testing.T) {
			raw := base + `,"codegraph":"` + choice + `"}`
			profile, err := DecodeUserExperience(raw)
			if err != nil {
				t.Fatalf("explicit CodeGraph selection refused: %v", err)
			}
			encoded, err := profile.Encode()
			if err != nil || encoded != raw {
				t.Fatalf("CodeGraph selection changed %q: %v", encoded, err)
			}
			confirmation, err := userExperienceConfirmationData([]string{"same"}, &profile)
			if err != nil {
				t.Fatal(err)
			}
			confirmations = append(confirmations, confirmation)
		})
	}
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			profile := UserExperience{Persona: persona, BackgroundSubagents: background, GlobalRDD: "on", Context7: "on", StrictTDD: "on", GGA: "on", GGAProvider: "codex", CodeGraph: "on"}
			encoded, err := profile.Encode()
			if err != nil || len(encoded) > 256 {
				t.Fatal("complete short-provider profile cannot be transported", encoded, err)
			}
			decoded, err := DecodeUserExperience(encoded)
			if err != nil || decoded != profile {
				t.Fatal("CodeGraph dropped another choice", decoded, err)
			}
		}
	}
	if len(confirmations) == 2 && string(confirmations[0]) == string(confirmations[1]) {
		t.Fatal("physical confirmation did not bind the CodeGraph selection")
	}
}

func TestUserCodeGraphExperienceInvalidSelection(t *testing.T) {
	const prefix = `{"persona":"unmanaged","background_subagents":"off",`
	for _, tail := range []string{
		`"codegraph":""}`, `"codegraph":null}`, `"codegraph":true}`,
		`"codegraph":17}`, `"codegraph":[]}`, `"codegraph":{}}`,
		`"codegraph":"ON"}`, `"codegraph":"inherit"}`, `"codegraph":"clear"}`,
		`"codegraph":"on","codegraph":"off"}`,
		`"codegraph":"on","code\u0067raph":"off"}`,
		`"codegraph":"on","codegraph_mcp":"on"}`,
	} {
		t.Run(tail, func(t *testing.T) {
			profile, err := DecodeUserExperience(prefix + tail)
			if err == nil || profile != (UserExperience{}) {
				t.Fatalf("invalid selection yielded %+v: %v", profile, err)
			}
		})
	}
}

func TestUserCodeGraphExperienceHistoricalAndRefusal(t *testing.T) {
	for _, raw := range []string{
		`{"persona":"gentleman","background_subagents":"on"}`,
		`{"persona":"unmanaged","background_subagents":"off","context7":"off","strict_tdd":"on","gga":"on","gga_provider":"codex"}`,
	} {
		profile, err := DecodeUserExperience(raw)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := profile.Encode()
		if err != nil || encoded != raw {
			t.Fatalf("historical bytes changed %q: %v", encoded, err)
		}
	}
	for _, choice := range []string{"on", "off"} {
		raw := `{"persona":"unmanaged","background_subagents":"off","codegraph":"` + choice + `"}`
		profile, err := DecodeUserExperience(raw)
		if err != nil {
			t.Fatal(err)
		}
		profile.SDKSource = "main-preview" // Main consumes the same CodeGraph runtime as stock.
		if err := userExperienceSupported(&profile); err != nil {
			t.Fatalf("qualified Main CodeGraph was refused: %v", err)
		}
		if files, err := userStageExperienceFiles(&profile); err == nil || files != nil || !strings.Contains(err.Error(), "Main SDK") {
			t.Fatal("generic/Windows stage silently accepted CodeGraph", files, err)
		}
		bounded := raw + strings.Repeat(" ", 256-len(raw))
		if _, err := DecodeUserExperience(bounded); err != nil {
			t.Fatal("256-byte input ABI changed", err)
		}
		if _, err := DecodeUserExperience(bounded + " "); err == nil {
			t.Fatal("experience input bound widened")
		}
	}
	if files, err := userStageExperienceFiles(nil); err != nil || files != nil {
		t.Fatal("nil profile changed", files, err)
	}
}
