//go:build linux && amd64 && shell_pnpm_publication

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIndependentCacheOrdinaryPrepare(t *testing.T) {
	if testing.Short() {
		t.Skip("requires the qualified private retained-DATA fixture")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before := independentOverlapHash(t)
	receipt := independentOverlapReceipt(t)
	loc := independentOverlapLocation(t)
	stage, root := "/fixture/stage", "/fixture/final"
	start := time.Now()
	staged, err := StageNativePNPMInstallation(ctx, independentOverlapSource, stage, root, loc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage + "/home/.npm"); !os.IsNotExist(err) {
		t.Fatal("cache retained", err)
	}
	if _, err := os.Lstat(stage + "/pnpm/store/v11/files"); !os.IsNotExist(err) {
		t.Fatal("PNPM cache retained", err)
	}
	profile := UserExperience{Persona: "neutral", BackgroundSubagents: "off"}
	if err := userStageExperience(ctx, stage, &profile); err != nil {
		t.Fatal(err)
	}
	sha, err := userCopySupervisor(ctx, "/qualified-supervisor", stage+"/supervisor")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stage+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	m := userManifest{Schema: userSchema, Destination: root, Prefix: root + "/pnpm", Agent: root + "/agent", Mode: "separate", SupervisorSHA: sha, PNPM: &userPNPMManifest{PackageRoot: strings.TrimSuffix(staged.Installation.ShellCLI, "/bin/gentle-shell.mjs"), ShellVersion: loc.ShellVersion, PeerVersion: loc.PiVersion, CodeSeal: staged.CodeSeal}}
	m.PrefixIdentity, err = userIdentity(stage + "/pnpm")
	if err != nil {
		t.Fatal(err)
	}
	m.AgentIdentity, err = userIdentity(stage + "/agent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := userPublishManifest(ctx, stage, stage+"/pnpm", stage+"/agent", m); err != nil {
		t.Fatal(err)
	}
	if _, err := userReadManifest(ctx, root); err != nil {
		t.Fatal(err)
	}
	if before != independentOverlapHash(t) || !bytes.Equal(receipt, independentOverlapReceipt(t)) {
		t.Fatal("original source changed")
	}
	b, err := json.Marshal(map[string]any{"seconds": time.Since(start).Seconds(), "supervisorSHA256": sha, "codeSeal": staged.CodeSeal, "sourceEntries": 84345, "sourceHash": before})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/report/ordinary-prepare.json", b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestIndependentCacheRefusal(t *testing.T) {
	if testing.Short() {
		t.Skip("requires the qualified private retained-DATA fixture")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	root := "/fixture/final"
	manifest, err := userReadManifest(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	loc := NativeInstallationLocation{PackageRoot: filepath.Join(root, manifest.PNPM.PackageRoot), PNPMHome: root + "/pnpm", GlobalBin: root + "/pnpm/bin", ShellVersion: manifest.PNPM.ShellVersion, PiVersion: manifest.PNPM.PeerVersion}
	initial, err := userPNPMPayloadInventory(ctx, root, userPNPMRoots)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, cache string }{
		{"home-non-directory-cache", "home/.npm"}, {"home-surviving-cache-link", "home/.npm"},
		{"CAS-non-directory-cache", "pnpm/store/v11/files"}, {"CAS-surviving-cache-link", "pnpm/store/v11/files"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cache := root + "/" + tt.cache
			top := strings.SplitN(tt.cache, "/", 2)[0]
			link := root + "/" + top + "/!surviving-cache-link"
			if _, err := os.Lstat(cache); !os.IsNotExist(err) {
				t.Fatal("fixture cache not absent", err)
			}
			if strings.HasSuffix(tt.name, "non-directory-cache") {
				if err := os.WriteFile(cache, []byte("not a cache directory"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(cache, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(cache+"/retained.txt", []byte("owned acquisition data"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(strings.TrimPrefix(tt.cache, top+"/")+"/retained.txt", link); err != nil {
					t.Fatal(err)
				}
			}
			before, err := userPNPMPayloadInventory(ctx, root, userPNPMRoots)
			if err != nil {
				t.Fatal(err)
			}
			stage, final := "/fixture/refused-stage", "/fixture/refused-final"
			_, callErr := StageNativePNPMInstallation(ctx, root, stage, final, loc)
			if callErr == nil {
				t.Fatal("unsafe omitted cache accepted")
			}
			for _, p := range []string{stage, final} {
				if _, err := os.Lstat(p); !os.IsNotExist(err) {
					t.Fatal("rejected stage published or survived", p, err)
				}
			}
			after, err := userPNPMPayloadInventory(ctx, root, userPNPMRoots)
			if err != nil || !equalCacheInventory(before, after) {
				t.Fatal("refusal mutated source", err)
			}
			if strings.HasSuffix(tt.name, "surviving-cache-link") {
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(cache + "/retained.txt"); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Remove(cache); err != nil {
				t.Fatal(err)
			}
		})
	}
	after, err := userPNPMPayloadInventory(ctx, root, userPNPMRoots)
	if err != nil || !equalCacheInventory(initial, after) {
		t.Fatal("fixture not restored", err)
	}
}
func equalCacheInventory(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for p, id := range a {
		if b[p] != id {
			return false
		}
	}
	return true
}

func TestIndependentCacheOrdinaryLaunch(t *testing.T) {
	if testing.Short() {
		t.Skip("requires the qualified private retained-DATA fixture")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/out", independentOverlapSource, "/qualified-supervisor"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("original/build source exposed", path, err)
		}
	}
	ctx, root := context.Background(), "/fixture/final"
	manifest, err := userReadManifest(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	before, err := PNPMCodeSeal(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root + "/home/.npm"); !os.IsNotExist(err) {
		t.Fatal("cache reappeared before launch", err)
	}
	if _, err := os.Lstat(root + "/pnpm/store/v11/files"); !os.IsNotExist(err) {
		t.Fatal("PNPM cache reappeared before launch", err)
	}
	project := t.TempDir()
	marker := project + "/preserve.txt"
	if err := os.WriteFile(marker, []byte("caller project"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"gentle-shell", "pi"} {
		cmd := exec.CommandContext(ctx, root+"/bin/"+role, "--version")
		cmd.Dir = project
		cmd.Env = append(os.Environ(), "NODE_OPTIONS=--require=/must-not-inherit.js", "PI_CODING_AGENT_DIR=/must-not-use")
		start := time.Now()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal(role, err, string(out))
		}
		version := "1.3.0"
		if role == "gentle-shell" {
			version = "4.3.0"
		}
		if !strings.Contains(string(out), version) {
			t.Fatal("wrong ordinary version", role, string(out))
		}
		t.Logf("ORDINARY %s --version exit0 seconds=%.3f output=%s", role, time.Since(start).Seconds(), out)
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "caller project" {
		t.Fatal("caller modified", err)
	}
	if after, err := PNPMCodeSeal(ctx, root); err != nil || after != before {
		t.Fatal("code changed", err)
	}
	fresh, err := userReadManifest(ctx, root)
	if err != nil || fresh.SupervisorSHA != manifest.SupervisorSHA {
		t.Fatal("manifest/supervisor changed", err)
	}
	if err := userLaunchPNPM(ctx, root, "not-a-role", nil, nil, io.Discard, io.Discard); err == nil {
		t.Fatal("unknown role accepted")
	}
}
