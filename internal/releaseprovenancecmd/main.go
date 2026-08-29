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

func run(args []string) error {
	flags := flag.NewFlagSet("releaseprovenancecmd", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("out", "", "")
	config := flags.String("config", "", "")
	goReleaser := flags.String("goreleaser-version", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *output == "" || *config == "" || *goReleaser != "v2.15.2" {
		return fmt.Errorf("usage: releaseprovenancecmd --out <file> --config <file> --goreleaser-version v2.15.2")
	}
	// Outside GitHub Actions there is no provenance to record. That is the
	// documented prerelease path -- release.yml never runs for a prerelease tag,
	// so its binaries are built by hand -- and a local checkout knows no run to
	// name. Refusing here took the whole build down with it, including the
	// snapshot a maintainer uses to check this configuration.
	if os.Getenv("GITHUB_ACTIONS") != "true" {
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
