package main

import (
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/app"
	"github.com/gentleman-programming/gentle-ai/v2/internal/cli"
)

// version is set by GoReleaser via ldflags at build time.
var version = "dev"

// commit is the source commit the running binary was built from. Stamped by
// GoReleaser (via the same ldflags mechanism as version) and recorded into
// the gentle-ai.managed-assets/v1 manifest as the producer's commit identity
// so doctor can correlate a binary against its bundle even when a release
// tag was not yet cut. Default "unknown" preserves the
// manifest as authoritative for whatever identity it does carry.
var commit = "unknown"

func main() {
	app.Version = app.ResolveVersion(version)
	cli.ProducerCommit = commit

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
