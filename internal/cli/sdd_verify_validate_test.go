package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
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

func TestVerifyValidateContractAuthority(t *testing.T) {
	if got, want := sddstatus.MaxVerifyReportBytes, 1<<20; got != want {
		t.Fatalf("maximum report bytes = %d, want %d", got, want)
	}
	if got, want := sddstatus.VerifyVerdicts(), []string{"pass", "pass_with_warnings", "fail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("verdicts = %v, want %v", got, want)
	}
}

func TestRunSDDVerifyValidateHelpIsSuccessfulAndInputFree(t *testing.T) {
	for _, args := range [][]string{
		{"-h"},
		{"--help"},
		{"--input", filepath.Join(t.TempDir(), "missing-report.md"), "--help"},
		{"--help", "--input", "-", "--requirements", "1", "--scenarios", "1"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var output bytes.Buffer
			if err := runSDDVerifyValidate(args, rejectingReader{}, &output); err != nil {
				t.Fatalf("runSDDVerifyValidate(%v) error = %v", args, err)
			}
			text := output.String()
			for _, want := range []string{
				"Usage: gentle-ai sdd-verify-validate [flags]",
				"--input",
				"--requirements",
				"--scenarios",
				"- for stdin",
				"gentle-ai.verify-result/v1",
				"base envelope fields: schema, evidence_revision",
				"pass|pass_with_warnings|fail",
				"nonnegative",
				"1 MiB",
				"Count semantics:",
				"completed/total",
				"must equal the report totals exactly",
				"Scenario and evidence dispositions:",
				"authority-only extension",
				"exit code 125",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("validator help missing %q:\n%s", want, text)
				}
			}
		})
	}
}

// TestRunSDDVerifyValidateHelpMarksRequiredFlags proves --input,
// --requirements, and --scenarios are explicitly marked required for normal
// execution while help itself stays input-free and successful.
func TestRunSDDVerifyValidateHelpMarksRequiredFlags(t *testing.T) {
	for _, args := range [][]string{
		{"-h"},
		{"--help"},
		{"--input", filepath.Join(t.TempDir(), "missing-report.md"), "--help"},
		{"--help", "--input", "-", "--requirements", "1", "--scenarios", "1"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var output bytes.Buffer
			if err := runSDDVerifyValidate(args, rejectingReader{}, &output); err != nil {
				t.Fatalf("runSDDVerifyValidate(%v) error = %v", args, err)
			}
			text := output.String()
			for _, want := range []string{
				"--input <path>         required",
				"--requirements <n>     required",
				"--scenarios <n>        required",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("required marker missing for %q:\n%s", want, text)
				}
			}
		})
	}
}

// TestRunSDDVerifyValidateHelpAuthorityOnlyContract asserts distinct, stable
// contract fragments for the authority-only extension: all five required
// extension fields with their required values, both exit-code conditions, and
// the empty output hash constraint. It prefers semantic fragments over one
// brittle full-output snapshot so a single cosmetic reformat cannot mask a
// real contract regression.
func TestRunSDDVerifyValidateHelpAuthorityOnlyContract(t *testing.T) {
	var output bytes.Buffer
	if err := runSDDVerifyValidate([]string{"--help"}, rejectingReader{}, &output); err != nil {
		t.Fatalf("runSDDVerifyValidate(--help) error = %v", err)
	}
	text := output.String()

	// All five extension fields are named by the help.
	for _, field := range []string{
		"authority_only_failure",
		"missing_review_authority",
		"substantive_failure",
		"command_failed",
		"observed_authority_revision",
	} {
		if !strings.Contains(text, field) {
			t.Errorf("authority-only help missing extension field %q:\n%s", field, text)
		}
	}

	// Each extension field is paired with its required value.
	valueFragments := []string{
		"authority_only_failure=true",
		"missing_review_authority=true",
		"substantive_failure=false",
		"command_failed=false",
	}
	for _, fragment := range valueFragments {
		if !strings.Contains(text, fragment) {
			t.Errorf("authority-only help missing required value %q:\n%s", fragment, text)
		}
	}
	if !strings.Contains(text, "observed_authority_revision is a lowercase sha256 evidence revision") {
		t.Errorf("authority-only help missing observed_authority_revision value constraint:\n%s", text)
	}

	// Both exit-code conditions: non-fail requires zero exit codes; fail with
	// exit code 125 requires the authority-only extension.
	if !strings.Contains(text, "test_exit_code 0, build_exit_code 0") {
		t.Errorf("authority-only help missing non-fail zero exit-code condition:\n%s", text)
	}
	if !strings.Contains(text, "exit code 125 requires the authority-only extension") {
		t.Errorf("authority-only help missing exit-code-125 condition:\n%s", text)
	}

	// Hash constraint: the empty output hash for test_output_hash and
	// build_output_hash is the single source-backed value.
	if !strings.Contains(text, sddstatus.VerifyEmptyOutputHash) {
		t.Errorf("authority-only help missing empty output hash constraint:\n%s", text)
	}
}

type rejectingReader struct{}

func (rejectingReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("validator help must not read input")
}
