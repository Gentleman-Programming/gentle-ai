package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestSDDAttemptRetiredOperationsDoNotAdvertiseRuntimeGovernance(t *testing.T) {
	for _, operation := range []string{"status", "begin", "finish", "acquire", "settle", "reset", "rescope", "supersede", "repair", "handoff"} {
		t.Run(operation, func(t *testing.T) {
			var output bytes.Buffer
			err := RunSDDAttempt([]string{operation, "--help"}, &output)
			if err == nil || strings.Contains(output.String(), "Usage: gentle-ai sdd-attempt "+operation) {
				t.Fatalf("retired operation remains available: %s, err=%v output=%s", operation, err, output.String())
			}
		})
	}
}
