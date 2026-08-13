// provider_test.go — change #3138, slice 2, RED-first (task 2.1): the typed
// Provider API must render byte-identical output to the freeze-pinned
// pre-extraction path for equal input (REQ-RPC-2, SEN-RPC-3). provider.go
// was missing until task 2.2, so this file failed to compile by construction.
package advisoryreview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestProviderPromptForRendersByteIdenticallyToPreExtractionPath(t *testing.T) {
	request := testRequest(t)
	provider := NewProvider()

	typed, err := provider.PromptFor(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{request.ArtifactSubject.SubjectHash, OutputSchema} {
		if !bytes.Contains(typed, []byte(required)) {
			t.Fatalf("typed prompt omits canonical content %q:\n%s", required, typed)
		}
	}
	canonical, err := Prompt(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(typed, []byte(canonical)) {
		t.Fatalf("Provider.PromptFor diverged from Prompt():\nprovider:\n%s\ncurrent:\n%s", typed, canonical)
	}
	for _, runtime := range SupportedRuntimes() {
		current, err := PromptFor(runtime, request)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(typed, []byte(current)) {
			t.Fatalf("Provider.PromptFor diverged from PromptFor(%s)", runtime)
		}
	}
}

// Admission differential (SEN-RPC-1/3) plus the count budget through the
// typed API (SEN-RPC-1/2): byte-identical admission and refusal.
func TestProviderValidateAdmitsByteIdenticallyToPreExtractionPath(t *testing.T) {
	request := testRequest(t)
	raw, err := json.Marshal(testResult(request))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	provider := NewProvider()

	typed, err := provider.Validate(ctx, raw, request)
	if err != nil {
		t.Fatalf("typed Validate refused natively admitted bytes: %v", err)
	}
	current, err := Validate(raw, request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(typed, current) {
		t.Fatalf("Provider.Validate diverged from pre-extraction Validate:\nprovider: %#v\ncurrent: %#v", typed, current)
	}
	rejections := []struct {
		name string
		raw  []byte
	}{
		{name: "not JSON", raw: []byte("not-json")},
		{name: "subject mismatch", raw: []byte(strings.Replace(string(raw), request.ArtifactSubject.SubjectHash, "sha256:"+strings.Repeat("d", 64), 1))},
		{name: "oversized raw", raw: []byte(strings.Repeat("x", maxResultBytes+1))},
	}
	for _, test := range rejections {
		t.Run(test.name, func(t *testing.T) {
			if _, err := provider.Validate(ctx, test.raw, request); err == nil {
				t.Fatal("typed API admitted bytes the pre-extraction path refuses")
			}
			if _, err := Validate(test.raw, request); err == nil {
				t.Fatal("pre-extraction path stopped refusing (fixture drift)")
			}
		})
	}
	overCount := freezeRequestWithEntries(t, MaxEvidenceEntries+1)
	if _, err := provider.Validate(ctx, []byte("{}"), overCount); err == nil {
		t.Fatal("typed Validate admitted evidence past MaxEvidenceEntries")
	}
}

// Pins the adapter seam (REQ-RPC-4/5): untouched raw bytes or transport error.
func TestInvokerSeamReturnsUntouchedRawBytesOrTransportError(t *testing.T) {
	var _ Invoker = stubInvoker{}

	raw := []byte(`{"subject_hash":"` + strings.Repeat("c", 64) + `"}`)
	request := testRequest(t)
	if got, err := (stubInvoker{raw: raw}).Invoke(context.Background(), request); err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("Invoker returned %q, %v; want raw bytes untouched", got, err)
	}
	if _, err := (failingInvoker{err: errors.New("transport down")}).Invoke(context.Background(), request); err == nil || !strings.Contains(err.Error(), "transport down") {
		t.Fatalf("Invoker transport failure = %v, want the transport error, not fabricated bytes", err)
	}
}

type stubInvoker struct{ raw []byte }

func (stub stubInvoker) Invoke(_ context.Context, _ Request) ([]byte, error) { return stub.raw, nil }

type failingInvoker struct{ err error }

func (failing failingInvoker) Invoke(_ context.Context, _ Request) ([]byte, error) {
	return nil, failing.err
}
