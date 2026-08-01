package main

import (
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/releasepolicy"
)

func main() {
	root, err := os.Getwd()
	if err == nil {
		err = releasepolicy.Validate(root, os.Getenv("RELEASE_POLICY_SNAPSHOT_MARKER"), os.Getenv("RELEASE_POLICY_SNAPSHOT_RUN_ID"))
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "release distribution policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release distribution policy: exact current Linux/macOS snapshot and sole Homebrew publisher verified")
}
