//go:build linux && amd64 && shell_pnpm_publication

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUserPNPMManifest(t *testing.T) {
	if testing.Short() {
		t.Skip("requires original installer DATA in the qualified private sandbox")
	}
	ctx := context.Background()
	data, err := os.ReadFile("/out/verified-location.json")
	if err != nil {
		t.Fatal(err)
	}
	var location NativeInstallationLocation
	if err := json.Unmarshal(data, &location); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(installStagePath(t))
	stage, root := filepath.Join(parent, "stage"), filepath.Join(parent, "final")
	staged, err := StageNativePNPMInstallation(ctx, "/out", stage, root, location)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, root); err != nil {
		t.Fatal(err)
	}
	manifest := userManifest{Schema: userSchema, Destination: root, Prefix: root + "/pnpm", Agent: root + "/agent", Mode: "separate"}
	manifest.PNPM = &userPNPMManifest{PackageRoot: strings.TrimPrefix(location.PackageRoot, "/out/"), ShellVersion: location.ShellVersion, PeerVersion: location.PiVersion, CodeSeal: staged.CodeSeal}
	manifest.PrefixIdentity, err = userIdentity(manifest.Prefix)
	if err != nil {
		t.Fatal(err)
	}
	manifest.AgentIdentity, err = userIdentity(manifest.Agent)
	if err != nil {
		t.Fatal(err)
	}
	supervisor := []byte("fixture supervisor, never executed\n")
	manifest.SupervisorSHA = fmt.Sprintf("%x", sha256.Sum256(supervisor))
	if err := userToolWrite(root+"/supervisor", supervisor, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pi", "gentle-shell"} {
		if err := userToolWrite(root+"/bin/"+name, []byte(userManifestBinding(root, name, manifest)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(m userManifest) []byte {
		t.Helper()
		data, err := json.Marshal(m)
		if err != nil || len(data) > 4096 {
			t.Fatal("manifest bound", err, len(data))
		}
		if err := os.WriteFile(root+"/installation.json", data, 0600); err != nil {
			t.Fatal(err)
		}
		return data
	}
	write(manifest)
	if got, err := userReadManifest(ctx, root); err != nil || !reflect.DeepEqual(got, manifest) {
		t.Fatal("authentic final PNPM readback", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*userManifest)
	}{
		{"shared", func(m *userManifest) { m.Mode = "shared" }},
		{"npm-node", func(m *userManifest) { m.NodeSHA = privateNativeNodeSHA }},
		{"foreign-prefix", func(m *userManifest) { m.Prefix = "/out/pnpm" }},
		{"foreign-agent", func(m *userManifest) { m.Agent = "/out/agent" }},
		{"gga", func(m *userManifest) { m.GGA = true }},
		{"codegraph", func(m *userManifest) { m.CodeGraph = true }},
		{"main-sdk", func(m *userManifest) { m.MainSDK = &userSnapshotArtifact{} }},
		{"parent-package", func(m *userManifest) { m.PNPM.PackageRoot = "../out/pnpm" }},
		{"malformed-peer", func(m *userManifest) { m.PNPM.PeerVersion = "1.2" }},
		{"wrong-seal", func(m *userManifest) { m.PNPM.CodeSeal = strings.Repeat("0", 64) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, descriptor := manifest, *manifest.PNPM
			m.PNPM = &descriptor
			tc.change(&m)
			before := write(m)
			if _, err := userReadManifest(ctx, root); err == nil {
				t.Fatal("invalid PNPM manifest accepted")
			}
			after, err := os.ReadFile(root + "/installation.json")
			if err != nil || string(after) != string(before) {
				t.Fatal("rejected readback changed manifest", err)
			}
		})
	}
	write(manifest)
	if err := os.WriteFile(manifest.Agent+"/session", []byte("mutable state"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := userReadManifest(ctx, root); err != nil {
		t.Fatal("mutable agent state invalidated code", err)
	}
	if err := os.WriteFile(root+"/pnpm/unexpected.js", []byte("unexpected code"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := userReadManifest(ctx, root); err == nil {
		t.Fatal("unsealed dependency accepted")
	}
}

func TestUserPNPMManifestLegacyEncoding(t *testing.T) {
	data, err := json.Marshal(userManifest{})
	const expected = `{"Schema":"","Destination":"","Prefix":"","Agent":"","Mode":"","SupervisorSHA":"","NodeSHA":"","PrefixIdentity":"","AgentIdentity":""}`
	if err != nil || string(data) != expected {
		t.Fatal("absent PNPM changed legacy manifest bytes", string(data), err)
	}
	if got := userBinding("/target", "pi"); got != "#!/bin/sh\nexec '/target/supervisor' shell launch '/target' \"$@\"\n" {
		t.Fatal("legacy binding changed", got)
	}
}
