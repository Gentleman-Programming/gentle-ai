package legacyassets

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The SDD release set closed with v4.0.0, so regenerating from the full local
// tag set must reproduce the committed registry exactly. Shallow clones skip.
func TestReleasedSDDAgentDigestsMatchGenerator(t *testing.T) {
	if testing.Short() {
		t.Skip("regenerates from every release tag")
	}
	requireReleaseTags(t)
	dir := t.TempDir()
	if output, err := runDigestGenerator(t, dir); err != nil {
		if replayUnavailable(err) {
			t.Skipf("release replays need modules missing from the local module cache, and the generator runs offline; fill the cache once with network access, then rerun:\n%s", output)
		}
		t.Fatalf("generator failed: %v\n%s", err, output)
	}
	for _, name := range []string{"sdd_agent_digests.go", "opencode_sdd_digests.go", "sdd_asset_digests.go"} {
		want, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s is stale; run go generate ./internal/components/legacyassets/", name)
		}
	}
}

// A release replay whose modules are not in the local cache is reported as
// unavailable, distinct from a generation failure, and never fetched: the
// generator forces GOPROXY=off even when the environment names a proxy.
func TestDigestGeneratorReportsMissingReplayModulesAsUnavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("harvests every release tag")
	}
	requireReleaseTags(t)
	output, err := runDigestGenerator(t, t.TempDir(), "GOMODCACHE="+t.TempDir(), "GOPROXY=http://127.0.0.1:9", "GOFLAGS=")
	if !replayUnavailable(err) || !strings.Contains(string(output), "GOPROXY=off") {
		t.Fatalf("generator with an empty module cache: %v\n%s", err, output)
	}
}

func requireReleaseTags(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable: %v", tool, err)
		}
	}
	for _, anchor := range []string{"v1.10.0", "v3.7.0", "v4.0.0"} {
		if exec.Command("git", "rev-parse", "-q", "--verify", "refs/tags/"+anchor+"^{commit}").Run() != nil {
			t.Skipf("release tag %s unavailable; run git fetch --tags", anchor)
		}
	}
}

// runDigestGenerator builds the generator with the ambient environment and
// runs it into dir with env added.
func runDigestGenerator(t *testing.T, dir string, env ...string) ([]byte, error) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "gen-sdd-agent-digests")
	if output, err := exec.Command("go", "build", "-o", bin, "../../../scripts/gen-sdd-agent-digests").CombinedOutput(); err != nil {
		t.Fatalf("build generator: %v\n%s", err, output)
	}
	cmd := exec.Command(bin, filepath.Join(dir, "sdd_agent_digests.go"), filepath.Join(dir, "opencode_sdd_digests.go"), filepath.Join(dir, "sdd_asset_digests.go"))
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}

// replayUnavailable reports the generator's exit code for a release replay
// whose modules are missing offline.
func replayUnavailable(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == 3
}

// genSDDRepo is a throwaway release history for the generator.
type genSDDRepo struct {
	t   *testing.T
	dir string
	env []string
}

func (r genSDDRepo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Env = r.env
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r genSDDRepo) release(tag, apply string) {
	r.t.Helper()
	for name, content := range map[string]string{
		"gentleman.yaml":                  "version: \"1\"\nagent:\n  name: gentleman\n  subagents:\n    sdd-apply:\n      path: ./sdd-apply.yaml\n",
		"sdd-apply.yaml":                  apply,
		"../../skills/sdd-apply/SKILL.md": apply,
	} {
		path := filepath.Join(r.dir, "internal", "assets", "kimi", "agents", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", tag)
	r.git("tag", tag)
}

// A local tag set missing an intermediate release must not silently drop
// that release's history from the committed registry.
func TestSDDAgentDigestGeneratorRefusesMissingIntermediateTags(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the generator")
	}
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable: %v", tool, err)
		}
	}
	generator := filepath.Join(t.TempDir(), "gen")
	if out, err := exec.Command("go", "build", "-o", generator, "../../../scripts/gen-sdd-agent-digests").CombinedOutput(); err != nil {
		t.Fatalf("build generator: %v\n%s", err, out)
	}
	repo := genSDDRepo{t: t, dir: t.TempDir(), env: append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")}
	repo.git("init", "-q")
	repo.release("v1.10.0", "name: sdd-apply\n")
	repo.release("v2.0.0", "name: sdd-apply\nonly: v2\n")
	repo.release("v3.7.0", "name: sdd-apply\n")
	repo.git("tag", "v4.0.0")
	registry := filepath.Join(t.TempDir(), "sdd_agent_digests.go")
	openCodeRegistry := filepath.Join(t.TempDir(), "opencode_sdd_digests.go")
	assetRegistry := filepath.Join(t.TempDir(), "sdd_asset_digests.go")
	run := func() (string, error) {
		cmd := exec.Command(generator, registry, openCodeRegistry, assetRegistry)
		cmd.Dir, cmd.Env = repo.dir, repo.env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run(); err != nil {
		t.Fatalf("complete tag set refused: %v\n%s", err, out)
	}
	committed, err := os.ReadFile(registry)
	if err != nil || !strings.Contains(string(committed), "v2.0.0") {
		t.Fatalf("registry does not record the intermediate tag: %v\n%s", err, committed)
	}

	repo.git("tag", "-d", "v2.0.0")
	if out, err := run(); err == nil || !strings.Contains(out, "v2.0.0") || !strings.Contains(out, "git fetch --tags") {
		t.Fatalf("missing recorded tag accepted: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(registry); string(got) != string(committed) {
		t.Fatal("refused generation rewrote the registry")
	}
	// Even with the tag list edited away, the release's unique digest proves
	// history is missing.
	edited := strings.ReplaceAll(string(committed), " v2.0.0", "")
	if err := os.WriteFile(registry, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{openCodeRegistry, assetRegistry} {
		recorded, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(strings.ReplaceAll(string(recorded), " v2.0.0", "")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := run(); err == nil || !strings.Contains(out, "was not regenerated") || !strings.Contains(out, "git fetch --tags") {
		t.Fatalf("dropped release digest accepted: %v\n%s", err, out)
	}
}
