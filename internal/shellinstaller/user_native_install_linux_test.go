//go:build linux && amd64 && shell_native_install

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestUserNativeInstallRefusal(t *testing.T) {
	for _, name := range []string{"unbound", "confirmation", "source drift", "occupied stage", "occupied destination", "overlap", "profile", "shared", "canceled"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			artifact, data := userNativeSourceFixture(t)
			root := userMainCustodyStage(t)
			if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
				t.Fatal(err)
			}
			if err := userStageMainSource(ctx, root, artifact); err != nil {
				t.Fatal(err)
			}
			w, err := userNativeRuntimeFromSnapshot(ctx, root, artifact)
			if err != nil {
				t.Fatal(err)
			}
			parent := filepath.Dir(installStagePath(t))
			stage := filepath.Join(parent, "publication")
			req := UserInstallRequest{Destination: filepath.Join(parent, "final"), Mode: "separate"}
			req.Confirmation, err = InspectUserInstall(req)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "unbound":
				w.sourceSnapshot = nil
			case "confirmation":
				req.Confirmation = "stale"
			case "source drift":
				err = os.WriteFile(filepath.Join(w.source, "scripts/dependency.mjs"), []byte("changed"), 0600)
			case "occupied stage":
				err = os.Mkdir(stage, 0700)
			case "occupied destination":
				err = os.Mkdir(req.Destination, 0700)
			case "overlap":
				stage = root
			case "profile":
				req.Experience = &UserExperience{}
			case "shared":
				req.Mode = "shared"
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := userTreeStamp(filepath.Dir(root))
			if err != nil {
				t.Fatal(err)
			}
			targetBefore, err := userTreeStamp(parent)
			if err != nil {
				t.Fatal(err)
			}
			native, location, err := w.produceForDestination(ctx, req, stage)
			if err == nil || native.Outcome != "" || location != nil {
				t.Fatal("unsafe transaction accepted", native, location, err)
			}
			if name == "source drift" && !errors.Is(err, errNativeSourceSnapshot) {
				t.Fatal("source drift did not refuse before prerequisites", err)
			}
			if after, err := userTreeStamp(filepath.Dir(root)); err != nil || before != after {
				t.Fatal("refusal changed producer", err)
			}
			if after, err := userTreeStamp(parent); err != nil || targetBefore != after {
				t.Fatal("refusal changed publication paths", err)
			}
		})
	}
}

func TestUserNativeInstallEnvironment(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "--require=/caller/injection.js")
	t.Setenv("UV_THREADPOOL_SIZE", "64")
	t.Setenv("GOMAXPROCS", "64")
	w := &NativeInstallerRuntime{root: "/private/producer", node: "/private/producer/runtime/node/bin/node"}
	env := w.nativeEnvironment()
	for _, required := range []string{"NODE_OPTIONS=--v8-pool-size=1", "UV_THREADPOOL_SIZE=1", "GOMAXPROCS=1", "NODE_USE_SYSTEM_CA=1", "SHELL=/usr/bin/bash", "HOME=/private/producer/home"} {
		if !slices.Contains(env, required) {
			t.Fatalf("private runtime environment missing %s", required)
		}
	}
	if slices.Contains(env, "NODE_OPTIONS=--require=/caller/injection.js") {
		t.Fatal("inherited caller runtime options")
	}
}

func TestUserNativePublicationData(t *testing.T) {
	for _, name := range []string{"new", "existing", "symlink"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			data := filepath.Join(root, "data")
			outside := t.TempDir()
			if name == "existing" {
				if err := os.Mkdir(data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if name == "symlink" {
				if err := os.Symlink(outside, data); err != nil {
					t.Fatal(err)
				}
			}
			w := &NativeInstallerRuntime{root: root}
			before := w.nativeEnvironment()
			if !slices.Contains(before, "XDG_DATA_HOME="+filepath.Join(root, "home/.local/share")) {
				t.Fatal("legacy data home changed")
			}
			copy, err := w.withPublicationData()
			if name == "symlink" {
				if err == nil || copy != nil {
					t.Fatal("foreign data directory accepted")
				}
				entries, err := os.ReadDir(outside)
				if err != nil || len(entries) != 0 {
					t.Fatal("foreign directory changed", err)
				}
				return
			}
			if err != nil || copy == w || !slices.Contains(copy.nativeEnvironment(), "XDG_DATA_HOME="+data) || !slices.Equal(before, w.nativeEnvironment()) {
				t.Fatal("publication data selection leaked to caller", err)
			}
			if _, err := privateDirectory(data); err != nil {
				t.Fatal("data directory is not private", err)
			}
		})
	}
}
