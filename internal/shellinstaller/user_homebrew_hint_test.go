//go:build !windows

package shellinstaller

import (
	"errors"
	"strings"
	"testing"
)

func TestUserHomebrewHintNamesPrivateRemediationForHomebrewAncestors(t *testing.T) {
	for _, path := range []string{"/opt/homebrew", "/opt/homebrew/Cellar", "/opt/homebrew/Cellar/gentle-ai/4.5.1/bin", "/usr/local/Cellar", "/usr/local/Cellar/gentle-ai/4.5.1"} {
		hint := userHomebrewHint(path)
		if hint == "" {
			t.Fatalf("userHomebrewHint(%q) is empty, want a remediation", path)
		}
		for _, want := range []string{"Homebrew", "go install github.com/gentleman-programming/gentle-ai/v5/cmd/gentle-ai@v5.0.0", "user-owned 0755 directory", "do not change Homebrew permissions"} {
			if !strings.Contains(hint, want) {
				t.Fatalf("userHomebrewHint(%q) = %q, want it to contain %q", path, hint, want)
			}
		}
		for _, refused := range []string{"chmod", "chgrp", "chown"} {
			if strings.Contains(hint, refused) {
				t.Fatalf("userHomebrewHint(%q) suggests %q on shared Homebrew paths: %q", path, refused, hint)
			}
		}
	}
}

func TestUserHomebrewHintIgnoresOtherAncestors(t *testing.T) {
	for _, path := range []string{"/", "/opt", "/usr/local", "/usr/local/bin", "/opt/homebrewx", "/usr/local/Cellarx/gentle-ai", "/Users/me/go/bin", "/private/tmp/x"} {
		if hint := userHomebrewHint(path); hint != "" {
			t.Fatalf("userHomebrewHint(%q) = %q, want empty", path, hint)
		}
	}
}

func TestUserSupervisorAncestorRefusalCarriesHomebrewHintOnlyForHomebrew(t *testing.T) {
	err := userSupervisorAncestorRefusal("/opt/homebrew/Cellar", 0, 0775)
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "source" || failure.Cause == nil {
		t.Fatalf("refusal = %v, want a source PrivateRuntimeError", err)
	}
	for _, want := range []string{`supervisor ancestor refused: path="/opt/homebrew/Cellar" owner=0 mode=0775`, "user-owned 0755 directory", "do not change Homebrew permissions"} {
		if !strings.Contains(failure.Cause.Error(), want) {
			t.Fatalf("refusal %q lacks %q", failure.Cause, want)
		}
	}
	plain := userSupervisorAncestorRefusal("/Users/me/bin", 501, 0777)
	if !errors.As(plain, &failure) || strings.Contains(failure.Cause.Error(), "Homebrew") {
		t.Fatalf("non-Homebrew refusal must not carry the hint: %v", plain)
	}
}
