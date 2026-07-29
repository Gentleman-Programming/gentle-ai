package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSDDVerifyValidate(t *testing.T) {
	report := "```yaml\nschema: gentle-ai.verify-result/v1\nevidence_revision: sha256:" + strings.Repeat("a", 64) + "\nverdict: fail\nblockers: 1\ncritical_findings: 0\nrequirements: 1/1\nscenarios: 1/1\ntest_command: go test ./...\ntest_exit_code: 0\ntest_output_hash: sha256:" + strings.Repeat("b", 64) + "\nbuild_command: go vet ./...\nbuild_exit_code: 0\nbuild_output_hash: sha256:" + strings.Repeat("c", 64) + "\n```"
	var output bytes.Buffer
	if err := runSDDVerifyValidate([]string{"--input", "-", "--requirements", "1", "--scenarios", "1"}, strings.NewReader(report), &output); err != nil {
		t.Fatalf("valid failure: %v", err)
	}
	if got := output.String(); !strings.Contains(got, `"valid": true`) || !strings.Contains(got, `"verdict": "fail"`) {
		t.Fatalf("output = %s", got)
	}
	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runSDDVerifyValidate([]string{"--input", path, "--requirements", "1", "--scenarios", "1"}, strings.NewReader("unused"), &bytes.Buffer{}); err != nil {
		t.Fatalf("file input: %v", err)
	}

	for _, tt := range []struct {
		name        string
		args        []string
		input, want string
	}{
		{"front matter", []string{"--input", "-", "--requirements", "1", "--scenarios", "1"}, "---\n" + report, "front matter"},
		{"missing count", []string{"--input", "-", "--requirements", "1"}, report, "requires --scenarios"},
		{"negative count", []string{"--input", "-", "--requirements", "-1", "--scenarios", "1"}, report, "nonnegative"},
		{"unexpected argument", []string{"--input", "-", "--requirements", "1", "--scenarios", "1", "extra"}, report, "unexpected"},
		{"oversized", []string{"--input", "-", "--requirements", "1", "--scenarios", "1"}, strings.Repeat("x", 1<<20+1), "exceeds"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := runSDDVerifyValidate(tt.args, strings.NewReader(tt.input), &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
