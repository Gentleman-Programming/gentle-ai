package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestWindowsPowerShell51ArtifactManifestFileFinalize(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a real binary and Windows PowerShell 5.1")
	}
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only PowerShell 5.1 acceptance test")
	}
	binary := os.Getenv("GENTLE_AI_TEST_BINARY")
	if binary == "" {
		t.Skip("requires GENTLE_AI_TEST_BINARY built from the branch under test")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("GENTLE_AI_TEST_BINARY: %v", err)
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("Windows PowerShell is not installed")
	}
	version, err := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", `$PSVersionTable.PSEdition + '|' + $PSVersionTable.PSVersion.ToString()`).CombinedOutput()
	if err != nil {
		t.Skipf("Windows PowerShell version probe unavailable: %v", err)
	}
	if got := strings.TrimSpace(string(version)); !strings.HasPrefix(got, "Desktop|5.1") {
		t.Skipf("requires Windows PowerShell 5.1, got %q", got)
	}

	// The spawned binary resolves the kill switch from the inherited HOME, so
	// the opt-in has to be persisted there or every `review start` below exits
	// non-zero on an install nobody enabled.
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeBinaryJSON(t, runReviewBinary(t, binary, true, "start", "--cwd", repo), &started)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.State.SelectedLenses) != 1 {
		t.Fatalf("selected lenses = %v, want one", record.State.SelectedLenses)
	}

	temp := t.TempDir()
	input := filepath.Join(temp, "reviewer.json")
	evidence := filepath.Join(temp, "evidence.txt")
	manifest := filepath.Join(temp, "manifest.json")
	script := filepath.Join(temp, "finalize.ps1")
	if err := os.WriteFile(input, powerShell51ReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidence, []byte("focused artifact transport acceptance passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const source = `param(
    [string]$Binary, [string]$Repo, [string]$Lineage, [string]$Target,
    [string]$Lens, [string]$Order, [string]$ResultPath, [string]$EvidencePath, [string]$Manifest
)
$captured = & $Binary review capture-result --cwd $Repo --lineage $Lineage --target $Target --lens $Lens --order $Order --input $ResultPath
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$manifestText = [string]::Join([Environment]::NewLine, [string[]]$captured)
[System.IO.File]::WriteAllText($Manifest, $manifestText, (New-Object System.Text.UTF8Encoding($true)))
& $Binary review finalize --cwd $Repo --lineage $Lineage --result-artifact-file $Manifest --evidence $EvidencePath
exit $LASTEXITCODE
`
	if err := os.WriteFile(script, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script,
		"-Binary", binary, "-Repo", repo, "-Lineage", started.LineageID, "-Target", record.State.InitialSnapshot.Identity,
		"-Lens", record.State.SelectedLenses[0], "-Order", "0", "-ResultPath", input, "-EvidencePath", evidence, "-Manifest", manifest)
	// Stdout and stderr stay separate: the script's stdout is the finalize
	// JSON, while any binary notice (such as the non-interactive consent
	// notice) lands on stderr and must never pollute the decode.
	var scriptStdout, scriptStderr bytes.Buffer
	command.Stdout = &scriptStdout
	command.Stderr = &scriptStderr
	if err := command.Run(); err != nil {
		t.Fatalf("PowerShell 5.1 artifact-file finalize: %v\nstdout:\n%s\nstderr:\n%s", err, scriptStdout.String(), scriptStderr.String())
	}
	manifestBytes, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifestBytes) < 3 || string(manifestBytes[:3]) != "\xef\xbb\xbf" {
		t.Fatal("PowerShell manifest file does not contain a UTF-8 BOM")
	}
	var finalized ReviewFacadeFinalizeResult
	decodeBinaryJSON(t, scriptStdout.Bytes(), &finalized)
	status := binaryReviewStatus(t, binary, repo, started.LineageID)
	if finalized.State != reviewtransaction.StateApproved || finalized.ReceiptPath != "" ||
		status.Applicability != reviewtransaction.TargetApplicabilityUnrelated ||
		status.Action != reviewtransaction.TargetStatusActionStart || status.Authority != nil ||
		status.Receipt.Status != ReviewReceiptNotApplicable || status.Receipt.Identity != "" {
		t.Fatalf("post-burn status = %#v, finalize = %#v", status, finalized)
	}
}

func TestPowerShell51ReviewerPayloadBindsProviderArtifactSubject(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, _, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	input := filepath.Join(t.TempDir(), "reviewer.json")
	if err := os.WriteFile(input, powerShell51ReviewerPayloadForTest(t, repo, record, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo,
		"--lineage", started.LineageID,
		"--target", record.State.InitialSnapshot.Identity,
		"--lens", lens,
		"--order", "0",
		"--input", input,
	}, &output); err != nil {
		t.Fatalf("PowerShell reviewer fixture omitted its provider-owned artifact subject: %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, output.Bytes(), &artifact)
	if artifact.SubjectHash == "" {
		t.Fatal("captured PowerShell reviewer fixture has no provider-owned artifact subject")
	}
}

func powerShell51ReviewerPayloadForTest(t *testing.T, repo string, record reviewtransaction.CompactRecord, lens string, order int) []byte {
	t.Helper()
	return admittedReviewerPayloadForTest(t, repo, record, lens, order,
		"checked exact target through Windows PowerShell 5.1")
}

func TestMainBinaryAcceptsCorrectedCandidateFromLinkedWorktree(t *testing.T) {
	binary := os.Getenv("GENTLE_AI_TEST_BINARY")
	if binary == "" {
		t.Skip("requires GENTLE_AI_TEST_BINARY built from the branch under test")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("GENTLE_AI_TEST_BINARY: %v", err)
	}
	reviewEnabledHome(t)

	t.Run("approves corrected linked worktree", func(t *testing.T) {
		_, corrected, started := prepareBinaryCorrection(t, binary)
		writeBinaryCandidate(t, corrected, "fixed-candidate")
		correctedStatus := binaryReviewStatus(t, binary, corrected, started.LineageID)
		if correctedStatus.Projection.InitialReviewTree == correctedStatus.Projection.CurrentCandidateTree {
			t.Fatal("corrected candidate tree remained unchanged")
		}
		request := capturePassedBinaryCorrectionEvidence(t, binary, corrected, started.LineageID)
		validation := filepath.Join(t.TempDir(), "validation.json")
		writeReviewCLIJSON(t, validation, facadeValidationResult{
			TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
			OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance passed"}},
			CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression passed"}},
			FollowUps:            []reviewtransaction.FollowUp{},
		})
		var approved ReviewFacadeFinalizeResult
		decodeBinaryJSON(t, runReviewBinary(t, binary, true, "finalize", "--cwd", corrected, "--validation", validation, "--captured-evidence"), &approved)
		status := binaryReviewStatus(t, binary, corrected, started.LineageID)
		if approved.State != reviewtransaction.StateApproved || approved.ReceiptPath != "" ||
			status.Applicability != reviewtransaction.TargetApplicabilityUnrelated ||
			status.Action != reviewtransaction.TargetStatusActionStart || status.Authority != nil ||
			status.Receipt.Status != ReviewReceiptNotApplicable || status.Receipt.Identity != "" {
			t.Fatalf("post-burn status = %#v, finalize = %#v", status, approved)
		}
	})

	for _, test := range []struct {
		name              string
		mutate            func(*testing.T, string)
		wantUnchangedStop bool
	}{
		{name: "rejects unchanged candidate", wantUnchangedStop: true, mutate: func(t *testing.T, repo string) { writeBinaryCandidate(t, repo, "wrong") }},
		{name: "rejects path expansion", mutate: func(t *testing.T, repo string) {
			writeBinaryCandidate(t, repo, "fixed-candidate")
			if err := os.WriteFile(filepath.Join(repo, "expanded.txt"), []byte("outside frozen scope\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, corrected, started := prepareBinaryCorrection(t, binary)
			test.mutate(t, corrected)
			if test.wantUnchangedStop {
				status := binaryReviewStatus(t, binary, corrected, started.LineageID, "--next-transition")
				if status.Action != reviewtransaction.TargetStatusActionFinalize || status.NextTransition == nil ||
					status.NextTransition.Kind != reviewNextTransitionStop || status.NextTransition.ReasonCode != "corrected_candidate_unavailable" ||
					status.NextTransition.Collect != nil || status.ValidationRequest != nil {
					t.Fatalf("unchanged candidate status = %#v", status)
				}
				if status.Authority == nil || status.Authority.State != reviewtransaction.StateCorrectionRequired || status.Receipt.Status != ReviewReceiptExpectedMissing {
					t.Fatalf("unchanged candidate mutated public authority: %#v", status)
				}
				return
			}
			validation := filepath.Join(t.TempDir(), "validation.json")
			result := facadeValidationResult{
				OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance passed"}},
				CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression passed"}},
				FollowUps:            []reviewtransaction.FollowUp{},
			}
			args := []string{"finalize", "--cwd", corrected, "--validation", validation}
			writeReviewCLIJSON(t, validation, result)
			runReviewBinary(t, binary, false, args...)
			status := binaryReviewStatus(t, binary, repo, started.LineageID)
			if status.Authority == nil || status.Authority.State != reviewtransaction.StateCorrectionRequired || status.Receipt.Status != ReviewReceiptExpectedMissing {
				t.Fatalf("rejected correction mutated public authority: %#v", status)
			}
		})
	}
}

func TestMainBinaryExecutesSubmissionDescriptorsFromArbitraryCWD(t *testing.T) {
	binary := os.Getenv("GENTLE_AI_TEST_BINARY")
	if binary == "" {
		t.Skip("requires GENTLE_AI_TEST_BINARY built from the branch under test")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("GENTLE_AI_TEST_BINARY: %v", err)
	}
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	outside := t.TempDir()
	writeBinaryCandidate(t, repo, "wrong")
	var started ReviewFacadeStartResult
	decodeBinaryJSON(t, runReviewBinaryAt(t, binary, outside, true, "start", "--cwd", repo, "--lineage", "binary-submission-descriptor"), &started)
	captureBinaryBlockingReviewerResult(t, binary, outside, repo, started.LineageID)
	runReviewBinaryAt(t, binary, outside, true, "finalize", "--cwd", repo, "--lineage", started.LineageID, "--captured-results=true")
	status := binarySubmissionDescriptorStatus(t, binary, outside, repo, started.LineageID)
	correction := submissionDescriptorInput(t, status).Submission
	assertBinarySubmissionDescriptor(t, *correction, repo, outside)
	runReviewBinaryAt(t, binary, outside, true, submissionDescriptorArguments(t, *correction, "1")...)

	writeBinaryCandidate(t, repo, "fixed-candidate")
	evidence := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidence, []byte("repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waiting := binarySubmissionDescriptorStatus(t, binary, outside, repo, started.LineageID)
	capture := captureEvidenceSubmissionInput(t, waiting)
	assertBinaryCaptureEvidenceRefusal(t, binary, outside, repo, started.LineageID, waiting,
		replaceBinaryDescriptorToken(t, binaryCaptureEvidenceSubmissionArguments(t, *capture.Submission, "passed", evidence), "--repository-context=", "--repository-context=rctx1_"+strings.Repeat("0", 64)))
	assertBinaryCaptureEvidenceRefusal(t, binary, outside, repo, started.LineageID, waiting,
		replaceBinaryDescriptorToken(t, binaryCaptureEvidenceSubmissionArguments(t, *capture.Submission, "passed", evidence), "--expected-revision=", "--expected-revision=sha256:"+strings.Repeat("0", 64)))
	assertBinaryCaptureEvidenceRefusal(t, binary, outside, repo, started.LineageID, waiting,
		replaceBinaryDescriptorToken(t, binaryCaptureEvidenceSubmissionArguments(t, *capture.Submission, "passed", evidence), "--target=", "--target=sha256:"+strings.Repeat("0", 64)))
	assertBinaryCaptureEvidenceRefusal(t, binary, outside, repo, started.LineageID, waiting,
		binaryCaptureEvidenceSubmissionArguments(t, *capture.Submission, "invalid", evidence))
	assertBinaryCaptureEvidenceRefusal(t, binary, outside, repo, started.LineageID, waiting,
		replaceBinaryDescriptorToken(t, binaryCaptureEvidenceSubmissionArguments(t, *capture.Submission, "passed", evidence), "--outcome=", "--outcome={{outcome}}"))
	assertBinaryCaptureEvidenceRefusal(t, binary, outside, repo, started.LineageID, waiting,
		withoutBinaryDescriptorToken(t, binaryCaptureEvidenceSubmissionArguments(t, *capture.Submission, "passed", evidence), "--input="))
	assertBinaryCaptureEvidenceRefusal(t, binary, outside, repo, started.LineageID, waiting,
		append(binaryCaptureEvidenceSubmissionArguments(t, *capture.Submission, "passed", evidence), "--unexpected-slot=extra"))
	emptyEvidence := filepath.Join(t.TempDir(), "empty-evidence.txt")
	if err := os.WriteFile(emptyEvidence, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	assertBinaryCaptureEvidenceRefusal(t, binary, outside, repo, started.LineageID, waiting,
		binaryCaptureEvidenceSubmissionArguments(t, *capture.Submission, "passed", emptyEvidence))
	runReviewBinaryAt(t, binary, outside, true, binaryCaptureEvidenceSubmissionArguments(t, *capture.Submission, "passed", evidence)...)

	ready := binarySubmissionDescriptorStatus(t, binary, outside, repo, started.LineageID)
	validation := submissionDescriptorInput(t, ready).Submission
	assertBinarySubmissionDescriptor(t, *validation, repo, outside)
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		TargetedValidationRequestHash: ready.ValidationRequest.RequestHash,
		CorrectionTargetIdentity:      ready.ValidationRequest.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"acceptance passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"regression passed"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	var operation ReviewIntegrationOperationResult
	decodeBinaryJSON(t, runReviewBinaryAt(t, binary, outside, true, submissionDescriptorArguments(t, *validation, validationPath)...), &operation)
	if err := operation.Validate(); err != nil {
		t.Fatalf("validate negotiated binary submission result: %v", err)
	}
	var finalized ReviewIntegrationFinalizeResult
	decodeBinaryJSON(t, operation.Result, &finalized)
	if finalized.State != reviewtransaction.StateApproved {
		t.Fatalf("binary validation descriptor finalized as %#v", finalized)
	}
}

func binarySubmissionDescriptorStatus(t *testing.T, binary, outside, repo, lineage string) ReviewTargetStatusResult {
	t.Helper()
	var status ReviewTargetStatusResult
	decodeBinaryJSON(t, runReviewBinaryAt(t, binary, outside, true,
		"status", "--contract", ReviewIntegrationContractV2, "--agent", "claude-code", "--next-transition", "--cwd", repo, "--lineage", lineage), &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	return status
}

func assertBinarySubmissionDescriptor(t *testing.T, descriptor ReviewTransitionSubmission, paths ...string) {
	t.Helper()
	if descriptor.OperationToken != "finalize" || descriptor.Value.SubstitutionLocation != 6 {
		t.Fatalf("binary submission descriptor = %#v", descriptor)
	}
	for _, token := range descriptor.ArgumentTokens {
		if strings.HasPrefix(token, "--cwd=") {
			t.Fatalf("binary descriptor contains cwd token %q", token)
		}
		for _, path := range paths {
			if strings.Contains(token, path) {
				t.Fatalf("binary descriptor leaks path %q in %q", path, token)
			}
		}
	}
}

func captureBinaryBlockingReviewerResult(t *testing.T, binary, outside, repo, lineage string) {
	t.Helper()
	status := binarySubmissionDescriptorStatus(t, binary, outside, repo, lineage)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("binary reviewer capture transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "review.capture-result" || input.ArtifactSubject == nil || len(status.Projection.Paths) == 0 {
		t.Fatalf("binary reviewer capture input = %#v", input)
	}
	paths := status.Projection.Paths
	reviewer := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, reviewer, map[string]any{
		"subject_hash": input.ArtifactSubject.SubjectHash,
		"inspection":   map[string]any{"status": "completed", "paths": paths},
		"findings": []map[string]any{{
			"location": "tracked.txt:5", "severity": "CRITICAL", "claim": "candidate returns the wrong terminal value",
			"proof_refs": []string{"tracked.txt:5 changed hunk"}, "evidence_class": "deterministic", "causal_disposition": "introduced",
		}},
		"evidence": []string{"focused differential test failed"},
	})
	arguments := []string{"capture-result"}
	for _, argument := range input.Arguments {
		arguments = append(arguments, argument.Token)
	}
	runReviewBinaryAt(t, binary, outside, true, append(arguments, "--input", reviewer)...)
}

func binaryCaptureEvidenceSubmissionArguments(t *testing.T, descriptor ReviewTransitionSubmission, outcome, input string) []string {
	t.Helper()
	arguments := append([]string{"capture-evidence"}, descriptor.ArgumentTokens...)
	for _, slot := range descriptor.Values {
		value := map[string]string{"outcome": outcome, "input": input}[slot.Slot]
		if value == "" {
			t.Fatalf("unexpected capture-evidence descriptor slot %q", slot.Slot)
		}
		index := slot.SubstitutionLocation + 1
		arguments[index] = strings.Replace(arguments[index], "{{"+slot.Slot+"}}", value, 1)
	}
	return arguments
}

func assertBinaryCaptureEvidenceRefusal(t *testing.T, binary, outside, repo, lineage string, before ReviewTargetStatusResult, args []string) {
	t.Helper()
	runReviewBinaryAt(t, binary, outside, false, args...)
	after := binarySubmissionDescriptorStatus(t, binary, outside, repo, lineage)
	if before.Authority == nil || after.Authority == nil || after.Authority.LineageID != before.Authority.LineageID ||
		after.Authority.Revision != before.Authority.Revision || after.TargetIdentity != before.TargetIdentity {
		t.Fatalf("rejected capture-evidence mutated authority: before=%#v after=%#v", before.Authority, after.Authority)
	}
	if got := captureEvidenceSubmissionInput(t, after).Submission; strings.Join(got.ArgumentTokens, "\x00") !=
		strings.Join(captureEvidenceSubmissionInput(t, before).Submission.ArgumentTokens, "\x00") {
		t.Fatalf("rejected capture-evidence changed its pending descriptor: %#v", after.NextTransition)
	}
}

func replaceBinaryDescriptorToken(t *testing.T, args []string, prefix, replacement string) []string {
	t.Helper()
	for index, argument := range args {
		if strings.HasPrefix(argument, prefix) {
			args[index] = replacement
			return args
		}
	}
	t.Fatalf("descriptor did not contain %q: %v", prefix, args)
	return nil
}

func withoutBinaryDescriptorToken(t *testing.T, args []string, prefix string) []string {
	t.Helper()
	for index, argument := range args {
		if strings.HasPrefix(argument, prefix) {
			return append(args[:index], args[index+1:]...)
		}
	}
	t.Fatalf("descriptor did not contain %q: %v", prefix, args)
	return nil
}

func prepareBinaryCorrection(t *testing.T, binary string) (string, string, ReviewFacadeStartResult) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	change := filepath.Join(repo, "openspec", "changes", "binary-review")
	for path, content := range map[string]string{
		"tasks.md":    "- [x] 1.1 Exercise the native review lifecycle\n",
		"proposal.md": "# Binary review acceptance\n",
	} {
		fullPath := filepath.Join(change, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runReviewCLIGit(t, repo, "add", "openspec")
	runReviewCLIGit(t, repo, "commit", "-qm", "add binary review fixture")
	writeBinaryCandidate(t, repo, "wrong")
	startStdout, startStderr := runReviewBinaryStreams(t, binary, true, "start", "--cwd", repo)
	var started ReviewFacadeStartResult
	decodeBinaryJSON(t, startStdout, &started)
	// The one-time consent notice is deliberate product behavior for
	// non-interactive sessions, and it must land on stderr — never on the
	// stdout JSON contract. Pinning it here keeps the discoverability
	// behavior intact instead of merely tolerated.
	if !strings.Contains(startStderr, "Gentle AI reviewed this change without asking") {
		t.Fatalf("non-interactive review start did not print the consent notice on stderr:\n%s", startStderr)
	}
	reviewer := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, reviewer, facadeReviewerResult{Findings: []facadeFinding{{
		Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value",
		ProofRefs: []string{"differential test fails only on candidate"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"focused differential test failed"}})
	// `--result` is retired: it admitted nothing, so the fixture now admits
	// its reviewer result through the real capture path and finalizes via the
	// binary on the native route. The binary is still what drives finalize --
	// only the admission moved to where production admission lives.
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, []string{reviewer}); err != nil {
		t.Fatalf("capture reviewer result: %v", err)
	}
	var correction ReviewFacadeFinalizeResult
	decodeBinaryJSON(t, runReviewBinary(t, binary, true, "finalize", "--cwd", repo, "--captured-results=true"), &correction)
	if correction.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("review state = %q", correction.State)
	}
	runReviewBinary(t, binary, true, "finalize", "--cwd", repo, "--correction-lines", "2")
	corrected := filepath.Join(t.TempDir(), "corrected")
	runReviewCLIGit(t, repo, "worktree", "add", "--detach", corrected, "HEAD")
	return repo, corrected, started
}

func writeBinaryCandidate(t *testing.T, repo, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\n"+value+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runReviewBinary(t *testing.T, binary string, wantSuccess bool, args ...string) []byte {
	t.Helper()
	stdout, _ := runReviewBinaryStreams(t, binary, wantSuccess, args...)
	return stdout
}

func runReviewBinaryAt(t *testing.T, binary, dir string, wantSuccess bool, args ...string) []byte {
	t.Helper()
	command := exec.Command(binary, append([]string{"review"}, args...)...)
	command.Dir = dir
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if (err == nil) != wantSuccess {
		t.Fatalf("gentle-ai review from %s %v: %v\nstdout:\n%s\nstderr:\n%s", dir, args, err, stdout.String(), stderr.String())
	}
	return stdout.Bytes()
}

// runReviewBinaryStreams captures stdout and stderr separately. Stdout carries
// the machine-readable JSON contract; stderr carries human-facing notices such
// as the non-interactive consent notice, and mixing the two corrupts the JSON
// decode that every acceptance assertion depends on.
func runReviewBinaryStreams(t *testing.T, binary string, wantSuccess bool, args ...string) ([]byte, string) {
	t.Helper()
	command := exec.Command(binary, append([]string{"review"}, args...)...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if (err == nil) != wantSuccess {
		t.Fatalf("gentle-ai review %v: %v\nstdout:\n%s\nstderr:\n%s", args, err, stdout.String(), stderr.String())
	}
	return stdout.Bytes(), stderr.String()
}

func decodeBinaryJSON(t *testing.T, payload []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatalf("decode binary output: %v\n%s", err, payload)
	}
}

func binaryReviewStatus(t *testing.T, binary, repo, lineage string, extra ...string) ReviewTargetStatusResult {
	t.Helper()
	arguments := []string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage}
	arguments = append(arguments, extra...)
	var status ReviewTargetStatusResult
	decodeBinaryJSON(t, runReviewBinary(t, binary, true, arguments...), &status)
	return status
}

func capturePassedBinaryCorrectionEvidence(t *testing.T, binary, repo, lineage string) reviewtransaction.TargetedValidationRequest {
	t.Helper()
	var waiting ReviewTargetStatusResult
	decodeBinaryJSON(t, runReviewBinary(t, binary, true, "status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", lineage), &waiting)
	if waiting.Authority == nil || waiting.NextTransition == nil || waiting.NextTransition.Kind != reviewNextTransitionCollect {
		t.Fatalf("correction evidence status = %#v", waiting)
	}
	target := transitionArgumentValue(t, waiting.NextTransition, "target")
	evidence := filepath.Join(t.TempDir(), "passed-correction-evidence.txt")
	if err := os.WriteFile(evidence, []byte("targeted and full repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runReviewBinary(t, binary, true, "capture-evidence", "--cwd", repo, "--lineage", lineage,
		"--target", target, "--expected-revision", waiting.Authority.Revision,
		"--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidence)
	var ready ReviewTargetStatusResult
	decodeBinaryJSON(t, runReviewBinary(t, binary, true, "status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", lineage), &ready)
	if ready.ValidationRequest == nil || ready.ValidationRequest.CorrectionTargetIdentity != target {
		t.Fatalf("captured correction evidence status = %#v", ready)
	}
	return *ready.ValidationRequest
}
