package main

// The results/v2 identity envelope and the canonical path-free projection
// (issue #1866, slice B).
//
// A results file is evidence, and evidence has to answer four questions
// without leaking the machine it was produced on:
//
//   - WHO measured: which bench source revision, which bench executable, which
//     Go toolchain, and which classifier rule set (classifier_version).
//   - WHAT population: a digest over the full registered corpus, plus the
//     sorted journey IDs this run actually resolved.
//   - WHICH target: the binary under test, pinned by content digest, its own
//     reported version, and its VCS stamp when it was built inside a git
//     repository.
//   - HOW: the mode, the corpus manifest name, and the raw --only selectors.
//
// Every string that could carry a machine-specific path is projected onto a
// closed token vocabulary at record time, so the written file is path-free by
// construction rather than by after-the-fact scrubbing. ValidateCanonical is
// the fail-closed net: a results file that still carries a known
// machine-specific path is never written at all.
//
// CanonicalDigest pins the exact bytes writeJSON emits, with the digest field
// itself cleared, so two runs of the same corpus produce byte-identical files
// and their digests can be compared like receipts. The digest lives OUTSIDE
// the digested content: it is the last field in the file, computed over the
// file without it.

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// The closed token vocabulary of the canonical projection. A token stands for
// a path this process knew at record time; everything after the token is the
// sandbox-relative tail, so a reader can still follow the flow without
// learning where the sandbox lived.
const (
	RunRootToken      = "<RUN_ROOT>"
	RepositoryToken   = "<REPOSITORY>"
	HomeToken         = "<HOME>"
	TargetBinaryToken = "<TARGET_BINARY>"
)

// ReleaseCorpusManifestName is the sentinel manifest name recorded in
// invocation.manifest_name. The named release-core-v1 manifest file lands in a
// later slice; until then the envelope says which corpus contract a run
// claims, and the sentinel is that claim.
const ReleaseCorpusManifestName = "release-corpus"

// EvidenceDigestPrefix starts every evidence digest value.
const EvidenceDigestPrefix = "sha256:"

// Identity is the provenance envelope of a results file: who measured, what
// population, against which target, on which runtime, with which invocation.
// It is populated on every write path, including the empty-selector early
// write and observed mode (which fills what is knowable and leaves the target
// section empty).
type Identity struct {
	Benchmark  BenchmarkIdentity  `json:"benchmark"`
	Corpus     CorpusIdentity     `json:"corpus"`
	Target     TargetIdentity     `json:"target"`
	Runtime    RuntimeIdentity    `json:"runtime"`
	Invocation InvocationIdentity `json:"invocation"`
}

// BenchmarkIdentity pins the harness that produced the file.
type BenchmarkIdentity struct {
	// SourceRevision is `git rev-parse HEAD` of the bench module's
	// repository, empty for a local non-git run.
	SourceRevision string `json:"source_revision,omitempty"`
	// BinarySHA256 is the digest of the bench executable itself, empty
	// when the executable is a throwaway `go run` / `go test` temp build.
	BinarySHA256 string `json:"binary_sha256,omitempty"`
	GoVersion    string `json:"go_version,omitempty"`
	// ClassifierVersion identifies the classifier rule set. Any change to
	// Classify's rules must bump it; see the README.
	ClassifierVersion string `json:"classifier_version,omitempty"`
}

// CorpusIdentity pins the population: the manifest digest over the FULL
// registered corpus (independent of --only) and the resolved selection this
// run actually drove.
type CorpusIdentity struct {
	ManifestSHA256 string `json:"manifest_sha256,omitempty"`
	// JourneyIDs is the sorted resolved selection. It is deliberately NOT
	// omitempty: an empty selection (the empty-selector envelope) must
	// read as [], never as an absent field.
	JourneyIDs []string `json:"journey_ids"`
}

// TargetIdentity pins the binary under test by content and VCS stamp.
type TargetIdentity struct {
	Version      string `json:"version,omitempty"`
	BinarySHA256 string `json:"binary_sha256,omitempty"`
	VCSRevision  string `json:"vcs_revision,omitempty"`
	// VCSModified is the build's `vcs.modified` setting: whether the
	// target was built from a dirty worktree. Nil when unknowable (a
	// binary built outside a git repository carries no such setting).
	VCSModified *bool `json:"vcs_modified,omitempty"`
}

// RuntimeIdentity pins the platform and the git the run inherited.
type RuntimeIdentity struct {
	GOOS       string `json:"goos,omitempty"`
	GOARCH     string `json:"goarch,omitempty"`
	GitVersion string `json:"git_version,omitempty"`
}

// InvocationIdentity records what the operator asked for.
type InvocationIdentity struct {
	Mode         string   `json:"mode,omitempty"`
	ManifestName string   `json:"manifest_name,omitempty"`
	Only         []string `json:"only,omitempty"`
}

// newDrivenIdentity builds the envelope for a driven run. journeyIDs is the
// run's resolved selection (sorted for the envelope); only is the raw
// selector list as typed.
func newDrivenIdentity(targetBinary, targetVersion string, journeyIDs, only []string) *Identity {
	revision, modified := targetVCSIdentity(targetBinary)
	return &Identity{
		Benchmark: benchmarkIdentity(),
		Corpus: CorpusIdentity{
			ManifestSHA256: registeredCorpusManifestDigest(),
			JourneyIDs:     sortedStrings(journeyIDs),
		},
		Target: TargetIdentity{
			Version:      targetVersion,
			BinarySHA256: fileSHA256(targetBinary),
			VCSRevision:  revision,
			VCSModified:  modified,
		},
		Runtime: runtimeIdentity(),
		Invocation: InvocationIdentity{
			Mode:         ModeDriven,
			ManifestName: ReleaseCorpusManifestName,
			Only:         only,
		},
	}
}

// newObservedIdentity builds the envelope for an observed run. Only what is
// knowable is filled: runtime, benchmark provenance, and the corpus manifest
// digest. An observed session has no target binary of the product under
// drive-by recording and no selector list, so those stay empty.
func newObservedIdentity() *Identity {
	return &Identity{
		Benchmark: benchmarkIdentity(),
		Corpus: CorpusIdentity{
			ManifestSHA256: registeredCorpusManifestDigest(),
			JourneyIDs:     []string{},
		},
		Runtime: runtimeIdentity(),
		Invocation: InvocationIdentity{
			Mode:         ModeObserved,
			ManifestName: ReleaseCorpusManifestName,
		},
	}
}

func benchmarkIdentity() BenchmarkIdentity {
	return BenchmarkIdentity{
		SourceRevision:    benchSourceRevision(),
		BinarySHA256:      benchExecutableSHA256(),
		GoVersion:         runtime.Version(),
		ClassifierVersion: ClassifierVersion,
	}
}

func runtimeIdentity() RuntimeIdentity {
	return RuntimeIdentity{
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		GitVersion: gitOutput("--version"),
	}
}

// registeredCorpusManifestDigest digests the FULL registered corpus —
// Journeys() as registered, retirement already applied — never a selection,
// so two envelopes claiming the same manifest digest describe the same
// population no matter what --only said.
func registeredCorpusManifestDigest() string {
	ids := make([]string, 0, len(Journeys()))
	for _, journey := range Journeys() {
		ids = append(ids, journey.ID)
	}
	return corpusManifestDigest(ids)
}

// corpusManifestDigest is sha256 hex over the sorted newline-joined IDs.
func corpusManifestDigest(ids []string) string {
	sum := sha256.Sum256([]byte(strings.Join(sortedStrings(ids), "\n")))
	return hex.EncodeToString(sum[:])
}

// benchModuleDir is the bench module root, resolved from this source file's
// location so git commands run inside the repository even when the binary is
// executed from anywhere else.
func benchModuleDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(file)
}

// gitOutput runs one git command in the bench module's repository and returns
// its trimmed stdout, or "" on any failure: a local non-git run is a legal
// environment and records empty provenance instead of crashing.
func gitOutput(args ...string) string {
	dir := benchModuleDir()
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func benchSourceRevision() string {
	return gitOutput("rev-parse", "HEAD")
}

// benchExecutableSHA256 digests the running bench executable. A `go run` or
// `go test` temp build lives under the process temp dir in a go-build path,
// vanishes afterwards and differs per invocation, so its digest is
// meaningless and recorded as empty instead.
func benchExecutableSHA256() string {
	executable, err := os.Executable()
	if err != nil || isGoRunTemporaryBuild(executable) {
		return ""
	}
	return fileSHA256(executable)
}

func isGoRunTemporaryBuild(executable string) bool {
	temp := os.TempDir()
	if temp == "" {
		return false
	}
	slashed := filepath.ToSlash(executable)
	if !strings.HasPrefix(slashed, filepath.ToSlash(filepath.Clean(temp))+"/") {
		return false
	}
	return strings.Contains(slashed, "/go-build")
}

// fileSHA256 digests a file's content, streaming; "" on any failure.
func fileSHA256(path string) string {
	if path == "" {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// targetVCSIdentity reads the target binary's own VCS stamp from its build
// info. A binary built outside a git repository carries no vcs settings, and
// that is recorded as empty/nil, never as a guess.
func targetVCSIdentity(path string) (string, *bool) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return "", nil
	}
	revision := ""
	var modified *bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			value := setting.Value == "true"
			modified = &value
		}
	}
	return revision, modified
}

func sortedStrings(values []string) []string {
	sorted := append([]string{}, values...)
	sort.Strings(sorted)
	return sorted
}

// PathNormalizer projects machine-specific paths onto the closed token
// vocabulary at record time. It is wired where records are folded
// (accumulator.observe) and where Results.Binary is set, so a results file is
// path-free by construction.
//
// Replacement order is longest-specific-first: the repository and the home
// (specific paths under the run root) are replaced before the run root, and
// the target binary is matched as its exact resolved path.
//
// A nil *PathNormalizer is the identity: call sites from before this envelope
// existed, and code paths with no paths to project, keep working unchanged.
type PathNormalizer struct {
	targetBinary string
	runRoot      string
	home         string
	repository   string
}

// newPathNormalizer builds a normalizer for one run and registers the paths
// it replaces as validation guards, so ValidateCanonical can fail closed on
// any string that escaped projection.
func newPathNormalizer(targetBinary, runRoot string) *PathNormalizer {
	normalizer := &PathNormalizer{targetBinary: targetBinary, runRoot: runRoot}
	normalizer.registerGuards()
	return normalizer
}

// setSandbox adds a journey's sandbox layout. The sandbox is created per
// journey inside runJourney, after the run-level normalizer exists.
func (n *PathNormalizer) setSandbox(home, repository string) {
	n.home = home
	n.repository = repository
	n.registerGuards()
}

// setRunRoot adds the sandbox root once it is known (it is canonicalized
// after creation, so the recorded spelling is the one replaced).
func (n *PathNormalizer) setRunRoot(root string) {
	n.runRoot = root
	n.registerGuards()
}

// newObservedNormalizer builds the home-only projection used by observed
// mode: a recorded session has no sandbox and no target binary, so the
// user home is the only path that can be tokenized.
func newObservedNormalizer() *PathNormalizer {
	normalizer := newPathNormalizer("", "")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		normalizer.setSandbox(home, "")
	}
	return normalizer
}

// Normalize replaces every known machine-specific path with its token,
// longest-specific-first, leaving everything else untouched.
func (n *PathNormalizer) Normalize(value string) string {
	if n == nil || value == "" {
		return value
	}
	for _, replacement := range []struct{ from, to string }{
		{n.repository, RepositoryToken},
		{n.home, HomeToken},
		{n.runRoot, RunRootToken},
		{n.targetBinary, TargetBinaryToken},
	} {
		if replacement.from == "" {
			continue
		}
		value = strings.ReplaceAll(value, replacement.from, replacement.to)
	}
	return value
}

// NormalizeAll projects a slice of strings (argv, notes, unsupported steps),
// preserving nil.
func (n *PathNormalizer) NormalizeAll(values []string) []string {
	if n == nil || values == nil {
		return values
	}
	normalized := make([]string, len(values))
	for index, value := range values {
		normalized[index] = n.Normalize(value)
	}
	return normalized
}

// Validation guards: every path a normalizer knows how to replace is also a
// path ValidateCanonical refuses to see in a written file. They accumulate
// for the life of the process, which only ever makes the check stricter, and
// the user home is appended at validation time.
var (
	pathGuardsMutex      sync.Mutex
	registeredPathGuards = map[string]bool{}
)

func registerPathGuard(path string) {
	guard := filepath.Clean(path)
	if !filepath.IsAbs(guard) || guard == string(filepath.Separator) {
		return
	}
	pathGuardsMutex.Lock()
	defer pathGuardsMutex.Unlock()
	registeredPathGuards[guard] = true
}

func (n *PathNormalizer) registerGuards() {
	registerPathGuard(n.targetBinary)
	registerPathGuard(n.runRoot)
	registerPathGuard(n.home)
	registerPathGuard(n.repository)
}

func activePathGuards() []string {
	pathGuardsMutex.Lock()
	defer pathGuardsMutex.Unlock()
	guards := make([]string, 0, len(registeredPathGuards))
	for guard := range registeredPathGuards {
		guards = append(guards, guard)
	}
	sort.Strings(guards)
	return guards
}

// ValidateCanonical fails closed when any recorded string still carries a
// machine-specific path this process knows about: a sandbox root, the
// repository or home under it, the target binary, or the user home prefix.
// Record-time projection makes this a property that holds by construction;
// this method is the net that stops a leaked path from ever reaching disk.
func (r Results) ValidateCanonical() error {
	guards := activePathGuards()
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != string(filepath.Separator) {
		guards = append(guards, home)
	}
	violations := []string{}
	record := func(field, value string) {
		if value == "" {
			return
		}
		for _, guard := range guards {
			if strings.Contains(value, guard) {
				violations = append(violations, fmt.Sprintf("%s still contains %s", field, guard))
			}
		}
	}
	record("binary", r.Binary)
	record("failure_reason", r.FailureReason)
	for index, note := range r.Notes {
		record(fmt.Sprintf("notes[%d]", index), note)
	}
	for _, journey := range r.Journeys {
		record(journey.ID+".failure_reason", journey.FailureReason)
		for index, step := range journey.UnsupportedSteps {
			record(fmt.Sprintf("%s.unsupported_steps[%d]", journey.ID, index), step)
		}
		for _, command := range journey.Commands {
			for index, arg := range command.Args {
				record(fmt.Sprintf("%s.commands[%d].args[%d]", journey.ID, command.Sequence, index), arg)
			}
			record(fmt.Sprintf("%s.commands[%d].message", journey.ID, command.Sequence), command.Message)
		}
	}
	if len(violations) == 0 {
		return nil
	}
	return errors.New("results are not canonical (recorded strings still carry machine-specific paths): " +
		strings.Join(violations, "; "))
}

// CanonicalDigest returns the digest of the exact canonical file bytes
// writeJSON emits for these results, with the evidence_digest field itself
// cleared: "sha256:" + hex. Two runs of the same corpus against the same
// target produce equal digests; that equality is the receipt. It returns ""
// if the results cannot be marshaled, which cannot happen for this struct's
// fixed JSON-safe field types in practice; writeResultsJSON surfaces the
// error instead.
func (r Results) CanonicalDigest() string {
	digest, _ := r.canonicalDigest()
	return digest
}

func (r Results) canonicalDigest() (string, error) {
	r.EvidenceDigest = ""
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(encoded, '\n'))
	return EvidenceDigestPrefix + hex.EncodeToString(sum[:]), nil
}
