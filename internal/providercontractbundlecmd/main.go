package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/providercontractbundle"
)

const contractSemverFile = "contracts/review-provider-contract/CONTRACT_SEMVER"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: providercontractbundlecmd <generate|verify>")
	}
	switch args[0] {
	case "generate":
		flags := flag.NewFlagSet("generate", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		output := flags.String("out", "", "")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *output == "" {
			return fmt.Errorf("usage: providercontractbundlecmd generate --out <directory>")
		}
		semver, err := providercontractbundle.ReadContractSemver(contractSemverFile)
		if err != nil {
			return err
		}
		if configured := os.Getenv("PROVIDER_CONTRACT_SEMVER"); configured != "" && configured != semver {
			return fmt.Errorf("PROVIDER_CONTRACT_SEMVER must equal %s", contractSemverFile)
		}
		if err := providercontractbundle.Generate(*output, semver); err != nil {
			return err
		}
		return providercontractbundle.VerifyStaging(*output)
	case "verify":
		flags := flag.NewFlagSet("verify", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		archive := flags.String("archive", "", "")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *archive == "" {
			return fmt.Errorf("usage: providercontractbundlecmd verify --archive <path>")
		}
		return providercontractbundle.VerifyArchive(*archive)
	default:
		return fmt.Errorf("usage: providercontractbundlecmd <generate|verify>")
	}
}
