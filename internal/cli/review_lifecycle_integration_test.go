package cli

import (
	"bytes"
	"testing"
)

func TestDormantLifecycleContractPreservesExistingReviewSurface(t *testing.T) {
	for _, command := range []string{"start", "finalize", "validate", "status", "recover", "invalidate"} {
		t.Run(command, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview([]string{command, "--help"}, &output); err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(output.Bytes(), []byte("archive-sdd")) || bytes.Contains(output.Bytes(), []byte("remediation")) {
				t.Fatalf("%s help activated a PR2 route: %q", command, output.String())
			}
		})
	}
}
