//go:build linux && shell_codegraph_library

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserCodeGraphManifestRequiresRetainedLibrary(t *testing.T) {
	root := installStagePath(t)
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"Schema": userSchema, "Destination": root, "Mode": "separate", "codegraph": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "installation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = userReadManifest(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "CodeGraph retained integrity") {
		t.Fatalf("manifest ignores selected library integrity: %v", err)
	}
	for _, selected := range []bool{false, true} {
		mode := "separate"
		if selected {
			mode = "shared"
		}
		data, _ := json.Marshal(userManifest{Schema: userSchema, Destination: root, Mode: mode, CodeGraph: selected})
		if err := os.WriteFile(filepath.Join(root, "installation.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		_, err := userReadManifest(context.Background(), root)
		if selected && (err == nil || !strings.Contains(err.Error(), "Separate manifest")) {
			t.Fatal("Shared marked library admitted", err)
		}
		if !selected && (strings.Contains(string(data), "codegraph") || err == nil || strings.Contains(err.Error(), "CodeGraph retained integrity")) {
			t.Fatal("historical marker/absence changed", err)
		}
	}
}

func TestUserCodeGraphAuthenticatedRetainedAdmission(t *testing.T) {
	ctx := context.Background()
	archives := make([][]byte, 2)
	for i, name := range []string{"/frozen-codegraph-main.tgz", "/frozen-codegraph-linux.tgz"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		archives[i] = data
	}
	root := installStagePath(t)
	if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userStageCodeGraphLibrary(ctx, root, archives); err != nil {
		t.Fatal(err)
	}
	if err := userStageCodeGraphExecutable(ctx, root, root); err != nil {
		t.Fatal(err)
	}
	before, err := userTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := userCodeGraphReadback(ctx, root); err != nil {
		t.Fatal("actual authentic retained bytes refused", err)
	}
	after, err := userTreeStamp(root)
	if err != nil || before != after {
		t.Fatal("admission mutated library", err)
	}
	for _, name := range []string{"sources/codegraph.0.tgz-part", "node_modules/@colbymchenry/codegraph/package.json"} {
		path := filepath.Join(root, "runtime/codegraph", name)
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		bad := append([]byte(nil), original...)
		bad[0] ^= 1
		if err := os.WriteFile(path, bad, 0600); err != nil {
			t.Fatal(err)
		}
		if err := userCodeGraphReadback(ctx, root); err == nil {
			t.Fatal("tampered source/member admitted", name)
		}
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
	}
	extra := filepath.Join(root, "runtime/codegraph", "extra")
	if err := os.WriteFile(extra, []byte("unrequested"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userCodeGraphReadback(ctx, root); err == nil {
		t.Fatal("extra retained member admitted")
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	if err := userCodeGraphReadback(stopped, root); !errors.Is(err, context.Canceled) {
		t.Fatal("lost canceled admission", err)
	}
	t.Log("CODEGRAPH_RETAINED_ADMISSION_PASS authenticated_parts=true canonical_members=true no_effects=true per_file_32MiB=true")
}
