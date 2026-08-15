package update

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestOfficialReleaseOmitsUnsignedWindowsDistribution(t *testing.T) {
	config := readRepositoryFile(t, ".goreleaser.yaml")
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?mi)^\s*-\s*windows\s*$`),
		regexp.MustCompile(`(?mi)^\s*scoops\s*:`),
	} {
		if forbidden.MatchString(config) {
			t.Errorf("GoReleaser config still enables forbidden Windows distribution: %s", forbidden)
		}
	}
	for _, required := range []string{"- linux", "- darwin", "brews:", "artifacts: checksum"} {
		if !strings.Contains(config, required) {
			t.Errorf("GoReleaser config lost non-Windows release behavior %q", required)
		}
	}

	workflow := readRepositoryFile(t, ".github", "workflows", "release.yml")
	if regexp.MustCompile(`(?i)mock[^\n]*sign`).MatchString(workflow) {
		t.Fatal("release workflow contains mock signing")
	}
	ci := readRepositoryFile(t, ".github", "workflows", "ci.yml")
	for _, required := range []string{"windows-runtime:", "runs-on: windows-latest", "go build -trimpath", "go test ./..."} {
		if !strings.Contains(ci, required) {
			t.Errorf("Windows source-compatibility CI is missing %q", required)
		}
	}

	verify := readRepositoryFile(t, "scripts", "verify-release-assets.sh")
	if strings.Contains(strings.ToLower(verify), "_windows_") {
		t.Fatal("remote release verifier still expects Windows assets")
	}
	for _, required := range []string{"_linux_amd64.tar.gz", "_linux_arm64.tar.gz", "_darwin_amd64.tar.gz", "_darwin_arm64.tar.gz"} {
		if !strings.Contains(verify, required) {
			t.Errorf("remote release verifier lost %q", required)
		}
	}

	if !strings.Contains(workflow, "Resolve release distribution plan without publishing") ||
		!strings.Contains(workflow, "./scripts/verify-release-distribution-policy.sh") {
		t.Fatal("release workflow does not resolve and validate the distribution plan before publication")
	}
}

func TestWindowsInstallAndUpgradeContainNoRemoteBinaryOrScriptPath(t *testing.T) {
	installer := readRepositoryFile(t, "scripts", "install.ps1")
	strategy := readRepositoryFile(t, "internal", "update", "upgrade", "strategy.go")
	instructions := readRepositoryFile(t, "internal", "update", "instructions.go")
	for name, content := range map[string]string{"scripts/install.ps1": installer, "strategy.go": strategy, "instructions.go": instructions} {
		for _, forbidden := range []string{"Install-ViaBinary", "_windows_", "scripts/install.ps1", "ExecutionPolicy", "checksumsUrl"} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s retains forbidden Windows distribution path %q", name, forbidden)
			}
		}
	}
	for _, required := range []string{
		"Windows binary distribution and Scoop are temporarily unavailable",
		"go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest",
	} {
		if !strings.Contains(installer, required) {
			t.Errorf("Windows installer is missing safe source guidance %q", required)
		}
	}
}

func TestReleaseDistributionPolicyAssertionFailsClosed(t *testing.T) {
	root := newReleasePolicyFixture(t)
	if output, err := runReleasePolicy(root); err != nil {
		t.Fatalf("policy rejected the approved release plan: %v\n%s", err, output)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "omitted goos uses unsafe GoReleaser defaults",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, ".goreleaser.yaml", "    goos:\n      - linux\n      - darwin\n", "")
			},
		},
		{
			name: "extra build target",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, ".goreleaser.yaml", "      - darwin\n    goarch:", "      - darwin\n      - freebsd\n    goarch:")
			},
		},
		{
			name: "extra archive format",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, ".goreleaser.yaml", "      - tar.gz\n", "      - tar.gz\n      - zip\n")
			},
		},
		{
			name: "provider contract archive version differs from committed semver",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join("dist", "artifacts.json"), "gentle-ai-review-provider-contract-1.1.0.tar.gz", "gentle-ai-review-provider-contract-2.0.0.tar.gz")
			},
		},
		{
			name: "alternate publication config",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"), "args: release --clean", "args: release --clean --config .goreleaser-alternate.yaml")
			},
		},
		{
			name: "separate workflow upload action",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"), "      - name: Verify published assets from GitHub\n", "      - name: Upload release asset separately\n        uses: actions/upload-artifact@v4\n\n      - name: Verify published assets from GitHub\n")
			},
		},
		{
			name: "renamed canonical config",
			mutate: func(t *testing.T, root string) {
				if err := os.Rename(filepath.Join(root, ".goreleaser.yaml"), filepath.Join(root, ".goreleaser-renamed.yaml")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unexpected resolved Windows artifact",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join("dist", "artifacts.json"), "\n]", ",\n  {"+`"name":"gentle-ai","path":"dist/gentle-ai_windows_amd64_v1/gentle-ai.exe","goos":"windows","goarch":"amd64","target":"windows_amd64_v1","type":"Binary","extra":{"Binary":"gentle-ai","ID":"gentle-ai"}`+"}\n]")
			},
		},
		{
			name: "stale metadata with absent outputs",
			mutate: func(t *testing.T, root string) {
				removeReleasePolicyOutputs(t, root)
			},
		},
		{
			name: "stale existing outputs from an earlier run",
			mutate: func(t *testing.T, root string) {
				marker, err := os.Stat(releasePolicyMarkerPath(root))
				if err != nil {
					t.Fatal(err)
				}
				setReleasePolicyOutputsMTime(t, root, marker.ModTime().Add(-time.Minute))
			},
		},
		{
			name: "snapshot metadata predates current run",
			mutate: func(t *testing.T, root string) {
				marker, err := os.Stat(releasePolicyMarkerPath(root))
				if err != nil {
					t.Fatal(err)
				}
				stamp := marker.ModTime().Add(-time.Minute)
				if err := os.Chtimes(filepath.Join(root, "dist", "artifacts.json"), stamp, stamp); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "snapshot marker belongs to another run",
			mutate: func(t *testing.T, root string) {
				if err := os.WriteFile(releasePolicyMarkerPath(root), []byte("earlier-run\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "artifact path escapes snapshot directory",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join("dist", "artifacts.json"),
					`"path":"dist/gentle-ai_linux_amd64_v1/gentle-ai"`,
					`"path":"dist/../outside/gentle-ai"`)
				outside := filepath.Join(root, "outside", "gentle-ai")
				if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(outside, []byte("escaped output"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "artifact path resolves through symlink",
			mutate: func(t *testing.T, root string) {
				output := filepath.Join(root, "dist", "gentle-ai_linux_amd64_v1", "gentle-ai")
				if err := os.Remove(output); err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(root, "outside-binary")
				if err := os.WriteFile(outside, []byte("stale external binary"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, output); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("Windows runner cannot create symlink fixture: %v", err)
					}
					t.Fatal(err)
				}
			},
		},
		{
			name: "snapshot can continue after failure",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"          MINISIGN_PUBLIC_KEYS_CANONICAL: release-policy-validation-only\n",
					"          MINISIGN_PUBLIC_KEYS_CANONICAL: release-policy-validation-only\n        continue-on-error: true\n")
			},
		},
		{
			name: "preflight can continue after failure",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"      - name: Verify tag, main, trust anchors, and module immutability\n        run: ./scripts/release-preflight.sh\n",
					"      - name: Verify tag, main, trust anchors, and module immutability\n        run: ./scripts/release-preflight.sh\n        continue-on-error: true\n")
			},
		},
		{
			name: "publication can continue after failure",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"      - name: Run GoReleaser\n        uses:",
					"      - name: Run GoReleaser\n        continue-on-error: true\n        uses:")
			},
		},
		{
			name: "post-publication verifier loses release dependency",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"  verify:\n    needs: release\n",
					"  verify:\n")
			},
		},
		{
			name: "post-publication verifier gains write permission",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"  verify:\n    needs: release\n    runs-on: ubuntu-24.04\n    timeout-minutes: 15\n    permissions:\n      contents: read\n",
					"  verify:\n    needs: release\n    runs-on: ubuntu-24.04\n    timeout-minutes: 15\n    permissions:\n      contents: write\n")
			},
		},
		{
			name: "post-publication verifier receives secret token",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"          GH_TOKEN: ${{ github.token }}\n",
					"          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n")
			},
		},
		{
			name: "release job executes post-publication verifier",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"      - name: Remove signing material\n",
					"      - name: Verify published assets with write authority\n        env:\n          GH_TOKEN: ${{ github.token }}\n          MINISIGN_PUBLIC_KEYS: ${{ vars.MINISIGN_PUBLIC_KEYS }}\n        run: ./scripts/verify-release-assets.sh\n\n      - name: Remove signing material\n")
			},
		},
		{
			name: "post-publication verifier job contains extra step",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"      - name: Verify published assets from GitHub\n",
					"      - name: Inspect release outside the approved verifier\n        env:\n          GH_TOKEN: ${{ github.token }}\n        run: gh release view \"$GITHUB_REF_NAME\" --repo \"$GITHUB_REPOSITORY\"\n\n      - name: Verify published assets from GitHub\n")
			},
		},
		{
			name: "prepublication script receives write-scoped job token",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"      - name: Recheck immutable release provenance\n        env:\n          MINISIGN_PUBLIC_KEYS: ${{ vars.MINISIGN_PUBLIC_KEYS }}\n",
					"      - name: Recheck immutable release provenance\n        env:\n          MINISIGN_PUBLIC_KEYS: ${{ vars.MINISIGN_PUBLIC_KEYS }}\n          GITHUB_TOKEN: ${{ github.token }}\n")
			},
		},
		{
			name: "direct GitHub API release creation",
			mutate: func(t *testing.T, root string) {
				replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
					"      - name: Verify published assets from GitHub\n",
					"      - name: Create release through GitHub API\n        run: gh api --method POST repos/Gentleman-Programming/gentle-ai/releases\n\n      - name: Verify published assets from GitHub\n")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newReleasePolicyFixture(t)
			tc.mutate(t, root)
			if output, err := runReleasePolicy(root); err == nil {
				t.Fatalf("policy accepted a release-plan bypass:\n%s", output)
			}
		})
	}
}

func TestReleaseDistributionPolicyAcceptsSemanticYAMLFormatting(t *testing.T) {
	root := newReleasePolicyFixture(t)
	replaceReleasePolicyFile(t, root, ".goreleaser.yaml",
		"version: 2\n\nproject_name: gentle-ai\n",
		"# Top-level key order and formatting are not release semantics.\nproject_name: gentle-ai\n\nversion: 2\n")
	replaceReleasePolicyFile(t, root, filepath.Join(".github", "workflows", "release.yml"),
		"permissions:\n  contents: read\n\nconcurrency:\n  group: release-${{ github.ref }}\n  cancel-in-progress: false\n",
		"concurrency:\n  group: release-${{ github.ref }}\n  cancel-in-progress: false\n\n# Mapping order is intentionally non-semantic.\npermissions:\n  contents: read\n")
	if output, err := runReleasePolicy(root); err != nil {
		t.Fatalf("policy rejected a semantically identical YAML reordering: %v\n%s", err, output)
	}
}

func TestModifiedReleaseVerifierCannotGainWriteAuthority(t *testing.T) {
	root := newReleasePolicyFixture(t)
	verifier := filepath.Join(root, "scripts", "verify-release-assets.sh")
	file, err := os.OpenFile(verifier, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\ngh api --method POST repos/Gentleman-Programming/gentle-ai/releases\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if output, err := runReleasePolicy(root); err != nil {
		t.Fatalf("opaque verifier content should be contained by workflow privilege, not source parsing: %v\n%s", err, output)
	}

	workflowBytes, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Needs       string            `yaml:"needs"`
			Permissions map[string]string `yaml:"permissions"`
			Steps       []struct {
				Name string            `yaml:"name"`
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(workflowBytes, &workflow); err != nil {
		t.Fatal(err)
	}
	owner := ""
	var tokenEnv map[string]string
	for jobName, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.Run == "./scripts/verify-release-assets.sh" {
				if owner != "" {
					t.Fatal("release verifier is executed by more than one job")
				}
				owner = jobName
				tokenEnv = step.Env
			}
		}
	}
	job, ok := workflow.Jobs[owner]
	if !ok || owner != "verify" || job.Needs != "release" || job.Permissions["contents"] != "read" {
		t.Fatalf("modified verifier is not contained by a dependent contents-read job: owner=%q needs=%q permissions=%v", owner, job.Needs, job.Permissions)
	}
	if len(tokenEnv) != 2 || tokenEnv["GH_TOKEN"] != "${{ github.token }}" || tokenEnv["MINISIGN_PUBLIC_KEYS"] != "${{ vars.MINISIGN_PUBLIC_KEYS }}" {
		t.Fatalf("release verifier receives more than the read-scoped job token: %v", tokenEnv)
	}

	release := workflow.Jobs["release"]
	writeTokenRecipients := 0
	for _, step := range release.Steps {
		for key, value := range step.Env {
			if key != "GH_TOKEN" && key != "GITHUB_TOKEN" {
				continue
			}
			writeTokenRecipients++
			if step.Name != "Run GoReleaser" || step.Run != "" || step.Uses == "" || key != "GITHUB_TOKEN" || value != "${{ github.token }}" {
				t.Fatalf("write-scoped repository token escapes the sole publisher: step=%q run=%q uses=%q env=%v", step.Name, step.Run, step.Uses, step.Env)
			}
		}
	}
	if writeTokenRecipients != 1 {
		t.Fatalf("write-scoped release job exposes %d repository tokens, want only GoReleaser", writeTokenRecipients)
	}
}

func newReleasePolicyFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	marker := releasePolicyMarkerPath(root)
	if err := os.WriteFile(marker, []byte(releasePolicyRunID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	markerTime := time.Now().Add(-time.Minute)
	if err := os.Chtimes(marker, markerTime, markerTime); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		".goreleaser.yaml": readRepositoryFile(t, ".goreleaser.yaml"),
		filepath.Join("contracts", "review-provider-contract", "CONTRACT_SEMVER"): readRepositoryFile(t, "contracts", "review-provider-contract", "CONTRACT_SEMVER"),
		"go.mod": readRepositoryFile(t, "go.mod"),
		"go.sum": readRepositoryFile(t, "go.sum"),
		filepath.Join(".github", "workflows", "release.yml"):              readRepositoryFile(t, ".github", "workflows", "release.yml"),
		filepath.Join("internal", "releasepolicy", "policy.go"):           readRepositoryFile(t, "internal", "releasepolicy", "policy.go"),
		filepath.Join("internal", "releasepolicycmd", "main.go"):          readRepositoryFile(t, "internal", "releasepolicycmd", "main.go"),
		filepath.Join("scripts", "verify-release-assets.sh"):              readRepositoryFile(t, "scripts", "verify-release-assets.sh"),
		filepath.Join("scripts", "verify-release-distribution-policy.sh"): readRepositoryFile(t, "scripts", "verify-release-distribution-policy.sh"),
		filepath.Join("dist", "artifacts.json"):                           releasePolicyArtifactsFixture,
	}
	for path, content := range files {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range releasePolicyOutputPaths(t) {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("current snapshot output\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runReleasePolicy(root string) ([]byte, error) {
	command := exec.Command("bash", filepath.Join("scripts", "verify-release-distribution-policy.sh"))
	command.Dir = root
	command.Env = append(os.Environ(),
		"RELEASE_POLICY_SNAPSHOT_MARKER="+releasePolicyMarkerPath(root),
		"RELEASE_POLICY_SNAPSHOT_RUN_ID="+releasePolicyRunID,
	)
	return command.CombinedOutput()
}

func releasePolicyMarkerPath(root string) string {
	return filepath.Join(root, "release-policy-snapshot-start")
}

func releasePolicyOutputPaths(t *testing.T) []string {
	t.Helper()
	var artifacts []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(releasePolicyArtifactsFixture), &artifacts); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		paths = append(paths, artifact.Path)
	}
	return paths
}

func removeReleasePolicyOutputs(t *testing.T, root string) {
	t.Helper()
	for _, path := range releasePolicyOutputPaths(t) {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatal(err)
		}
	}
}

func setReleasePolicyOutputsMTime(t *testing.T, root string, stamp time.Time) {
	t.Helper()
	for _, path := range releasePolicyOutputPaths(t) {
		if err := os.Chtimes(filepath.Join(root, filepath.FromSlash(path)), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
}

func replaceReleasePolicyFile(t *testing.T, root, path, old, replacement string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), old) {
		t.Fatalf("fixture %s does not contain mutation target %q", path, old)
	}
	updated := strings.Replace(string(content), old, replacement, 1)
	if err := os.WriteFile(fullPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
}

const releasePolicyArtifactsFixture = `[
  {"name":"metadata.json","path":"dist/metadata.json","type":"Metadata"},
  {"name":"gentle-ai","path":"dist/gentle-ai_linux_amd64_v1/gentle-ai","goos":"linux","goarch":"amd64","target":"linux_amd64_v1","type":"Binary","extra":{"Binary":"gentle-ai","ID":"gentle-ai"}},
  {"name":"gentle-ai","path":"dist/gentle-ai_linux_arm64_v8.0/gentle-ai","goos":"linux","goarch":"arm64","target":"linux_arm64_v8.0","type":"Binary","extra":{"Binary":"gentle-ai","ID":"gentle-ai"}},
  {"name":"gentle-ai","path":"dist/gentle-ai_darwin_amd64_v1/gentle-ai","goos":"darwin","goarch":"amd64","target":"darwin_amd64_v1","type":"Binary","extra":{"Binary":"gentle-ai","ID":"gentle-ai"}},
  {"name":"gentle-ai","path":"dist/gentle-ai_darwin_arm64_v8.0/gentle-ai","goos":"darwin","goarch":"arm64","target":"darwin_arm64_v8.0","type":"Binary","extra":{"Binary":"gentle-ai","ID":"gentle-ai"}},
  {"name":"gentle-ai_0.0.0-SNAPSHOT_linux_amd64.tar.gz","path":"dist/gentle-ai_0.0.0-SNAPSHOT_linux_amd64.tar.gz","goos":"linux","goarch":"amd64","target":"linux_amd64_v1","type":"Archive","extra":{"Binaries":["gentle-ai"],"Format":"tar.gz","ID":"default"}},
  {"name":"gentle-ai_0.0.0-SNAPSHOT_linux_arm64.tar.gz","path":"dist/gentle-ai_0.0.0-SNAPSHOT_linux_arm64.tar.gz","goos":"linux","goarch":"arm64","target":"linux_arm64_v8.0","type":"Archive","extra":{"Binaries":["gentle-ai"],"Format":"tar.gz","ID":"default"}},
  {"name":"gentle-ai_0.0.0-SNAPSHOT_darwin_amd64.tar.gz","path":"dist/gentle-ai_0.0.0-SNAPSHOT_darwin_amd64.tar.gz","goos":"darwin","goarch":"amd64","target":"darwin_amd64_v1","type":"Archive","extra":{"Binaries":["gentle-ai"],"Format":"tar.gz","ID":"default"}},
  {"name":"gentle-ai_0.0.0-SNAPSHOT_darwin_arm64.tar.gz","path":"dist/gentle-ai_0.0.0-SNAPSHOT_darwin_arm64.tar.gz","goos":"darwin","goarch":"arm64","target":"darwin_arm64_v8.0","type":"Archive","extra":{"Binaries":["gentle-ai"],"Format":"tar.gz","ID":"default"}},
  {"name":"gentle-ai-review-provider-contract-1.1.0.tar.gz","path":"dist/gentle-ai-review-provider-contract-1.1.0.tar.gz","type":"Archive","extra":{"Binaries":[],"Format":"tar.gz","ID":"review-provider-contract"}},
  {"name":"checksums.txt","path":"dist/checksums.txt","type":"Checksum","extra":{}},
  {"name":"gentle-ai.rb","path":"dist/homebrew/Formula/gentle-ai.rb","type":"Homebrew Formula","extra":{"BrewConfig":{"name":"gentle-ai","repository":{"owner":"Gentleman-Programming","name":"homebrew-tap","token":"{{ .Env.HOMEBREW_TAP_TOKEN }}"},"directory":"Formula"}}}
]`

const releasePolicyRunID = "release-policy-test-run"

func TestWindowsDistributionRestorationGateIsDocumented(t *testing.T) {
	docs := readRepositoryFile(t, "README.md") + readRepositoryFile(t, "docs", "release-signing.md")
	for _, required := range []string{
		"publicly trusted RSA Authenticode",
		"Azure Artifact Signing",
		"amd64 and arm64",
		"before archive and checksum generation",
		"fails if either executable is unsigned",
	} {
		if !strings.Contains(docs, required) {
			t.Errorf("Windows distribution restoration gate is missing %q", required)
		}
	}
}
