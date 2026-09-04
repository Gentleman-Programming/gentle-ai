package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func TestRunSDDVerifyValidateHelpIsSuccessfulAndInputFree(t *testing.T) {
	stdin := &sddVerifyValidateReadSpy{}
	var output bytes.Buffer
	err := runSDDVerifyValidate([]string{
		"--input", filepath.Join(t.TempDir(), "missing.md"), "--requirements", "-1", "--help", "--scenarios", "-1",
	}, stdin, &output)
	if err != nil {
		t.Fatalf("runSDDVerifyValidate(--help): %v", err)
	}
	if stdin.reads != 0 {
		t.Fatalf("help read stdin %d times", stdin.reads)
	}
	for _, want := range []string{"Usage: gentle-ai sdd-verify-validate", "--input <path|->", "--requirements <n>", "--scenarios <n>"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, output.String())
		}
	}
}

// #2828: the help never said what the first line must be, so a report that
// opened with a BOM, a `yml` tag, or a heading was refused with no way to
// learn the contract from the command itself.
func TestRunSDDVerifyValidateHelpNamesTheFenceContract(t *testing.T) {
	var output bytes.Buffer
	if err := runSDDVerifyValidate([]string{"-h"}, strings.NewReader("must not be read"), &output); err != nil {
		t.Fatalf("runSDDVerifyValidate(-h): %v", err)
	}
	for _, want := range []string{"first non-empty line must be ```yaml", "yml", "UTF-8 BOM"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help omits the fence contract %q:\n%s", want, output.String())
		}
	}
}

func TestRunSDDVerifyValidateHelpExcludesMissingReviewSkipProtocol(t *testing.T) {
	var output bytes.Buffer
	if err := runSDDVerifyValidate([]string{"-h"}, strings.NewReader("must not be read"), &output); err != nil {
		t.Fatalf("runSDDVerifyValidate(-h): %v", err)
	}
	contract := sddstatus.VerifyReportValidationContract()
	if !strings.Contains(output.String(), contract.Schema) ||
		!strings.Contains(output.String(), fmt.Sprintf("maximum report size: %d bytes (%s)", contract.MaxBytes, formatSDDVerifyValidateByteLimit(contract.MaxBytes))) {
		t.Fatalf("help omits the verification contract:\n%s", output.String())
	}
	for _, forbidden := range []string{
		"Authority-only fail extension",
		"missing_review_authority",
		"authority_only_failure",
		"test_exit_code 125",
		"build_exit_code 125",
	} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("help retains missing-review skip protocol %q:\n%s", forbidden, output.String())
		}
	}
}

func TestRunSDDVerifyValidateHelpRespectsFlagValueArity(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.md")
	tests := []struct {
		name     string
		args     []string
		wantHelp bool
	}{
		{"long help is input value", []string{"--input", "--help", "--requirements", "1", "--scenarios", "1"}, false},
		{"short help is input value", []string{"--input", "-h", "--requirements", "1", "--scenarios", "1"}, false},
		{"long help first", []string{"--help", "--input", missing, "--requirements", "-1", "--scenarios", "-1"}, true},
		{"short help first", []string{"-h", "--input", missing, "--requirements", "-1", "--scenarios", "-1"}, true},
		{"long help after input value", []string{"--input", missing, "--help", "--requirements", "-1", "--scenarios", "-1"}, true},
		{"short help after input value", []string{"--input", missing, "-h", "--requirements", "-1", "--scenarios", "-1"}, true},
		{"long help at end", []string{"--input", missing, "--requirements", "-1", "--scenarios", "-1", "--help"}, true},
		{"short help at end", []string{"--input", missing, "--requirements", "-1", "--scenarios", "-1", "-h"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runSDDVerifyValidate(tt.args, &sddVerifyValidateReadSpy{}, &output)
			if tt.wantHelp {
				if err != nil || !strings.Contains(output.String(), "Usage: gentle-ai sdd-verify-validate") {
					t.Fatalf("runSDDVerifyValidate(%v) = output %q, err %v", tt.args, output.String(), err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "read verify report") || output.Len() != 0 {
				t.Fatalf("runSDDVerifyValidate(%v) = output %q, err %v", tt.args, output.String(), err)
			}
		})
	}
}

func TestRunSDDVerifyValidateRequiredFlagsRemainRequired(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "requires --input"},
		{[]string{"--input", "-"}, "requires --requirements"},
		{[]string{"--input", "-", "--requirements", "1"}, "requires --scenarios"},
	}
	for _, tt := range tests {
		var output bytes.Buffer
		err := runSDDVerifyValidate(tt.args, strings.NewReader(""), &output)
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

func TestRunSDDVerifyValidateHelpDocumentsSHA256Grammar(t *testing.T) {
	var output bytes.Buffer
	if err := runSDDVerifyValidate([]string{"-h"}, strings.NewReader("unused"), &output); err != nil {
		t.Fatalf("runSDDVerifyValidate(-h): %v", err)
	}
	for _, want := range []string{
		"evidence_revision, test_output_hash, and build_output_hash must match sha256:<64 lowercase hex>.",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help omits sha256 grammar %q:\n%s", want, output.String())
		}
	}
}

func TestRunSDDVerifyValidateRejectsInvalidSHA256WithSelfDiagnosingError(t *testing.T) {
	report := "```yaml\nschema: gentle-ai.verify-result/v1\nevidence_revision: invalid_revision\nverdict: fail\nblockers: 1\ncritical_findings: 0\nrequirements: 1/1\nscenarios: 1/1\ntest_command: go test ./...\ntest_exit_code: 0\ntest_output_hash: sha256:" + strings.Repeat("b", 64) + "\nbuild_command: go vet ./...\nbuild_exit_code: 0\nbuild_output_hash: sha256:" + strings.Repeat("c", 64) + "\n```"
	var output bytes.Buffer
	err := runSDDVerifyValidate([]string{"--input", "-", "--requirements", "1", "--scenarios", "1"}, strings.NewReader(report), &output)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	wantErrParts := []string{
		"invalid evidence_revision in verify result envelope",
		"expected sha256:<64 lowercase hex>",
		`got "invalid_revision"`,
		"length 16",
		"missing 'sha256:' prefix",
	}
	for _, want := range wantErrParts {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err.Error(), want)
		}
	}
}
