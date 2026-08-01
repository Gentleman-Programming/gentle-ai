package main

import (
	"context"
	"errors"
	"fmt"
)

// Identity carries the full identity tuple for the v2 release-evidence contract.
// It is embedded in Results and used for the pre-measurement verification gate.
type Identity struct {
	TargetBinarySHA256  string `json:"target_binary_sha256"`
	EmbeddedVCSRevision string `json:"embedded_vcs_revision"`
	ClassifierVersion   string `json:"classifier_version"`
	SourceRevision      string `json:"source_revision"`
	VCSModified         bool   `json:"vcs_modified"`
	RuntimeGOOS         string `json:"runtime_goos"`
	RuntimeGOARCH       string `json:"runtime_goarch"`
	Mode                string `json:"mode"`
}

// Verify is the identity verification function. It is a variable so tests can
// replace it with a mock. By default it calls verifyImpl.
var Verify func(ctx context.Context, expected, observed Identity) error = verifyImpl

// verifyImpl is the default closed-box verification.
func verifyImpl(ctx context.Context, expected, observed Identity) error {
	var mismatches []string

	if expected.RuntimeGOOS != "" && observed.RuntimeGOOS != expected.RuntimeGOOS {
		mismatches = append(mismatches,
			fmt.Sprintf("runtime_goos: expected %q, got %q", expected.RuntimeGOOS, observed.RuntimeGOOS))
	}
	if expected.RuntimeGOARCH != "" && observed.RuntimeGOARCH != expected.RuntimeGOARCH {
		mismatches = append(mismatches,
			fmt.Sprintf("runtime_goarch: expected %q, got %q", expected.RuntimeGOARCH, observed.RuntimeGOARCH))
	}
	if expected.EmbeddedVCSRevision != "" && observed.EmbeddedVCSRevision != expected.EmbeddedVCSRevision {
		mismatches = append(mismatches,
			fmt.Sprintf("embedded_vcs_revision: expected %q, got %q",
				expected.EmbeddedVCSRevision, observed.EmbeddedVCSRevision))
	}
	if !expected.VCSModified && observed.VCSModified {
		mismatches = append(mismatches, "vcs_modified: expected false, got true")
	}
	if expected.TargetBinarySHA256 != "" && observed.TargetBinarySHA256 != expected.TargetBinarySHA256 {
		mismatches = append(mismatches,
			fmt.Sprintf("target_binary_sha256: expected %q, got %q",
				expected.TargetBinarySHA256, observed.TargetBinarySHA256))
	}

	if len(mismatches) > 0 {
		return errors.New("benchmark identity mismatch: " + joinErrors(mismatches))
	}
	return nil
}

func joinErrors(errs []string) string {
	switch len(errs) {
	case 0:
		return ""
	case 1:
		return errs[0]
	default:
		result := errs[0]
		for _, e := range errs[1:] {
			result += "; " + e
		}
		return result
	}
}
