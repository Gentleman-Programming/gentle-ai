package releaseprovenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

const (
	schema = "gentle-ai.release-provenance/v1"
	// localSchema marks a manifest produced outside GitHub Actions. It exists
	// because the archive that packages the manifest needs the file to be there,
	// not because a local build has provenance to report: prerelease tags do not
	// trigger release.yml, so building one by hand is the documented path, and a
	// local checkout knows no tag, run or workflow it could honestly name.
	localSchema         = "gentle-ai.release-provenance/local-build"
	repository          = "Gentleman-Programming/gentle-ai"
	goReleaserVersion   = "v2.15.2"
	providerArchiveKind = "provider-contract"
)

var (
	canonicalNumber = `(0|[1-9][0-9]*)`
	tagPattern      = regexp.MustCompile(`^v` + canonicalNumber + `\.` + canonicalNumber + `\.` + canonicalNumber + `(?:-rc\.[1-9][0-9]*)?$`)
	semverPattern   = regexp.MustCompile(`^` + canonicalNumber + `\.` + canonicalNumber + `\.` + canonicalNumber + `$`)
	shaPattern      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	runIDPattern    = regexp.MustCompile(`^[1-9][0-9]*$`)
	goPattern       = regexp.MustCompile(`^go1\.[0-9]+(?:\.[0-9]+)?$`)
	textPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._:/@+-]{0,255}$`)
)

type Input struct {
	Tag                    string
	SourceSHA              string
	WorkflowName           string
	RunID                  string
	RunAttempt             int
	Job                    string
	GoVersion              string
	ProviderContractSemver string
	GoReleaserVersion      string
}

type manifest struct {
	Schema                 string    `json:"schema"`
	Repository             string    `json:"repository"`
	Tag                    string    `json:"tag"`
	SourceSHA              string    `json:"source_sha"`
	Workflow               workflow  `json:"workflow"`
	Toolchain              toolchain `json:"toolchain"`
	ProviderContractSemver string    `json:"provider_contract_semver"`
	ConfigurationSHA256    string    `json:"configuration_sha256"`
	Artifacts              []any     `json:"artifacts"`
}

type workflow struct {
	Name       string `json:"name"`
	RunID      string `json:"run_id"`
	RunAttempt int    `json:"run_attempt"`
	Job        string `json:"job"`
}

type localManifest struct {
	Schema string `json:"schema"`
	Reason string `json:"reason"`
}

type toolchain struct {
	GoReleaser string `json:"goreleaser"`
	Go         string `json:"go"`
}

type binaryArtifact struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	CGOEnabled string `json:"cgo_enabled"`
	Trimpath   bool   `json:"trimpath"`
}

type contractArtifact struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Build returns a detached canonical manifest for the supplied release inputs.
func Build(config []byte, input Input) ([]byte, error) {
	if !tagPattern.MatchString(input.Tag) || !shaPattern.MatchString(input.SourceSHA) ||
		!textPattern.MatchString(input.WorkflowName) || !runIDPattern.MatchString(input.RunID) ||
		input.RunAttempt < 1 || !textPattern.MatchString(input.Job) || !goPattern.MatchString(input.GoVersion) ||
		!semverPattern.MatchString(input.ProviderContractSemver) || input.GoReleaserVersion != goReleaserVersion {
		return nil, errors.New("release provenance input is invalid")
	}
	version := input.Tag[1:]
	platforms := [][2]string{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}}
	artifacts := make([]any, 0, len(platforms)+1)
	for _, platform := range platforms {
		artifacts = append(artifacts, binaryArtifact{
			Name: "gentle-ai_" + version + "_" + platform[0] + "_" + platform[1] + ".tar.gz", Kind: "binary",
			GOOS: platform[0], GOARCH: platform[1], CGOEnabled: "0", Trimpath: true,
		})
	}
	artifacts = append(artifacts, contractArtifact{Name: "gentle-ai-review-provider-contract-" + input.ProviderContractSemver + ".tar.gz", Kind: providerArchiveKind})
	digest := sha256.Sum256(config)
	encoded, err := json.Marshal(manifest{
		Schema:                 schema,
		Repository:             repository,
		Tag:                    input.Tag,
		SourceSHA:              input.SourceSHA,
		Workflow:               workflow{Name: input.WorkflowName, RunID: input.RunID, RunAttempt: input.RunAttempt, Job: input.Job},
		Toolchain:              toolchain{GoReleaser: input.GoReleaserVersion, Go: input.GoVersion},
		ProviderContractSemver: input.ProviderContractSemver, ConfigurationSHA256: "sha256:" + hex.EncodeToString(digest[:]), Artifacts: artifacts,
	})
	if err != nil {
		return nil, errors.New("release provenance encoding failed")
	}
	return append(encoded, '\n'), nil
}

// Write creates a manifest at output without replacing an existing file.
func Write(output, configPath string, input Input) error {
	config, err := readReleaseConfiguration(configPath)
	if err != nil {
		return err
	}
	payload, err := Build(config, input)
	if err != nil {
		return err
	}
	return writeManifestOnce(output, payload)
}

// WriteLocal records that this build has no release provenance to report.
//
// The archive that packages the manifest needs the file to exist, and a
// prerelease tag does not trigger release.yml, so building one by hand is the
// documented path. A local checkout knows no tag, run, or workflow it could
// honestly name, so this states only that, under its own schema. Every field the
// canonical manifest binds is absent rather than invented, and no reader can
// mistake it for the evidence a release carries.
func WriteLocal(output, configPath string) error {
	if _, err := readReleaseConfiguration(configPath); err != nil {
		return err
	}
	encoded, err := json.Marshal(localManifest{
		Schema: localSchema,
		Reason: "built outside GitHub Actions; this build reports no release provenance",
	})
	if err != nil {
		return errors.New("release provenance encoding failed")
	}
	return writeManifestOnce(output, append(encoded, '\n'))
}

func readReleaseConfiguration(configPath string) ([]byte, error) {
	info, err := os.Lstat(configPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("release provenance configuration is invalid")
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.New("release provenance configuration is unreadable")
	}
	return config, nil
}

// manifestPerm is what a just-written 0o644 manifest reads back as. Windows
// has no POSIX permission bits: Go projects the read-only attribute onto the
// mode, so a writable regular file always reports 0o666 there.
func manifestPerm() os.FileMode {
	if runtime.GOOS == "windows" {
		return 0o666
	}
	return 0o644
}

func writeManifestOnce(output string, payload []byte) error {
	parent, err := os.Stat(filepath.Dir(output))
	if err != nil || !parent.IsDir() {
		return errors.New("release provenance output parent is unavailable")
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return errors.New("release provenance output already exists or cannot be created")
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return errors.New("release provenance output cannot be secured")
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return errors.New("release provenance output cannot be written")
	}
	if err := file.Close(); err != nil {
		return errors.New("release provenance output cannot be closed")
	}
	info, err := os.Lstat(output)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != manifestPerm() {
		return errors.New("release provenance output is invalid")
	}
	readback, err := os.ReadFile(output)
	if err != nil || string(readback) != string(payload) {
		return errors.New("release provenance output readback failed")
	}
	return nil
}
