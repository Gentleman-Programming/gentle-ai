//go:build !windows

// Package scripts exposes the fixed Linux Shell runtime installation assets.
package scripts

import "embed"

//go:embed bootstrap-gentle-shell-private-node.sh complete-generated-lock-sri.mjs normalize-private-optional-platform-closure.mjs
var runtimeHelpers embed.FS

func ReadPrivateHelper(name string) ([]byte, error) {
	return runtimeHelpers.ReadFile(name)
}
