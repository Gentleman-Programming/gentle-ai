//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The reuse pre-flight verifies exactly the components it keeps: a component in
// scope is replaced from the running build, so pinning its recorded bytes would
// refuse the update that is about to replace them.
func TestUserRecordedChecksVerifyOnlyReusedComponents(t *testing.T) {
	if got := userRecordedChecksFor(nil); got != (userRecordedChecks{node: true, sdk: true, codegraph: true}) {
		t.Fatalf("strict read must verify every recorded component: %+v", got)
	}
	reuse := userInstallScope{reused: map[string]bool{UserComponentNode: true}}
	if got := userRecordedChecksFor(&reuse); got != (userRecordedChecks{node: true}) {
		t.Fatalf("SDK and CodeGraph in scope must not verify their recorded bytes: %+v", got)
	}
	reuse = userInstallScope{reused: map[string]bool{UserComponentSDK: true, UserComponentCodeGraph: true, UserComponentContext7: true}}
	if got := userRecordedChecksFor(&reuse); got != (userRecordedChecks{sdk: true, codegraph: true}) {
		t.Fatalf("Node in scope must not verify the recorded Node bytes: %+v", got)
	}
}

// userRecordedInstallFixture writes the smallest recorded installation the reuse
// pre-flight must accept: a manifest, its identities, the bin bindings, the
// recorded supervisor and the provisioner an older build left behind. Node, the
// SDK prefix and the agent are absent because the update in scope replaces them.
func userRecordedInstallFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(userUnixTempRoot(t), "install")
	for _, dir := range []string{root, filepath.Join(root, "prefix"), filepath.Join(root, "agent"), filepath.Join(root, "bin")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	supervisor := []byte("supervisor recorded by another build\n")
	if err := os.WriteFile(filepath.Join(root, "supervisor"), supervisor, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := userManifest{
		Schema: userSchema, Destination: root, Prefix: filepath.Join(root, "prefix"),
		Agent: filepath.Join(root, "agent"), Mode: "separate",
		SupervisorSHA: fmtSHA256(supervisor),
	}
	var err error
	if manifest.PrefixIdentity, err = userIdentity(manifest.Prefix); err != nil {
		t.Fatal(err)
	}
	if manifest.AgentIdentity, err = userSelectedAgentIdentity(manifest.Mode, manifest.Agent); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"gentle-shell", "pi"} {
		if err := os.WriteFile(filepath.Join(root, "bin", name), []byte(userManifestBinding(root, name, manifest)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "installation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "provision.mjs"), []byte("// provisioner recorded by an older build\n"), 0400); err != nil {
		t.Fatal(err)
	}
	return root
}

// The reuse pre-flight accepts a recorded tree whose provisioner another build
// wrote, only while every component it keeps is still verified.
func TestUserReadReusableManifestReplacesScopedComponents(t *testing.T) {
	ctx := context.Background()
	root := userRecordedInstallFixture(t)
	replaced := userInstallScope{reused: map[string]bool{UserComponentCodeGraph: true, UserComponentContext7: true}}
	if _, err := userReadReusableManifest(ctx, root, replaced); err != nil {
		t.Fatalf("reuse refused a scope that replaces Node and the SDK: %v", err)
	}
	kept := userInstallScope{reused: map[string]bool{
		UserComponentNode: true, UserComponentSDK: true, UserComponentCodeGraph: true, UserComponentContext7: true,
	}}
	if _, err := userReadReusableManifest(ctx, root, kept); err == nil {
		t.Fatal("reuse accepted a scope that keeps a component the recorded tree does not hold")
	}
	if _, err := userReadManifest(ctx, root); err == nil {
		t.Fatal("strict read accepted a recorded tree from another build")
	}
	// The supervisor is copied from the running build, never reused, so the
	// reuse pre-flight does not pin the one the recorded tree left behind.
	if err := os.WriteFile(filepath.Join(root, "supervisor"), []byte("supervisor recorded by another build\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := userReadReusableManifest(ctx, root, replaced); err != nil {
		t.Fatalf("reuse pinned the supervisor it replaces: %v", err)
	}
	if _, err := userReadManifest(ctx, root); err == nil {
		t.Fatal("strict read accepted a foreign recorded supervisor")
	}
}

// A full reconfigure rebuilds the agent from a fresh stage. The recorded agent
// of a separate installation is fixed by the destination, so a tree whose
// strict verification refused still carries its operator state, and an absent
// agent is never synthesized.
func TestUserRecordedAgentFollowsTheDestination(t *testing.T) {
	root := userUnixTempRoot(t)
	destination := filepath.Join(root, "install")
	agent := filepath.Join(destination, "agent")
	if err := os.MkdirAll(agent, 0700); err != nil {
		t.Fatal(err)
	}
	if got, ok := userRecordedAgent(destination, nil); !ok || got != agent {
		t.Fatalf("recorded agent = %q %v, want %q", got, ok, agent)
	}
	if _, ok := userRecordedAgent(filepath.Join(root, "absent"), nil); ok {
		t.Fatal("an absent agent was synthesized")
	}
	verified := userManifest{Mode: "separate", Agent: agent}
	if got, ok := userRecordedAgent(destination, &verified); !ok || got != agent {
		t.Fatalf("a verified manifest was ignored: %q %v", got, ok)
	}
	foreign := userManifest{Mode: "separate", Agent: filepath.Join(root, "elsewhere")}
	if _, ok := userRecordedAgent(destination, &foreign); ok {
		t.Fatal("an absent recorded agent was synthesized")
	}
}
