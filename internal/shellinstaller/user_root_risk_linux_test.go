//go:build linux

package shellinstaller

import (
	"errors"
	"strings"
	"testing"
)

func TestRootRiskAccepted(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"unset", "", false},
		{"wrong value", "yes", false},
		{"binding only", RootRiskBinding, true},
		{"binding with whitespace", RootRiskBinding + " ", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == "" {
				t.Setenv(RootRiskEnv, "")
			} else {
				t.Setenv(RootRiskEnv, tc.value)
			}
			if got := RootRiskAccepted(); got != tc.want {
				t.Fatalf("RootRiskAccepted() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRootRiskRequiredErrorNamesRemedies(t *testing.T) {
	message := (&RootRiskRequiredError{}).Error()
	for _, want := range []string{"adduser", "--accept-root-risk", "root powers"} {
		if !strings.Contains(message, want) {
			t.Fatalf("root refusal %q does not name %q", message, want)
		}
	}
}

func TestAsRootRiskRequired(t *testing.T) {
	if AsRootRiskRequired(nil) || AsRootRiskRequired(errors.New("other")) {
		t.Fatal("unrelated errors must not classify as the root refusal")
	}
	if !AsRootRiskRequired(&RootRiskRequiredError{}) {
		t.Fatal("the root refusal must classify as itself")
	}
}

func TestRootRiskGateUID(t *testing.T) {
	tests := []struct {
		name     string
		uid      int
		accepted bool
		wantGate bool
	}{
		{"non-root never gated", 1000, false, false},
		{"root without acceptance is gated", 0, false, true},
		{"root with acceptance passes", 0, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.accepted {
				t.Setenv(RootRiskEnv, RootRiskBinding)
			} else {
				t.Setenv(RootRiskEnv, "")
			}
			err := rootRiskGateUID(tc.uid)
			if got := err != nil; got != tc.wantGate {
				t.Fatalf("gate = %v (err %v), want gated %v", got, err, tc.wantGate)
			}
			if tc.wantGate && !AsRootRiskRequired(err) {
				t.Fatalf("gated error %v must be the informed root refusal", err)
			}
		})
	}
}

const rootKernelStatusOK = "CapInh:\t0000000000000000\nCapPrm:\t0000000000000000\nCapEff:\t0000000000000000\nCapAmb:\t0000000000000000\nUid:\t0\t0\t0\t0\nNoNewPrivs:\t1\n"

func TestUserKernelStatusRoot(t *testing.T) {
	tests := []struct {
		name   string
		status string
		bad    bool
		cause  string
	}{
		{"accepted root process passes", rootKernelStatusOK, false, ""},
		{"retained capability refused", strings.Replace(rootKernelStatusOK, "CapEff:\t0000000000000000", "CapEff:\t00000000ffffffffff", 1), true, "must be zero"},
		{"NoNewPrivs refused", strings.Replace(rootKernelStatusOK, "NoNewPrivs:\t1", "NoNewPrivs:\t0", 1), true, "UID or NoNewPrivs differs"},
		{"non-root Uid row refused", strings.Replace(rootKernelStatusOK, "Uid:\t0\t0\t0\t0", "Uid:\t1000\t1000\t1000\t1000", 1), true, "UID or NoNewPrivs differs"},
		{"duplicate key refused", rootKernelStatusOK + "CapEff:\t0000000000000000\n", true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := userKernelStatusRoot(tc.status)
			if (err != nil) != tc.bad {
				t.Fatalf("userKernelStatusRoot() error = %v, want bad %v", err, tc.bad)
			}
			if tc.cause != "" && !strings.Contains(err.Error(), tc.cause) {
				t.Fatalf("error %v does not name cause %q", err, tc.cause)
			}
		})
	}
}

func TestUserKernelStatusStillRefusesRoot(t *testing.T) {
	if err := userKernelStatus(rootKernelStatusOK, 0); err == nil {
		t.Fatal("the strict status check must keep refusing uid 0; only the entry route admits the accepted root binding")
	}
}

func TestUserEntryKernelStatusUID(t *testing.T) {
	nonRoot := strings.Replace(rootKernelStatusOK, "Uid:\t0\t0\t0\t0", "Uid:\t1000\t1000\t1000\t1000", 1)
	tests := []struct {
		name     string
		uid      int
		accepted bool
		status   string
		wantRoot bool
	}{
		{"non-root keeps the strict path", 1000, true, nonRoot, false},
		{"root without binding is the informed refusal", 0, false, rootKernelStatusOK, false},
		{"root with binding uses the root checks", 0, true, rootKernelStatusOK, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.accepted {
				t.Setenv(RootRiskEnv, RootRiskBinding)
			} else {
				t.Setenv(RootRiskEnv, "")
			}
			err := userEntryKernelStatusUID(tc.status, tc.uid)
			switch {
			case tc.wantRoot && err != nil:
				t.Fatalf("accepted root status must pass the identical physical checks, got %v", err)
			case !tc.wantRoot && tc.uid == 0 && !AsRootRiskRequired(err):
				t.Fatalf("unaccepted root must yield the informed refusal, got %v", err)
			case !tc.wantRoot && tc.uid != 0 && err != nil:
				t.Fatalf("non-root status must keep its ordinary behavior, got %v", err)
			}
		})
	}
}
