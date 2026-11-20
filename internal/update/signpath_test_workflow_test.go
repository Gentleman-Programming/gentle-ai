package update

import (
	"encoding/xml"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSignPathWorkflowProvenanceAndGates(t *testing.T) {
	text := readRepositoryFile(t, ".github", "workflows", "signpath-test.yml")
	var workflow struct {
		On          map[string]any    `yaml:"on"`
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			If          string `yaml:"if"`
			RunsOn      string `yaml:"runs-on"`
			Environment string `yaml:"environment"`
			Timeout     int    `yaml:"timeout-minutes"`
			Steps       []struct {
				Name     string            `yaml:"name"`
				ID       string            `yaml:"id"`
				Uses     string            `yaml:"uses"`
				Run      string            `yaml:"run"`
				If       string            `yaml:"if"`
				Continue bool              `yaml:"continue-on-error"`
				With     map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(text), &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.On) != 2 || workflow.On["workflow_dispatch"] != nil {
		t.Errorf("expected input-free dispatch and pull request: %v", workflow.On)
	}
	if len(workflow.Permissions) != 2 || workflow.Permissions["contents"] != "read" || workflow.Permissions["actions"] != "read" {
		t.Errorf("permissions: %v", workflow.Permissions)
	}
	job, ok := workflow.Jobs["signpath-test"]
	if !ok || len(workflow.Jobs) != 2 || job.RunsOn != "windows-latest" || job.Environment != "signpath-test" || job.Timeout <= 0 || job.Timeout > 60 || job.If != "github.event_name == 'workflow_dispatch' && github.ref == 'refs/heads/main'" {
		t.Fatalf("unsafe job: %+v", job)
	}
	positions := map[string]int{}
	for i, step := range job.Steps {
		positions[step.Name] = i
		if step.Continue || step.If != "" {
			t.Errorf("step may bypass a failed gate: %s", step.Name)
		}
		if step.Uses != "" && !regexp.MustCompile(`^[^@]+@[a-f0-9]{40}$`).MatchString(step.Uses) {
			t.Errorf("unpinned action: %s", step.Uses)
		}
	}
	names := []string{"Checkout", "Set up Go", "Check expected certificate", "Build test binaries", "Test verifier", "Upload unsigned artifact", "Sign with SignPath", "Verify signed artifacts", "Upload verified test artifacts"}
	last := -1
	for _, name := range names {
		i, found := positions[name]
		if !found || i <= last {
			t.Fatalf("missing or out-of-order step %q", name)
		}
		last = i
	}
	checkout := job.Steps[positions["Checkout"]]
	if checkout.With["persist-credentials"] != "false" {
		t.Error("checkout persists credentials")
	}
	build := job.Steps[positions["Build test binaries"]].Run
	for _, want := range []string{"./cmd/gentle-ai", "main.version=signpath-test", "amd64", "arm64", "CGO_ENABLED"} {
		if !strings.Contains(build, want) {
			t.Errorf("build missing %q", want)
		}
	}
	unsigned := job.Steps[positions["Upload unsigned artifact"]]
	sign := job.Steps[positions["Sign with SignPath"]]
	if unsigned.ID != "unsigned" || !strings.HasPrefix(unsigned.Uses, "actions/upload-artifact@") {
		t.Error("unsigned upload has no artifact ID")
	}
	wantInputs := map[string]string{"api-token": "${{ secrets.SIGNPATH_API_TOKEN }}", "organization-id": "9a59d047-c41a-4405-b96e-e8ba1b034c28", "project-slug": "gentle-ai", "signing-policy-slug": "test-signing", "artifact-configuration-slug": "github-test-zip", "github-artifact-id": "${{ steps.unsigned.outputs.artifact-id }}", "wait-for-completion": "true", "output-artifact-directory": "${{ runner.temp }}\\signpath-signed"}
	if sign.Uses != "SignPath/github-action-submit-signing-request@f6d04783b4569d051e0c80105fe66e82819d0092" || len(sign.With) != len(wantInputs) {
		t.Errorf("unexpected SignPath action or inputs: %+v", sign)
	}
	for k, v := range wantInputs {
		if sign.With[k] != v {
			t.Errorf("SignPath input %s = %q", k, sign.With[k])
		}
	}
	if strings.Count(text, "secrets.SIGNPATH_API_TOKEN") != 1 || !strings.Contains(text, "vars.SIGNPATH_TEST_CERTIFICATE_SHA256") {
		t.Error("incorrect secret or fingerprint scope")
	}
	for _, name := range []string{"Test verifier", "Verify signed artifacts"} {
		if !strings.Contains(job.Steps[positions[name]].Run, "scripts/verify-signpath-test") {
			t.Errorf("%s does not run verifier", name)
		}
	}
	verified := job.Steps[positions["Upload verified test artifacts"]]
	if !strings.HasPrefix(verified.Uses, "actions/upload-artifact@") || !strings.Contains(verified.With["path"], "signpath-signed") || verified.With["if-no-files-found"] != "error" {
		t.Error("verified upload does not use signed output")
	}
}

func TestSignPathPremergeFixturesAreSecretFree(t *testing.T) {
	text := readRepositoryFile(t, ".github", "workflows", "signpath-test.yml")
	var workflow struct {
		On map[string]struct {
			Paths []string `yaml:"paths"`
		} `yaml:"on"`
		Jobs map[string]struct {
			If          string            `yaml:"if"`
			RunsOn      string            `yaml:"runs-on"`
			Environment string            `yaml:"environment"`
			Permissions map[string]string `yaml:"permissions"`
			Env         map[string]string `yaml:"env"`
			Timeout     int               `yaml:"timeout-minutes"`
			Steps       []struct {
				Name     string            `yaml:"name"`
				Uses     string            `yaml:"uses"`
				Run      string            `yaml:"run"`
				If       string            `yaml:"if"`
				Continue bool              `yaml:"continue-on-error"`
				Env      map[string]string `yaml:"env"`
				With     map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(text), &workflow); err != nil {
		t.Fatal(err)
	}
	pr, ok := workflow.On["pull_request"]
	if !ok || len(workflow.On) != 2 {
		t.Fatalf("missing bounded pull_request trigger: %v", workflow.On)
	}
	wantPaths := map[string]bool{
		".github/workflows/signpath-test.yml":            false,
		".github/signpath/github-test-zip.xml":           false,
		"scripts/verify-signpath-test.ps1":               false,
		"scripts/verify-signpath-test.tests.ps1":         false,
		"internal/update/signpath_test_workflow_test.go": false,
		"docs/signpath-test.md":                          false,
	}
	for _, path := range pr.Paths {
		if _, found := wantPaths[path]; !found {
			t.Errorf("unexpected PR path filter: %q", path)
		} else {
			wantPaths[path] = true
		}
	}
	for path, found := range wantPaths {
		if !found {
			t.Errorf("PR path filter missing %q", path)
		}
	}
	job, ok := workflow.Jobs["premerge-fixtures"]
	if !ok || job.If != "github.event_name == 'pull_request'" || job.RunsOn != "windows-latest" || job.Environment != "" || len(job.Env) != 0 || len(job.Permissions) != 0 || job.Timeout <= 0 || job.Timeout > 60 {
		t.Fatalf("unsafe or missing premerge job: %+v", job)
	}
	if len(job.Steps) != 4 {
		t.Fatalf("premerge job has unexpected steps: %+v", job.Steps)
	}
	checkout, setup, build, fixtures := job.Steps[0], job.Steps[1], job.Steps[2], job.Steps[3]
	if checkout.Uses != "actions/checkout@93cb6efe18208431cddfb8368fd83d5badbf9bfd" || checkout.With["persist-credentials"] != "false" || setup.Uses != "actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c" || setup.With["go-version-file"] != "go.mod" {
		t.Error("premerge checkout or setup is not safely pinned")
	}
	for _, step := range job.Steps {
		if step.If != "" || step.Continue || len(step.Env) != 0 || strings.Contains(strings.ToLower(step.Run), "secrets.") || strings.Contains(strings.ToLower(step.Uses), "signpath/") || strings.Contains(strings.ToLower(step.Uses), "upload-artifact") {
			t.Errorf("premerge step has authority or bypass: %+v", step)
		}
		for _, value := range step.With {
			if strings.Contains(strings.ToLower(value), "secrets.") || strings.Contains(strings.ToLower(value), "github.token") {
				t.Errorf("premerge step receives a token: %+v", step)
			}
		}
	}
	for _, want := range []string{"./cmd/gentle-ai", "main.version=signpath-test", "amd64", "arm64", "CGO_ENABLED", "GOOS", "GOARCH", "LASTEXITCODE"} {
		if !strings.Contains(build.Run, want) {
			t.Errorf("premerge build missing %q", want)
		}
	}
	if build.Uses != "" || fixtures.Uses != "" || !strings.Contains(fixtures.Run, "./scripts/verify-signpath-test.tests.ps1") || !strings.Contains(fixtures.Run, "signpath-unsigned") {
		t.Error("premerge job does not execute the synthetic fixtures on built binaries")
	}
	if workflow.Jobs["signpath-test"].If != "github.event_name == 'workflow_dispatch' && github.ref == 'refs/heads/main'" {
		t.Error("signing job is not restricted to manual main dispatch")
	}
}

func TestSignPathArtifactConfiguration(t *testing.T) {
	text := readRepositoryFile(t, ".github", "signpath", "github-test-zip.xml")
	var config struct {
		XMLName xml.Name
		Zip     struct {
			Files []struct {
				Path string     `xml:"path,attr"`
				Sign []struct{} `xml:"authenticode-sign"`
			} `xml:"pe-file"`
		} `xml:"zip-file"`
	}
	if err := xml.Unmarshal([]byte(text), &config); err != nil {
		t.Fatal(err)
	}
	if config.XMLName.Space != "http://signpath.io/artifact-configuration/v1" || config.XMLName.Local != "artifact-configuration" {
		t.Errorf("unexpected XML root: %v", config.XMLName)
	}
	if len(config.Zip.Files) != 2 {
		t.Fatalf("expected two PE entries: %+v", config.Zip.Files)
	}
	seen := map[string]bool{}
	for _, file := range config.Zip.Files {
		if len(file.Sign) != 1 {
			t.Errorf("missing sign directive for %s", file.Path)
		}
		seen[file.Path] = true
	}
	for _, path := range []string{"gentle-ai-windows-amd64.exe", "gentle-ai-windows-arm64.exe"} {
		if !seen[path] {
			t.Errorf("missing %s", path)
		}
	}
}

func TestSignPathVerifierIsFailClosed(t *testing.T) {
	verifier := readRepositoryFile(t, "scripts", "verify-signpath-test.ps1")
	for _, want := range []string{"Get-AuthenticodeSignature", "SignerCertificate", "SHA256", "RawData", "HashMismatch", "NotTrusted", "CustomRootTrust", "CustomTrustStore", "NoCheck", "finally", "0x8664", "0xAA64", "SIGNPATH-STAGE"} {
		if !strings.Contains(verifier, want) {
			t.Errorf("verifier missing %q", want)
		}
	}
	// The verifier must never touch a certificate store: store writes can block
	// on a Windows trust-confirmation prompt in a non-interactive session.
	for _, banned := range []string{"LocalMachine", "X509Store", "certutil", "Cert:\\", ".Remove("} {
		if strings.Contains(verifier, banned) {
			t.Errorf("verifier must not use %q", banned)
		}
	}
	tests := readRepositoryFile(t, "scripts", "verify-signpath-test.tests.ps1")
	for _, want := range []string{"tampered", "wrong signer", "unsigned", "missing", "extra", "architecture", "intact", "WaitForExit", "Kill($true)", "timed out", "SIGNPATH-STAGE", "Verifier leaked trust anchor"} {
		if !strings.Contains(tests, want) {
			t.Errorf("native fixture coverage missing %q", want)
		}
	}
	if strings.Contains(tests, "$store.Add(") {
		t.Error("fixtures must not add trust anchors (can block on a Windows prompt)")
	}
}
