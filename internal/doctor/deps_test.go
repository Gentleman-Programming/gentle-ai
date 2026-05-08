package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

func TestDuplicateBinaryCheck(t *testing.T) {
	tests := []struct {
		name       string
		binary     string
		fakePaths  []string
		wantStatus verify.CheckStatus
		wantSubstr string // expected substring in Message
	}{
		{
			name:       "single copy healthy",
			binary:     "engram",
			fakePaths:  []string{"/usr/local/bin/engram"},
			wantStatus: verify.CheckStatusPassed,
			wantSubstr: "single copy",
		},
		{
			name:       "not found warning",
			binary:     "engram",
			fakePaths:  []string{},
			wantStatus: verify.CheckStatusWarning,
			wantSubstr: "not found",
		},
		{
			name:       "two copies shadowing",
			binary:     "engram",
			fakePaths:  []string{"/usr/local/bin/engram", "/home/user/go/bin/engram"},
			wantStatus: verify.CheckStatusWarning,
			wantSubstr: "2 copies",
		},
		{
			name:       "three copies shadowing",
			binary:     "gga",
			fakePaths:  []string{"/a/gga", "/b/gga", "/c/gga"},
			wantStatus: verify.CheckStatusWarning,
			wantSubstr: "3 copies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := findAllBinaries
			findAllBinaries = func(name string) []string {
				if name == tt.binary {
					return tt.fakePaths
				}
				return nil
			}
			defer func() { findAllBinaries = original }()

			check := duplicateBinaryCheck(tt.binary)
			result := check(context.Background())

			if result.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", result.Status, tt.wantStatus)
			}
			if !strings.Contains(result.Message, tt.wantSubstr) {
				t.Errorf("Message = %q, want substring %q", result.Message, tt.wantSubstr)
			}
		})
	}
}

func TestDuplicateBinaryCheckRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	findAllBinaries = func(string) []string {
		t.Fatal("should not be called when context is cancelled")
		return nil
	}
	defer func() { findAllBinaries = nil }()

	check := duplicateBinaryCheck("engram")
	result := check(ctx)

	if result.Status != verify.CheckStatusSkipped {
		t.Errorf("Status = %q, want skipped on cancelled context", result.Status)
	}
}

func TestDuplicateBinaryCheckNilLookup(t *testing.T) {
	original := findAllBinaries
	findAllBinaries = nil
	defer func() { findAllBinaries = original }()

	check := duplicateBinaryCheck("engram")
	result := check(context.Background())

	if result.Status != verify.CheckStatusSkipped {
		t.Errorf("Status = %q, want skipped when lookup not configured", result.Status)
	}
}

func TestDuplicateBinaryCheckDetailsMarkActiveShadowed(t *testing.T) {
	findAllBinaries = func(string) []string {
		return []string{"/usr/local/bin/engram", "/home/user/go/bin/engram"}
	}
	defer func() { findAllBinaries = nil }()

	check := duplicateBinaryCheck("engram")
	result := check(context.Background())

	if len(result.Details) == 0 {
		t.Fatal("expected details for duplicate binary")
	}

	joined := strings.Join(result.Details, "\n")
	if !strings.Contains(joined, "active") {
		t.Errorf("details should mark first copy as active:\n%s", joined)
	}
	if !strings.Contains(joined, "shadowed") {
		t.Errorf("details should mark extra copies as shadowed:\n%s", joined)
	}
}

func TestNewDepsChecks(t *testing.T) {
	checks := NewDepsChecks()
	if len(checks) != len(managedBinaries) {
		t.Fatalf("len(checks) = %d, want %d", len(checks), len(managedBinaries))
	}
	for i, c := range checks {
		if c.Category != "deps" {
			t.Errorf("checks[%d].Category = %q, want deps", i, c.Category)
		}
	}
}
