package assets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type ciResilience struct {
	Env         map[string]string `yaml:"env"`
	Concurrency struct {
		Group  string `yaml:"group"`
		Cancel string `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Steps []struct {
			Uses            string `yaml:"uses"`
			Timeout         int    `yaml:"timeout-minutes"`
			TolerateFailure bool   `yaml:"continue-on-error"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readCIResilience(t *testing.T) ciResilience {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow ciResilience
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

func TestCIResilienceOnlyCancelsSupersededPullRequests(t *testing.T) {
	workflow := readCIResilience(t)
	if workflow.Concurrency.Group != "${{ github.workflow }}-${{ github.event_name }}-${{ github.event.pull_request.number || github.run_id }}" {
		t.Fatal("PRs must share their own group; non-PR runs need a unique run ID")
	}
	if workflow.Concurrency.Cancel != "${{ github.event_name == 'pull_request' }}" {
		t.Fatal("only superseded pull-request runs may be cancelled")
	}
}

func TestCIResilienceBoundsGoSetupWithoutToleratingFailure(t *testing.T) {
	workflow := readCIResilience(t)
	if workflow.Env["SEGMENT_DOWNLOAD_TIMEOUT_MINS"] != "2" {
		t.Error("cache segment downloads must stop waiting after two minutes")
	}
	count := 0
	for id, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/setup-go@") {
				count++
				if step.Timeout != 4 || step.TolerateFailure {
					t.Errorf("%s Go setup must fail closed after four minutes", id)
				}
			}
		}
	}
	if count == 0 {
		t.Fatal("no Go setup steps checked")
	}
}
