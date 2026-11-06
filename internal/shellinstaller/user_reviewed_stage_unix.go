//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// userStageSDKReviewedList stages the selected reviewed supplier list exactly
// when the request takes the generated reviewed-list route: only a Stable
// request does (S23/M4). A Main preview request stages nothing and keeps the
// frozen modern lock's stock base, exactly as before the route existed.
func userStageSDKReviewedList(ctx context.Context, root string, req UserInstallRequest) (userReviewedList, error) {
	if userMainPreviewRequest(req) {
		return userReviewedList{}, nil
	}
	return userStageReviewedList(ctx, root)
}

// userStageReviewedList resolves the selected reviewed supplier list (the live
// repository copy with the embedded fallback) and writes it into the install
// stage, so the provisioner and the Go supplier check use exactly the list
// detection used — the same stage contract the Windows worker follows.
func userStageReviewedList(ctx context.Context, root string) (userReviewedList, error) {
	list, listBytes, err := userSelectedReviewedListFunc(ctx)
	if err != nil {
		return userReviewedList{}, err
	}
	if err := userToolWrite(filepath.Join(root, "gentle-pi-suppliers.json"), listBytes, 0400); err != nil {
		return userReviewedList{}, err
	}
	return list, nil
}

// userStageGentlePiVersion parses the exact gentle-pi version the staged
// generated lock pinned: the version this install actually provisioned, which
// the manifest records for the stable update check. Unlike the frozen lock,
// the generated lock changes as the reviewed list grows, so the version is
// read from the stage instead of the build.
func userStageGentlePiVersion(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "runtime", "project", "package-lock.json"))
	if err != nil {
		return "", err
	}
	if len(data) > 8<<20 {
		return "", fmt.Errorf("generated package lock exceeds the read bound")
	}
	var record struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return "", err
	}
	version := record.Packages["node_modules/gentle-pi"].Version
	if !userReviewedListSemver(version) {
		return "", fmt.Errorf("generated package lock pins no exact gentle-pi version")
	}
	return version, nil
}

// userRecordedSDKVersion resolves the sdk_version a stage records: a rebuilt
// SDK stage reads the generated lock it just provisioned; a reused-SDK stage
// never reads a lock, because it may not carry one (S23/M3) — the recorded
// manifest's version is the version of record, or the frozen lock's version
// for a manifest predating the recorded field.
func userRecordedSDKVersion(root string, reused bool, recorded string) (string, error) {
	if reused {
		if recorded != "" {
			return recorded, nil
		}
		return userFrozenLockGentlePiVersion()
	}
	return userStageGentlePiVersion(root)
}

// userVerifyRecordedSuppliers keeps supplier authentication on every launch-
// time verify of an installation that carries no staged reviewed list
// (S23/M1): installs recorded before the reviewed-list route have none, and a
// Main preview install never stages one (M4). Their supplier files are
// authenticated against the embedded list entry for the installed gentle-pi
// version — the recorded version, or the frozen lock's version for a manifest
// predating the field — and tampered bytes are refused with the pre-list
// refusal. An installation with a staged list was already authenticated by
// its own provisioner verify against that staged copy: verify trusts the
// installation's own list, the same-user boundary the trust-boundary doc
// states (S23/L3).
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
// given version, with the shared refusal messages.
func userAuthenticateSupplierFiles(ctx context.Context, pkg string, list userReviewedList, version string) error {
	installer, resolver, err := userStableSupplierPins(list, version)
	if err != nil {
		return err
	}
	for _, source := range []struct{ name, pin string }{{"scripts/gentle-ai-installer.mjs", installer}, {"runtime/gentle-ai-binary.mjs", resolver}} {
		if _, err := userSourceFile(ctx, filepath.Join(pkg, source.name), -1, source.pin); err != nil {
			return fmt.Errorf("authenticate native supplier %q: %w", source.name, err)
		}
	}
	return nil
}

// userReusedSupplierPins resolves the supplier digests for a reused SDK: the
// selected list's entry for its recorded version, or the embedded entry when
// the selected list has none (S23/L1). A reused SDK runs no supplier, so a
// version absent from both lists requires no entry and nothing is
// authenticated; ok=false reports that skip.
func userReusedSupplierPins(ctx context.Context, version string) (installer, resolver string, ok bool, err error) {
	selected, _, err := userSelectedReviewedListFunc(ctx)
	if err != nil {
		return "", "", false, err
	}
	if _, found := selected.Releases[version]; found {
		installer, resolver, err = userStableSupplierPins(selected, version)
		return installer, resolver, true, err
	}
	embedded, _, err := userEmbeddedReviewedList()
	if err != nil {
		return "", "", false, err
	}
	if _, found := embedded.Releases[version]; found {
		installer, resolver, err = userStableSupplierPins(embedded, version)
		return installer, resolver, true, err
	}
	return "", "", false, nil
}
