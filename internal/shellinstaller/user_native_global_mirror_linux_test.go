//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A transport qualification, not a replacement for original PNPM resolution.
func TestUserNativeGlobalMirror(t *testing.T) {
	if testing.Short() {
		t.Skip("requires bounded offline Guest and authenticated global fixture")
	}
	if os.Getuid() != 1000 {
		t.Fatal("global replay requires Guest UID1000")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	workspace, err := os.MkdirTemp("/tmp", "native-global-mirror-")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := privateDirectory(workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("owned global mirror workspace:", workspace)
	node := filepath.Join(workspace, "node")
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
	project := filepath.Join(workspace, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "preserved"), []byte("private project"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := userTreeStamp(project)
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + workspace, "TMPDIR=" + workspace, "PATH=/usr/bin:/bin", "NODE_USE_SYSTEM_CA=1"}
	for _, mode := range []string{"original-responses", "body-alias", "owner-stop"} {
		t.Run(mode, func(t *testing.T) {
			owned := filepath.Join(workspace, mode)
			if err := os.Mkdir(owned, 0700); err != nil {
				t.Fatal(err)
			}
			fixture := "/global-fixture"
			if mode == "body-alias" {
				fixture = filepath.Join(owned, "fixture")
				if err := os.Mkdir(fixture, 0700); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile("/global-fixture/requests.json")
				var rows []struct{ File string }
				if err != nil || len(data) > 256*1024 || json.Unmarshal(data, &rows) != nil || len(rows) < 40 {
					t.Fatal("global inventory unavailable", err)
				}
				if err := os.WriteFile(filepath.Join(fixture, "requests.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join("/global-fixture", rows[0].File), filepath.Join(fixture, rows[0].File)); err != nil {
					t.Fatal(err)
				}
			}
			stop := userNativeMirror(t, ctx, owned, env, "shell-native-global-mirror.py", fixture)
			defer stop()
			cmd := exec.CommandContext(ctx, node, "--liftoff-only", "--input-type=module", "-e", globalMirrorReplay, mode)
			cmd.Dir, cmd.Env = project, env
			out, err := userOwnedRun(ctx, cmd, cancel)
			if err != nil || strings.TrimSpace(out) != "GLOBAL_REPLAY_OK" {
				t.Error("genuine Node HTTPS replay failed", out, err)
			}
			if mode == "owner-stop" {
				for range 4 {
					connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", "registry.npmjs.org:443", &tls.Config{MinVersion: tls.VersionTLS12})
					if err != nil {
						t.Fatal(err)
					}
					defer connection.Close()
					if _, err := connection.Write([]byte("GET / HTTP/1.1\r\nHost: registry.npmjs.org\r\nX-Slow: ")); err != nil {
						t.Fatal(err)
					}
				}
			}
			stop()
			data, err := userPersistenceReport(owned)
			var report struct {
				Counts          struct{ Requests, Refused, Errors int }
				Samples         map[string][]string
				ServedUnique    int
				Interrupted     int
				Upstream        bool
				SamplesComplete bool
			}
			if err != nil || json.Unmarshal(data, &report) != nil {
				t.Fatal("global report unavailable", err)
			}
			requests, failures, refusals := 40, 0, 2
			if mode == "body-alias" {
				requests, failures, refusals = 0, 1, 1
			}
			if mode == "owner-stop" && report.Interrupted < 4 {
				t.Error("four pending header connections were not observed at owner stop")
			}
			if report.Upstream || report.Counts.Requests != requests || report.ServedUnique != requests || report.Counts.Errors != failures || report.Counts.Refused != refusals {
				t.Errorf("global response accounting differs: %+v", report)
			}
			if mode == "original-responses" && (report.SamplesComplete || len(report.Samples["requests"]) != 32) {
				t.Error("sample truncation must be explicit, not a serving failure")
			}
			for _, bucket := range report.Samples {
				if len(bucket) > 32 {
					t.Error("sample count bound")
				}
				for _, sample := range bucket {
					if len(sample) > 256 {
						t.Error("sample byte bound")
					}
				}
			}
		})
	}
	if after, err := userTreeStamp(project); err != nil || before != after {
		t.Error("project changed during TLS readbacks", err)
	}
	if _, err := privateNativeFile(ctx, node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		t.Error("Node changed during TLS readbacks", err)
	}
	if t.Failed() {
		return
	}
	if err := privateCleanup(workspace, identity); err != nil {
		t.Fatal(err)
	}
	t.Log("NATIVE_GLOBAL_MIRROR_READBACK forty_responses=true unknown_404=true alias_502=true bounded_samples=true node_authenticated=true project_preserved=true settled=true cleanup=true pnpm_resolution=false installed_acceptance=false")
}

const globalMirrorReplay = `
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { connect } from "node:tls";
const rows = JSON.parse(readFileSync("/global-fixture/requests.json", "utf8"));
if (rows.length < 40 || rows.length > 512) throw new Error("inventory count");
const alias = process.argv[1] === "body-alias";
for (const row of rows.slice(0, alias ? 1 : 40)) {
  const response = await fetch(row.url);
  if (response.status !== (alias ? 502 : 200)) throw new Error("held status: " + response.status);
  const body = Buffer.from(await response.arrayBuffer());
  if (!alias && (body.length !== row.size || createHash("sha256").update(body).digest("hex") !== row.sha256)) {
    throw new Error("original body differs");
  }
}
const unknown = await fetch("https://registry.npmjs.org/controlled-non-inventoried-global-request");
if (unknown.status !== 404) throw new Error("unknown not refused");
await unknown.arrayBuffer();
if (!alias) await new Promise((resolve, reject) => {
  const socket = connect({ host: "registry.npmjs.org", port: 443, servername: "registry.npmjs.org" });
  let reply = "";
  socket.once("secureConnect", () => socket.write(Buffer.from("GET /" + "\u00ff".repeat(256) + " HTTP/1.0\r\nHost: registry.npmjs.org\r\n\r\n", "latin1")));
  socket.on("data", chunk => { reply += chunk.toString(); if (reply.length > 4096) socket.destroy(new Error("reply bound")); });
  socket.once("end", () => reply.startsWith("HTTP/1.0 404 ") ? resolve() : reject(new Error("raw-target refusal")));
  socket.once("error", reject);
});
process.stdout.write("GLOBAL_REPLAY_OK");
`
