package releasepolicy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Validate proves that the canonical release configuration, workflow, and
// current no-publish snapshot describe the one approved distribution path.
func Validate(root, markerPath, runID string) error {
	root, err := canonicalDirectory(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	markerTime, err := validateRunMarker(root, markerPath, runID)
	if err != nil {
		return err
	}

	config, err := readRegularFile(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		return fmt.Errorf("read canonical GoReleaser config: %w", err)
	}
	if err := validateYAMLSemantics("GoReleaser config", config, expectedGoReleaserYAML); err != nil {
		return err
	}

	workflow, err := readRegularFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		return fmt.Errorf("read canonical release workflow: %w", err)
	}
	if err := validateYAMLSemantics("release workflow", workflow, expectedReleaseWorkflowYAML); err != nil {
		return err
	}

	artifactsPath := filepath.Join(root, "dist", "artifacts.json")
	if err := validateSnapshotFile(root, "dist/artifacts.json", markerTime); err != nil {
		return fmt.Errorf("snapshot metadata: %w", err)
	}
	artifacts, err := readRegularFile(artifactsPath)
	if err != nil {
		return fmt.Errorf("read GoReleaser snapshot metadata: %w", err)
	}
	if err := validateArtifacts(root, artifacts, markerTime); err != nil {
		return err
	}
	return nil
}

func canonicalDirectory(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("path is not a directory")
	}
	return resolved, nil
}

func validateRunMarker(root, markerPath, runID string) (time.Time, error) {
	if runID == "" || len(runID) > 512 || strings.ContainsAny(runID, "\x00\r\n") {
		return time.Time{}, errors.New("snapshot run identity is invalid")
	}
	if markerPath == "" || !filepath.IsAbs(markerPath) {
		return time.Time{}, errors.New("snapshot marker must be an absolute path")
	}
	markerPath = filepath.Clean(markerPath)
	resolvedMarker, err := filepath.EvalSymlinks(markerPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("resolve snapshot marker: %w", err)
	}
	// Both operands go through the same filesystem-identity question. Asking
	// filepath.Rel here compared a resolved marker against an unresolved
	// dist, so a dist that was itself a symlink made a marker inside the
	// snapshot directory look like a marker outside it.
	if directoryContains(filepath.Join(root, "dist"), resolvedMarker) {
		return time.Time{}, errors.New("snapshot marker must remain outside the clean snapshot directory")
	}
	info, err := os.Lstat(markerPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("read snapshot marker: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return time.Time{}, errors.New("snapshot marker must be a regular non-symlink file")
	}
	payload, err := os.ReadFile(markerPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("read snapshot marker: %w", err)
	}
	if !bytes.Equal(payload, []byte(runID+"\n")) {
		return time.Time{}, errors.New("snapshot marker does not bind the current run identity")
	}
	return info.ModTime(), nil
}

// directoryContains reports whether path is directory itself or lies beneath
// it, deciding identity by device and inode rather than by comparing strings.
// Two spellings the operating system resolves to one directory -- a symlinked
// ancestor, a case-insensitive volume, a Unicode-equivalent name -- are one
// directory here.
//
// This repeats internal/pathidentity.Contains on purpose, and it is the only
// copy that is allowed to exist. This package is compiled in isolation by the
// release-policy verifier, which copies policy.go and releasepolicycmd/main.go
// into a bare module so the thing that validates a release cannot depend on
// the tree it is validating (see TestReleaseDistributionPolicyAssertionFails-
// Closed in internal/update). An import would break that isolation, so the
// rule is duplicated rather than the policy being changed. Keep the two in
// step: internal/pathidentity states the policy and its limits in full.
func directoryContains(directory, path string) bool {
	directoryInfo, err := os.Stat(directory)
	if err != nil || !directoryInfo.IsDir() {
		return false
	}
	current, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for {
		if info, statErr := os.Stat(current); statErr == nil && info.IsDir() && os.SameFile(directoryInfo, info) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func readRegularFile(filename string) ([]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("path is not a regular non-symlink file")
	}
	return os.ReadFile(filename)
}

func validateYAMLSemantics(label string, actual []byte, expected string) error {
	actualNode, err := decodeSingleYAML(actual)
	if err != nil {
		return fmt.Errorf("%s is invalid YAML: %w", label, err)
	}
	expectedNode, err := decodeSingleYAML([]byte(expected))
	if err != nil {
		return fmt.Errorf("internal %s policy is invalid: %w", label, err)
	}
	if err := compareYAML(actualNode, expectedNode, "$", label); err != nil {
		return err
	}
	return nil
}

func decodeSingleYAML(payload []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, errors.New("document must contain exactly one root value")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("multiple YAML documents are forbidden")
	}
	return document.Content[0], nil
}

func compareYAML(actual, expected *yaml.Node, location, label string) error {
	if actual == nil || expected == nil {
		return fmt.Errorf("%s differs from the approved plan at %s", label, location)
	}
	if actual.Anchor != "" || actual.Kind == yaml.AliasNode {
		return fmt.Errorf("%s uses a forbidden YAML anchor or alias at %s", label, location)
	}
	if actual.Kind != expected.Kind || actual.Tag != expected.Tag {
		return fmt.Errorf("%s has a different value type at %s", label, location)
	}
	switch expected.Kind {
	case yaml.ScalarNode:
		if actual.Value != expected.Value {
			return fmt.Errorf("%s changed at %s", label, location)
		}
	case yaml.SequenceNode:
		if len(actual.Content) != len(expected.Content) {
			return fmt.Errorf("%s sequence length changed at %s", label, location)
		}
		for index := range expected.Content {
			if err := compareYAML(actual.Content[index], expected.Content[index], fmt.Sprintf("%s[%d]", location, index), label); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		actualEntries, err := yamlMapping(actual, location, label)
		if err != nil {
			return err
		}
		expectedEntries, err := yamlMapping(expected, location, label)
		if err != nil {
			return fmt.Errorf("internal %s policy is invalid: %w", label, err)
		}
		if len(actualEntries) != len(expectedEntries) {
			return fmt.Errorf("%s key set changed at %s", label, location)
		}
		for index := 0; index < len(expected.Content); index += 2 {
			key := expected.Content[index].Value
			expectedEntry := expectedEntries[key]
			actualEntry, ok := actualEntries[key]
			if !ok {
				return fmt.Errorf("%s is missing approved key %q at %s", label, key, location)
			}
			child := location + "." + key
			if err := compareYAML(actualEntry.key, expectedEntry.key, child, label); err != nil {
				return err
			}
			if err := compareYAML(actualEntry.value, expectedEntry.value, child, label); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s contains unsupported YAML at %s", label, location)
	}
	return nil
}

type yamlEntry struct {
	key   *yaml.Node
	value *yaml.Node
}

func yamlMapping(node *yaml.Node, location, label string) (map[string]yamlEntry, error) {
	if len(node.Content)%2 != 0 {
		return nil, fmt.Errorf("%s has an invalid mapping at %s", label, location)
	}
	entries := make(map[string]yamlEntry, len(node.Content)/2)
	for index := 0; index < len(node.Content); index += 2 {
		key := node.Content[index]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "" || key.Anchor != "" {
			return nil, fmt.Errorf("%s has a non-string mapping key at %s", label, location)
		}
		if _, exists := entries[key.Value]; exists {
			return nil, fmt.Errorf("%s repeats key %q at %s", label, key.Value, location)
		}
		entries[key.Value] = yamlEntry{key: key, value: node.Content[index+1]}
	}
	return entries, nil
}

type artifact struct {
	Name   string         `json:"name"`
	Path   string         `json:"path"`
	GOOS   string         `json:"goos"`
	GOARCH string         `json:"goarch"`
	Target string         `json:"target"`
	Type   string         `json:"type"`
	Extra  map[string]any `json:"extra"`
}

func validateArtifacts(root string, payload []byte, markerTime time.Time) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var artifacts []artifact
	if err := decoder.Decode(&artifacts); err != nil {
		return fmt.Errorf("cannot decode GoReleaser snapshot metadata: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	expectedCounts := map[string]int{"Metadata": 1, "Binary": 4, "Archive": 4, "Checksum": 1, "Homebrew Formula": 1}
	byType := make(map[string][]artifact)
	counts := make(map[string]int)
	paths := make(map[string]struct{})
	for _, item := range artifacts {
		counts[item.Type]++
		byType[item.Type] = append(byType[item.Type], item)
		if item.Path == "" {
			return errors.New("resolved artifact path is empty")
		}
		if _, exists := paths[item.Path]; exists {
			return fmt.Errorf("resolved artifact path is repeated: %s", item.Path)
		}
		paths[item.Path] = struct{}{}
	}
	if !reflect.DeepEqual(counts, expectedCounts) {
		return fmt.Errorf("resolved GoReleaser artifact types changed: %v", counts)
	}

	expectedTargets := map[string]string{
		"linux/amd64":  "linux_amd64_v1",
		"linux/arm64":  "linux_arm64_v8.0",
		"darwin/amd64": "darwin_amd64_v1",
		"darwin/arm64": "darwin_arm64_v8.0",
	}
	seenBinaries := make(map[string]struct{})
	for _, item := range byType["Binary"] {
		platform := item.GOOS + "/" + item.GOARCH
		target, ok := expectedTargets[platform]
		if !ok || item.Target != target {
			return fmt.Errorf("resolved binary matrix changed at %s", platform)
		}
		expectedPath := fmt.Sprintf("dist/gentle-ai_%s/gentle-ai", target)
		if item.Name != "gentle-ai" || item.Path != expectedPath || extraString(item.Extra, "Binary") != "gentle-ai" || extraString(item.Extra, "ID") != "gentle-ai" {
			return fmt.Errorf("resolved binary identity changed at %s", platform)
		}
		if _, exists := seenBinaries[platform]; exists {
			return fmt.Errorf("resolved binary target is repeated: %s", platform)
		}
		seenBinaries[platform] = struct{}{}
	}
	if len(seenBinaries) != len(expectedTargets) {
		return errors.New("resolved binary matrix is incomplete")
	}

	seenArchives := make(map[string]struct{})
	snapshotVersion := ""
	for _, item := range byType["Archive"] {
		platform := item.GOOS + "/" + item.GOARCH
		target, ok := expectedTargets[platform]
		if !ok || item.Target != target {
			return fmt.Errorf("resolved archive matrix changed at %s", platform)
		}
		suffix := fmt.Sprintf("_%s_%s.tar.gz", item.GOOS, item.GOARCH)
		version := strings.TrimSuffix(strings.TrimPrefix(item.Name, "gentle-ai_"), suffix)
		if !strings.HasPrefix(item.Name, "gentle-ai_") || !strings.HasSuffix(item.Name, suffix) || !validSnapshotVersion(version) {
			return fmt.Errorf("resolved archive name changed at %s", platform)
		}
		if snapshotVersion == "" {
			snapshotVersion = version
		} else if version != snapshotVersion {
			return errors.New("resolved archives do not share one snapshot version")
		}
		if item.Path != "dist/"+item.Name || extraString(item.Extra, "Format") != "tar.gz" || extraString(item.Extra, "ID") != "default" || !reflect.DeepEqual(extraStrings(item.Extra, "Binaries"), []string{"gentle-ai"}) {
			return fmt.Errorf("resolved archive identity changed at %s", platform)
		}
		if _, exists := seenArchives[platform]; exists {
			return fmt.Errorf("resolved archive target is repeated: %s", platform)
		}
		seenArchives[platform] = struct{}{}
	}
	if len(seenArchives) != len(expectedTargets) {
		return errors.New("resolved archive matrix is incomplete")
	}

	if item := byType["Checksum"][0]; item.Name != "checksums.txt" || item.Path != "dist/checksums.txt" {
		return errors.New("resolved checksum output changed")
	}
	if item := byType["Metadata"][0]; item.Name != "metadata.json" || item.Path != "dist/metadata.json" {
		return errors.New("resolved metadata output changed")
	}
	formula := byType["Homebrew Formula"][0]
	if formula.Name != "gentle-ai.rb" || formula.Path != "dist/homebrew/Formula/gentle-ai.rb" {
		return errors.New("resolved Homebrew formula output changed")
	}
	brewConfig := extraMap(formula.Extra, "BrewConfig")
	repository := extraMap(brewConfig, "repository")
	if extraString(brewConfig, "name") != "gentle-ai" || extraString(brewConfig, "directory") != "Formula" ||
		extraString(repository, "owner") != "Gentleman-Programming" || extraString(repository, "name") != "homebrew-tap" || extraString(repository, "token") != "{{ .Env.HOMEBREW_TAP_TOKEN }}" {
		return errors.New("resolved Homebrew publisher changed")
	}

	orderedPaths := make([]string, 0, len(paths))
	for artifactPath := range paths {
		orderedPaths = append(orderedPaths, artifactPath)
	}
	sort.Strings(orderedPaths)
	for _, artifactPath := range orderedPaths {
		if err := validateSnapshotFile(root, artifactPath, markerTime); err != nil {
			return fmt.Errorf("resolved artifact %q: %w", artifactPath, err)
		}
	}
	return nil
}

func validSnapshotVersion(version string) bool {
	if version == "" || !strings.Contains(version, "-SNAPSHOT") {
		return false
	}
	for _, character := range version {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune(".-+_", character) {
			continue
		}
		return false
	}
	return true
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return fmt.Errorf("cannot decode GoReleaser snapshot metadata: %w", err)
		}
		return errors.New("GoReleaser snapshot metadata contains trailing JSON")
	}
	return nil
}

func extraString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func extraStrings(values map[string]any, key string) []string {
	items, ok := values[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil
		}
		result = append(result, value)
	}
	return result
}

func extraMap(values map[string]any, key string) map[string]any {
	value, _ := values[key].(map[string]any)
	return value
}

func validateSnapshotFile(root, artifactPath string, markerTime time.Time) error {
	clean := path.Clean(artifactPath)
	if artifactPath == "" || clean != artifactPath || path.IsAbs(artifactPath) || !strings.HasPrefix(artifactPath, "dist/") {
		return errors.New("path must be canonical and remain under dist")
	}
	relative := strings.TrimPrefix(artifactPath, "dist/")
	if relative == "" || strings.HasPrefix(relative, "../") {
		return errors.New("path escapes the clean snapshot directory")
	}
	dist := filepath.Join(root, "dist")
	info, err := os.Lstat(dist)
	if err != nil {
		return fmt.Errorf("read clean snapshot directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("clean snapshot directory must be a real directory")
	}
	current := dist
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return errors.New("path contains an invalid component")
		}
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err = os.Lstat(current)
		if err != nil {
			return fmt.Errorf("snapshot output is missing: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("snapshot output path contains a symlink")
		}
		if index < len(parts)-1 && !info.IsDir() {
			return errors.New("snapshot output parent is not a directory")
		}
	}
	if !info.Mode().IsRegular() {
		return errors.New("snapshot output is not a regular file")
	}
	if info.ModTime().Before(markerTime) {
		return errors.New("snapshot output predates the current run marker")
	}
	return nil
}

const expectedGoReleaserYAML = `version: 2
project_name: gentle-ai
builds:
  - main: ./cmd/gentle-ai
    binary: gentle-ai
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    flags:
      - -trimpath
    ldflags:
      - >-
        -s -w
        -X main.version={{ .Version }}
        -X github.com/gentleman-programming/gentle-ai/v2/internal/update/upgrade.releaseMinisignPublicKeys={{ .Env.MINISIGN_PUBLIC_KEYS_CANONICAL }}
archives:
  - formats:
      - tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    files:
      - LICENSE
      - README.md
      - docs/review-integration.md
      - contracts/review-integration/v1/schemas/*.schema.json
      - contracts/review-integration/v1/fixtures/*.fixture.json
checksum:
  name_template: "checksums.txt"
  algorithm: sha256
signs:
  - cmd: minisign
    artifacts: checksum
    signature: ${artifact}.minisig
    args:
      - "-S"
      - "-s"
      - "{{ .Env.MINISIGN_SECRET_KEY_FILE }}"
      - "-m"
      - "${artifact}"
      - "-x"
      - "${signature}"
      - "-c"
      - "signature from gentle-ai release"
      - "-t"
      - "repo=Gentleman-Programming/gentle-ai;tag={{ .Tag }}"
    output: true
changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"
brews:
  - repository:
      owner: Gentleman-Programming
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    directory: Formula
    name: gentle-ai
    homepage: "https://github.com/Gentleman-Programming/gentle-ai"
    description: "Gentle-AI — Ecosystem, Frameworks, Workflows for AI coding agents."
    license: "MIT"
    commit_msg_template: "chore: update gentle-ai formula to {{ .Tag }}"
`

const expectedReleaseWorkflowYAML = `name: Release
on:
  push:
    tags:
      - "v*"
      - "!v*-*"
permissions:
  contents: read
concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false
jobs:
  preflight:
    runs-on: ubuntu-24.04
    timeout-minutes: 30
    permissions:
      contents: read
      actions: read
    env:
      MINISIGN_PUBLIC_KEYS: ${{ vars.MINISIGN_PUBLIC_KEYS }}
    steps:
      - name: Checkout exact tag
        uses: actions/checkout@93cb6efe18208431cddfb8368fd83d5badbf9bfd
        with:
          fetch-depth: 0
          fetch-tags: true
          persist-credentials: false
      - name: Require successful CI for release commit
        env:
          GH_TOKEN: ${{ github.token }}
        run: ./scripts/require-ci-success.sh
      - name: Set up Go
        uses: actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c
        with:
          go-version-file: go.mod
      - name: Mark release distribution snapshot start
        run: |
          set -euo pipefail
          run_id="${GITHUB_RUN_ID}:${GITHUB_RUN_ATTEMPT}:${GITHUB_JOB}"
          marker="$RUNNER_TEMP/gentle-ai-release-policy-snapshot-start"
          rm -f -- "$marker"
          umask 077
          printf '%s\n' "$run_id" >"$marker"
          printf 'RELEASE_POLICY_SNAPSHOT_MARKER=%s\n' "$marker" >>"$GITHUB_ENV"
          printf 'RELEASE_POLICY_SNAPSHOT_RUN_ID=%s\n' "$run_id" >>"$GITHUB_ENV"
      - name: Resolve release distribution plan without publishing
        uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94
        with:
          version: v2.15.2
          args: release --snapshot --clean --skip=sign,publish
        env:
          MINISIGN_PUBLIC_KEYS_CANONICAL: release-policy-validation-only
      - name: Verify release distribution policy
        run: ./scripts/verify-release-distribution-policy.sh
      - name: Verify tag, main, trust anchors, and module immutability
        run: ./scripts/release-preflight.sh
      - name: Unit tests
        run: go test ./...
      - name: Go vet
        run: go vet ./...
      - name: Go format
        run: go run ./internal/gofmtcheck
  release:
    needs: preflight
    runs-on: ubuntu-24.04
    timeout-minutes: 45
    environment: release
    permissions:
      contents: write
    steps:
      - name: Checkout exact tag
        uses: actions/checkout@93cb6efe18208431cddfb8368fd83d5badbf9bfd
        with:
          fetch-depth: 0
          fetch-tags: true
          persist-credentials: false
      - name: Set up Go
        uses: actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c
        with:
          go-version-file: go.mod
      - name: Recheck immutable release provenance
        env:
          MINISIGN_PUBLIC_KEYS: ${{ vars.MINISIGN_PUBLIC_KEYS }}
        run: ./scripts/release-preflight.sh
      - name: Export canonical release trust anchors
        id: trust-anchors
        env:
          MINISIGN_PUBLIC_KEYS: ${{ vars.MINISIGN_PUBLIC_KEYS }}
        run: |
          set -euo pipefail
          canonical=$(./scripts/canonicalize-release-public-keys.sh)
          printf 'canonical=%s\n' "$canonical" >>"$GITHUB_OUTPUT"
      - name: Configure ephemeral signing paths
        run: |
          printf 'MINISIGN_SECRET_KEY_FILE=%s/gentle-ai-release.key\n' "$RUNNER_TEMP" >>"$GITHUB_ENV"
          printf 'MINISIGN_SIGNING_PUBLIC_KEY_FILE=%s/gentle-ai-release-signing.pub\n' "$RUNNER_TEMP" >>"$GITHUB_ENV"
      - name: Install Minisign
        run: |
          sudo apt-get update
          sudo apt-get install --no-install-recommends -y minisign
      - name: Materialize signing key
        env:
          SIGNING_KEY_B64: ${{ secrets.MINISIGN_SECRET_KEY_BASE64 }}
        run: |
          set -euo pipefail
          umask 077
          test -n "$SIGNING_KEY_B64"
          printf '%s' "$SIGNING_KEY_B64" | base64 --decode >"$MINISIGN_SECRET_KEY_FILE"
          test -s "$MINISIGN_SECRET_KEY_FILE"
          chmod 600 "$MINISIGN_SECRET_KEY_FILE"
      - name: Verify signing credential and identity binding
        env:
          MINISIGN_PUBLIC_KEYS: ${{ vars.MINISIGN_PUBLIC_KEYS }}
          MINISIGN_PUBLIC_KEYS_CANONICAL: ${{ steps.trust-anchors.outputs.canonical }}
        run: ./scripts/release-signing-preflight.sh
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94
        with:
          version: v2.15.2
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ github.token }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
          MINISIGN_SECRET_KEY_FILE: ${{ env.MINISIGN_SECRET_KEY_FILE }}
          MINISIGN_PUBLIC_KEYS_CANONICAL: ${{ steps.trust-anchors.outputs.canonical }}
      - name: Remove signing material
        if: always()
        run: |
          if command -v shred >/dev/null 2>&1; then
            shred --remove "$MINISIGN_SECRET_KEY_FILE" 2>/dev/null || true
          else
            rm -f "$MINISIGN_SECRET_KEY_FILE"
          fi
          rm -f "$MINISIGN_SIGNING_PUBLIC_KEY_FILE"
  verify:
    needs: release
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    permissions:
      contents: read
    steps:
      - name: Checkout exact tag
        uses: actions/checkout@93cb6efe18208431cddfb8368fd83d5badbf9bfd
        with:
          fetch-depth: 0
          fetch-tags: true
          persist-credentials: false
      - name: Install Minisign
        run: |
          sudo apt-get update
          sudo apt-get install --no-install-recommends -y minisign
      - name: Verify published assets from GitHub
        env:
          GH_TOKEN: ${{ github.token }}
          MINISIGN_PUBLIC_KEYS: ${{ vars.MINISIGN_PUBLIC_KEYS }}
        run: ./scripts/verify-release-assets.sh
`
