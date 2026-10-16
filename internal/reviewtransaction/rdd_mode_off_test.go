//go:build unix

package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// unvalidatableGitCommonDir makes the Git common directory fail RAR ownership
// validation the same way a share owned by another principal does: the
// directory exists, Git resolves it, and validateRARRepositoryParent refuses
// it. It is the POSIX stand-in for the #4670 SMB/UNC common directory.
func unvalidatableGitCommonDir(t *testing.T) {
	t.Helper()
	previous := rarCurrentEUID
	rarCurrentEUID = func() int { return os.Geteuid() + 1 }
	t.Cleanup(func() { rarCurrentEUID = previous })
}

// #4670: with the global mode off nothing can turn receipt-driven development
// back on -- the clone-local override may only disable -- so an override the
// common directory refuses to yield must not fail resolution.
func TestResolveRDDModeGlobalOffResolvesWhenTheOverrideCannotBeRead(t *testing.T) {
	repo := initSnapshotRepo(t)
	ctx := context.Background()
	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{}); err != nil {
		t.Fatalf("disable clone-local mode: %v", err)
	}
	unvalidatableGitCommonDir(t)

	status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{Value: "off"})
	if err != nil {
		t.Fatalf("global off resolution failed on an unvalidatable common dir: %v", err)
	}
	if status.Effective != RDDModeOff || status.Source != RDDModeSourceGlobal || status.Global != RDDModeOff {
		t.Fatalf("global off status = %#v, want off decided by global", status)
	}
	if status.CloneLocal != RDDModeUnset || status.Revision != "" {
		t.Fatalf("an unread override leaked into the status: %#v", status)
	}
}

// The degrade belongs to the global-off case alone. With the global mode on or
// unset the override can still decide, so an existing override behind an
// unvalidatable common directory keeps failing closed exactly as #1918 asks.
func TestResolveRDDModeExistingOverrideStillFailsClosedUnlessGlobalOff(t *testing.T) {
	for _, global := range []string{"", "on"} {
		t.Run("global="+global, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			ctx := context.Background()
			if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{}); err != nil {
				t.Fatalf("disable clone-local mode: %v", err)
			}
			unvalidatableGitCommonDir(t)

			status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{Value: global})
			if !errors.Is(err, errUnsafeRARAuthorityPath) {
				t.Fatalf("err = %v, want the unsafe-path refusal", err)
			}
			if status.Enabled() {
				t.Fatalf("unvalidatable override did not fail closed: %#v", status)
			}
		})
	}
}

// A clone that never recorded an override has nothing under its common
// directory to validate, so reading "no override" must not depend on the
// directory passing validation.
func TestResolveRDDModeWithoutAnOverrideNeverValidatesTheCommonDir(t *testing.T) {
	tests := []struct {
		global     string
		wantEff    RDDMode
		wantSource RDDModeSource
	}{
		{"", RDDModeOn, RDDModeSourceDefault},
		{"on", RDDModeOn, RDDModeSourceGlobal},
		{"off", RDDModeOff, RDDModeSourceGlobal},
	}
	for _, tt := range tests {
		t.Run("global="+tt.global, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			unvalidatableGitCommonDir(t)

			status, err := ResolveRDDMode(context.Background(), repo, RDDGlobalMode{Value: tt.global})
			if err != nil {
				t.Fatalf("resolution without an override validated the common dir: %v", err)
			}
			if status.Effective != tt.wantEff || status.Source != tt.wantSource || status.Revision != "" {
				t.Fatalf("status = %#v, want %s decided by %s", status, tt.wantEff, tt.wantSource)
			}
			if path, err := CloneLocalRDDModeRecordPath(context.Background(), repo); err != nil || path != "" {
				t.Fatalf("record path = %q, %v; want no record and no error", path, err)
			}
		})
	}
}

// Writes never take the read-path shortcut: recording an override still
// validates the common directory, whatever the global mode says.
func TestSetCloneLocalRDDModeStillValidatesAnUnvalidatableCommonDir(t *testing.T) {
	for _, global := range []string{"", "off"} {
		t.Run("global="+global, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			unvalidatableGitCommonDir(t)

			_, err := SetCloneLocalRDDMode(context.Background(), repo, RDDModeOff, "", RDDGlobalMode{Value: global})
			if !errors.Is(err, errUnsafeRARAuthorityPath) {
				t.Fatalf("err = %v, want the unsafe-path refusal", err)
			}
			switchRoot := filepath.Join(repo, ".git", "gentle-ai", rddModeSwitchDirectory, rarAuthorityDirectory)
			if _, statErr := os.Lstat(switchRoot); !os.IsNotExist(statErr) {
				t.Fatalf("a refused write created %s: %v", switchRoot, statErr)
			}
		})
	}
}

// An override that predates the switch relocation is still an override: its
// presence alone sends the read through full validation.
func TestResolveRDDModeLegacyOverrideStillValidatesTheCommonDir(t *testing.T) {
	repo := initSnapshotRepo(t)
	legacy := filepath.Join(repo, ".git", "gentle-ai", rddModeLegacySwitchDirectory, rarAuthorityDirectory, rarAuthorityVersion, rddModeDirectory)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	unvalidatableGitCommonDir(t)

	if _, err := ResolveRDDMode(context.Background(), repo, RDDGlobalMode{}); !errors.Is(err, errUnsafeRARAuthorityPath) {
		t.Fatalf("err = %v, want the unsafe-path refusal", err)
	}
}

// The clone-scope compare-and-set token is read with the strict resolver: a
// global OFF must not hide the head a disable --scope clone has to replace.
func TestSetCloneLocalRDDModeKeepsItsRevisionWhileGlobalOff(t *testing.T) {
	repo := initSnapshotRepo(t)
	ctx := context.Background()
	first, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{})
	if err != nil {
		t.Fatalf("disable clone-local mode: %v", err)
	}
	off := RDDGlobalMode{Value: "off"}
	status, err := ResolveRDDMode(ctx, repo, off)
	if err != nil || status.Source != RDDModeSourceCloneLocal || status.Revision != first.Revision {
		t.Fatalf("readable override under global off = %#v, %v; want clone_local at %q", status, err, first.Revision)
	}
	again, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, first.Revision, off)
	if err != nil || again.Revision != first.Revision {
		t.Fatalf("idempotent disable under global off = %#v, %v", again, err)
	}
}
