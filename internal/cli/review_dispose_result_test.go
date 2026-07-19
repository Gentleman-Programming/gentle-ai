package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

// unreplayableReviewerOutput reproduces the #1469 payload: syntactically
// invalid JSON (note the trailing comma) that capture-result can never decode.
// Because it never decoded, the only claim it can prove is transport_syntax.
const unreplayableReviewerOutput = `{"findings":[{"id":"R1-001","location":"internal/billing/charge.go:42"},],"evidence":["read internal/billing/charge.go"]}`

// wrongTargetReviewerOutput is the other #1469 shape: the payload decodes
// perfectly, so the reviewer genuinely produced a result, but every claim in it
// describes a candidate the frozen target never contained.
const wrongTargetReviewerOutput = `{"findings":[{"id":"R1-001","location":"internal/billing/charge.go:42"}],"evidence":["read internal/billing/charge.go"]}`

func disposeResultAuthorization(repository, lineage, revision, target, lens string, order int, digest, class, actor, reason string) string {
	return "gentle-ai.review-result-disposition-authorization/v1" +
		"\nrepository=" + repository +
		"\nlineage=" + lineage +
		"\nrevision=" + revision +
		"\ntarget_identity=" + target +
		"\nlens=" + lens +
		"\norder=" + strconv.Itoa(order) +
		"\nartifact_digest=" + digest +
		"\nclass=" + class +
		"\nactor=" + actor +
		"\nreason=" + reason
}

// TestReviewDisposeResultEscalatesStrandedLineage drives the whole #1469
// Case A recovery through the real facade: one lens captures a valid result,
// another preserves an unreplayable payload, and the disposition terminally
// escalates the lineage without touching either artifact.
func TestReviewDisposeResultEscalatesStrandedLineage(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, true)
	lenses := record.State.SelectedLenses
	if len(lenses) != 4 {
		t.Fatalf("selected lenses = %v, want the high-risk 4R sweep", lenses)
	}
	target := record.State.InitialSnapshot.Identity

	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, []byte(`{"findings":[],"evidence":["checked exact target"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", target,
		"--lens", lenses[0], "--order", "0", "--input", input,
	}, &captured); err != nil {
		t.Fatalf("capture-result: %v", err)
	}
	var capturedArtifact reviewResultArtifact
	decodeStrictReviewJSON(t, captured.Bytes(), &capturedArtifact)
	capturedBytes, err := os.ReadFile(capturedArtifact.Path)
	if err != nil {
		t.Fatal(err)
	}

	raw := filepath.Join(t.TempDir(), "raw.txt")
	if err := os.WriteFile(raw, []byte(unreplayableReviewerOutput), 0o600); err != nil {
		t.Fatal(err)
	}
	var preserved bytes.Buffer
	if err := RunReviewPreserveResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", target,
		"--lens", lenses[3], "--order", "3", "--input", raw,
	}, &preserved); err != nil {
		t.Fatalf("preserve-result: %v", err)
	}
	var incident reviewIncidentArtifact
	decodeStrictReviewJSON(t, preserved.Bytes(), &incident)

	// The preserved payload genuinely cannot be replayed through capture.
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", target,
		"--lens", lenses[3], "--order", "3", "--input", incident.Path,
	}, io.Discard); err == nil {
		t.Fatal("the unreplayable payload was accepted by capture-result")
	}

	repository, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	const actor, reason = "maintainer@example.com", "reviewer output never decoded"
	// The preserved payload never decoded, so transport_syntax is the only class
	// it proves. Claiming wrong_target over it would assert that a decodable
	// reviewer result described a different candidate, which this payload cannot
	// support; the facade refuses exactly that below.
	const class = "transport_syntax"
	authorization := disposeResultAuthorization(repository, started.LineageID, record.Revision, target,
		lenses[3], 3, incident.SHA256, class, actor, reason)
	args := []string{
		"dispose-result", "--cwd", repo, "--lineage", started.LineageID,
		"--expected-revision", record.Revision, "--target", target, "--lens", lenses[3], "--order", "3",
		"--artifact-digest", incident.SHA256, "--class", class,
		"--diagnostic", "decode reviewer result: invalid character after array element",
		"--reason", reason, "--actor", actor, "--maintainer-authorization", authorization,
	}

	// An invalid-JSON payload may never be audited as the stronger semantic
	// class, and that refusal must land before any authority is touched.
	const wrongClass = "wrong_target"
	wrongArgs := []string{
		"dispose-result", "--cwd", repo, "--lineage", started.LineageID,
		"--expected-revision", record.Revision, "--target", target, "--lens", lenses[3], "--order", "3",
		"--artifact-digest", incident.SHA256, "--class", wrongClass,
		"--diagnostic", "decode reviewer result: invalid character after array element",
		"--absent-path", "internal/billing/charge.go",
		"--reason", reason, "--actor", actor,
		"--maintainer-authorization", disposeResultAuthorization(repository, started.LineageID, record.Revision,
			target, lenses[3], 3, incident.SHA256, wrongClass, actor, reason),
	}
	if err := RunReview(wrongArgs, io.Discard); err == nil {
		t.Fatal("wrong_target disposition of a payload that never decoded was accepted")
	}

	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("review dispose-result: %v\n%s", err, output.String())
	}
	var result ReviewDisposeResultResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Operation != reviewtransaction.CompactResultDispositionOperation ||
		result.Record.State != reviewtransaction.StateEscalated || result.Record.Replayed ||
		result.Record.Disposition.Class != reviewtransaction.ResultDispositionTransportSyntax ||
		result.Record.Disposition.PayloadDecodable ||
		result.Record.Disposition.ArtifactDigest != incident.SHA256 ||
		len(result.Record.RetainedLensResults) != 1 {
		t.Fatalf("dispose-result = %#v", result)
	}

	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.State.State != reviewtransaction.StateEscalated || len(after.State.ResultDispositions) != 1 ||
		len(after.State.LensResults) != 0 {
		t.Fatalf("authority after disposition = %#v", after.State)
	}
	if got, readErr := os.ReadFile(incident.Path); readErr != nil || !bytes.Equal(got, []byte(unreplayableReviewerOutput)) {
		t.Fatalf("preserved incident artifact changed: %v", readErr)
	}
	if got, readErr := os.ReadFile(capturedArtifact.Path); readErr != nil || !bytes.Equal(got, capturedBytes) {
		t.Fatalf("captured reviewer result changed: %v", readErr)
	}

	// Requirement 5: the exact replay converges instead of double-applying.
	var replay bytes.Buffer
	if err := RunReview(args, &replay); err != nil {
		t.Fatalf("replayed dispose-result: %v\n%s", err, replay.String())
	}
	var replayed ReviewDisposeResultResult
	decodeStrictReviewJSON(t, replay.Bytes(), &replayed)
	if !replayed.Record.Replayed || replayed.Record.Revision != result.Record.Revision {
		t.Fatalf("replay = %#v, want convergence on %#v", replayed.Record, result.Record)
	}

	var help bytes.Buffer
	if err := RunReview([]string{"help"}, &help); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(help.String(), "dispose-result") {
		t.Fatalf("review help omits dispose-result: %s", help.String())
	}
}

// TestReviewDisposeResultWrongTargetRequiresADecodablePayload pins the class
// boundary through the facade: wrong_target is admitted only over a payload
// that genuinely decoded, and the committed audit record says so.
func TestReviewDisposeResultWrongTargetRequiresADecodablePayload(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, true)
	lenses := record.State.SelectedLenses
	target := record.State.InitialSnapshot.Identity

	raw := filepath.Join(t.TempDir(), "raw.json")
	if err := os.WriteFile(raw, []byte(wrongTargetReviewerOutput), 0o600); err != nil {
		t.Fatal(err)
	}
	var preserved bytes.Buffer
	if err := RunReviewPreserveResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", target,
		"--lens", lenses[2], "--order", "2", "--input", raw,
	}, &preserved); err != nil {
		t.Fatalf("preserve-result: %v", err)
	}
	var incident reviewIncidentArtifact
	decodeStrictReviewJSON(t, preserved.Bytes(), &incident)

	repository, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	const actor, reason = "maintainer@example.com", "reviewer output describes a different candidate"
	const class = "wrong_target"
	args := []string{
		"dispose-result", "--cwd", repo, "--lineage", started.LineageID,
		"--expected-revision", record.Revision, "--target", target, "--lens", lenses[2], "--order", "2",
		"--artifact-digest", incident.SHA256, "--class", class,
		"--diagnostic", "reviewer result cites internal/billing/charge.go, absent from the frozen candidate",
		"--absent-path", "internal/billing/charge.go",
		"--reason", reason, "--actor", actor,
		"--maintainer-authorization", disposeResultAuthorization(repository, started.LineageID, record.Revision,
			target, lenses[2], 2, incident.SHA256, class, actor, reason),
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("wrong-target dispose-result: %v\n%s", err, output.String())
	}
	var result ReviewDisposeResultResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Record.Disposition.Class != reviewtransaction.ResultDispositionWrongTarget ||
		!result.Record.Disposition.PayloadDecodable ||
		result.Record.State != reviewtransaction.StateEscalated {
		t.Fatalf("wrong-target dispose-result = %#v", result.Record)
	}
	after, err := store.Load()
	if err != nil || len(after.State.ResultDispositions) != 1 || !after.State.ResultDispositions[0].PayloadDecodable {
		t.Fatalf("persisted wrong-target disposition = %+v (%v)", after.State.ResultDispositions, err)
	}
}

// TestReviewDisposeResultRequiresCompleteBinding keeps the facade fail-closed:
// every missing flag and every unproven evidence claim is refused before any
// authority is touched.
func TestReviewDisposeResultRequiresCompleteBinding(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, true)
	lenses := record.State.SelectedLenses
	target := record.State.InitialSnapshot.Identity

	raw := filepath.Join(t.TempDir(), "raw.txt")
	if err := os.WriteFile(raw, []byte(unreplayableReviewerOutput), 0o600); err != nil {
		t.Fatal(err)
	}
	var preserved bytes.Buffer
	if err := RunReviewPreserveResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", target,
		"--lens", lenses[1], "--order", "1", "--input", raw,
	}, &preserved); err != nil {
		t.Fatal(err)
	}
	var incident reviewIncidentArtifact
	decodeStrictReviewJSON(t, preserved.Bytes(), &incident)

	repository, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	const actor, reason, class = "maintainer@example.com", "unreplayable", "transport_syntax"
	authorization := disposeResultAuthorization(repository, started.LineageID, record.Revision, target,
		lenses[1], 1, incident.SHA256, class, actor, reason)
	complete := map[string]string{
		"--cwd": repo, "--lineage": started.LineageID, "--expected-revision": record.Revision,
		"--target": target, "--lens": lenses[1], "--order": "1", "--artifact-digest": incident.SHA256,
		"--class": class, "--diagnostic": "invalid character after array element",
		"--reason": reason, "--actor": actor, "--maintainer-authorization": authorization,
	}
	argsWithout := func(dropped string) []string {
		args := []string{"dispose-result"}
		for flag, value := range complete {
			if flag != dropped {
				args = append(args, flag, value)
			}
		}
		return args
	}
	for flag := range complete {
		if flag == "--cwd" {
			continue
		}
		if err := RunReview(argsWithout(flag), io.Discard); err == nil {
			t.Fatalf("dispose-result without %s was accepted", flag)
		}
	}
	// A transport/syntax claim may not carry wrong-target path evidence.
	if err := RunReview(append(argsWithout(""), "--absent-path", "internal/billing/charge.go"), io.Discard); err == nil {
		t.Fatal("transport_syntax disposition with path evidence was accepted")
	}
	after, err := store.Load()
	if err != nil || after.Revision != record.Revision || after.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("refused dispose-result mutated authority: %v", err)
	}
	if got, readErr := os.ReadFile(incident.Path); readErr != nil || !bytes.Equal(got, []byte(unreplayableReviewerOutput)) {
		t.Fatalf("refused dispose-result touched the preserved output: %v", readErr)
	}
}
