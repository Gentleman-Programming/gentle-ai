package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func provenanceArgs(t *testing.T) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "manifest.json")
	return out, []string{"--out", out, "--config", "../../.goreleaser.yaml", "--goreleaser-version", "v2.15.2"}
}

func ciEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY", "Gentleman-Programming/gentle-ai")
	t.Setenv("GITHUB_REF_NAME", "v1.2.3-rc.4")
	t.Setenv("GITHUB_SHA", strings.Repeat("0123456789abcdef", 2)+strings.Repeat("0", 8))
	t.Setenv("GITHUB_WORKFLOW", "Release candidate")
	t.Setenv("GITHUB_RUN_ID", "42")
	t.Setenv("GITHUB_RUN_ATTEMPT", "2")
	t.Setenv("GITHUB_JOB", "publish")
	t.Setenv("PROVIDER_CONTRACT_SEMVER", "1.4.0")
}

// TestLocalBuildWritesNoReleaseClaim covers the only path that reaches this
// command outside GitHub Actions: a maintainer building a prerelease by hand, or
// checking the GoReleaser configuration with a snapshot. Prerelease tags do not
// trigger release.yml, so that build is the documented path, and it does not
// need provenance at all -- but the archive that packages the manifest still
// requires the file to exist.
//
// It must not invent one. The manifest it writes here says it is a local build
// and claims no tag, run or commit, so nothing can mistake it for the evidence a
// release carries.
func TestLocalBuildWritesNoReleaseClaim(t *testing.T) {
	out, args := provenanceArgs(t)
	for _, name := range []string{"GITHUB_ACTIONS", "GITHUB_REPOSITORY", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT", "GITHUB_REF_NAME", "GITHUB_SHA", "GITHUB_WORKFLOW", "GITHUB_JOB"} {
		t.Setenv(name, "")
	}
	if err := run(args); err != nil {
		t.Fatalf("local build refused to produce a manifest: %v", err)
	}
	payload, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("local build wrote no manifest, so the archive that packages it cannot be built: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("local manifest is not JSON: %v", err)
	}
	if body["schema"] != "gentle-ai.release-provenance/local-build" {
		t.Fatalf("local manifest schema = %v, want the local-build schema: %s", body["schema"], payload)
	}
	if reason, _ := body["reason"].(string); !strings.Contains(reason, "no release provenance") {
		t.Fatalf("local manifest does not say why it claims nothing: %s", payload)
	}
	for _, claim := range []string{"tag", "source_sha", "workflow"} {
		if _, present := body[claim]; present {
			t.Fatalf("a local manifest claims %q, which no local build can know: %s", claim, payload)
		}
	}
}

// TestPartialReleaseIdentityStillRefuses is the case the local path must never
// swallow: a release build that lost one environment variable. Keying the local
// route on GITHUB_ACTIONS alone would turn that hard failure into a published
// archive whose manifest quietly claims nothing, which is the outcome the whole
// guard exists to prevent.
func TestPartialReleaseIdentityNeverDowngrades(t *testing.T) {
	for _, missing := range releaseIdentityEnvironment {
		t.Run("without "+missing, func(t *testing.T) {
			out, args := provenanceArgs(t)
			ciEnvironment(t)
			t.Setenv(missing, "")
			// GITHUB_ACTIONS is the one variable the canonical manifest does not
			// bind, so losing it alone leaves every recorded fact real and the
			// build still produces provenance. Losing any bound variable refuses.
			// Neither outcome may ever be the local manifest: that downgrade is
			// what this gate exists to prevent.
			wantProvenance := missing == "GITHUB_ACTIONS"
			err := run(args)
			payload, readErr := os.ReadFile(out)
			if wantProvenance {
				if err != nil || readErr != nil {
					t.Fatalf("losing %s alone must still record real provenance: %v, %v", missing, err, readErr)
				}
				if !strings.Contains(string(payload), `"schema":"gentle-ai.release-provenance/v1"`) {
					t.Fatalf("a release build missing %s downgraded: %s", missing, payload)
				}
				return
			}
			if err == nil {
				t.Fatalf("a release build missing the bound %s produced a manifest instead of refusing: %s", missing, payload)
			}
			if readErr == nil {
				t.Fatalf("a refused release build missing %s still wrote a manifest: %s", missing, payload)
			}
		})
	}
}

// TestCIBuildStillProducesReleaseProvenance keeps the guard the local path must
// not weaken: inside Actions the manifest is the canonical v1 evidence, and an
// input that cannot be trusted still refuses rather than degrading.
func TestCIBuildStillProducesReleaseProvenance(t *testing.T) {
	out, args := provenanceArgs(t)
	ciEnvironment(t)
	if err := run(args); err != nil {
		t.Fatalf("CI build refused: %v", err)
	}
	payload, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"schema":"gentle-ai.release-provenance/v1"`) {
		t.Fatalf("CI manifest is not release provenance: %s", payload)
	}

	t.Run("an untrusted run attempt still refuses", func(t *testing.T) {
		out, args := provenanceArgs(t)
		ciEnvironment(t)
		t.Setenv("GITHUB_RUN_ATTEMPT", "not-a-number")
		if err := run(args); err == nil {
			t.Fatal("CI build accepted an unusable run attempt")
		}
		if _, err := os.Stat(out); err == nil {
			t.Fatal("a refused CI build still wrote a manifest")
		}
	})
	t.Run("another repository still refuses", func(t *testing.T) {
		out, args := provenanceArgs(t)
		ciEnvironment(t)
		t.Setenv("GITHUB_REPOSITORY", "someone-else/gentle-ai")
		if err := run(args); err == nil {
			t.Fatal("CI build accepted another repository")
		}
		if _, err := os.Stat(out); err == nil {
			t.Fatal("a refused CI build still wrote a manifest")
		}
	})
}
