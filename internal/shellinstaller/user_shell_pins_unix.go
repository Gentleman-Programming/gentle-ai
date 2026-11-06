//go:build linux || darwin

package shellinstaller

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

// Component version pins. Every installer build pins exactly one version of
// each shell component; the manifest records the pin that produced an
// installation, so a later build can compare pins instead of bytes and only
// refresh the components whose pin moved. The canonical component names live
// in user_components.go: they are shared with the Windows report path.

// userCodeGraphPinSHA digests the pinned CodeGraph source set of this build.
// The staged library is derived from these authenticated archives alone, so
// the digest identifies the resulting CodeGraph version without hashing the
// unpacked tree.
func userCodeGraphPinSHA() (string, error) {
	data, err := json.Marshal(userCodeGraphSources)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// userReadUserAssets exposes the embedded user assets for pin comparison.
func userReadUserAssets() (map[string][]byte, error) {
	return assets.ReadUserAssets()
}

// userSDKLockSHA digests the embedded modern package lock: the exact stable
// dependency set the global provisioning installs. A new lock in a later
// build means the stable SDK component changed.
func userSDKLockSHA() (string, error) {
	files, err := userReadUserAssets()
	if err != nil {
		return "", err
	}
	lock, ok := files["user-locks/modern/package-lock.json"]
	if !ok {
		return "", fmt.Errorf("embedded modern package lock is absent")
	}
	return fmt.Sprintf("%x", sha256.Sum256(lock)), nil
}

// userFrozenLockGentlePiVersion parses the exact gentle-pi version the
// embedded modern lock installs. The Stable SDK is provisioned from the
// selected reviewed list's newest release (T4), so this is no longer the
// update target; it remains the installed version of record for component
// updates that reuse the SDK of a manifest predating the recorded version.
func userFrozenLockGentlePiVersion() (string, error) {
	files, err := userReadUserAssets()
	if err != nil {
		return "", err
	}
	lock, ok := files["user-locks/modern/package-lock.json"]
	if !ok {
		return "", fmt.Errorf("embedded modern package lock is absent")
	}
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
		return "", fmt.Errorf("embedded modern package lock pins no exact gentle-pi version")
	}
	return version, nil
}

// userToolsPinSHA digests the pinned fd/rg sources of this build. The bound
// agent binaries are part of the SDK component: a tool pin change refreshes
// the SDK so the reused agent can never fail the bind verification.
func userToolsPinSHA() (string, error) {
	data, err := json.Marshal(userToolSources)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// userContext7EntrySHA digests the canonical Context7 server entry this build
// stages into agent mcp.json for the given choice. Context7 has no binary
// version; the deterministic entry is its version.
func userContext7EntrySHA(choice string) (string, error) {
	fields := map[string]json.RawMessage{}
	if err := userContext7Update(choice)(fields); err != nil {
		return "", err
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(fields["mcpServers"], &servers); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(servers["context7"])), nil
}
