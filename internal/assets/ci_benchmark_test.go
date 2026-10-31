package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type ciNeeds []string

func (n *ciNeeds) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*n = ciNeeds{node.Value}
		return nil
	}
	var values []string
	if err := node.Decode(&values); err != nil {
		return err
	}
	*n = values
	return nil
}

type ciJob struct {
	Name  string  `yaml:"name"`
	Needs ciNeeds `yaml:"needs"`
	If    string  `yaml:"if"`
	Steps []struct {
		Name             string            `yaml:"name"`
		If               string            `yaml:"if"`
		ContinueOnError  bool              `yaml:"continue-on-error"`
		Run              string            `yaml:"run"`
		WorkingDirectory string            `yaml:"working-directory"`
		Uses             string            `yaml:"uses"`
		With             map[string]string `yaml:"with"`
		Env              map[string]string `yaml:"env"`
	} `yaml:"steps"`
}

func ciJobs(t *testing.T) map[string]ciJob {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]ciJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow.Jobs
}

func TestCIParallelRootAndBenchmarkBranches(t *testing.T) {
	jobs := ciJobs(t)
	for _, id := range []string{"root-unit-tests", "benchmark"} {
		job, ok := jobs[id]
		if !ok {
			t.Errorf("missing independent branch %s", id)
			continue
		}
		if len(job.Needs) != 0 || job.If != "" {
			t.Errorf("%s must start independently: needs=%v if=%q", id, job.Needs, job.If)
		}
		if len(job.Steps) == 0 || job.Steps[0].With["ref"] != "${{ github.event.pull_request.head.sha || github.sha }}" || job.Steps[0].With["repository"] != "${{ github.event.pull_request.head.repo.full_name || github.repository }}" {
			t.Errorf("%s must check out the exact event candidate", id)
		}
		for _, step := range job.Steps {
			if step.If != "" || step.ContinueOnError {
				t.Errorf("%s step %q can bypass a check", id, step.Name)
			}
		}
	}
}

func TestCIAggregateFailsClosedForEveryBranchOutcome(t *testing.T) {
	job, ok := ciJobs(t)["unit-tests"]
	if !ok || job.Name != "Unit Tests" || job.If != "always()" || !reflect.DeepEqual(job.Needs, ciNeeds{"go-format", "root-unit-tests", "benchmark"}) {
		t.Fatalf("required aggregate missing or incomplete: %+v", job)
	}
	if len(job.Steps) != 1 {
		t.Fatalf("aggregate must have exactly one unconditional guard: %+v", job.Steps)
	}
	guard := job.Steps[0]
	if guard.If != "" || guard.ContinueOnError {
		t.Fatal("aggregate guard must not be conditional or tolerate failure")
	}
	bindings := map[string]string{"FORMAT_RESULT": "${{ needs.go-format.result }}", "ROOT_RESULT": "${{ needs.root-unit-tests.result }}", "BENCHMARK_RESULT": "${{ needs.benchmark.result }}"}
	if !reflect.DeepEqual(guard.Env, bindings) {
		t.Fatalf("aggregate result bindings = %v, want %v", guard.Env, bindings)
	}
	cases := []struct {
		name string
		env  []string
		pass bool
	}{{"all successful", []string{"success", "success", "success"}, true}}
	for i, branch := range []string{"format", "root", "benchmark"} {
		for _, outcome := range []string{"failure", "skipped", "cancelled", ""} {
			values := []string{"success", "success", "success"}
			values[i] = outcome
			cases = append(cases, struct {
				name string
				env  []string
				pass bool
			}{branch + "/" + outcome, values, false})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "-e", "-c", guard.Run)
			cmd.Env = append(os.Environ(), "FORMAT_RESULT="+tc.env[0], "ROOT_RESULT="+tc.env[1], "BENCHMARK_RESULT="+tc.env[2])
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.pass {
				t.Fatalf("guard pass=%v, want %v: %v\n%s", err == nil, tc.pass, err, out)
			}
		})
	}
}

func TestCIBenchmarkKeepsEvidenceAndRootChecks(t *testing.T) {
	jobs := ciJobs(t)
	root := jobs["root-unit-tests"]
	benchmark := jobs["benchmark"]
	rootRuns := ""
	for _, step := range root.Steps {
		rootRuns += step.Run + "\n"
	}
	for _, command := range []string{"go test ./...", "./scripts/deadcode-ratchet.sh"} {
		if strings.Count(rootRuns, command) != 1 {
			t.Errorf("root must run %q once", command)
		}
	}
	var module, evidence string
	for _, step := range benchmark.Steps {
		if step.Name == "Build, vet, and test benchmark module" && step.WorkingDirectory == "bench" {
			module = step.Run
		}
		if step.Name == "Run benchmark evidence" {
			evidence = step.Run
		}
	}
	for _, command := range []string{`go build -o "$RUNNER_TEMP/gentle-ai-bench" .`, "go vet ./...", "go test ./..."} {
		if strings.Count(module, command) != 1 {
			t.Errorf("benchmark module must run %q once", command)
		}
	}
	for _, command := range []string{`go build -trimpath -o "$RUNNER_TEMP/gentle-ai" ./cmd/gentle-ai`, `go build -tags bench_fixture -trimpath -o "$RUNNER_TEMP/gentle-ai-bench-fixture" ./cmd/gentle-ai`, `--out "$RUNNER_TEMP/bench-portable.json"`, `--out "$RUNNER_TEMP/bench-provider-capture.json"`, `--out "$RUNNER_TEMP/bench-transition.json"`, `--out "$RUNNER_TEMP/bench-model-picker-untagged.json"`, `--out "$RUNNER_TEMP/bench-model-picker.json"`} {
		if strings.Count(evidence, command) != 1 {
			t.Errorf("benchmark evidence must run %q once", command)
		}
	}
	if strings.Count(evidence, "jq -e '") != 5 {
		t.Errorf("benchmark evidence must retain five jq assertions")
	}
}
