package reviewerprovider

import (
	"bytes"
	"context"
	"testing"
)

type testAdapter struct{}

func (testAdapter) Review(context.Context, Invocation) ([]byte, error) { return nil, nil }

var _ Adapter = testAdapter{}

func TestInvocationPreservesPromptBytesWithoutAliasing(t *testing.T) {
	prompt := []byte("frozen provider prompt\x00\xff")
	invocation := NewInvocation(prompt)
	prompt[0] = 'x'

	first := invocation.Prompt()
	if !bytes.Equal(first, []byte("frozen provider prompt\x00\xff")) {
		t.Fatalf("Prompt() = %q, want original provider bytes", first)
	}
	first[0] = 'y'
	if got := invocation.Prompt(); !bytes.Equal(got, []byte("frozen provider prompt\x00\xff")) {
		t.Fatalf("Prompt() leaked mutable invocation bytes: %q", got)
	}
}
