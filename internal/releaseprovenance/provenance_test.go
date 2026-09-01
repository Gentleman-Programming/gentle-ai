package releaseprovenance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validInput() Input {
	return Input{
		Tag:                    "v1.2.3-rc.4",
		SourceSHA:              "0123456789abcdef0123456789abcdef01234567",
		WorkflowName:           "Release candidate",
		RunID:                  "42",
		RunAttempt:             2,
		Job:                    "publish",
		GoVersion:              "go1.25.10",
		ProviderContractSemver: "1.4.0",
		GoReleaserVersion:      "v2.15.2",
	}
}

func TestBuildCanonicalPayload(t *testing.T) {
	actual, err := Build([]byte("version: 2\n"), validInput())
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"schema":"gentle-ai.release-provenance/v1","repository":"Gentleman-Programming/gentle-ai","tag":"v1.2.3-rc.4","source_sha":"0123456789abcdef0123456789abcdef01234567","workflow":{"name":"Release candidate","run_id":"42","run_attempt":2,"job":"publish"},"toolchain":{"goreleaser":"v2.15.2","go":"go1.25.10"},"provider_contract_semver":"1.4.0","configuration_sha256":"sha256:16e0d1afd6e96036b776ab20684da35f075c50d6f3c0fd599ddc8218ebbdfb56","artifacts":[{"name":"gentle-ai_1.2.3-rc.4_darwin_amd64.tar.gz","kind":"binary","goos":"darwin","goarch":"amd64","cgo_enabled":"0","trimpath":true},{"name":"gentle-ai_1.2.3-rc.4_darwin_arm64.tar.gz","kind":"binary","goos":"darwin","goarch":"arm64","cgo_enabled":"0","trimpath":true},{"name":"gentle-ai_1.2.3-rc.4_linux_amd64.tar.gz","kind":"binary","goos":"linux","goarch":"amd64","cgo_enabled":"0","trimpath":true},{"name":"gentle-ai_1.2.3-rc.4_linux_arm64.tar.gz","kind":"binary","goos":"linux","goarch":"arm64","cgo_enabled":"0","trimpath":true},{"name":"gentle-ai-review-provider-contract-1.4.0.tar.gz","kind":"provider-contract"}]}` + "\n"
	if string(actual) != want {
		t.Fatalf("payload = %s\nwant %s", actual, want)
	}
}

func TestBuildRejectsNonCanonicalInputs(t *testing.T) {
	for name, mutate := range map[string]func(*Input){
		"tag":       func(in *Input) { in.Tag = "v1.2.3-rc.0" },
		"sha":       func(in *Input) { in.SourceSHA = strings.Repeat("A", 40) },
		"run ID":    func(in *Input) { in.RunID = "042" },
		"semver":    func(in *Input) { in.ProviderContractSemver = "1.4.0-rc.1" },
		"toolchain": func(in *Input) { in.GoReleaserVersion = "v2.15.3" },
	} {
		t.Run(name, func(t *testing.T) {
			input := validInput()
			mutate(&input)
			if _, err := Build([]byte("config"), input); err == nil {
				t.Fatal("Build accepted invalid input")
			}
		})
	}
}

func TestWriteCreatesCanonicalFileOnce(t *testing.T) {
	directory := t.TempDir()
	config := filepath.Join(directory, "config.yaml")
	if err := os.WriteFile(config, []byte("version: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "manifest.json")
	if err := Write(output, config, validInput()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(output)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != manifestPerm() {
		t.Fatalf("output metadata = %#v, %v", info, err)
	}
	if err := Write(output, config, validInput()); err == nil {
		t.Fatal("Write overwrote existing output")
	}
}
