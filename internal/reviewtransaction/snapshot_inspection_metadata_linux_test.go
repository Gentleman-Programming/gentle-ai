//go:build linux

package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSnapshotInspectionPreservesGitDirectoryMetadataWithUsableProcessTemp(t *testing.T) {
	requireSnapshotGit(t)
	for _, kind := range []string{"workspace", "staged", "head", "relative-workspace", "relative-staged", "relative-head"} {
		t.Run(kind, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			processTemp := t.TempDir()
			selection := processTemp
			if strings.HasPrefix(kind, "relative-") {
				kind = strings.TrimPrefix(kind, "relative-")
				launch := t.TempDir()
				t.Chdir(launch)
				processTemp = filepath.Join(launch, "scratch")
				if err := os.Mkdir(processTemp, 0700); err != nil {
					t.Fatal(err)
				}
				selection = "scratch"
			}
			for _, name := range []string{"TMPDIR", "TEMP", "TMP"} {
				t.Setenv(name, selection)
			}
			gitDir := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--absolute-git-dir"))
			indexPath := filepath.Join(gitDir, "index")
			indexBefore, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			inspect := func() string {
				t.Helper()
				builder := SnapshotBuilder{Repo: repo}
				if kind == "head" {
					tree, _, err := builder.buildHeadWithIntended(context.Background(), nil)
					if err != nil {
						t.Fatal(err)
					}
					return tree
				}
				projection := ProjectionWorkspace
				if kind == "staged" {
					projection = ProjectionStaged
				}
				snapshot, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, Projection: projection, IntendedUntracked: []string{}})
				if err != nil {
					t.Fatal(err)
				}
				if err := builder.ValidateEvidence(context.Background(), snapshot); err != nil {
					t.Fatal(err)
				}
				return snapshot.Identity
			}
			want := inspect() // Seed any initial Git objects before measuring a repeated read.
			before, err := os.Stat(gitDir)
			if err != nil {
				t.Fatal(err)
			}
			if got := inspect(); got != want {
				t.Fatal("inspection changed the snapshot identity/tree")
			}
			after, err := os.Stat(gitDir)
			if err != nil {
				t.Fatal(err)
			}
			old, fresh := before.Sys().(*syscall.Stat_t), after.Sys().(*syscall.Stat_t)
			if !os.SameFile(before, after) || old.Mode != fresh.Mode || old.Uid != fresh.Uid || old.Gid != fresh.Gid || old.Size != fresh.Size || old.Mtim != fresh.Mtim || old.Ctim != fresh.Ctim {
				t.Fatal("repeated inspection changed Git-directory metadata")
			}
			indexAfter, err := os.ReadFile(indexPath)
			if err != nil || string(indexAfter) != string(indexBefore) {
				t.Fatal("inspection changed the real index")
			}
			entries, err := os.ReadDir(processTemp)
			if err != nil || len(entries) != 0 {
				t.Fatal("inspection retained process-temp artifacts")
			}
		})
	}
}
