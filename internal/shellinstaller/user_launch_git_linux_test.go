//go:build linux

package shellinstaller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUserLaunchGitProbePreservesProject(t *testing.T) {
	if testing.Short() {
		t.Skip("requires the stock Git subprocess")
	}
	t.Setenv("GIT_OPTIONAL_LOCKS", "1")
	root, project := t.TempDir(), t.TempDir()
	launch := userLaunchCommand(context.Background(), root, userManifest{Prefix: root, Agent: root}, "/fixture/cli.js", nil)
	if launch.Dir != "" {
		t.Fatal("launch changed the caller project")
	}
	env := append(append([]string{}, launch.Env...), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("/usr/bin/git", args...)
		cmd.Dir, cmd.Env = project, env
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture Git %q: %v %s", args, err, data)
		}
		return data
	}
	run("init", "--quiet")
	if err := os.WriteFile(filepath.Join(project, "tracked"), []byte("preserve\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", "--", "tracked")
	before := privateFixture(t, project)
	beforeGit, err := os.Stat(filepath.Join(project, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	status := run("status", "--porcelain", "--branch")
	afterGit, err := os.Stat(filepath.Join(project, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(status), "tracked") {
		t.Fatal("sealed environment hid the real staged file")
	}
	if privateStamp(beforeGit) != privateStamp(afterGit) || !reflect.DeepEqual(before, privateFixture(t, project)) {
		t.Fatal("ordinary Git read probe changed project metadata or content")
	}
	if !strings.Contains(strings.Join(launch.Env, "\n"), "GIT_OPTIONAL_LOCKS=0") {
		t.Fatal("caller-controlled optional locking reached launch")
	}
}
