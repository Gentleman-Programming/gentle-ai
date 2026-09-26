package main

// The gentle-ai-bench.results/v2 identity envelope and its canonical
// path-free projection (issue #1866, slice B).
//
// A results file must answer "who measured what, how, and with which rules"
// without carrying a single machine-specific path, and two runs of the same
// corpus must produce byte-identical files so their digests can be compared
// like receipts. These tests pin:
//
//   - the envelope: the schema bump, the identity sections, the classifier
//     version, the corpus manifest digest, the target digest, and the
//     empty-selector early write;
//   - the projection: the closed token vocabulary and the fail-closed
//     validation that refuses to write a file still carrying a known
//     machine-specific path;
//   - the digest: stable across different sandboxes and independent of the
//     evidence_digest field itself;
//   - the read path: an unknown schema fails closed naming the file.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// testJourneyCorpus is the minimal corpus the end-to-end tests drive through
// the command boundary (same shape as TestCommandRunSelection's).
func testJourneyCorpus() []Journey {
	return []Journey{{
		ID: "known",
		// This journey drives a stub binary through the command
		// boundary, not the review lifecycle, so the runner must leave
		// the kill switch alone.
		Review: reviewUntouched,
		Steps: []Step{{
			Name: "invoke test binary",
			Fixture: func(sandbox *Sandbox) error {
				return os.MkdirAll(sandbox.Repo, 0o755)
			},
			Args: func(*Sandbox) ([]string, error) {
				return []string{"review"}, nil
			},
		}},
	}}
}

func testFileDigest(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// testManifestDigest is the expected corpus.manifest_sha256: sha256 hex over
// the sorted newline-joined IDs of the full registered corpus, independent of
// any --only selection.
func testManifestDigest(t *testing.T) string {
	t.Helper()
	ids := make([]string, 0, len(Journeys()))
	for _, journey := range Journeys() {
		ids = append(ids, journey.ID)
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(sum[:])
}

// runDrivenAndRead drives one full run through the command boundary and
// returns the decoded results file.
func runDrivenAndRead(t *testing.T, binary, out string) Results {
	t.Helper()
	args := []string{"--binary", binary, "--out", out}
	if exit := commandRunWith(args, func(string) bool { return true }, testJourneyCorpus); exit != 0 {
		t.Fatalf("commandRunWith() exit = %d, want 0", exit)
	}
	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	var results Results
	if err := json.Unmarshal(content, &results); err != nil {
		t.Fatalf("decode results: %v", err)
	}
	return results
}

// TestCommandRunWritesV2IdentityEnvelope pins the envelope a driven run must
// carry: schema v2, every identity section populated with what the run can
// know, and a top-level binary field projected onto the target token instead
// of the local path.
func TestCommandRunWritesV2IdentityEnvelope(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a temporary executable to exercise the benchmark command boundary")
	}

	binary, _ := benchmarkTestBinary(t)
	out := filepath.Join(t.TempDir(), "results.json")
	results := runDrivenAndRead(t, binary, out)

	if results.Schema != ResultsSchema {
		t.Fatalf("schema = %q, want %q", results.Schema, ResultsSchema)
	}
	if results.Schema != "gentle-ai-bench.results/v2" {
		t.Fatalf("schema = %q, want the v2 identity envelope", results.Schema)
	}
	identity := results.Identity
	if identity == nil {
		t.Fatal("results carry no identity envelope")
	}
	if identity.Benchmark.SourceRevision == "" {
		t.Error("benchmark.source_revision is empty; the worktree is a git repository, so the source revision is knowable")
	}
	if identity.Benchmark.ClassifierVersion != ClassifierVersion {
		t.Errorf("benchmark.classifier_version = %q, want %q", identity.Benchmark.ClassifierVersion, ClassifierVersion)
	}
	if ClassifierVersion != "classify-v1" {
		t.Errorf("ClassifierVersion = %q, want classify-v1", ClassifierVersion)
	}
	if identity.Benchmark.GoVersion != runtime.Version() {
		t.Errorf("benchmark.go_version = %q, want %q", identity.Benchmark.GoVersion, runtime.Version())
	}
	// The bench executable under `go test` is a throwaway go-build temp
	// binary; its digest is meaningless and must be recorded as empty.
	if identity.Benchmark.BinarySHA256 != "" && len(identity.Benchmark.BinarySHA256) != 64 {
		t.Errorf("benchmark.binary_sha256 = %q, want empty or 64 hex chars", identity.Benchmark.BinarySHA256)
	}
	if identity.Corpus.ManifestSHA256 != testManifestDigest(t) {
		t.Errorf("corpus.manifest_sha256 = %q, want the digest of the sorted full registered corpus", identity.Corpus.ManifestSHA256)
	}
	if want := []string{"known"}; !reflect.DeepEqual(identity.Corpus.JourneyIDs, want) {
		t.Errorf("corpus.journey_ids = %#v, want %q (the sorted resolved selection)", identity.Corpus.JourneyIDs, want)
	}
	if identity.Target.BinarySHA256 != testFileDigest(t, binary) {
		t.Errorf("target.binary_sha256 = %q, want the digest of the resolved target binary", identity.Target.BinarySHA256)
	}
	if identity.Target.Version != "test" {
		t.Errorf("target.version = %q, want the target's own --version output", identity.Target.Version)
	}
	if identity.Target.VCSRevision != "" || identity.Target.VCSModified != nil {
		t.Errorf("target vcs fields = (%q, %v), want empty for a stub binary built outside a git repository", identity.Target.VCSRevision, identity.Target.VCSModified)
	}
	if identity.Runtime.GOOS != runtime.GOOS || identity.Runtime.GOARCH != runtime.GOARCH {
		t.Errorf("runtime = (%q, %q), want (%q, %q)", identity.Runtime.GOOS, identity.Runtime.GOARCH, runtime.GOOS, runtime.GOARCH)
	}
	if !strings.HasPrefix(identity.Runtime.GitVersion, "git version") {
		t.Errorf("runtime.git_version = %q, want git's own version output", identity.Runtime.GitVersion)
	}
	if identity.Invocation.Mode != ModeDriven {
		t.Errorf("invocation.mode = %q, want %q", identity.Invocation.Mode, ModeDriven)
	}
	if identity.Invocation.ManifestName != "release-corpus" {
		t.Errorf("invocation.manifest_name = %q, want the release-corpus sentinel", identity.Invocation.ManifestName)
	}
	if len(identity.Invocation.Only) != 0 {
		t.Errorf("invocation.only = %#v, want empty without --only", identity.Invocation.Only)
	}
	if results.Binary != TargetBinaryToken {
		t.Errorf("binary = %q, want the %s token instead of a local path", results.Binary, TargetBinaryToken)
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if bytes.Contains(content, []byte(binary)) {
		t.Error("results file still contains the target binary path")
	}
	if bytes.Contains(content, []byte(t.TempDir())) {
		t.Error("results file still contains the run's temp directory path")
	}
	if err := results.ValidateCanonical(); err != nil {
		t.Errorf("ValidateCanonical() = %v, want nil", err)
	}
}

// TestCommandRunDigestStableAcrossSandboxes is the receipt property: two full
// runs of the same corpus against the same binary, each in its own temp
// sandbox and its own output directory, must produce byte-identical canonical
// files and therefore equal EvidenceDigest values.
func TestCommandRunDigestStableAcrossSandboxes(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a temporary executable to exercise the benchmark command boundary")
	}

	binary, _ := benchmarkTestBinary(t)
	outA := filepath.Join(t.TempDir(), "a.json")
	outB := filepath.Join(t.TempDir(), "b.json")
	resultsA := runDrivenAndRead(t, binary, outA)
	resultsB := runDrivenAndRead(t, binary, outB)

	if resultsA.EvidenceDigest == "" {
		t.Fatal("evidence_digest is empty; every written results file must carry it")
	}
	if resultsA.EvidenceDigest != resultsB.EvidenceDigest {
		t.Fatalf("evidence_digest differs across sandboxes: %q vs %q", resultsA.EvidenceDigest, resultsB.EvidenceDigest)
	}
	contentA, err := os.ReadFile(outA)
	if err != nil {
		t.Fatalf("read results a: %v", err)
	}
	contentB, err := os.ReadFile(outB)
	if err != nil {
		t.Fatalf("read results b: %v", err)
	}
	if !bytes.Equal(contentA, contentB) {
		t.Fatal("canonical files differ across sandboxes; the projection is not path-free")
	}
	if err := resultsA.ValidateCanonical(); err != nil {
		t.Errorf("ValidateCanonical() on file a = %v, want nil", err)
	}
	if err := resultsB.ValidateCanonical(); err != nil {
		t.Errorf("ValidateCanonical() on file b = %v, want nil", err)
	}
}

// TestEmptySelectorEarlyWriteCarriesV2Identity: the diagnostic envelope
// written when no journey matched --only is a results file too, and it must
// carry the same schema and identity contract.
func TestEmptySelectorEarlyWriteCarriesV2Identity(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a temporary executable to exercise the benchmark command boundary")
	}

	binary, _ := benchmarkTestBinary(t)
	out := filepath.Join(t.TempDir(), "results.json")
	args := []string{"--binary", binary, "--out", out, "--only", "does-not-exist"}
	if exit := commandRunWith(args, func(string) bool { return true }, testJourneyCorpus); exit != 1 {
		t.Fatalf("commandRunWith() exit = %d, want 1 for an empty selected population", exit)
	}
	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	var results Results
	if err := json.Unmarshal(content, &results); err != nil {
		t.Fatalf("decode results: %v", err)
	}
	if results.Schema != ResultsSchema {
		t.Fatalf("schema = %q, want %q", results.Schema, ResultsSchema)
	}
	if results.Identity == nil {
		t.Fatal("empty-selector envelope carries no identity")
	}
	if results.Identity.Corpus.JourneyIDs == nil || len(results.Identity.Corpus.JourneyIDs) != 0 {
		t.Errorf("corpus.journey_ids = %#v, want an empty (non-null) list", results.Identity.Corpus.JourneyIDs)
	}
	if results.Identity.Corpus.ManifestSHA256 != testManifestDigest(t) {
		t.Errorf("corpus.manifest_sha256 = %q, want the digest of the full registered corpus", results.Identity.Corpus.ManifestSHA256)
	}
	if want := []string{"does-not-exist"}; !reflect.DeepEqual(results.Identity.Invocation.Only, want) {
		t.Errorf("invocation.only = %#v, want %q", results.Identity.Invocation.Only, want)
	}
	if results.Binary != TargetBinaryToken {
		t.Errorf("binary = %q, want the %s token", results.Binary, TargetBinaryToken)
	}
}

// TestReadResultsFailsClosedOnUnknownSchema: compare/analyze must refuse a
// results file whose schema they do not understand, naming the file and the
// schema it found, instead of silently misreading it.
func TestReadResultsFailsClosedOnUnknownSchema(t *testing.T) {
	dir := t.TempDir()
	v1Path := filepath.Join(dir, "v1.json")
	v1Content := `{"schema":"gentle-ai-bench.results/v1","mode":"driven"}`
	if err := os.WriteFile(v1Path, []byte(v1Content), 0o644); err != nil {
		t.Fatalf("write v1 file: %v", err)
	}
	if _, err := readResults(v1Path); err == nil {
		t.Fatal("readResults accepted a v1 results file; the read path must fail closed on unknown schemas")
	} else if !strings.Contains(err.Error(), v1Path) || !strings.Contains(err.Error(), "gentle-ai-bench.results/v1") {
		t.Fatalf("readResults error %q must name both the file and the schema it found", err)
	}

	v2Path := filepath.Join(dir, "v2.json")
	if err := writeJSON(v2Path, Results{Schema: ResultsSchema, Mode: ModeDriven}); err != nil {
		t.Fatalf("write v2 file: %v", err)
	}
	results, err := readResults(v2Path)
	if err != nil {
		t.Fatalf("readResults() = %v, want nil for the current schema", err)
	}
	if results.Mode != ModeDriven {
		t.Errorf("mode = %q, want %q", results.Mode, ModeDriven)
	}
}

// TestCanonicalDigestExcludesEvidenceDigestAndIsWrittenLast pins the digest
// semantics: it is computed over the exact file bytes with the digest field
// cleared, the field is printed last in the file, and tampering with the
// field never changes the recomputed digest.
func TestCanonicalDigestExcludesEvidenceDigestAndIsWrittenLast(t *testing.T) {
	results := Results{
		Schema:   ResultsSchema,
		Mode:     ModeDriven,
		Binary:   TargetBinaryToken,
		Journeys: []JourneyResult{{ID: "j", Status: StatusCompleted}},
	}
	path := filepath.Join(t.TempDir(), "results.json")
	if err := writeJSON(path, results); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	digestIndex := bytes.Index(content, []byte(`"evidence_digest"`))
	if digestIndex < 0 {
		t.Fatal("written file carries no evidence_digest field")
	}
	if totalsIndex := bytes.Index(content, []byte(`"totals"`)); digestIndex < totalsIndex {
		t.Fatal("evidence_digest must be printed last in the file, outside the digested content")
	}

	var written Results
	if err := json.Unmarshal(content, &written); err != nil {
		t.Fatalf("decode results: %v", err)
	}
	if written.EvidenceDigest == "" {
		t.Fatal("written file carries an empty evidence_digest")
	}
	if !strings.HasPrefix(written.EvidenceDigest, "sha256:") {
		t.Errorf("evidence_digest = %q, want the sha256: prefix", written.EvidenceDigest)
	}
	if got := written.CanonicalDigest(); got != written.EvidenceDigest {
		t.Fatalf("CanonicalDigest() = %q, want the file's own %q", got, written.EvidenceDigest)
	}
	untampered := written.EvidenceDigest
	written.EvidenceDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	if got := written.CanonicalDigest(); got != untampered {
		t.Fatalf("CanonicalDigest() after tampering = %q, want %q: the digest must exclude the evidence_digest field itself", got, untampered)
	}
}

// TestPathNormalizerAppliesLongestSpecificTokensFirst pins the closed token
// vocabulary: repository and home (the specific paths under the run root)
// must be replaced before the run root itself, and the target binary is
// matched as its exact resolved path. A nil normalizer is the identity.
func TestPathNormalizerAppliesLongestSpecificTokensFirst(t *testing.T) {
	normalizer := newPathNormalizer("/opt/binaries/gentle-ai", "/tmp/prov-root")
	normalizer.setSandbox("/tmp/prov-root/home", "/tmp/prov-root/home/demo")
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"repository beats home and run root", "/tmp/prov-root/home/demo/.git/config", RepositoryToken + "/.git/config"},
		{"home beats run root", "/tmp/prov-root/home/.config/x", HomeToken + "/.config/x"},
		{"run root catches the rest", "/tmp/prov-root/tmp/scratch", RunRootToken + "/tmp/scratch"},
		{"target binary exact path", "/opt/binaries/gentle-ai", TargetBinaryToken},
		{"target binary inside an argv string", "exec /opt/binaries/gentle-ai --json", "exec " + TargetBinaryToken + " --json"},
		{"untouched string", "review start --json", "review start --json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizer.Normalize(tt.in); got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
	var nilNormalizer *PathNormalizer
	if got := nilNormalizer.Normalize("/tmp/x"); got != "/tmp/x" {
		t.Fatalf("nil normalizer changed %q into %q", "/tmp/x", got)
	}
	if got := nilNormalizer.NormalizeAll([]string{"/tmp/x"}); !reflect.DeepEqual(got, []string{"/tmp/x"}) {
		t.Fatalf("nil normalizer changed args: %q", got)
	}
	if got := normalizer.NormalizeAll(nil); got != nil {
		t.Fatalf("NormalizeAll(nil) = %#v, want nil", got)
	}
	if got := normalizer.NormalizeAll([]string{"/tmp/prov-root/home/demo", "plain"}); !reflect.DeepEqual(got, []string{RepositoryToken, "plain"}) {
		t.Fatalf("NormalizeAll = %#v", got)
	}
}

// TestValidateCanonicalFailsClosedOnRecordedPaths pins the fail-closed
// direction of the projection: a Results carrying any string that still
// contains a machine-specific path this process recorded — the sandbox root,
// the repository or home under it, the target binary, or the user home — must
// refuse to validate, and the normalized projection must pass.
func TestValidateCanonicalFailsClosedOnRecordedPaths(t *testing.T) {
	normalizer := newPathNormalizer("/tmp/prov-target/gentle-ai", "/tmp/prov-root")
	normalizer.setSandbox("/tmp/prov-root/home", "/tmp/prov-root/home/demo")

	for _, tt := range []struct {
		name   string
		build  func() Results
		wantOK bool
	}{
		{"clean projection passes", func() Results {
			return Results{
				Binary: TargetBinaryToken,
				Journeys: []JourneyResult{{ID: "j", Status: StatusCompleted, Commands: []CommandRecord{{
					Args: []string{"review", "start", "--cwd", RepositoryToken},
				}}}},
				Notes: []string{"no paths here"},
			}
		}, true},
		{"binary leaks the target path", func() Results {
			return Results{Binary: "/tmp/prov-target/gentle-ai"}
		}, false},
		{"command args leak the repository path", func() Results {
			return Results{Journeys: []JourneyResult{{ID: "j", Commands: []CommandRecord{{
				Args: []string{"review", "start", "--cwd", "/tmp/prov-root/home/demo"},
			}}}}}
		}, false},
		{"command message leaks the sandbox root", func() Results {
			return Results{Journeys: []JourneyResult{{ID: "j", Commands: []CommandRecord{{
				Args:    []string{"review"},
				Message: "bench: cannot read /tmp/prov-root/state",
			}}}}}
		}, false},
		{"journey failure reason leaks the home path", func() Results {
			return Results{Journeys: []JourneyResult{{ID: "j", Status: StatusFailed, FailureReason: "fixture: cannot write /tmp/prov-root/home/x"}}}
		}, false},
		{"unsupported steps leak the sandbox root", func() Results {
			return Results{Journeys: []JourneyResult{{ID: "j", Status: StatusUnsupported, UnsupportedSteps: []string{"step (surface not present: /tmp/prov-root/probe)"}}}}
		}, false},
		{"notes leak the run root", func() Results {
			return Results{Notes: []string{"see /tmp/prov-root/scratch"}}
		}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build().ValidateCanonical()
			if tt.wantOK && err != nil {
				t.Fatalf("ValidateCanonical() = %v, want nil", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("ValidateCanonical() = nil, want a fail-closed error")
			}
		})
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
		leaking := Results{FailureReason: filepath.Join(home, "somewhere")}
		if err := leaking.ValidateCanonical(); err == nil {
			t.Fatal("ValidateCanonical() = nil for a string carrying the user home prefix, want a fail-closed error")
		}
	}
}

func TestAnalyzeSessionProjectsRecordedHomePaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no user home available: %v", err)
	}
	records := []SessionRecord{
		{Argv: []string{"review", "start", "--cwd", home + "/projects/demo"}, ExitCode: 0,
			Stdout: `{"state":"reviewing"}`, StderrCaptured: true},
		{Argv: []string{"review", "capture-result", "--input", home + "/projects/demo/r.json"}, ExitCode: 1,
			Stderr: "Error: capture it first with `gentle-ai review capture-evidence`\n", StderrCaptured: true},
	}
	normalizer := newObservedNormalizer()
	result := analyzeSession(records, normalizer)

	for _, record := range result.Commands {
		for _, arg := range record.Args {
			if strings.Contains(arg, home) {
				t.Fatalf("record arg still carries the user home: %q", arg)
			}
		}
		if strings.Contains(record.Message, home) {
			t.Fatalf("record message still carries the user home: %q", record.Message)
		}
	}
	for _, step := range result.UnsupportedSteps {
		if strings.Contains(step, home) {
			t.Fatalf("unsupported step still carries the user home: %q", step)
		}
	}
	found := false
	for _, record := range result.Commands {
		for _, arg := range record.Args {
			if strings.Contains(arg, HomeToken) {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected at least one %s token in recorded args", HomeToken)
	}

	results := Results{
		Schema:   ResultsSchema,
		Mode:     ModeObserved,
		Binary:   normalizer.Normalize(home + "/sessions/agent.jsonl"),
		Journeys: []JourneyResult{result},
		Identity: newObservedIdentity(),
	}
	results.Totals, results.JourneysCounted, results.JourneysUnsupported, results.JourneysFailed = aggregate(results.Journeys)
	path := filepath.Join(t.TempDir(), "observed.json")
	if err := writeResultsJSON(path, results); err != nil {
		t.Fatalf("observed results with home paths must write canonically, got: %v", err)
	}
}
