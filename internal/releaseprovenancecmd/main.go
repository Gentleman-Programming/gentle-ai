package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"

	"github.com/gentleman-programming/gentle-ai/v2/internal/releaseprovenance"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// releaseIdentityEnvironment lists every variable the canonical manifest binds.
// One of them present is enough to mean a release build is being attempted.
var releaseIdentityEnvironment = []string{
	"GITHUB_ACTIONS", "GITHUB_REPOSITORY", "GITHUB_REF_NAME", "GITHUB_SHA",
	"GITHUB_WORKFLOW", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT", "GITHUB_JOB",
}

func anyReleaseIdentityPresent() bool {
	for _, name := range releaseIdentityEnvironment {
		if os.Getenv(name) != "" {
			return true
		}
	}
	return false
}

func run(args []string) error {
	flags := flag.NewFlagSet("releaseprovenancecmd", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("out", "", "")
	config := flags.String("config", "", "")
	goReleaser := flags.String("goreleaser-version", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *output == "" || *config == "" || *goReleaser != "v2.15.2" {
		return fmt.Errorf("usage: releaseprovenancecmd --out <file> --config <file> --goreleaser-version v2.15.2")
	}
	// A build with no release identity at all has no provenance to record. That
	// is the documented prerelease path -- release.yml never runs for a
	// prerelease tag, so its binaries are built by hand -- and a local checkout
	// knows no run to name. Refusing there took the whole build down with it,
	// including the snapshot a maintainer uses to check this configuration.
	//
	// Absence has to be total. Keying this on GITHUB_ACTIONS alone would let a CI
	// job that lost only that one variable downgrade to a local manifest and
	// still publish, turning a hard failure into an archive that quietly claims
	// nothing. Any release identity present means the strict path decides.
	if !anyReleaseIdentityPresent() {
		return releaseprovenance.WriteLocal(*output, *config)
	}
	if os.Getenv("GITHUB_REPOSITORY") != "Gentleman-Programming/gentle-ai" {
		return fmt.Errorf("release provenance input is invalid")
	}
	runAttempt, err := strconv.Atoi(os.Getenv("GITHUB_RUN_ATTEMPT"))
	if err != nil {
		return fmt.Errorf("release provenance input is invalid")
	}
	return releaseprovenance.Write(*output, *config, releaseprovenance.Input{
		Tag: os.Getenv("GITHUB_REF_NAME"), SourceSHA: os.Getenv("GITHUB_SHA"), WorkflowName: os.Getenv("GITHUB_WORKFLOW"),
		RunID: os.Getenv("GITHUB_RUN_ID"), RunAttempt: runAttempt, Job: os.Getenv("GITHUB_JOB"), GoVersion: runtime.Version(),
		ProviderContractSemver: os.Getenv("PROVIDER_CONTRACT_SEMVER"), GoReleaserVersion: *goReleaser,
	})
}
