package cli

import (
	"reflect"
	"strings"
	"testing"
)

const revisionCanonHexLower = "27a44ad82d6e4f1d9f2c3b4a5d6e7f8091a2b3c4d5e6f70818293a4b5c6d7e8f"

func TestCanonicalSHA256Revision(t *testing.T) {
	canonical := "sha256:" + revisionCanonHexLower
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"uppercase prefixed", "sha256:" + strings.ToUpper(revisionCanonHexLower), canonical},
		{"uppercase bare", strings.ToUpper(revisionCanonHexLower), canonical},
		{"lowercase bare", revisionCanonHexLower, canonical},
		{"canonical passthrough", canonical, canonical},
		{"whitespace-wrapped uppercase bare", "  " + strings.ToUpper(revisionCanonHexLower) + "\n", canonical},
		{"63 hex chars unchanged", revisionCanonHexLower[:63], revisionCanonHexLower[:63]},
		{"non-hex unchanged", "g" + revisionCanonHexLower[1:], "g" + revisionCanonHexLower[1:]},
		{"wrong prefix unchanged", "sha1:" + revisionCanonHexLower, "sha1:" + revisionCanonHexLower},
		{"uppercase prefix unchanged", "SHA256:" + revisionCanonHexLower, "SHA256:" + revisionCanonHexLower},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalSHA256Revision(tt.input); got != tt.want {
				t.Fatalf("canonicalSHA256Revision(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCanonicalizeSDDAttemptRevisionArgs(t *testing.T) {
	upper := strings.ToUpper(revisionCanonHexLower)
	canonical := "sha256:" + revisionCanonHexLower
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"separate value form",
			[]string{"settle", "--evidence-revision", upper, "--outcome", "failed"},
			[]string{"settle", "--evidence-revision", canonical, "--outcome", "failed"}},
		{"inline value form",
			[]string{"finish", "--evidence-revision=" + upper},
			[]string{"finish", "--evidence-revision=" + canonical}},
		{"remediates flag both forms",
			[]string{"finish", "--remediates-evidence-revision", upper, "--remediates-evidence-revision=" + upper},
			[]string{"finish", "--remediates-evidence-revision", canonical, "--remediates-evidence-revision=" + canonical}},
		{"ledger-issued revisions untouched",
			[]string{"finish", "--expected-revision", upper, "--token", upper},
			[]string{"finish", "--expected-revision", upper, "--token", upper}},
		{"ambiguous value untouched",
			[]string{"settle", "--evidence-revision", "sha1:" + revisionCanonHexLower},
			[]string{"settle", "--evidence-revision", "sha1:" + revisionCanonHexLower}},
		{"trailing flag without value untouched",
			[]string{"settle", "--evidence-revision"},
			[]string{"settle", "--evidence-revision"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			input := append([]string(nil), tt.args...)
			if got := CanonicalizeSDDAttemptRevisionArgs(input); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("CanonicalizeSDDAttemptRevisionArgs(%v) = %v, want %v", tt.args, got, tt.want)
			}
			if !reflect.DeepEqual(input, tt.args) {
				t.Fatalf("input argv mutated: %v, want %v", input, tt.args)
			}
		})
	}
}
