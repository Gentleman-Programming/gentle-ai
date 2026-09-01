package assets

import (
	"crypto/sha256"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// #3817: the SDD orchestrator contract lives in twelve hand-maintained
// near-duplicates. Measured across them, only two of the twenty-one shared
// subsections were byte-identical; Delegation Rules alone had eleven variants
// across eleven runtimes. Reconciling those is a decision per section, not a
// refactor, so this ratchet does the next best thing: it makes the drift
// visible and stops it growing.
//
// Lowering a number here is progress. Raising one means a section was edited in
// some runtimes and not the others — the exact failure that left ten runtimes
// carrying a contradicted dispatcher guard after the first correction.
var orchestratorSectionDriftRatchet = []struct {
	name        string
	maxVariants int
}{
	{"Delegation Rules", 11},
	{"Execution Mode", 10},
	{"Review Workload Guard (MANDATORY)", 10},
	{"State and Conventions", 10},
	{"Automatic Mode Gatekeeper (MANDATORY)", 9},
	{"Commands", 9},
	{"Skill Resolution Feedback", 8},
	{"Artifact Store Mode", 7},
	{"SDD Init Guard (MANDATORY)", 7},
	{"Agent Teams Orchestrator", 6},
	{"Chain Strategy", 6},
	{"Delivery Strategy", 4},
	{"Lossless Blocking Prompts (MANDATORY)", 4},
	{"Artifact Store Policy", 3},
	{"Result Contract", 3},
	{"Dependency Graph", 2},
	{"Language Domain Contract", 2},
	{"Recovery Rule", 2},
	{"SDD Workflow (Spec-Driven Development)", 2},
	{"Native Runtime Attempt Authority (MANDATORY)", 1},
	{"Native SDD Dispatcher Guard", 1},
}

var orchestratorHeading = regexp.MustCompile(`(?m)^#{2,3} .+$`)

// orchestratorSectionVariants maps each subsection name to the distinct bodies
// the runtime orchestrators give it.
func orchestratorSectionVariants(t *testing.T) map[string]map[string]struct{} {
	t.Helper()
	variants := map[string]map[string]struct{}{}
	err := fs.WalkDir(FS, ".", func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Base(assetPath) != "sdd-orchestrator.md" {
			return nil
		}
		content := MustRead(assetPath)
		headings := orchestratorHeading.FindAllStringIndex(content, -1)
		for i, span := range headings {
			name := strings.TrimSpace(strings.TrimLeft(content[span[0]:span[1]], "# "))
			end := len(content)
			if i+1 < len(headings) {
				end = headings[i+1][0]
			}
			body := strings.TrimSpace(content[span[1]:end])
			digest := sha256.Sum256([]byte(body))
			if variants[name] == nil {
				variants[name] = map[string]struct{}{}
			}
			variants[name][string(digest[:])] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk orchestrator assets: %v", err)
	}
	return variants
}

// TestOrchestratorSectionDriftDoesNotGrow fails when a shared subsection gains
// a new per-runtime variant.
func TestOrchestratorSectionDriftDoesNotGrow(t *testing.T) {
	variants := orchestratorSectionVariants(t)
	for _, pinned := range orchestratorSectionDriftRatchet {
		got := len(variants[pinned.name])
		if got == 0 {
			t.Errorf("section %q vanished from every runtime orchestrator; remove its ratchet entry deliberately", pinned.name)
			continue
		}
		if got > pinned.maxVariants {
			t.Errorf("section %q drifted to %d variants, ratchet allows %d — edit every runtime or move the section to the shared asset", pinned.name, got, pinned.maxVariants)
		}
		if got < pinned.maxVariants {
			t.Logf("section %q converged to %d variants (ratchet %d); lower the ratchet", pinned.name, got, pinned.maxVariants)
		}
	}
}
