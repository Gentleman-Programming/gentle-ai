package pi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func must(t *testing.T, err error) { if err != nil { t.Fatal(err) } }
func writeManifest(t *testing.T, root, manifest string) { must(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0o644)) }
func writeTarget(t *testing.T, root, relative string, mode os.FileMode) string {
	path := filepath.Join(root, filepath.FromSlash(relative))
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte("#!/bin/sh\n"), mode))
	return path
}
func assertKind(t *testing.T, err error, domain, kind string) {
	var got string
	switch e := err.(type) {
	case *PackageError:
		got = "package:" + e.Kind
	case *ManifestError:
		got = "manifest:" + e.Kind
	case *BinError:
		got = "bin:" + e.Kind
	case *RoutingError:
		got = "routing:" + e.Kind
	}
	if got != domain+":"+kind {
		t.Fatalf("error = %T %v; want %s error %q", err, err, domain, kind)
	}
}
func TestResolvePackageBinForms(t *testing.T) {
	const valid, malformed, manifestBound = `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, `{"bin":`, MaxPackageManifestBytes
	snapshot := func(path string) []byte { data, err := os.ReadFile(path); must(t, err); return data }
	cases := []struct{ name, manifest, target, link, kind string }{
		{"string", `{"name":"gentle-pi-models","bin":"bin/gentle-pi-models"}`, "bin/gentle-pi-models", "", ""}, {"object and canonical symlink", `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, "real/gentle-pi-models", "symlink", ""},
		{"exact bound", valid + strings.Repeat(" ", manifestBound-len(valid)), "bin/gentle-pi-models", "", ""}, {"bound plus one", valid + strings.Repeat(" ", manifestBound+1-len(valid)), "bin/gentle-pi-models", "", "manifest-too-large"}, {"malformed within bound", malformed + strings.Repeat(" ", manifestBound-len(malformed)), "bin/gentle-pi-models", "", "malformed-manifest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.link != "" && runtime.GOOS == "windows" {
				t.Skip("symlink permissions vary on Windows")
			}
			root := t.TempDir()
			want := writeTarget(t, root, tc.target, 0o755)
			// ResolvePackageBin returns the canonical executable, so the
			// expectation must be canonical too: on the Windows runners
			// t.TempDir() is an 8.3 short name that resolves to its long
			// spelling.
			canonicalWant, err := filepath.EvalSymlinks(want)
			must(t, err)
			want = canonicalWant
			if tc.link != "" {
				bin := filepath.Join(root, "bin", "gentle-pi-models")
				must(t, os.MkdirAll(filepath.Dir(bin), 0o755))
				must(t, os.Symlink(filepath.Join("..", tc.target), bin))
			}
			writeManifest(t, root, tc.manifest)
			beforeManifest, beforeTarget := snapshot(filepath.Join(root, "package.json")), snapshot(want)
			got, err := ResolvePackageBin(root)
			if tc.kind != "" {
				assertKind(t, err, "manifest", tc.kind)
				return
			}
			if err != nil || got != want {
				t.Fatalf("ResolvePackageBin() = %q, %v; want %q", got, err, want)
			}
			afterManifest, afterTarget := snapshot(filepath.Join(root, "package.json")), snapshot(want)
			if string(beforeManifest) != string(afterManifest) || string(beforeTarget) != string(afterTarget) {
				t.Fatal("resolution mutated the manifest or executable")
			}
		})
	}
}
func TestResolvePackageBinErrors(t *testing.T) {
	cases := []struct{ name, manifest, domain, kind, setup string }{
		{"missing package", "", "package", "missing-package", "package-missing"}, {"missing manifest", "", "manifest", "missing-manifest", ""},
		{"string bin with another package name", `{"name":"other-package","bin":"bin/gentle-pi-models"}`, "bin", "absent-bin", ""}, {"malformed manifest", `{"bin":`, "manifest", "malformed-manifest", ""},
		{"malformed bin", `{"bin":true}`, "bin", "malformed-bin", ""}, {"absent bin", `{"name":"gentle-pi-models"}`, "bin", "absent-bin", ""},
		{"absent object bin", `{"bin":{"other":"bin/other"}}`, "bin", "absent-bin", ""}, {"missing target", `{"bin":{"gentle-pi-models":"bin/missing"}}`, "bin", "missing-bin-target", ""},
		{"non-regular target", `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, "bin", "non-regular-bin-target", "directory"}, {"non-executable target", `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, "bin", "non-executable-bin-target", "nonexec"},
		{"absolute target", `{"bin":{"gentle-pi-models":"/outside"}}`, "bin", "unsafe-bin", ""}, {"lexical escape", `{"bin":{"gentle-pi-models":"../outside"}}`, "bin", "unsafe-bin", ""},
		{"duplicate top-level bin", `{"bin":"bin/x","bin":"bin/y"}`, "bin", "malformed-bin", ""}, {"duplicate selected bin", `{"bin":{"gentle-pi-models":"bin/x","gentle-pi-models":"bin/y"}}`, "bin", "malformed-bin", ""}, {"symlink escape", `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, "bin", "unsafe-bin", "symlink"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.setup == "nonexec" || tc.setup == "symlink") && runtime.GOOS == "windows" {
				t.Skip("Windows does not use executable permission bits")
			}
			root := t.TempDir()
			switch tc.setup {
			case "package-missing":
				root = filepath.Join(root, "missing")
			case "directory":
				must(t, os.MkdirAll(filepath.Join(root, "bin", "gentle-pi-models"), 0o755))
			case "nonexec":
				writeTarget(t, root, "bin/gentle-pi-models", 0o644)
			case "symlink":
				outside := writeTarget(t, t.TempDir(), "outside", 0o755)
				bin := filepath.Join(root, "bin", "gentle-pi-models")
				must(t, os.MkdirAll(filepath.Dir(bin), 0o755))
				must(t, os.Symlink(outside, bin))
			}
			if tc.manifest != "" {
				writeManifest(t, root, tc.manifest)
			}
			_, err := ResolvePackageBin(root)
			assertKind(t, err, tc.domain, tc.kind)
			if tc.setup == "symlink" && !errors.As(err, new(*UnsafeBinError)) {
				t.Fatalf("error = %T %v; want UnsafeBinError cause", err, err)
			}
			if (tc.kind == "missing-package" || tc.kind == "missing-manifest" || tc.kind == "missing-bin-target") && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("error = %v; want os.ErrNotExist cause", err)
			}
		})
	}
}
func TestResolveModelRoutingExecutable_PATH(t *testing.T) {
	cases := []struct{ name string; hit, ok, wantPath bool }{
		{"path hit probed", true, true, true}, {"path hit probe fails falls through", true, false, false}, {"path miss falls through", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := filepath.Join(t.TempDir(), "gentle-pi-models")
			must(t, os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755))
			ol, or, od := modelRoutingLookPath, modelRoutingRunner, modelRoutingReadFile
			t.Cleanup(func() { modelRoutingLookPath = ol; modelRoutingRunner = or; modelRoutingReadFile = od })
			modelRoutingLookPath = func(f string) (string, error) {
				if f == packageBinName && tc.hit {
					return fake, nil
				}
				return "", errors.New("not found")
			}
			pc := 0
			modelRoutingRunner = func(ctx context.Context, bin string, req []byte) ([]byte, int, error) {
				pc++
				if tc.ok {
					return []byte(`{"contract":"gentle-pi.model-routing/v1","ok":true}`), 0, nil
				}
				return []byte(`{}`), 1, nil
			}
			modelRoutingReadFile = func(p string) ([]byte, error) { return nil, os.ErrNotExist }
			got, _ := ResolveModelRoutingExecutable(context.Background(), t.TempDir(), t.TempDir(), TargetGlobal)
			if tc.hit && tc.ok {
				if got != fake || pc != 1 {
					t.Fatalf("want %q pc1 got %q pc%d", fake, got, pc)
				}
			} else if got == fake {
				t.Fatalf("probe-failed PATH should not be returned")
			}
		})
	}
}
func TestResolveModelRoutingExecutable_Precedence(t *testing.T) {
	cases := []struct{ name, proj, user, want string }{{"project overrides user", "npm:project-pkg", "npm:user-pkg", "npm:project-pkg"}, {"user when no project", "", "npm:user-pkg", "npm:user-pkg"}, {"no source", "", "", ""}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ol, or := modelRoutingLookPath, modelRoutingRunner
			t.Cleanup(func() { modelRoutingLookPath = ol; modelRoutingRunner = or })
			modelRoutingLookPath = func(string) (string, error) { return "", errors.New("not found") }
			modelRoutingRunner = func(context.Context, string, []byte) ([]byte, int, error) { return []byte(`{"contract":"gentle-pi.model-routing/v1"}`), 0, nil }
			cwd, ad := t.TempDir(), t.TempDir()
			ws := func(dir, src string) {
				if src == "" {
					return
				}
				must(t, os.MkdirAll(dir, 0o755))
				must(t, os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"packages":["`+src+`"]}`), 0o644))
			}
			ws(filepath.Join(cwd, ".pi", "agent"), tc.proj)
			ws(ad, tc.user)
			for _, src := range []string{tc.proj, tc.user} {
				if src == "" {
					continue
				}
				rt, _ := packageRootForSource(src, ad)
				must(t, os.MkdirAll(rt, 0o755))
				writeManifest(t, rt, `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`)
				writeTarget(t, rt, "bin/gentle-pi-models", 0o755)
			}
			if tc.want != "" {
				wr, _ := packageRootForSource(tc.want, ad)
				wb, _ := ResolvePackageBin(wr)
				got, err := ResolveModelRoutingExecutable(context.Background(), cwd, ad, TargetProject)
				if err != nil || got != wb {
					t.Fatalf("precedence got %q want %q err %v", got, wb, err)
				}
			}
		})
	}
}
func TestResolveModelRoutingExecutable_AgentDirOverrides(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(t.TempDir(), "env-agent"))
	exp := filepath.Join(t.TempDir(), "explicit-agent")
	must(t, os.MkdirAll(exp, 0o755))
	must(t, os.WriteFile(filepath.Join(exp, "settings.json"), []byte(`{"packages":["npm:explicit-pkg"]}`), 0o644))
	ol, or := modelRoutingLookPath, modelRoutingRunner
	t.Cleanup(func() { modelRoutingLookPath = ol; modelRoutingRunner = or })
	modelRoutingLookPath = func(string) (string, error) { return "", errors.New("not found") }
	modelRoutingRunner = func(context.Context, string, []byte) ([]byte, int, error) { return []byte(`{"contract":"gentle-pi.model-routing/v1"}`), 0, nil }
	rt, _ := packageRootForSource("npm:explicit-pkg", exp)
	must(t, os.MkdirAll(rt, 0o755))
	writeManifest(t, rt, `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`)
	writeTarget(t, rt, "bin/gentle-pi-models", 0o755)
	got, err := ResolveModelRoutingExecutable(context.Background(), t.TempDir(), exp, TargetGlobal)
	if err != nil || got == "" {
		t.Fatalf("explicit AgentDir should win err %v got %q", err, got)
	}
}
func TestPackageRootForSource_Layouts(t *testing.T) {
	ad := "/tmp/agent"
	cases := []struct{ src, want string }{{"npm:foo", "npm"}, {"git:owner/repo", "git"}, {"local:/abs/path", "/abs/path"}}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			got, err := packageRootForSource(tc.src, ad)
			if err != nil || !strings.Contains(got, tc.want) {
				t.Fatalf("got %q err %v want contain %q", got, err, tc.want)
			}
		})
	}
}
func TestResolveModelRoutingExecutable_ManifestBinReuse(t *testing.T) {
	cases := []struct{ name, kind string }{{"unsafe bin", "unsafe-bin"}, {"missing target", "missing-bin-target"}, {"absent bin", "absent-bin"}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ol, or := modelRoutingLookPath, modelRoutingRunner
			t.Cleanup(func() { modelRoutingLookPath = ol; modelRoutingRunner = or })
			modelRoutingLookPath = func(string) (string, error) { return "", errors.New("not found") }
			modelRoutingRunner = func(context.Context, string, []byte) ([]byte, int, error) { return nil, 0, nil }
			cwd, ad := t.TempDir(), t.TempDir()
			must(t, os.MkdirAll(filepath.Join(cwd, ".pi", "agent"), 0o755))
			must(t, os.WriteFile(filepath.Join(cwd, ".pi", "agent", "settings.json"), []byte(`{"packages":["npm:bad-pkg"]}`), 0o644))
			rt, _ := packageRootForSource("npm:bad-pkg", ad)
			must(t, os.MkdirAll(rt, 0o755))
			var mf string
			switch tc.kind {
			case "unsafe-bin":
				mf = `{"bin":{"gentle-pi-models":"../escape"}}`
			case "missing-bin-target":
				mf = `{"bin":{"gentle-pi-models":"bin/missing"}}`
			case "absent-bin":
				mf = `{"name":"other","bin":"bin/x"}`
			}
			writeManifest(t, rt, mf)
			if _, err := ResolveModelRoutingExecutable(context.Background(), cwd, ad, TargetProject); err == nil {
				t.Fatalf("expected %q", tc.kind)
			}
		})
	}
}
func TestResolveModelRoutingExecutable_BoundedProbeNoWrite(t *testing.T) {
	cwd, ad := t.TempDir(), t.TempDir()
	fake := filepath.Join(t.TempDir(), "gentle-pi-models")
	must(t, os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755))
	ol, or, od := modelRoutingLookPath, modelRoutingRunner, modelRoutingReadFile
	t.Cleanup(func() { modelRoutingLookPath = ol; modelRoutingRunner = or; modelRoutingReadFile = od })
	modelRoutingLookPath = func(string) (string, error) { return fake, nil }
	modelRoutingRunner = func(ctx context.Context, bin string, req []byte) ([]byte, int, error) {
		var m map[string]any
		_ = json.Unmarshal(req, &m)
		if m["contract"] != "gentle-pi.model-routing/v1" {
			t.Fatalf("contract %v", m["contract"])
		}
		return []byte(strings.Repeat("a", MaxModelRoutingResponseBytes+2)), 0, nil
	}
	modelRoutingReadFile = func(p string) ([]byte, error) { return nil, os.ErrNotExist }
	snap := func(dir string) map[string]string {
		m := map[string]string{}
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if !d.IsDir() {
				b, _ := os.ReadFile(p)
				m[p] = string(b)
			}
			return nil
		})
		return m
	}
	before := snap(cwd)
	if _, err := ResolveModelRoutingExecutable(context.Background(), cwd, ad, TargetGlobal); err == nil {
		t.Fatal("oversized should fail")
	}
	if len(before) != len(snap(cwd)) {
		t.Fatal("mutated FS")
	}
	modelRoutingRunner = func(ctx context.Context, bin string, req []byte) ([]byte, int, error) { return nil, 0, context.DeadlineExceeded }
	if _, err := ResolveModelRoutingExecutable(context.Background(), cwd, ad, TargetGlobal); err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("want timeout got %v", err)
	}
}
func TestModelRoutingClient_Contract(t *testing.T) {
	cases := []struct{ op string; call func(c *ModelRoutingClient, ctx context.Context) error }{
		{"capabilities", func(c *ModelRoutingClient, ctx context.Context) error { _, err := c.Capabilities(ctx); return err }},
		{"inspect", func(c *ModelRoutingClient, ctx context.Context) error { _, err := c.Inspect(ctx, InspectRequest{Path: "x"}); return err }},
		{"validate", func(c *ModelRoutingClient, ctx context.Context) error { _, err := c.Validate(ctx, json.RawMessage(`{}`)); return err }},
		{"apply", func(c *ModelRoutingClient, ctx context.Context) error { _, err := c.Apply(ctx, json.RawMessage(`{}`)); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			var cap []byte
			c := &ModelRoutingClient{Bin: "/bin/fake", Timeout: time.Second, Runner: func(ctx context.Context, bin string, req []byte) ([]byte, int, error) {
				cap = req
				var m map[string]any
				if err := json.Unmarshal(req, &m); err != nil {
					t.Fatalf("bad json %v", err)
				}
				if m["contract"] != "gentle-pi.model-routing/v1" || m["op"] != tc.op {
					t.Fatalf("bad req %v", m)
				}
				return []byte(`{"contract":"gentle-pi.model-routing/v1","ok":true,"extra":"preserve"}`), 0, nil
			}}
			if err := tc.call(c, context.Background()); err != nil {
				t.Fatalf("err %v", err)
			}
			if len(cap) == 0 {
				t.Fatal("no req")
			}
		})
	}
}
func TestModelRoutingClient_ErrorTaxonomy(t *testing.T) {
	cases := []struct{ name, payload string; exit int; want string }{{"invalid json", "not-json", 0, "invalid-json"}, {"unsupported contract", `{"contract":"other/v1"}`, 0, "unsupported-contract"}, {"non-zero exit", `{"contract":"gentle-pi.model-routing/v1"}`, 1, "probe-failed"}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &ModelRoutingClient{Bin: "/bin/fake", Timeout: time.Second, Runner: func(context.Context, string, []byte) ([]byte, int, error) { return []byte(tc.payload), tc.exit, nil }}
			_, err := c.Capabilities(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
			var re *RoutingError
			if !errors.As(err, &re) || re.Kind != tc.want {
				t.Fatalf("want %q got %v", tc.want, err)
			}
		})
	}
}
func TestModelRoutingClient_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &ModelRoutingClient{Bin: "/bin/fake", Timeout: time.Second, Runner: func(ctx context.Context, bin string, req []byte) ([]byte, int, error) {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return []byte(`{"contract":"gentle-pi.model-routing/v1"}`), 0, nil
		}
	}}
	if _, err := c.Capabilities(ctx); err == nil || (!errors.Is(err, context.Canceled) && !strings.Contains(strings.ToLower(err.Error()), "cancel") && !strings.Contains(strings.ToLower(err.Error()), "timeout")) {
		t.Fatalf("want cancel got %v", err)
	}
}
func TestResolveModelRoutingExecutable_ReStat(t *testing.T) {
	cwd, ad := t.TempDir(), t.TempDir()
	must(t, os.MkdirAll(filepath.Join(cwd, ".pi", "agent"), 0o755))
	must(t, os.WriteFile(filepath.Join(cwd, ".pi", "agent", "settings.json"), []byte(`{"packages":["npm:restat-pkg"]}`), 0o644))
	rt, _ := packageRootForSource("npm:restat-pkg", ad)
	must(t, os.MkdirAll(rt, 0o755))
	writeManifest(t, rt, `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`)
	bp := writeTarget(t, rt, "bin/gentle-pi-models", 0o755)
	ol, or := modelRoutingLookPath, modelRoutingRunner
	t.Cleanup(func() { modelRoutingLookPath = ol; modelRoutingRunner = or })
	modelRoutingLookPath = func(string) (string, error) { return "", errors.New("not found") }
	called := false
	modelRoutingRunner = func(ctx context.Context, bin string, req []byte) ([]byte, int, error) {
		called = true
		if bin != bp {
			t.Fatalf("bin %q want %q", bin, bp)
		}
		return []byte(`{"contract":"gentle-pi.model-routing/v1"}`), 0, nil
	}
	must(t, os.Remove(bp))
	if _, err := ResolveModelRoutingExecutable(context.Background(), cwd, ad, TargetProject); err == nil {
		t.Fatal("removed bin should fail")
	}
	if called {
		t.Fatal("runner should not be called after re-stat")
	}
}
