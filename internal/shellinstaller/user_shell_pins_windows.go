//go:build windows

package shellinstaller

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

// Windows component version pins. The recorded manifest pins let the update
// check compare the installed composition against this build and scope the
// update to the components whose pin moved. Legacy manifests without the
// pins report those components as changed once, so the next update
// refreshes them and records the pins.

// userWindowsSDKLockSHA digests the embedded Windows provision helper: the
// exact dependency seed and provisioning logic the stock runtime installs.
func userWindowsSDKLockSHA() (string, error) {
	helper, err := assets.ReadWindowsUserHelper()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(helper)), nil
}

// userWindowsLockGentlePiVersion parses the exact gentle-pi version a
// generated lock resolved: the installed stable SDK version recorded in the
// manifest for the update check.
func userWindowsLockGentlePiVersion(lock []byte) (string, error) {
	var record struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(lock, &record); err != nil {
		return "", err
	}
	version := record.Packages["node_modules/gentle-pi"].Version
	if !userReviewedListSemver(version) {
		return "", errors.New("generated lock pins no exact gentle-pi version")
	}
	return version, nil
}

// userWindowsToolsPinSHA digests the pinned fd/rg artifacts of this build.
// The bound agent binaries are part of the SDK component: a tool pin change
// refreshes the SDK so a reused agent never fails the verification.
func userWindowsToolsPinSHA() (string, error) {
	data, err := json.Marshal(userWindowsArtifacts[2:4])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// userWindowsGoPinSHA digests the pinned Go artifact of this build. The Go
// runtime is acquired with the Node runtime component.
func userWindowsGoPinSHA() (string, error) {
	data, err := json.Marshal(userWindowsArtifacts[1])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// userWindowsCodeGraphPinSHA digests the pinned CodeGraph source set of this
// build; the staged library derives from these authenticated archives alone.
func userWindowsCodeGraphPinSHA() (string, error) {
	data, err := json.Marshal(userWindowsCodeGraphSources)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// userWindowsContext7EntrySHA digests the canonical Context7 server entry
// this build stages into agent mcp.json for the given choice. Context7 has
// no binary version; the deterministic entry is its version.
func userWindowsContext7EntrySHA(choice string) (string, error) {
	entry := map[string]any{
		"mcpServers": map[string]any{
			"context7": map[string]any{
				"url":      "https://mcp.context7.com/mcp",
				"enabled":  choice == "on",
				"exposure": "codemode",
			},
		},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}
