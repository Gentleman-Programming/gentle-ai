//go:build linux

package scripts

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBootstrapPackageLauncherRelocation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires external bootstrap and Node processes")
	}

	archive := os.Getenv("GENTLE_SHELL_NODE_ARCHIVE")
	if archive == "" {
		t.Skip("requires protected authenticated archive")
	}
	archiveBefore, e := os.ReadFile(archive)
	if e != nil {
		t.Fatal(e)
	}
	beforeHash := sha256.Sum256(archiveBefore)
	destination, output, e := bootstrapRun(t, archive)
	if e != nil || output != "Node bootstrap only: Node=24.18.0 npm=11.16.0 package-install=not-run launch=not-run Ready=false\n" {
		t.Fatal("bootstrap outcome changed", e, output)
	}
	f, e := os.Open(archive)
	if e != nil {
		t.Fatal(e)
	}
	gz, e := gzip.NewReader(f)
	if e != nil {
		t.Fatal(e)
	}
	tr := tar.NewReader(gz)
	checked := 0
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		name := strings.TrimPrefix(h.Name, "node-v24.18.0-linux-x64/")
		if h.Typeflag != tar.TypeReg || name != "bin/node" && !strings.HasPrefix(name, "lib/node_modules/npm/") {
			continue
		}
		expected := sha256.New()
		if _, e := io.Copy(expected, tr); e != nil {
			t.Fatal(e)
		}
		data, e := os.ReadFile(filepath.Join(destination, name))
		if e != nil || !bytes.Equal(expected.Sum(nil), func() []byte { sum := sha256.Sum256(data); return sum[:] }()) {
			t.Fatal("vendor bytes changed", name, e)
		}
		checked++
	}
	if e := gz.Close(); e != nil {
		t.Fatal(e)
	}
	if e := f.Close(); e != nil {
		t.Fatal(e)
	}
	if checked < 100 {
		t.Fatal("vendor inventory incomplete", checked)
	}
	t.Log("AUTHENTIC_VENDOR_REGULAR_FILES", checked)
	if e := filepath.WalkDir(destination, func(path string, entry os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !entry.IsDir() {
			info, e := entry.Info()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				t.Fatalf("nonregular normalized entry %s", path)
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	moved := filepath.Join(filepath.Dir(destination), "relocated runtime")
	if e := os.Rename(destination, moved); e != nil {
		t.Fatal(e)
	}
	pkg := t.TempDir()
	script := `process.stdout.write(JSON.stringify(process.argv.slice(2)));process.exit(37);`
	if e := os.WriteFile(filepath.Join(pkg, "echo.cjs"), []byte(script), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"fixture-package","version":"1.0.0","scripts":{"probe":"node echo.cjs"}}`), 0600); e != nil {
		t.Fatal(e)
	}
	argv := []string{"value with spaces", "$(not executed)", "quote'and\"double", "", "--literal"}
	want, e := json.Marshal(argv)
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"npm", "npx"} {
		t.Run(name, func(t *testing.T) {
			info, e := os.Lstat(filepath.Join(moved, "bin", name))
			if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0700 {
				t.Fatal("wrapper mode/type differs", e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			run := func(args []string) ([]byte, error) {
				cmd := exec.CommandContext(ctx, "/bin/sh", append([]string{"-c", `exec "$@"`, "sh"}, args...)...)
				cmd.Dir = pkg
				cmd.Env = []string{"HOME=" + pkg, "PATH=" + filepath.Join(moved, "bin") + ":/usr/bin:/bin", "UV_THREADPOOL_SIZE=1", "NODE_OPTIONS=--v8-pool-size=1", "npm_config_offline=true", "npm_config_audit=false", "npm_config_fund=false"}
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				e := cmd.Run()
				if e != nil {
					t.Log("bounded fixture stderr", stderr.String())
				}
				return stdout.Bytes(), e
			}
			out, e := run([]string{name, "--version"})
			if e != nil || string(out) != "11.16.0\n" {
				t.Fatal("relocated PATH version failed", e, string(out))
			}
			args := []string{name, "--silent", "--prefix", pkg, "run", "probe", "--"}
			if name == "npx" {
				args = []string{name, "--offline", "--no", "--", "node", filepath.Join(pkg, "echo.cjs")}
			}
			out, e = run(append(args, argv...))
			exit, ok := e.(*exec.ExitError)
			if !ok || exit.ExitCode() != 37 || !bytes.Equal(out, want) {
				t.Fatal("argv/exit not preserved", e, string(out), string(want))
			}
		})
	}
	archiveAfter, e := os.ReadFile(archive)
	if e != nil || sha256.Sum256(archiveAfter) != beforeHash {
		t.Fatal("archive changed", e)
	}
}
