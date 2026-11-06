//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUserNativeService(t *testing.T) {
	for _, scenario := range []string{
		"pty",
		"pipe",
		"duplicate runtime",
		"duplicate bus",
		"missing runtime",
		"missing bus",
		"wrong runtime",
		"runtime symlink",
		"mask parent alias",
		"mask runtime alias",
		"mask source overlap",
		"runtime source overlap",
		"source changed",
		"source replaced",
		"archive replaced",
		"mask occupied",
		"mask replaced",
		"mask symlink",
		"runtime replaced",
		"malformed descriptor",
		"canceled",
		"wrong bus",
	} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			root := userMainCustodyStage(t)
			artifact, data := userNativeSourceFixture(t)
			if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
				t.Fatal(err)
			}
			if err := userStageMainSource(ctx, root, artifact); err != nil {
				t.Fatal(err)
			}
			parent := t.TempDir()
			mask, runtime := filepath.Join(parent, "empty"), filepath.Join(parent, "manager")
			for _, path := range []string{mask, runtime} {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			env := []string{"HOME=" + parent, "XDG_RUNTIME_DIR=" + runtime, "DBUS_SESSION_BUS_ADDRESS=unix:path=" + runtime + "/bus", "PATH=/usr/bin:/bin"}
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := userNativeServiceFileFixture(ctx, root, artifact, mask, runtime); err != nil {
				t.Fatal("unmodified file-custody baseline refused", err)
			}
			refusePreparation := false
			switch scenario {
			case "runtime symlink":
				err = os.Rename(runtime, runtime+".old")
				if err == nil {
					err = os.Symlink(runtime+".old", runtime)
				}
				refusePreparation = true
			case "mask parent alias":
				alias := filepath.Join(t.TempDir(), "alias")
				err = os.Symlink(parent, alias)
				mask = filepath.Join(alias, "empty")
				refusePreparation = true
			case "mask runtime alias":
				mask = runtime
				refusePreparation = true
			case "mask source overlap":
				mask = filepath.Join(root, "runtime")
				refusePreparation = true
			case "runtime source overlap":
				runtime = filepath.Join(root, "runtime")
				refusePreparation = true
			}
			if err != nil {
				t.Fatal(err)
			}
			custody, err := userNativeServiceFileFixture(ctx, root, artifact, mask, runtime)
			if refusePreparation {
				if err == nil || custody != nil {
					t.Fatal("unsafe preparation accepted")
				}
				if after, err := userTreeStamp(root); err != nil || after != before {
					t.Fatal("refused preparation mutated source", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if after, err := userTreeStamp(root); err != nil || after != before {
				t.Fatal("preparation mutated source", err)
			}
			switch scenario {
			case "source changed":
				err = os.WriteFile(filepath.Join(root, "runtime/main-source/scripts/dependency.mjs"), []byte("changed"), 0600)
			case "source replaced", "archive replaced":
				path := filepath.Join(root, "runtime/main-source/scripts/dependency.mjs")
				if scenario == "archive replaced" {
					path = filepath.Join(root, "runtime/main/main.zip")
				}
				var bytes []byte
				bytes, err = os.ReadFile(path)
				if err == nil {
					err = os.Rename(path, path+".old")
				}
				if err == nil {
					err = os.WriteFile(path, bytes, 0600)
				}
				if err == nil {
					err = os.Remove(path + ".old")
				}
			case "mask occupied":
				err = os.WriteFile(filepath.Join(mask, "entry"), nil, 0600)
			case "mask replaced", "mask symlink":
				err = os.Rename(mask, mask+".old")
				if err == nil && scenario == "mask replaced" {
					err = os.Mkdir(mask, 0700)
				}
				if err == nil && scenario == "mask symlink" {
					err = os.Symlink(mask+".old", mask)
				}
			case "runtime replaced":
				err = os.Rename(runtime, runtime+".old")
				if err == nil {
					err = os.Mkdir(runtime, 0700)
				}
			case "malformed descriptor":
				custody.artifact.SHA = strings.Repeat("b", 64)
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "duplicate runtime":
				env = append(env, env[1])
			case "duplicate bus":
				env = append(env, env[2])
			case "missing runtime":
				env = append(env[:1], env[2:]...)
			case "missing bus":
				env = append(env[:2], env[3:]...)
			case "wrong runtime":
				env[1] = "XDG_RUNTIME_DIR=" + parent
			case "wrong bus":
				env[2] = "DBUS_SESSION_BUS_ADDRESS=unix:path=/other/bus"
			}
			if err != nil {
				t.Fatal(err)
			}
			preimage, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"internal-install", "selected"}
			interactive := scenario != "pipe"
			got, buildErr := userNativeServiceFileArguments(custody, ctx, "owned-unit", interactive, "/qualified/self", parent, args, env)
			if scenario != "pty" && scenario != "pipe" {
				if buildErr == nil || got != nil {
					t.Fatal("unsafe custody accepted", got, buildErr)
				}
			} else {
				if buildErr != nil {
					t.Fatal(buildErr)
				}
				base := userServiceArgs("owned-unit", interactive, "/qualified/self", parent, args, env)
				var retained []string
				additions := map[string]bool{
					"--property=PrivateUsers=yes":       false,
					"--property=CapabilityBoundingSet=": false,
					"--property=RuntimeMaxSec=300s":     false,
					"--property=ReadOnlyPaths=" + filepath.Join(root, "runtime/main") + " " + filepath.Join(root, "runtime/main-source") + " /sys/fs/cgroup": false,
					"--property=BindReadOnlyPaths=" + mask + ":" + runtime: false,
				}
				for _, arg := range got {
					if _, ok := additions[arg]; ok {
						additions[arg] = true
					} else {
						retained = append(retained, arg)
					}
				}
				for arg, found := range additions {
					if !found {
						t.Fatal("missing native control", arg)
					}
				}
				if !reflect.DeepEqual(retained, base) {
					t.Fatal("generic supervision changed", retained, base)
				}
			}
			after, stampErr := userTreeStamp(root)
			if stampErr != nil || preimage != after {
				t.Fatal("builder changed source", stampErr)
			}
		})
	}
}

func TestUserNativeServiceRejectsUnprepared(t *testing.T) {
	var custody *userNativeServiceCustody
	if args, err := custody.args(context.Background(), "unit", false, "/self", "/cwd", nil, nil); err == nil || args != nil {
		t.Fatal("nil custody accepted")
	}
	root := userMainCustodyStage(t)
	artifact, _ := userNativeSourceFixture(t)
	parent := t.TempDir()
	if custody, err := userNativeServiceFileFixture(context.Background(), root, artifact, parent, parent); err == nil || custody != nil {
		t.Fatal("unprepared/overlapping source accepted")
	}
}
