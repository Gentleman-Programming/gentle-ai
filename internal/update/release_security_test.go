package update

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func readRepositoryFile(t *testing.T, path ...string) string {
	t.Helper()
	parts := append([]string{"..", ".."}, path...)
	data, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(path...), err)
	}
	return string(data)
}

func TestReleaseWorkflowUsesFailClosedLeastPrivilegeGates(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "release.yml")
	for _, required := range []string{
		"permissions:\n  contents: read",
		"preflight:",
		"release:",
		"needs: preflight",
		"environment: release",
		"contents: write",
		"actions: read",
		"./scripts/require-ci-success.sh",
		"GH_TOKEN: ${{ github.token }}",
		"./scripts/release-preflight.sh",
		"./scripts/canonicalize-release-public-keys.sh",
		"id: trust-anchors",
		"./scripts/release-signing-preflight.sh",
		"./scripts/verify-release-assets.sh",
		"Verify provider contract bundle",
		"providercontractbundlecmd verify --archive",
		"MINISIGN_PUBLIC_KEYS: ${{ vars.MINISIGN_PUBLIC_KEYS }}",
		"MINISIGN_PUBLIC_KEYS_CANONICAL: ${{ steps.trust-anchors.outputs.canonical }}",
		"MINISIGN_SECRET_KEY_FILE:",
		"version: v2.15.2",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow is missing %q", required)
		}
	}
	if count := strings.Count(workflow, "MINISIGN_SECRET_KEY_BASE64"); count != 1 {
		t.Errorf("MINISIGN_SECRET_KEY_BASE64 occurs %d times, want exactly once in the isolated materialization step", count)
	}
	if count := strings.Count(workflow, "persist-credentials: false"); count != 3 {
		t.Errorf("persist-credentials: false occurs %d times, want all three checkouts to avoid retaining repository credentials", count)
	}
	if strings.Contains(workflow, "version: \"~> v2\"") {
		t.Error("release workflow uses a floating GoReleaser version")
	}
	if strings.Contains(workflow, "MINISIGN_PUBLIC_KEYS_CANONICAL=%s") {
		t.Error("canonical trust anchors are persisted through GITHUB_ENV instead of a scoped step output")
	}

	action := regexp.MustCompile(`^\s*uses:\s*[^@\s]+@([0-9a-f]{40})(?:\s|$)`)
	scanner := bufio.NewScanner(strings.NewReader(workflow))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "uses:") && !action.MatchString(line) {
			t.Errorf("release action is not pinned to a full commit SHA: %s", strings.TrimSpace(line))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestStablePromotionWorkflowUsesBoundSourceAndProtectedPublication(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "promote-stable-rc.yml")
	for _, required := range []string{
		"workflow_dispatch:",
		"source_prerelease_tag:",
		"stable_tag:",
		"release_environment_policy_id:",
		"concurrency:",
		"./scripts/promote-stable-preflight.sh",
		"ref: ${{ steps.provenance.outputs.source_sha }}",
		"reset-empty-draft",
		"environment: release",
		"actions/github-script@ed597411d8f924073f98dfc5c65a23a2325f34cd",
		"recovery_state",
		"stable tag recovery identity changed",
		"test -n \"$GH_TOKEN\"",
		"./scripts/release-signing-preflight.sh",
		"RELEASE_SIGNING_TAG: ${{ inputs.stable_tag }}",
		"GORELEASER_CURRENT_TAG",
		"ref: ${{ github.sha }}",
		"path: orchestration",
		"RELEASE_VERIFICATION_TAG: ${{ inputs.stable_tag }}",
		"./scripts/verify-release-assets.sh",
		"stable published recovery state changed",
		"Read immutable provider contract semver",
		"provider_contract_semver",
		"PROVIDER_CONTRACT_SEMVER: ${{ needs.preflight.outputs.provider_contract_semver }}",
		"RELEASE_ENVIRONMENT_POLICY_TOKEN",
		"--method DELETE",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("stable promotion workflow is missing %q", required)
		}
	}
	if regexp.MustCompile(`(?i)actions/workflows/.*/disable`).MatchString(workflow) {
		t.Error("stable promotion workflow must not disable the legacy publisher")
	}
	if regexp.MustCompile(`(?m)\bgit push\b`).MatchString(workflow) {
		t.Error("stable promotion workflow must not push a tag from a shell step")
	}
	if strings.Contains(workflow, "GITHUB_REF_NAME: ${{ inputs.stable_tag }}") {
		t.Error("stable promotion workflow must not override reserved GITHUB_REF_NAME")
	}
	action := regexp.MustCompile(`^\s*(-\s*)?uses:\s*[^@\s]+@([0-9a-f]{40})(?:\s|$)`)
	scanner := bufio.NewScanner(strings.NewReader(workflow))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "uses:") && !action.MatchString(line) {
			t.Errorf("stable promotion action is not pinned to a full commit SHA: %s", strings.TrimSpace(line))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestSigningMaterialCleanupFallsBackAndProvesAbsence(t *testing.T) {
	for _, workflowPath := range [][]string{
		{".github", "workflows", "release.yml"},
		{".github", "workflows", "promote-stable-rc.yml"},
	} {
		workflow := readRepositoryFile(t, workflowPath...)
		for _, required := range []string{
			"- name: Remove signing material",
			"if: always()",
			`shred --remove "$MINISIGN_SECRET_KEY_FILE" 2>/dev/null || rm -f "$MINISIGN_SECRET_KEY_FILE"`,
			`rm -f "$MINISIGN_SIGNING_PUBLIC_KEY_FILE"`,
			`test ! -e "$MINISIGN_SECRET_KEY_FILE"`,
			`test ! -e "$MINISIGN_SIGNING_PUBLIC_KEY_FILE"`,
		} {
			if !strings.Contains(workflow, required) {
				t.Errorf("%s cleanup is missing %q", filepath.Join(workflowPath...), required)
			}
		}
		if strings.Contains(workflow, `shred --remove "$MINISIGN_SECRET_KEY_FILE" 2>/dev/null || true`) {
			t.Errorf("%s cleanup masks shred failure", filepath.Join(workflowPath...))
		}
	}
}

func TestReleaseAssetVerifierPreservesReadOnlyRotationVerification(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("release verification runtime is Ubuntu-specific")
	}
	for _, command := range []string{"bash", "jq", "sha256sum"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s is unavailable: %v", command, err)
		}
	}

	makeKey := func(fill byte) string {
		payload := bytes.Repeat([]byte{fill}, 42)
		payload[0], payload[1] = 'E', 'd'
		return base64.StdEncoding.EncodeToString(payload)
	}
	firstKey, signingKey := makeKey(1), makeKey(2)
	fakeBin := t.TempDir()
	ghLog := filepath.Join(t.TempDir(), "gh-calls.log")
	writeExecutable := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeExecutable("gh", `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$GH_CALL_LOG"
tag=${RELEASE_VERIFICATION_TAG:-$GITHUB_REF_NAME}
if [[ "$1" == api && "$2" == "repos/$GITHUB_REPOSITORY/releases/tags/$tag" ]]; then
  cat <<JSON
{"tag_name":"$tag","draft":false,"prerelease":false,"assets":[{"name":"gentle-ai_1.2.3_darwin_amd64.tar.gz"},{"name":"gentle-ai_1.2.3_darwin_arm64.tar.gz"},{"name":"gentle-ai_1.2.3_linux_amd64.tar.gz"},{"name":"gentle-ai_1.2.3_linux_arm64.tar.gz"},{"name":"gentle-ai-review-provider-contract-1.1.0.tar.gz"},{"name":"gentle-ai-release-provenance-v1.tar.gz"},{"name":"checksums.txt"},{"name":"checksums.txt.minisig"}]}
JSON
  exit 0
fi
if [[ "$1" == release && "$2" == download && "$3" == "$tag" ]]; then
  shift 3
  directory=
  while (( $# > 0 )); do
    case "$1" in
      --dir) directory=$2; shift 2 ;;
      *) shift ;;
    esac
  done
  [[ -n "$directory" ]]
  mkdir -p "$directory"
  for platform in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
    printf 'archive %s\n' "$platform" >"$directory/gentle-ai_1.2.3_${platform}.tar.gz"
  done
  printf 'provider contract\n' >"$directory/gentle-ai-review-provider-contract-1.1.0.tar.gz"
  printf 'release provenance\n' >"$directory/gentle-ai-release-provenance-v1.tar.gz"
  (cd "$directory" && sha256sum gentle-ai_1.2.3_*.tar.gz gentle-ai-review-provider-contract-1.1.0.tar.gz gentle-ai-release-provenance-v1.tar.gz >checksums.txt)
  printf 'test signature\n' >"$directory/checksums.txt.minisig"
  exit 0
fi
exit 64
`)
	writeExecutable("minisign", `#!/usr/bin/env bash
set -euo pipefail
public_key=
while (( $# > 0 )); do
  case "$1" in
    -P) public_key=$2; shift 2 ;;
    *) shift ;;
  esac
done
[[ "$public_key" == "$EXPECTED_SIGNING_KEY" ]] || exit 1
printf 'repo=%s;tag=%s\n' "$GITHUB_REPOSITORY" "${RELEASE_VERIFICATION_TAG:-$GITHUB_REF_NAME}"
`)

	root := filepath.Clean(filepath.Join("..", ".."))
	for _, tc := range []struct {
		name            string
		githubRef       string
		verificationTag string
		setExplicitTag  bool
		wantSuccess     bool
		wantOutput      string
	}{
		{
			name:            "promotion tag overrides workflow dispatch main ref",
			githubRef:       "main",
			verificationTag: "v1.2.3",
			setExplicitTag:  true,
			wantSuccess:     true,
		},
		{
			name:        "native tag ref remains supported",
			githubRef:   "v1.2.3",
			wantSuccess: true,
		},
		{
			name:            "empty explicit promotion tag fails closed",
			githubRef:       "v1.2.3",
			verificationTag: "",
			setExplicitTag:  true,
			wantOutput:      "RELEASE_VERIFICATION_TAG is empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(ghLog, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("bash", "scripts/verify-release-assets.sh")
			command.Dir = root
			environment := make([]string, 0, len(os.Environ())+8)
			for _, value := range os.Environ() {
				name, _, _ := strings.Cut(value, "=")
				switch name {
				case "PATH", "GH_CALL_LOG", "GH_TOKEN", "GITHUB_REPOSITORY", "GITHUB_REF_NAME", "MINISIGN_PUBLIC_KEYS", "EXPECTED_SIGNING_KEY", "RELEASE_VERIFICATION_TAG":
					continue
				}
				environment = append(environment, value)
			}
			command.Env = append(environment,
				"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GH_CALL_LOG="+ghLog,
				"GH_TOKEN=read-only-test-token",
				"GITHUB_REPOSITORY=Gentleman-Programming/gentle-ai",
				"GITHUB_REF_NAME="+tc.githubRef,
				"MINISIGN_PUBLIC_KEYS="+firstKey+","+signingKey,
				"EXPECTED_SIGNING_KEY="+signingKey,
			)
			if tc.setExplicitTag {
				command.Env = append(command.Env, "RELEASE_VERIFICATION_TAG="+tc.verificationTag)
			}
			output, err := command.CombinedOutput()
			if tc.wantSuccess && err != nil {
				t.Fatalf("read-only release verification failed: %v\n%s", err, output)
			}
			if !tc.wantSuccess && err == nil {
				t.Fatalf("release verification accepted an unsafe tag binding:\n%s", output)
			}
			if tc.wantOutput != "" && !strings.Contains(string(output), tc.wantOutput) {
				t.Fatalf("release verification output = %q, want %q", output, tc.wantOutput)
			}
			if !tc.wantSuccess {
				return
			}
			calls, err := os.ReadFile(ghLog)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
			if len(lines) != 2 || !strings.HasPrefix(lines[0], "api repos/") || !strings.HasPrefix(lines[1], "release download v1.2.3 ") {
				t.Fatalf("release verifier used commands outside the approved read-only surface: %q", lines)
			}
		})
	}
}

func TestStablePromotionVerifyExistingRecoveryIsVerificationOnly(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "promote-stable-rc.yml")
	for _, required := range []string{
		"if (recovery === 'verify-existing' && (!release || release.data.draft || release.data.prerelease || !release.data.immutable",
		"core.setOutput('publish', recovery === 'verify-existing' ? 'false' : 'true');",
		"if: steps.tag.outputs.publish == 'true'",
		"HOMEBREW_TAP_TOKEN",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("stable promotion recovery contract is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"github.rest.repos.updateRelease",
		"github.rest.repos.uploadReleaseAsset",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("verify-existing recovery must not mutate published releases: found %q", forbidden)
		}
	}
}

func TestStablePromotionPreflightUsesRESTCompatibleReleaseID(t *testing.T) {
	preflight := readRepositoryFile(t, "scripts", "promote-stable-preflight.sh")
	for _, required := range []string{
		"release(tagName: $tag) { databaseId }",
		".data.repository.release.databaseId // empty",
		"recovery_state=verify-existing",
		"repos/$GITHUB_REPOSITORY/releases/$stable_release",
	} {
		if !strings.Contains(preflight, required) {
			t.Errorf("stable promotion preflight is missing %q", required)
		}
	}
	if strings.Contains(preflight, "release(tagName: $tag) { id }") {
		t.Error("stable promotion preflight must not pass a GraphQL node ID to the REST release endpoint")
	}
}

func TestGoReleaserSignsBoundManifestAndInjectsTrustAnchors(t *testing.T) {
	config := readRepositoryFile(t, ".goreleaser.yaml")
	for _, required := range []string{
		"artifacts: checksum",
		`signature: ${artifact}.minisig`,
		`- "${artifact}"`,
		`- "${signature}"`,
		`repo=Gentleman-Programming/gentle-ai;tag={{ .Tag }}`,
		`github.com/gentleman-programming/gentle-ai/v2/internal/update/upgrade.releaseMinisignPublicKeys={{ .Env.MINISIGN_PUBLIC_KEYS_CANONICAL }}`,
		"-trimpath",
		"go run ./internal/releaseprovenancecmd --out .goreleaser-provenance/manifest.json --config .goreleaser.yaml --goreleaser-version v2.15.2",
		"id: release-provenance",
		"gentle-ai-release-provenance-v1",
		"src: .goreleaser-provenance/manifest.json",
		"mode: 0644",
		"mtime: \"1970-01-01T00:00:00Z\"",
	} {
		if !strings.Contains(config, required) {
			t.Errorf("GoReleaser config is missing %q", required)
		}
	}
	if strings.Contains(config, "go mod tidy") {
		t.Error("GoReleaser must not mutate go.mod/go.sum; release preflight uses go mod tidy -diff")
	}
	if strings.Contains(config, "{{ .ArtifactName }}") {
		t.Error("signing uses filename-only ArtifactName instead of GoReleaser's full ${artifact} path")
	}
	if strings.Contains(config, `.Env.MINISIGN_PUBLIC_KEYS }}`) {
		t.Error("GoReleaser injects the unvalidated raw MINISIGN_PUBLIC_KEYS value")
	}
}

func TestGoReleaserRecreatesCleanProvenanceStaging(t *testing.T) {
	config := readRepositoryFile(t, ".goreleaser.yaml")
	cleanup := "rm -rf .goreleaser-provenance"
	mkdir := "mkdir -p .goreleaser-provenance"
	generate := "go run ./internal/releaseprovenancecmd --out .goreleaser-provenance/manifest.json --config .goreleaser.yaml --goreleaser-version v2.15.2"

	cleanupIndex := strings.Index(config, cleanup)
	mkdirIndex := strings.Index(config, mkdir)
	generateIndex := strings.Index(config, generate)
	if cleanupIndex < 0 || mkdirIndex < 0 || generateIndex < 0 || !(cleanupIndex < mkdirIndex && mkdirIndex < generateIndex) {
		t.Errorf("provenance staging hooks must run cleanup, mkdir, then generate; indexes = %d, %d, %d", cleanupIndex, mkdirIndex, generateIndex)
	}
	if !strings.Contains(readRepositoryFile(t, ".gitignore"), ".goreleaser-provenance/") {
		t.Error(".gitignore does not ignore provenance staging")
	}
}

func TestReleaseSecurityScriptsAreSyntacticallyValidAndFailClosed(t *testing.T) {
	tests := []struct {
		path         string
		supportPaths []string
		required     []string
	}{
		{
			path: "canonicalize-release-public-keys.sh",
			required: []string{
				`MINISIGN_PUBLIC_KEYS`,
				`configure one canonical key or a two-key rotation overlap`,
				`public key payload must decode to 42 bytes`,
			},
		},
		{
			path: "require-ci-success.sh",
			required: []string{
				`require_env GITHUB_REPOSITORY`,
				`require_env GITHUB_SHA`,
				`require_env GH_TOKEN`,
				`head_sha=$GITHUB_SHA`,
				`actions/workflows/$workflow_ref/runs`,
				`[[ "$conclusion" == "success" ]]`,
			},
		},
		{
			path: "promote-stable-preflight.sh",
			required: []string{
				`databaseId`,
				`source prerelease tag must be canonical`,
				`workflow revision is not exact current origin/main`,
				`stable tag is incompatible with the admitted source`,
				`stable release state is incompatible with safe recovery`,
				`recovery_state=%s`,
				`GITHUB_SHA=$source_sha ./scripts/require-ci-success.sh`,
			},
		},
		{
			path: "release-preflight.sh",
			required: []string{
				`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`,
				`refs/remotes/origin/main`,
				`refs/tags/$tag^{commit}`,
				`go mod tidy -diff`,
				`git status --porcelain=v1 --untracked-files=all`,
			},
		},
		{
			path: "release-signing-preflight.sh",
			required: []string{
				`RELEASE_SIGNING_TAG`,
				`MINISIGN_PUBLIC_KEYS_CANONICAL`,
				`minisign -R`,
				`minisign -S`,
				`minisign -VQ`,
				`internal/update/upgrade/testdata/minisign-test.pub`,
			},
		},
		{
			path: "verify-release-assets.sh",
			required: []string{
				`RELEASE_VERIFICATION_TAG`,
				`gh release download`,
				`minisign -VQ`,
				`canonicalize-release-public-keys.sh`,
				`MINISIGN_PUBLIC_KEYS`,
				`sha256sum --check --strict`,
				`gentle-ai_${version}_linux_amd64.tar.gz`,
				`gentle-ai-release-provenance-v1.tar.gz`,
				`checksums.txt.minisig`,
			},
		},
		{
			path: "verify-release-distribution-policy.sh",
			supportPaths: []string{
				filepath.Join("internal", "releasepolicy", "policy.go"),
				filepath.Join("internal", "releasepolicycmd", "main.go"),
			},
			required: []string{
				`go run ./internal/releasepolicycmd`,
				`expectedGoReleaserYAML`,
				`expectedReleaseWorkflowYAML`,
				`resolved Homebrew publisher changed`,
				`snapshot output predates the current run marker`,
				`snapshot output path contains a symlink`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			path := filepath.Join("..", "..", "scripts", tc.path)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			for _, supportPath := range tc.supportPaths {
				support, err := os.ReadFile(filepath.Join("..", "..", supportPath))
				if err != nil {
					t.Fatalf("read %s support %s: %v", tc.path, supportPath, err)
				}
				content = append(content, support...)
			}
			for _, required := range tc.required {
				if !strings.Contains(string(content), required) {
					t.Errorf("%s is missing %q", tc.path, required)
				}
			}
			cmd := exec.Command("bash", "-n", path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("bash -n %s: %v\n%s", tc.path, err, output)
			}
		})
	}
}

func TestRequireCISuccessSelectsNewestExactCommitRun(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name        string
		ghOutput    string
		ghFails     bool
		wantSuccess bool
		wantOutput  string
	}{
		{
			name:       "API failure",
			ghFails:    true,
			wantOutput: "could not read workflow runs for " + sha,
		},
		{
			name:       "no matching run",
			wantOutput: "no CI run exists for " + sha,
		},
		{
			name: "newest run in progress",
			ghOutput: strings.Join([]string{
				"2026-07-27T10:00:00Z\t100\tcompleted\tsuccess\thttps://example.test/runs/100",
				"2026-07-27T11:00:00Z\t101\tin_progress\tnone\thttps://example.test/runs/101",
			}, "\n"),
			wantOutput: "CI is still in_progress for " + sha + " (https://example.test/runs/101)",
		},
		{
			name:       "newest run failure",
			ghOutput:   "2026-07-27T11:00:00Z\t101\tcompleted\tfailure\thttps://example.test/runs/101",
			wantOutput: "CI concluded failure for " + sha + " (https://example.test/runs/101)",
		},
		{
			name: "newest run success",
			ghOutput: strings.Join([]string{
				"2026-07-27T11:00:00Z\t101\tcompleted\tsuccess\thttps://example.test/runs/101",
				"2026-07-27T10:00:00Z\t100\tcompleted\tfailure\thttps://example.test/runs/100",
			}, "\n"),
			wantSuccess: true,
			wantOutput:  "release CI gate: CI succeeded for " + sha,
		},
		{
			name:       "same-display-name alternate success cannot mask intended failure",
			ghOutput:   "2026-07-27T11:00:00Z\t101\tcompleted\tfailure\thttps://example.test/runs/101",
			wantOutput: "CI concluded failure for " + sha + " (https://example.test/runs/101)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			if err := os.Mkdir(home, 0o700); err != nil {
				t.Fatal(err)
			}
			fakeBin := filepath.Join(root, "bin")
			if err := os.Mkdir(fakeBin, 0o700); err != nil {
				t.Fatal(err)
			}
			responsePath := filepath.Join(root, "response")
			if err := os.WriteFile(responsePath, []byte(tc.ghOutput), 0o600); err != nil {
				t.Fatal(err)
			}
			ghPath := filepath.Join(fakeBin, "gh")
			if err := os.WriteFile(ghPath, []byte(`#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == api ]]
[[ "$2" == --paginate ]]
printf '%s\n' "$*" >"$FAKE_GH_LOG"
if [[ "$3" == "repos/$GITHUB_REPOSITORY/actions/runs?head_sha=$GITHUB_SHA&per_page=100" ]]; then
  printf '2026-07-27T12:00:00Z\t200\tcompleted\tsuccess\thttps://example.test/runs/200\n'
  exit 0
fi
[[ "$3" == "repos/$GITHUB_REPOSITORY/actions/workflows/ci.yml/runs?head_sha=$GITHUB_SHA&per_page=100" ]]
[[ "$4" == --jq ]]
[[ "$5" == '.workflow_runs[] | [.created_at, .id, .status, (.conclusion // "none"), .html_url] | @tsv' ]]
[[ "${FAKE_GH_FAILS:-0}" != 1 ]] || exit 42
cat "$FAKE_GH_RESPONSE"
`), 0o700); err != nil {
				t.Fatal(err)
			}

			command := exec.Command("bash", "scripts/require-ci-success.sh")
			command.Dir = filepath.Clean(filepath.Join("..", ".."))
			environment := make([]string, 0, len(os.Environ())+7)
			for _, value := range os.Environ() {
				name, _, _ := strings.Cut(value, "=")
				switch name {
				case "HOME", "PATH", "GH_TOKEN", "GITHUB_REPOSITORY", "GITHUB_SHA", "RELEASE_REQUIRED_WORKFLOW", "FAKE_GH_RESPONSE", "FAKE_GH_LOG", "FAKE_GH_FAILS":
					continue
				}
				environment = append(environment, value)
			}
			command.Env = append(environment,
				"HOME="+home,
				"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GH_TOKEN=test-token",
				"GITHUB_REPOSITORY=Gentleman-Programming/gentle-ai",
				"GITHUB_SHA="+sha,
				"FAKE_GH_RESPONSE="+responsePath,
				"FAKE_GH_LOG="+filepath.Join(root, "gh.log"),
			)
			if tc.ghFails {
				command.Env = append(command.Env, "FAKE_GH_FAILS=1")
			}
			output, err := command.CombinedOutput()
			if tc.wantSuccess && err != nil {
				t.Fatalf("gate rejected newest successful run: %v\n%s", err, output)
			}
			if !tc.wantSuccess && err == nil {
				t.Fatalf("gate accepted unsafe run state:\n%s", output)
			}
			if !strings.Contains(string(output), tc.wantOutput) {
				t.Fatalf("gate output = %q, want substring %q", output, tc.wantOutput)
			}
		})
	}
}

func TestCanonicalReleasePublicKeysControlRealLinkerBuild(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	publicKey := strings.TrimSpace(readRepositoryFile(t, "internal", "update", "upgrade", "testdata", "minisign-test.pub"))
	const linkerTarget = "github.com/gentleman-programming/gentle-ai/v2/internal/update/upgrade.releaseMinisignPublicKeys"
	const injectedOverride = "AUDIT_OVERRIDE"

	build := func(t *testing.T, raw string) (string, []byte, error) {
		t.Helper()
		outPath := filepath.Join(t.TempDir(), "gentle-ai")
		cmd := exec.Command("bash", "-c", `
set -euo pipefail
canonical=$(./scripts/canonicalize-release-public-keys.sh)
go build -trimpath -o "$OUT" -ldflags "-X $LINKER_TARGET=$canonical" ./cmd/gentle-ai
`)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(),
			"MINISIGN_PUBLIC_KEYS="+raw,
			"OUT="+outPath,
			"LINKER_TARGET="+linkerTarget,
		)
		output, err := cmd.CombinedOutput()
		return outPath, output, err
	}

	invalid := []struct {
		name string
		raw  string
	}{
		{
			name: "newline linker override",
			raw:  publicKey + "\n-X " + linkerTarget + "=" + injectedOverride,
		},
		{name: "same-line linker argument", raw: publicKey + " -X " + linkerTarget + "=" + injectedOverride},
		{name: "trailing comma", raw: publicKey + ","},
		{name: "leading whitespace", raw: " " + publicKey},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			outPath, output, err := build(t, tc.raw)
			if err == nil {
				t.Fatalf("linker build accepted noncanonical keys; output:\n%s", output)
			}
			if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
				t.Fatalf("rejected key input produced a binary: %v", statErr)
			}
		})
	}

	t.Run("canonical key is the only linker value", func(t *testing.T) {
		outPath, output, err := build(t, publicKey)
		if err != nil {
			t.Fatalf("canonical linker build failed: %v\n%s", err, output)
		}
		binary, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(binary, []byte(publicKey)) {
			t.Fatal("built binary does not contain the canonical validated public key")
		}
		if bytes.Contains(binary, []byte(injectedOverride)) {
			t.Fatal("built binary contains the rejected linker override")
		}
	})
}

func TestReleaseDocumentationStatesArchiveDownloadCeiling(t *testing.T) {
	docs := readRepositoryFile(t, "docs", "release-signing.md") + readRepositoryFile(t, "README.md")
	if !strings.Contains(docs, "128 MiB") {
		t.Fatal("release documentation does not state the updater's 128 MiB archive ceiling")
	}
}

func TestIsolatedMinisignTestPublicKeyFixture(t *testing.T) {
	fixture := strings.TrimSpace(readRepositoryFile(t, "internal", "update", "upgrade", "testdata", "minisign-test.pub"))
	const expected = "RWS5glvo7U0Evs9J03vF/Lma+BY/2PMol//qa7T4gLxl7+KLNlSIDk0X"
	if fixture != expected {
		t.Fatalf("isolated Minisign test public key = %q, want %q", fixture, expected)
	}
}
