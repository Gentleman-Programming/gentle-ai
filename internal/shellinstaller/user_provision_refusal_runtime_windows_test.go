//go:build windows

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

// The provision helper's identity loop must refuse an acquired package.json
// that diverges from its lock record, and the refusal must name the
// lock-relative path plus both identities (see #5996). This proof runs the
// embedded helper itself — not a copy on disk — against a minimal verify
// fixture whose acquired tree carries one mutated identity, then against the
// same fixture unmutated, which must not produce the refusal.
func TestWindowsUserHelperAcquiredIdentityRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("external Node proof is not a short test")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable for the provision-helper runtime proof")
	}
	helper, err := assets.ReadWindowsUserHelper()
	if err != nil {
		t.Fatalf("read embedded provision helper: %v", err)
	}
	const wantRefusal = "Windows provision refused: acquired identity: node_modules/gentle-pi expected gentle-pi@1.2.3, acquired gentle-pi@9.9.9"
	for name, mutated := range map[string]bool{"mutated": true, "pristine": false} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "shell")
			if err := os.MkdirAll(filepath.Join(root, "source"), 0o755); err != nil {
				t.Fatal(err)
			}
			pins := [][2]string{
				{"gentle-pi", "1.2.3"},
				{"@earendil-works/pi-coding-agent", "1.0.0"},
				{"@earendil-works/pi-tui", "1.0.0"},
				{"@heyhuynhgiabuu/pi-pretty", "1.0.0"},
				{"typebox", "1.0.0"},
			}
			packages := map[string]any{
				"": map[string]any{
					"dependencies": func() map[string]string {
						deps := map[string]string{}
						for _, pin := range pins {
							deps[pin[0]] = pin[1]
						}
						return deps
					}(),
				},
			}
			for _, pin := range pins {
				base := pin[0] + pin[1]
				packages["node_modules/"+pin[0]] = map[string]any{
					"version":   pin[1],
					"resolved":  "https://registry.npmjs.org/" + pin[0] + "/-/" + pin[0][strings.LastIndex(pin[0], "/")+1:] + "-" + pin[1] + ".tgz",
					"integrity": "sha512-" + base64.StdEncoding.EncodeToString(func() []byte { sum := sha512.Sum512([]byte(base)); return sum[:] }()),
				}
				identity := map[string]any{"name": pin[0], "version": pin[1]}
				if pin[0] == "gentle-pi" && mutated {
					identity["version"] = "9.9.9"
				}
				dir := filepath.Join(root, "prefix", "node_modules", filepath.FromSlash(pin[0]))
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(mustJSON(t, identity)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "source", "package-lock.json"), []byte(mustJSON(t, map[string]any{"lockfileVersion": 3, "packages": packages})), 0o644); err != nil {
				t.Fatal(err)
			}
			confirmation := hex.EncodeToString(func() []byte { sum := sha256.Sum256([]byte(root)); return sum[:] }())
			if err := os.WriteFile(filepath.Join(root, "selection.json"), []byte(mustJSON(t, map[string]any{
				"Channel": "stable", "Confirmation": confirmation, "Destination": root, "Mode": "separate", "SharedAgent": "", "SharedPrefix": "",
			})), 0o644); err != nil {
				t.Fatal(err)
			}
			helperPath := filepath.Join(root, "provision.mjs")
			if err := os.WriteFile(helperPath, helper, 0o644); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, node, helperPath, root, "verify")
			cmd.Env = append(os.Environ(), "GENTLE_PROOF_HOME="+os.TempDir())
			output, runErr := cmd.CombinedOutput()
			text := string(output)
			if mutated {
				if runErr == nil {
					t.Fatalf("expected the mutated identity to be refused, helper exited 0")
				}
				if !strings.Contains(text, wantRefusal) {
					t.Fatalf("refusal must name the lock-relative path and both identities\nwant: %s\ngot: %s", wantRefusal, text)
				}
			} else if strings.Contains(text, "acquired identity") {
				t.Fatalf("pristine fixture must not produce the acquired-identity refusal: %s", text)
			}
		})
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
