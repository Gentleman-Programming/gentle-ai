//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Issue #5918: the pinned preimage identity includes st_dev, which macOS
// reassigns per mount, so a reboot must not turn into a refusal. These tests
// pin the guard at each wired unix site: an identity differing only in the
// device field is accepted, while any inode, uid or mode change still refuses
// with the preimage kind the strict comparison used to produce.

func identityShiftDev(identity string) string {
	parts := strings.SplitN(identity, ":", 4)
	if len(parts) != 4 {
		return identity
	}
	var dev uint64
	fmt.Sscanf(parts[0], "%d", &dev)
	return fmt.Sprintf("%d:%s:%s:%s", dev+1, parts[1], parts[2], parts[3])
}

func identityShiftIno(identity string) string {
	parts := strings.SplitN(identity, ":", 4)
	if len(parts) != 4 {
		return identity
	}
	var ino uint64
	fmt.Sscanf(parts[1], "%d", &ino)
	return fmt.Sprintf("%s:%d:%s:%s", parts[0], ino+1, parts[2], parts[3])
}

func identityShiftMode(identity string) string {
	parts := strings.SplitN(identity, ":", 4)
	if len(parts) != 4 {
		return identity
	}
	mode := parts[3]
	if strings.HasPrefix(mode, "d") {
		mode = "drwxr-xr-x"
	} else {
		mode = mode + "x"
	}
	return fmt.Sprintf("%s:%s:%s:%s", parts[0], parts[1], parts[2], mode)
}

func rewriteRecordedManifest(t *testing.T, root string, mutate func(*userManifest)) {
	t.Helper()
	data, err := os.ReadFile(root + "/installation.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest userManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	mutate(&manifest)
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/installation.json", data, 0600); err != nil {
		t.Fatal(err)
	}
}

func reusableContextScope() userInstallScope {
	return userInstallScope{reused: map[string]bool{UserComponentCodeGraph: true, UserComponentContext7: true}}
}

func TestUserReadReusableManifestIdentityGuardSites(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		field     string
		mutate    func(string) string
		wantPreim bool
	}{
		{"prefix dev-only", "prefix", identityShiftDev, false},
		{"agent dev-only", "agent", identityShiftDev, false},
		{"prefix inode", "prefix", identityShiftIno, true},
		{"prefix mode", "prefix", identityShiftMode, true},
		{"agent inode", "agent", identityShiftIno, true},
		{"agent truncated", "agent", func(s string) string {
			parts := strings.SplitN(s, ":", 4)
			if len(parts) == 4 {
				return strings.Join(parts[1:], ":")
			}
			return s
		}, true},
		{"agent replaced", "agent", func(string) string { return "changed" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := userRecordedInstallFixture(t)
			rewriteRecordedManifest(t, root, func(m *userManifest) {
				if tc.field == "prefix" {
					m.PrefixIdentity = tc.mutate(m.PrefixIdentity)
				} else {
					m.AgentIdentity = tc.mutate(m.AgentIdentity)
				}
			})
			_, err := userReadReusableManifest(ctx, root, reusableContextScope())
			if tc.wantPreim {
				var failure *PrivateRuntimeError
				if !errors.As(err, &failure) || failure.Kind != "preimage" {
					t.Fatalf("recorded %s tampering must refuse with preimage, got %v", tc.field, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("a reboot-shifted device number must not refuse the %s identity: %v", tc.field, err)
			}
		})
	}
}

func TestUserPublishManifestIdentityGuardSites(t *testing.T) {
	parent, req, stage, manifest, _, _ := agentTargetCommitFixture(t)
	_ = parent
	ctx, committed := userWithPublicationCommit(context.Background())
	// Control: a tampered portable field is refused by the identity guard
	// itself (preimage with no cause), before any publication.
	tampered := manifest
	tampered.PrefixIdentity = identityShiftIno(manifest.PrefixIdentity)
	_, publishErr := userPublishManifest(ctx, stage, stage+"/prefix", req.SharedAgent, tampered)
	var failure *PrivateRuntimeError
	if !errors.As(publishErr, &failure) || failure.Kind != "preimage" || failure.Unwrap() != nil || *committed {
		t.Fatalf("tampered portable prefix identity = %v committed=%v", publishErr, *committed)
	}
	// A reboot-shifted device number must pass the identity guard: the run
	// may still fail later in the sandboxed fixture (privateDestination, the
	// rename), but never with the cause-less preimage the identity guard
	// produces on an identity mismatch.
	parent, req, stage, manifest, _, _ = agentTargetCommitFixture(t)
	shifted := manifest
	shifted.PrefixIdentity = identityShiftDev(manifest.PrefixIdentity)
	_, publishErr = userPublishManifest(ctx, stage, stage+"/prefix", req.SharedAgent, shifted)
	if errors.As(publishErr, &failure) && failure.Kind == "preimage" && failure.Unwrap() == nil {
		t.Fatalf("reboot-shifted device number refused at the publish identity guard: %v", publishErr)
	}
}
