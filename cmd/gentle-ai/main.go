package main

import (
	"fmt"
	"io"
	"os"

	"github.com/gentleman-programming/gentle-ai/v3/internal/app"
)

// version is set by GoReleaser via ldflags at build time.
var version = "dev"

const deprecationNotice = "gentle-ai CLI está deprecado; usa 'axiom'"

func run(args []string, stdout, stderr io.Writer) error {
	fmt.Fprintln(stderr, deprecationNotice)
	app.Version = app.ResolveVersion(version)
	return app.RunArgs(args, stdout)
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
