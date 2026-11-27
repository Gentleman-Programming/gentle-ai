package update

import (
	"archive/zip"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

func TestSignPathFixturesExcludedFromModuleZip(t *testing.T) {
	requireSignPathFixtures(t)
	// Export the index so untracked files are never packaged. Unlike
	// CreateFromVCS, checkout-index also supports linked Git worktrees.
	source := t.TempDir()
	cmd := exec.Command("git", "checkout-index", "--all", "--prefix="+filepath.ToSlash(source)+"/")
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export module source: %v: %s", err, out)
	}
	m := module.Version{Path: "github.com/gentleman-programming/gentle-ai/v5", Version: "v5.0.0"}
	file, err := os.Create(filepath.Join(t.TempDir(), "module.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := modzip.CreateFromDir(file, m, source); err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(file, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	prefix := m.Path + "@" + m.Version + "/"
	if _, err := fs.Stat(archive, prefix+"cmd/gentle-ai/main.go"); err != nil {
		t.Fatalf("CLI entry point missing from module ZIP: %v", err)
	}
	for _, entry := range archive.File {
		name := strings.TrimPrefix(entry.Name, prefix)
		if strings.HasPrefix(name, ".github/signpath/") || name == "scripts/verify-signpath-test.ps1" || name == "scripts/verify-signpath-test.tests.ps1" {
			t.Errorf("unexpected SignPath CI file in module ZIP: %s", name)
		}
	}
}
