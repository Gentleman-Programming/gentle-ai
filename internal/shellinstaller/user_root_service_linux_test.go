//go:build linux

package shellinstaller

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRootServiceArgsMirrorUserSandbox(t *testing.T) {
	user := userServiceArgs("gentle-shell-abcd1234abcd1234", false, "/bin/self", "/cwd", []string{"internal-check"}, []string{"HOME=/root"})
	root := rootServiceArgs("gentle-shell-abcd1234abcd1234", false, "/bin/self", "/cwd", []string{"internal-check"}, []string{"HOME=/root"})
	if user[0] != "--user" {
		t.Fatalf("user args must stay user-scoped, first = %q", user[0])
	}
	for _, arg := range root {
		if arg == "--user" {
			t.Fatal("system args must not carry the --user flag")
		}
	}
	joined := strings.Join(root, " ")
	for _, want := range []string{
		"MemoryMax=3221225472", "MemorySwapMax=0", "TasksMax=64", "NoNewPrivileges=yes",
		"/usr/bin/setpriv", "--no-new-privs", "--inh-caps=-all", "--ambient-caps=-all",
		"/bin/self", "shell", "internal-check",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("system args miss %q: %s", want, joined)
		}
	}
}

func TestRootServiceEnvironmentSealsAndCarriesBinding(t *testing.T) {
	env, err := rootServiceEnvironment()
	if err != nil {
		t.Fatalf("rootServiceEnvironment: %v", err)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, AcceptRootRiskEnvironment()) {
		t.Fatalf("the system unit environment must carry the acceptance binding: %s", joined)
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, "DBUS_SESSION_BUS_ADDRESS=") || strings.HasPrefix(entry, "XDG_RUNTIME_DIR=") {
			t.Fatalf("the system path must not require user-manager sockets: %s", entry)
		}
	}
}

func TestUserManifestRootRiskRoundTrip(t *testing.T) {
	published := userManifest{Schema: userSchema, Destination: "/t", Mode: "separate", RootRisk: true}
	data, err := json.Marshal(published)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var read userManifest
	if err := json.Unmarshal(data, &read); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !read.RootRisk {
		t.Fatal("rootRisk must survive the manifest round trip")
	}
	// Older manifests omit the field entirely and stay readable.
	var legacy userManifest
	if err := json.Unmarshal([]byte(`{"schema":"gentle-shell-user-install/v1","destination":"/t","mode":"separate"}`), &legacy); err != nil {
		t.Fatalf("legacy unmarshal: %v", err)
	}
	if legacy.RootRisk {
		t.Fatal("legacy manifests must read as non-root installations")
	}
}
