package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func assertInspectAuthorityJSON(t *testing.T, payload []byte) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("inspect-authority JSON: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("inspect-authority top-level fields = %#v", fields)
	}
	for _, key := range []string{"summary", "edges", "diagnostics"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("inspect-authority JSON missing %q", key)
		}
	}
	positions := []int{bytes.Index(payload, []byte(`"summary"`)), bytes.Index(payload, []byte(`"edges"`)), bytes.Index(payload, []byte(`"diagnostics"`))}
	if positions[0] < 0 || positions[0] > positions[1] || positions[1] > positions[2] {
		t.Fatalf("inspect-authority JSON key order = %q", payload)
	}
	var report reviewtransaction.CompactAuthorityInspection
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.Summary != (reviewtransaction.CompactAuthorityInspectionSummary{}) {
		t.Fatalf("empty inspect-authority report = %#v", report)
	}
}

func TestRunReviewInspectAuthorityDispatch(t *testing.T) {
	repo := initReviewCLIRepo(t)
	var output bytes.Buffer
	if err := RunReview([]string{"inspect-authority", "--cwd", repo}, &output); err != nil {
		t.Fatalf("review inspect-authority: %v\n%s", err, output.String())
	}
	assertInspectAuthorityJSON(t, output.Bytes())
}

func TestRunReviewInspectAuthorityCwdResolution(t *testing.T) {
	repo := initReviewCLIRepo(t)
	var output bytes.Buffer
	if err := RunReviewInspectAuthority([]string{"--cwd", repo}, &output); err != nil {
		t.Fatalf("inspect-authority cwd resolution: %v\n%s", err, output.String())
	}
	assertInspectAuthorityJSON(t, output.Bytes())
}
