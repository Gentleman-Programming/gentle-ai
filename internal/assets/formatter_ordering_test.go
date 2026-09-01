package assets

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/versions"
)

func TestSDDOrchestratorsRequireSafeFormatterOrdering(t *testing.T) {
	for _, path := range allSDDOrchestratorAssetPaths(t) {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			for _, required := range []string{
				"Normalization ordering rule",
				"before review START and its identity freeze",
				"run every source-mutating normalizer",
				"re-snapshot the candidate",
				"exact bytes, paths, and modes",
				"only check-only formatting, typechecking, tests, and native gates",
				"already convergent and therefore a no-op",
				"any byte, path, or mode change invalidates the receipt",
				"normalization followed by a new review",
				"never formatter-only tolerance",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing formatter-ordering contract %q", path, required)
				}
			}
		})
	}
}

func TestRequiredChecksFailClosedWhenFormatFails(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	guard := "    steps:\n" +
		"      - name: Require successful Go format\n" +
		"        if: needs.go-format.result != 'success'\n" +
		"        run: exit 1\n"

	jobs := []struct{ id, next string }{
		{id: "unit-tests", next: "windows-runtime"},
		// The sharded Windows full suite is deliberately NOT here: it lives in
		// its own workflow while the pre-existing Windows failures it exposed
		// are paid down, so it is not one of ci.yml's required checks and owes
		// no fail-closed contract to a go-format job in another file.
		{id: "windows-runtime", next: "darwin-runtime"},
		// darwin-runtime is a required check like its siblings, so it owes the
		// same fail-closed contract. Listing it here is also what keeps the
		// section slicing honest: a job this list does not know about gets
		// swallowed into its predecessor's section, and its own guard step
		// then reads as the predecessor bypassing the format gate.
		{id: "darwin-runtime", next: "organic-runtime-e2e"},
		{id: "organic-runtime-e2e", next: "e2e-tests"},
		{id: "e2e-tests"},
	}
	for _, job := range jobs {
		t.Run(job.id, func(t *testing.T) {
			marker := "  " + job.id + ":\n"
			start := strings.Index(workflow, marker)
			if start < 0 {
				t.Fatalf("missing required job %q", job.id)
			}
			section := workflow[start:]
			if job.next != "" {
				end := strings.Index(section, "\n  "+job.next+":")
				if end < 0 {
					t.Fatalf("missing job boundary %q", job.next)
				}
				section = section[:end]
			}
			for _, required := range []string{"    needs: go-format\n", "    if: always()\n", guard} {
				if !strings.Contains(section, required) {
					t.Fatalf("%s missing fail-closed contract %q", job.id, required)
				}
			}

			expensiveSteps := section[strings.Index(section, guard)+len(guard):]
			if strings.Contains(expensiveSteps, "\n        if:") || strings.Contains(section, "continue-on-error:") {
				t.Fatalf("%s can bypass the failed format guard", job.id)
			}
		})
	}
}

func TestOrganicRuntimeE2EUsesInstalledOpenCodePin(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	install := "npm install --global opencode-ai@" + versions.OpenCode
	if strings.Count(string(data), install) != 1 {
		t.Fatalf("organic runtime E2E must install exact supported OpenCode pin %q once", install)
	}
	for _, required := range []string{
		"TestOpenCodeRuntimeIsPinnedForTheLiveProviderTransport",
		`GENTLE_AI_OPENCODE_RUNTIME_E2E: ${{ matrix.os == 'ubuntu-latest' && '1' || '0' }}`,
	} {
		if !strings.Contains(string(data), required) {
			t.Fatalf("organic runtime E2E is missing live OpenCode provider isolation guard %q", required)
		}
	}
}

func TestWindowsReleaseBlockerCannotSkipOwnerRebinding(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(
		"..",
		"..",
		".github",
		"workflows",
		"ci.yml",
	))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"TestRARPrivateOwnerRemainsTokenUserOnly",
		`GENTLE_AI_REQUIRE_DISTINCT_WINDOWS_TOKEN_OWNER: "1"`,
	} {
		if !strings.Contains(string(workflow), required) {
			t.Fatalf(
				"Windows release-blocker workflow missing %q",
				required,
			)
		}
	}
	if strings.Contains(string(workflow), "internal/deliveryadmission") {
		t.Fatal(
			"Windows release-blocker workflow still tests the retired deliveryadmission package",
		)
	}

	testSource, err := os.ReadFile(filepath.Join(
		"..",
		"reviewtransaction",
		"rar_path_safety_windows_test.go",
	))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		string(testSource),
		`os.Getenv("GENTLE_AI_REQUIRE_DISTINCT_WINDOWS_TOKEN_OWNER") == "1"`,
	) {
		t.Fatal(
			"native Windows owner-rebinding test can skip the release precondition",
		)
	}
}

type formatterCandidate struct {
	path    string
	mode    string
	content string
}

type formatterOrderingFixture struct {
	candidate formatterCandidate
	reviewed  formatterCandidate
	receipt   bool
	commits   int
}

func normalizeFixture(candidate *formatterCandidate) {
	candidate.content = strings.ReplaceAll(candidate.content, "x=1", "x = 1")
}

func (fixture *formatterOrderingFixture) startReview() {
	fixture.reviewed = fixture.candidate
	fixture.receipt = true
}

func (fixture *formatterOrderingFixture) runCommitHook(hook func(*formatterCandidate)) bool {
	frozen := fixture.identity(fixture.reviewed)
	hook(&fixture.candidate)
	if fixture.identity(fixture.candidate) != frozen {
		fixture.receipt = false
		return false
	}
	fixture.commits++
	return true
}

func (fixture formatterOrderingFixture) identity(candidate formatterCandidate) string {
	return fmt.Sprintf("%s\x00%s\x00%s", candidate.path, candidate.mode, candidate.content)
}

func TestFormatterOrderingBehaviorContract(t *testing.T) {
	t.Run("formatter mutates before review then converges at commit", func(t *testing.T) {
		fixture := formatterOrderingFixture{candidate: formatterCandidate{path: "main.go", mode: "100644", content: "var x=1\n"}}
		normalizeFixture(&fixture.candidate)
		fixture.startReview()

		if !fixture.runCommitHook(normalizeFixture) {
			t.Fatal("convergent formatter hook rejected frozen reviewed bytes")
		}
		if !fixture.receipt || fixture.commits != 1 || fixture.candidate != fixture.reviewed {
			t.Fatalf("receipt=%t commits=%d candidate=%+v reviewed=%+v", fixture.receipt, fixture.commits, fixture.candidate, fixture.reviewed)
		}
	})

	mutations := map[string]func(*formatterCandidate){
		"byte": func(candidate *formatterCandidate) { candidate.content += "// changed\n" },
		"path": func(candidate *formatterCandidate) { candidate.path = "renamed.go" },
		"mode": func(candidate *formatterCandidate) { candidate.mode = "100755" },
	}
	for name, mutate := range mutations {
		t.Run(name+" mutation after freeze fails closed", func(t *testing.T) {
			fixture := formatterOrderingFixture{candidate: formatterCandidate{path: "main.go", mode: "100644", content: "var x = 1\n"}}
			fixture.startReview()

			if fixture.runCommitHook(mutate) || fixture.receipt || fixture.commits != 0 {
				t.Fatalf("mutation passed: receipt=%t commits=%d", fixture.receipt, fixture.commits)
			}
		})
	}
}

// TestWindowsFullSuiteShardsCoverEveryTestName proves the sharded Windows lane
// still runs every test. Sharding trades one long job for several short ones,
// and its failure mode is silent: a selector edited to stop matching some tests
// skips them while every shard, and the lane, still reports green.
//
// The shards used to be letter ranges tiling A-Z, checked by range algebra.
// That was cheap to verify but impossible to balance: Go test names cluster by
// prefix and so does their cost, so one letter (TestReview*, 423 of the 649
// TestR* tests in internal/cli) can own more work than a whole shard's budget.
// Selectors are therefore arbitrary -run regexes now, and coverage is proved
// against the package's real `go test -list` inventory instead: every test that
// exists must be claimed by exactly one shard. That is strictly stronger than
// the range check -- it catches an uncovered name even when the ranges look
// contiguous -- and it leaves the split free to follow measured cost.
func TestWindowsFullSuiteShardsCoverEveryTestName(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "windows-full-suite.yml"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(data), "  windows-full-suite:")
	if start < 0 {
		t.Fatal("missing windows-full-suite job")
	}
	section := string(data)[start:]
	// Stop at the step list: steps carry their own `run:` keys, and a shell
	// command is not a shard selector.
	if end := strings.Index(section, "\n    steps:"); end >= 0 {
		section = section[:end]
	}

	// Selectors are read straight out of the matrix, so this cannot drift from
	// what actually runs the way a copy of the list would.
	type shard struct {
		name     string
		pkg      string
		selector string
		matcher  *regexp.Regexp
	}
	var shards []shard
	var name string
	var pkg string
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(trimmed, "- name: "); ok {
			name = strings.Trim(value, `"`)
		}
		if value, ok := strings.CutPrefix(trimmed, "packages: "); ok {
			pkg = strings.Trim(value, `"`)
		}
		value, ok := strings.CutPrefix(trimmed, "run: ")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"`)
		if value == "" {
			// The `rest` shard selects by package, not by name.
			continue
		}
		matcher, err := regexp.Compile(value)
		if err != nil {
			t.Fatalf("shard %q selector %q does not compile as a -run regex: %v", name, value, err)
		}
		shards = append(shards, shard{name: name, pkg: pkg, selector: value, matcher: matcher})
	}
	if len(shards) == 0 {
		t.Fatal("no name-selecting shards found; the guard would pass vacuously")
	}

	if strings.Contains(string(data), "reviewtransaction-a-k") {
		t.Fatal("former reviewtransaction-a-k aggregate is still present")
	}

	byPackage := map[string][]shard{}
	for _, s := range shards {
		if s.pkg == "" {
			t.Fatalf("shard %q selects tests by name but names no package", s.name)
		}
		byPackage[s.pkg] = append(byPackage[s.pkg], s)
	}

	for pkg, packageShards := range byPackage {
		// Ask go test for the current top-level inventory instead of
		// maintaining a second list that would rot.
		command := exec.Command("go", "test", pkg, "-list", "^Test")
		command.Dir = filepath.Join("..", "..")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("list %s tests: %v\n%s", pkg, err, output)
		}
		inventory := 0
		claimed := map[string]int{}
		for _, testName := range strings.Split(string(output), "\n") {
			testName = strings.TrimSpace(testName)
			if !strings.HasPrefix(testName, "Test") {
				continue
			}
			inventory++
			var owners []string
			for _, s := range packageShards {
				// -run splits on "/" for subtests; these selectors address
				// top-level names only, so a plain match is the same rule.
				if s.matcher.MatchString(testName) {
					owners = append(owners, s.name)
				}
			}
			if len(owners) != 1 {
				t.Fatalf("%s test %q is claimed by %d shards (%v), want exactly one", pkg, testName, len(owners), owners)
			}
			claimed[owners[0]]++
		}
		if inventory == 0 {
			t.Fatalf("%s test inventory is empty; coverage guard would pass vacuously", pkg)
		}
		// A shard that matches nothing is the same silent hole seen from the
		// other side, and the lane's own runtime guard throws on it. Catch it
		// here instead of one push later.
		for _, s := range packageShards {
			if claimed[s.name] == 0 {
				t.Fatalf("%s shard %q selector %q matches no test", pkg, s.name, s.selector)
			}
		}
	}
}
