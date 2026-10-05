package opencoderuntimeplugins

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// genRepo is a throwaway git repository the digest generator runs in.
type genRepo struct {
	t    *testing.T
	dir  string
	env  []string
	tool string
}

func newGenRepo(t *testing.T) *genRepo {
	t.Helper()
	for _, tool := range []string{"bash", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable: %v", tool, err)
		}
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "scripts", "gen-opencode-plugin-digests.sh"))
	if err != nil {
		t.Fatal(err)
	}
	r := &genRepo{t: t, dir: t.TempDir(), env: append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1"), tool: script}
	r.git("init", "-q")
	return r
}

func (r *genRepo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Env = r.env
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *genRepo) write(rel, content string) {
	r.t.Helper()
	path := filepath.Join(r.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *genRepo) commit(message string, tags ...string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", message)
	for _, tag := range tags {
		r.git("tag", tag)
	}
}

// generate runs the generator to stdout and returns its output and stderr.
func (r *genRepo) generate() (string, string, error) {
	r.t.Helper()
	cmd := exec.Command("bash", r.tool)
	cmd.Dir = r.dir
	cmd.Env = r.env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return string(out), stderr.String(), err
}

const (
	genAsset    = "internal/assets/opencode/plugins/skill-registry.ts"
	genRegistry = "internal/components/opencoderuntimeplugins/released_digests.go"
)

func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// The registry only grows: regenerating it from an incomplete local tag set
// must refuse instead of silently dropping released digests.
func TestGenerateDigestsRefusesIncompleteTags(t *testing.T) {
	committed := "package opencoderuntimeplugins\n\nvar releasedPluginDigests = map[string][]string{\n" +
		"\t\"skill-registry.ts\": {\n\t\t\"" + strings.Repeat("ab", 32) + "\", // plugins/ v1.39.1\n\t},\n}\n"
	for _, tc := range []struct {
		name string
		tags []string
		want string
	}{
		{"missing anchor tag", []string{"v9.9.9"}, "v1.7.19"},
		{"fewer digests than committed", []string{"v1.7.19", "v4.0.0"}, "skill-registry.ts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newGenRepo(t)
			repo.write(genRegistry, committed)
			repo.commit("registry", tc.tags...)
			out, stderr, err := repo.generate()
			if err == nil {
				t.Fatalf("generator accepted incomplete tags:\n%s", out)
			}
			if !strings.Contains(stderr, tc.want) || !strings.Contains(stderr, "git fetch --tags") {
				t.Fatalf("refusal lacks %q and remedy: %s", tc.want, stderr)
			}
		})
	}
}

// An asset edited twice between releases replaces its unreleased digest: only
// digests a release shipped are guarded against drops.
func TestGenerateDigestsReplacesUnreleasedDigest(t *testing.T) {
	repo := newGenRepo(t)
	released, first, second := "released\n", "first edit\n", "second edit\n"
	repo.write(genAsset, released)
	repo.commit("release", "v1.7.19", "v4.0.0")
	repo.write(genAsset, first)
	registry, stderr, err := repo.generate()
	if err != nil {
		t.Fatalf("first regeneration: %v\n%s", err, stderr)
	}
	if !strings.Contains(registry, digestOf(first)+"\", // plugins/ unreleased") {
		t.Fatalf("first edit not registered as unreleased:\n%s", registry)
	}
	repo.write(genRegistry, registry)
	repo.commit("first edit")
	repo.write(genAsset, second)
	out, stderr, err := repo.generate()
	if err != nil {
		t.Fatalf("second edit refused: %v\n%s", err, stderr)
	}
	if !strings.Contains(out, digestOf(released)) || !strings.Contains(out, digestOf(second)) || strings.Contains(out, digestOf(first)) {
		t.Fatalf("registry after second edit:\n%s", out)
	}
}

// The current-tree digest is the digest of the bytes go:embed reads, staged or
// not, and a read failure is fatal instead of registering a wrong digest.
func TestGenerateDigestsHashesWorkingTreeBytes(t *testing.T) {
	repo := newGenRepo(t)
	repo.write(genAsset, "released\n")
	repo.commit("release", "v1.7.19", "v4.0.0")
	unstaged := "unstaged edit\n"
	repo.write(genAsset, unstaged)
	out, stderr, err := repo.generate()
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, stderr)
	}
	if !strings.Contains(out, digestOf(unstaged)+"\", // plugins/ unreleased") || strings.Contains(out, digestOf("")) {
		t.Fatalf("unstaged asset digest wrong:\n%s", out)
	}

	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	path := filepath.Join(repo.dir, filepath.FromSlash(genAsset))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })
	if out, stderr, err := repo.generate(); err == nil || !strings.Contains(stderr, "skill-registry.ts") {
		t.Fatalf("unreadable asset accepted: %v\n%s\n%s", err, stderr, out)
	}
}
