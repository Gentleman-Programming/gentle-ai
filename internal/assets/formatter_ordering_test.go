package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		{id: "windows-runtime", next: "e2e-tests"},
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
