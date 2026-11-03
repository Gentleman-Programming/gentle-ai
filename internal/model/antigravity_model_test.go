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
