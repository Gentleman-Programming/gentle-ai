package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Staging a zero-byte file made START report `risk_evidence` describing "an
// executable change" in it. Refusing to treat unclassifiable content as passive
// is correct; describing content that does not exist is not. This drives the
// real classification input -- an actual empty blob in an actual repository --
// rather than the phrasing function in isolation.
func TestStartDescribesAZeroByteFileAsEmptyRatherThanExecutable(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	empty := filepath.Join(repo, "EMPTY")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "EMPTY")

	info, err := os.Stat(empty)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("fixture is not a zero-byte file: %d bytes", info.Size())
	}

	var output bytes.Buffer
	if err := RunReview([]string{"start", "--cwd", repo}, &output); err != nil {
		t.Fatalf("review start: %v\n%s", err, output.String())
	}
	var started struct {
		RiskLevel    string   `json:"risk_level"`
		RiskEvidence []string `json:"risk_evidence"`
	}
	if err := json.Unmarshal(reviewStartJSONPayload(t, output.String()), &started); err != nil {
		t.Fatalf("decode start result: %v\n%s", err, output.String())
	}

	if started.RiskLevel != "medium" {
		t.Fatalf("empty file tier = %q, want medium: the fail-closed classification must not move", started.RiskLevel)
	}
	joined := strings.Join(started.RiskEvidence, "\n")
	if strings.Contains(joined, "executable change") {
		t.Fatalf("a zero-byte file was described as content that is not there:\n%s", joined)
	}
	if !strings.Contains(joined, "EMPTY") || !strings.Contains(joined, "empty") {
		t.Fatalf("risk evidence does not describe the file as empty:\n%s", joined)
	}
}

// reviewStartJSONPayload strips the non-JSON console notice START may print
// ahead of its result.
func reviewStartJSONPayload(t *testing.T, output string) []byte {
	t.Helper()
	index := strings.Index(output, "{")
	if index < 0 {
		t.Fatalf("start emitted no JSON result: %s", output)
	}
	return []byte(output[index:])
}
