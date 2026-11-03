package model

import (
	"reflect"
	"testing"
)

func TestAntigravityModelIDFallsBackToInherit(t *testing.T) {
	for tier, want := range map[AntigravityModelTier]string{
		AntigravityModelInherit:   "inherit",
		AntigravityModelFlashLite: "flash_lite",
		AntigravityModelFlash:     "flash",
		AntigravityModelPro:       "pro",
		"":                        "inherit",
		"opus":                    "inherit",
	} {
		if got := AntigravityModelID(tier); got != want {
			t.Errorf("AntigravityModelID(%q) = %q, want %q", tier, got, want)
		}
	}
}

func TestAntigravityModelPresetsCoverEveryRole(t *testing.T) {
	presets := map[string]map[string]AntigravityModelTier{
		"inherit":     AntigravityModelPresetInherit(),
		"performance": AntigravityModelPresetPerformance(),
		"economy":     AntigravityModelPresetEconomy(),
	}
	for name, preset := range presets {
		if len(preset) != len(AntigravityModelRoles) {
			t.Errorf("%s preset roles = %v, want exactly %v", name, preset, AntigravityModelRoles)
		}
		for _, role := range AntigravityModelRoles {
			if !preset[role].Valid() {
				t.Errorf("%s preset has invalid tier for %s", name, role)
			}
		}
	}
	for _, role := range AntigravityModelRoles {
		if presets["inherit"][role] != AntigravityModelInherit {
			t.Errorf("default preset must inherit for %s", role)
		}
	}
}

func TestAntigravityModelTiersStringRoundTrip(t *testing.T) {
	typed := map[string]AntigravityModelTier{"jd-judge-a": AntigravityModelPro, "jd-fix-agent": AntigravityModelFlash}
	plain := AntigravityModelTiersToStrings(typed)
	if want := map[string]string{"jd-judge-a": "pro", "jd-fix-agent": "flash"}; !reflect.DeepEqual(plain, want) {
		t.Fatalf("AntigravityModelTiersToStrings = %v, want %v", plain, want)
	}
	if got := AntigravityModelTiersFromStrings(plain); !reflect.DeepEqual(got, typed) {
		t.Fatalf("AntigravityModelTiersFromStrings = %v, want %v", got, typed)
	}
	if got := AntigravityModelTiersFromStrings(map[string]string{"jd-judge-a": "ultra"}); got != nil {
		t.Fatalf("unknown persisted tiers must be dropped so rendering inherits, got %v", got)
	}
	if AntigravityModelTiersToStrings(nil) != nil || AntigravityModelTiersFromStrings(nil) != nil {
		t.Fatal("empty assignments must stay nil")
	}
}
