//go:build linux || darwin

package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// userVerifyRecordedSuppliers keeps supplier authentication on every launch-
// time verify of an installation that carries no staged reviewed list: every
// install recorded before the reviewed-list route, and every Main preview
// install. Their supplier files are authenticated against the embedded list
// entry for the installed gentle-pi version (the recorded version, or the
// frozen lock's version for a manifest predating the field), and tampered
// bytes are refused with the pre-list refusal. An installation with a staged
// list is authenticated by its own provisioner verify against that copy.
func userVerifyRecordedSuppliers(ctx context.Context, root string, manifest userManifest) error {
	if _, err := os.Lstat(filepath.Join(root, "gentle-pi-suppliers.json")); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		version := manifest.SDKVersion
		if version == "" {
			if version, err = userFrozenLockGentlePiVersion(); err != nil {
				return err
			}
		}
		list, _, err := userEmbeddedReviewedList()
		if err != nil {
			return err
		}
		if err := userAuthenticateSupplierFiles(ctx, filepath.Join(manifest.Prefix, "lib/node_modules/gentle-pi"), list, version); err != nil {
			return fmt.Errorf("stock supplier pin: %w", err)
		}
	}
	return nil
}

// userAuthenticateSupplierFiles authenticates the two native supplier files
// of the gentle-pi package at pkg against the reviewed list entry for the
// given version.
func userAuthenticateSupplierFiles(ctx context.Context, pkg string, list userReviewedList, version string) error {
	entry, ok := list.Releases[version]
	if !ok {
		return fmt.Errorf("reviewed supplier list has no entry for gentle-pi %s", version)
	}
	for _, source := range []struct{ name, pin string }{{"scripts/gentle-ai-installer.mjs", entry.Installer}, {"runtime/gentle-ai-binary.mjs", entry.Binary}} {
		if _, err := userSourceFile(ctx, filepath.Join(pkg, source.name), -1, source.pin); err != nil {
			return fmt.Errorf("authenticate native supplier %q: %w", source.name, err)
		}
	}
	return nil
}
