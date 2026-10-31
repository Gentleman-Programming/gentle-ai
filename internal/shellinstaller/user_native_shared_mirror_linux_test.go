//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Shared storage changes neither the inventory authority nor response bytes.
func TestUserNativeGlobalMirrorShared(t *testing.T) {
	if testing.Short() {
		t.Skip("requires bounded offline Guest with authenticated Node and fixture TLS")
	}
	if os.Getuid() != 1000 {
		t.Fatal("shared replay requires Guest UID1000")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root, err := os.MkdirTemp("/tmp", "native-shared-mirror-")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := privateDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("owned shared mirror workspace:", root)
	node := filepath.Join(root, "node")
	cmd := exec.CommandContext(ctx, "/usr/bin/cp", "--no-dereference", "/opt/node/bin/node", node)
	if _, err := userOwnedRun(ctx, cmd, cancel); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(node, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := privateNativeFile(ctx, node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		t.Fatal(err)
	}
	type response struct {
		URL  string `json:"url"`
		File string `json:"file"`
		Size int    `json:"size"`
		SHA  string `json:"sha256"`
	}
	for _, mode := range []string{"shared", "corrupt", "body-alias", "index-alias", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			owned := filepath.Join(root, mode)
			primary, secondary := filepath.Join(owned, "primary"), filepath.Join(owned, "secondary")
			for _, dir := range []string{owned, primary, secondary} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path string, data []byte) {
				t.Helper()
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			var rows []response
			for i := range 8 {
				body := []byte(fmt.Sprintf("controlled shared response %d", i))
				row := response{fmt.Sprintf("https://registry.npmjs.org/controlled-shared-%d", i), fmt.Sprintf("body-%d.data", i), len(body), fmt.Sprintf("%x", sha256.Sum256(body))}
				rows = append(rows, row)
				dir := secondary
				if i == 7 {
					dir = primary
				}
				write(filepath.Join(dir, row.File), body)
			}
			index, err := json.Marshal(rows)
			if err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(primary, "requests.json"), index)
			shared := append([]response(nil), rows[:7]...)
			if mode == "conflict" {
				shared[0].SHA = strings.Repeat("0", 64)
			}
			index, err = json.Marshal(shared)
			if err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(secondary, "requests.json"), index)
			if mode == "corrupt" {
				write(filepath.Join(secondary, rows[0].File), []byte(strings.Repeat("x", rows[0].Size)))
			}
			if mode == "body-alias" || mode == "index-alias" {
				name := rows[0].File
				if mode == "index-alias" {
					name = "requests.json"
				}
				path := filepath.Join(secondary, name)
				if err := os.Rename(path, path+".held"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".held", path); err != nil {
					t.Fatal(err)
				}
			}
			stampInputs := func() string {
				t.Helper()
				var result string
				for _, path := range []string{primary, secondary} {
					value, err := userTreeStamp(path)
					if err != nil {
						t.Fatal(err)
					}
					result += value
				}
				return result
			}
			before := stampInputs()
			env := []string{"HOME=" + owned, "TMPDIR=" + owned, "PATH=/usr/bin:/bin", "NODE_USE_SYSTEM_CA=1"}
			if mode == "conflict" || mode == "index-alias" {
				ready := filepath.Join(owned, "ready")
				cmd := exec.CommandContext(ctx, "/usr/bin/python3", "-I", "/source/e2e/shell-native-global-mirror.py", primary, ready, filepath.Join(owned, "report"), secondary)
				cmd.Dir, cmd.Env, cmd.Stdin = owned, env, strings.NewReader("stop\n")
				want := "shared inventory conflict"
				if mode == "index-alias" {
					want = "shared inventory physical"
				}
				out, err := userOwnedRun(ctx, cmd, cancel)
				if err == nil || !strings.Contains(out, want) {
					t.Error("specific shared-inventory refusal missing", out, err)
				}
				if _, err := os.Lstat(ready); !os.IsNotExist(err) {
					t.Error("invalid inventory published ready", err)
				}
				if before != stampInputs() {
					t.Error("invalid inventory mutated inputs")
				}
				return
			}
			stop := userNativeMirror(t, ctx, owned, env, "shell-native-global-mirror.py", primary, secondary)
			defer stop()
			cmd := exec.CommandContext(ctx, node, "--liftoff-only", "--input-type=module", "-e", sharedMirrorReplay, primary, mode)
			cmd.Dir, cmd.Env = owned, env
			out, err := userOwnedRun(ctx, cmd, cancel)
			if err != nil || strings.TrimSpace(out) != "SHARED_REPLAY_OK" {
				t.Error("shared TLS replay differs", out, err)
			}
			stop()
			data, err := userPersistenceReport(owned)
			var report struct {
				Counts       struct{ Requests, Refused, Errors int }
				ServedUnique int
				Upstream     bool
			}
			if err != nil || json.Unmarshal(data, &report) != nil {
				t.Fatal("shared report unavailable", err)
			}
			requests, failures := 8, 0
			if mode != "shared" {
				requests, failures = 0, 1
			}
			if report.Upstream || report.Counts.Requests != requests || report.ServedUnique != requests || report.Counts.Refused != 1 || report.Counts.Errors != failures {
				t.Errorf("shared accounting differs: %+v", report)
			}
			if before != stampInputs() {
				t.Error("shared replay mutated inputs")
			}
		})
	}
	if t.Failed() {
		return
	}
	if err := privateCleanup(root, identity); err != nil {
		t.Fatal(err)
	}
	t.Log("NATIVE_SHARED_MIRROR_READBACK exact_bytes=true unknown_404=true corrupt_alias_502=true conflicting_inventory_refused=true source_preserved=true installed_acceptance=false")
}

const sharedMirrorReplay = `
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
const rows = JSON.parse(readFileSync(process.argv[1] + "/requests.json", "utf8"));
const valid = process.argv[2] === "shared";
for (const row of rows.slice(0, valid ? 8 : 1)) {
  const response = await fetch(row.url);
  if (response.status !== (valid ? 200 : 502)) throw new Error("shared response status " + response.status);
  const bytes = Buffer.from(await response.arrayBuffer());
  if (valid && (bytes.length !== row.size || createHash("sha256").update(bytes).digest("hex") !== row.sha256)) throw new Error("shared bytes changed");
}
const unknown = await fetch("https://registry.npmjs.org/controlled-shared-unknown");
if (unknown.status !== 404) throw new Error("unknown URL escaped inventory");
await unknown.arrayBuffer();
process.stdout.write("SHARED_REPLAY_OK");
`
