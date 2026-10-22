//go:build linux || darwin

package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Independent literals: the fixed official Node archive size per kernel.
var bootstrapArchiveBytes = map[string]int64{"linux": 57224421, "darwin": 52087559}

// bootstrapRun executes the embedded helper exactly as the installer does: from
// a private canonical parent with the fixed minimal environment.
func bootstrapRun(t *testing.T, archive string) (string, string, error) {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	script, err := ReadPrivateHelper("bootstrap-gentle-shell-private-node.sh")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "bootstrap.sh")
	if err := os.WriteFile(path, script, 0400); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(parent, "node")
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("/bin/sh", path, "--destination", destination, "--node-archive", archive)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = parent, []string{"PATH=/usr/bin:/bin", "HOME=" + parent, "TMPDIR=" + parent}, &stdout, &stderr
	err = cmd.Run()
	return destination, stdout.String() + stderr.String(), err
}

func bootstrapFixture(t *testing.T, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "node.tgz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBootstrapRefusesUnpinnedArchive(t *testing.T) {
	size, ok := bootstrapArchiveBytes[runtime.GOOS]
	if !ok {
		t.Skipf("no pinned Node archive for %s", runtime.GOOS)
	}
	for _, tc := range []struct {
		name string
		size int64
		want string
	}{
		{"size", size - 1, "STOP: archive size differs\n"},
		{"hash", size, "STOP: archive hash differs\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			destination, output, err := bootstrapRun(t, bootstrapFixture(t, tc.size))
			if err == nil || output != tc.want {
				t.Fatalf("bootstrap = %v, %q; want refusal %q", err, output, tc.want)
			}
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatalf("refused bootstrap left destination: %v", err)
			}
		})
	}
}

// Opt-in: the full bootstrap against the real publisher archive for this kernel.
func TestBootstrapPublishesPinnedArchive(t *testing.T) {
	archive := os.Getenv("GENTLE_SHELL_NODE_ARCHIVE")
	if archive == "" {
		t.Skip("GENTLE_SHELL_NODE_ARCHIVE names no downloaded publisher archive")
	}
	if runtime.GOOS == "darwin" {
		t.Skip("darwin leaves the verified tree staged; see TestBootstrapDarwinStagesPinnedArchive")
	}
	destination, output, err := bootstrapRun(t, archive)
	if err != nil || output != "Node bootstrap only: Node=24.18.0 npm=11.16.0 package-install=not-run launch=not-run Ready=false\n" {
		t.Fatalf("bootstrap = %v, %q", err, output)
	}
	provenance, err := os.ReadFile(filepath.Join(destination, "BOOTSTRAP-PROVENANCE"))
	if err != nil || !strings.Contains(string(provenance), "\nNode=24.18.0\nnpm=11.16.0\n") {
		t.Fatalf("provenance = %v, %q", err, provenance)
	}
	if _, err := os.Stat(filepath.Join(destination, "node")); !os.IsNotExist(err) {
		t.Fatal("stage published inside an existing destination")
	}
}

// Opt-in: BSD mv cannot rename without replacement, so darwin never publishes
// from the shell and reports the verified tree for an atomic Go publication.
func TestBootstrapDarwinStagesPinnedArchive(t *testing.T) {
	archive := os.Getenv("GENTLE_SHELL_NODE_ARCHIVE")
	if archive == "" {
		t.Skip("GENTLE_SHELL_NODE_ARCHIVE names no downloaded publisher archive")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("only darwin stages instead of publishing")
	}
	destination, output, err := bootstrapRun(t, archive)
	match := regexp.MustCompile(`^Node bootstrap staged: Node=24\.18\.0 npm=11\.16\.0 package-install=not-run launch=not-run Ready=false stage=(.+)\n$`).FindStringSubmatch(output)
	if err != nil || match == nil {
		t.Fatalf("bootstrap = %v, %q", err, output)
	}
	stage := match[1]
	container := filepath.Dir(stage)
	if filepath.Base(stage) != "node" || filepath.Dir(container) != filepath.Dir(destination) ||
		!regexp.MustCompile(`^\.gentle-node-stage\.[A-Za-z0-9]{8}$`).MatchString(filepath.Base(container)) {
		t.Fatalf("stage = %q outside the private parent of %q", stage, destination)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("darwin bootstrap published with mv: %v", err)
	}
	entries, err := os.ReadDir(container)
	if err != nil || len(entries) != 1 || entries[0].Name() != "node" || !entries[0].IsDir() {
		t.Fatalf("stage container = %v, %v; want only the verified tree", err, entries)
	}
	provenance, err := os.ReadFile(filepath.Join(stage, "BOOTSTRAP-PROVENANCE"))
	if err != nil || !strings.Contains(string(provenance), "\nNode=24.18.0\nnpm=11.16.0\n") {
		t.Fatalf("provenance = %v, %q", err, provenance)
	}
	if _, err := os.Stat(filepath.Join(stage, "BOOTSTRAP-SHA256SUMS")); err != nil {
		t.Fatalf("staged inventory: %v", err)
	}
}
