package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSDDVerifyValidate(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "openspec", "changes", "thin", "specs")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "spec.md"), []byte("### Requirement: One\n#### Scenario: One\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := "```yaml\nschema: gentle-ai.verify-result/v1\nevidence_revision: sha256:" + strings.Repeat("a", 64) + "\nverdict: fail\nblockers: 1\ncritical_findings: 0\nrequirements: 1/1\nscenarios: 1/1\ntest_command: go test ./...\ntest_exit_code: 0\ntest_output_hash: sha256:" + strings.Repeat("b", 64) + "\nbuild_command: go vet ./...\nbuild_exit_code: 0\nbuild_output_hash: sha256:" + strings.Repeat("c", 64) + "\n```"
	args := []string{"--input", "-", "--cwd", root, "--change", "thin"}
	var output bytes.Buffer
	if err := runSDDVerifyValidate(args, strings.NewReader(report), &output); err != nil {
		t.Fatalf("valid failure: %v", err)
	}
	if got := output.String(); !strings.Contains(got, `"valid": true`) || !strings.Contains(got, `"verdict": "fail"`) {
		t.Fatalf("output = %s", got)
	}
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"missing authority", []string{"--input", "-", "--cwd", root}, "requires --cwd and --change"},
		{"unknown retired total", append(args, "--requirements", "1"), "not defined"},
		{"slice requires id", append(args, "--scope", "slice"), "requires a slice_id"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := runSDDVerifyValidate(tt.args, strings.NewReader(report), &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRunSDDVerifyValidateHelpIsSuccessfulAndInputFree(t *testing.T) {
	stdin := &sddVerifyValidateReadSpy{}
	var output bytes.Buffer
	if err := runSDDVerifyValidate([]string{"--help"}, stdin, &output); err != nil {
		t.Fatal(err)
	}
	if stdin.reads != 0 {
		t.Fatalf("help read stdin %d times", stdin.reads)
	}
	for _, want := range []string{"--input <path|->", "--cwd <repo>", "--change <name>", "--scope <whole|slice>", "--slice-id <id>", "scope, slice_id"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "--requirements") || strings.Contains(output.String(), "--scenarios") {
		t.Fatalf("help retained caller totals:\n%s", output.String())
	}
}

func TestRunSDDVerifyValidateRequiredFlagsRemainRequired(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{nil, "requires --input"},
		{[]string{"--input", "-"}, "requires --cwd and --change"},
	} {
		err := runSDDVerifyValidate(tt.args, strings.NewReader(""), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("runSDDVerifyValidate(%v) = %v, want %q", tt.args, err, tt.want)
		}
	}
}

type sddVerifyValidateReadSpy struct{ reads int }

func (spy *sddVerifyValidateReadSpy) Read([]byte) (int, error) {
	spy.reads++
	return 0, errors.New("help must not read stdin")
}
