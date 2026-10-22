//go:build darwin

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const darwinQuarantine = "0081;00000000;Safari;"

// darwinSharedFixture selects an operator-owned prefix holding the stock Pi
// bundle and an agent holding settings.json, beside an absent target. raw
// spells every path through the /var system link.
func darwinSharedFixture(t *testing.T) (raw, canonical UserInstallRequest) {
	t.Helper()
	rawParent, parent := darwinPrivateParent(t)
	canonical = UserInstallRequest{Destination: filepath.Join(parent, "shell"), Mode: "shared",
		SharedPrefix: filepath.Join(parent, "prefix"), SharedAgent: filepath.Join(parent, "agent")}
	raw = UserInstallRequest{Destination: filepath.Join(rawParent, "shell"), Mode: "shared",
		SharedPrefix: filepath.Join(rawParent, "prefix"), SharedAgent: filepath.Join(rawParent, "agent")}
	for path, data := range map[string]string{darwinSharedCLI(canonical): "stock Pi DATA", filepath.Join(canonical.SharedAgent, "settings.json"): "{\"theme\":\"dark\"}\n"} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return raw, canonical
}

func darwinSharedCLI(req UserInstallRequest) string {
	return filepath.Join(req.SharedPrefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js")
}

// darwinTreeRecord lists names, modes, link targets and byte hashes without
// following links, so a refused operation can prove it changed nothing.
func darwinTreeRecord(t *testing.T, root string) string {
	t.Helper()
	var records []string
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		content := ""
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			content, err = os.Readlink(path)
		case info.Mode().IsRegular():
			var data []byte
			data, err = os.ReadFile(path)
			content = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		records = append(records, fmt.Sprintf("%q:%v:%s", strings.TrimPrefix(path, root), info.Mode(), content))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(records, "\n")
}

func darwinAllowACL(t *testing.T, path string) {
	t.Helper()
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", path).CombinedOutput(); err != nil {
		t.Fatalf("chmod +a %s: %v %s", path, err, output)
	}
}

func darwinQuarantined(t *testing.T, path string) {
	t.Helper()
	if err := unix.Lsetxattr(path, "com.apple.quarantine", []byte(darwinQuarantine), 0); err != nil {
		t.Fatal(err)
	}
}

func TestDarwinSharedSelectionInspectsAndPreviewsWithoutEffects(t *testing.T) {
	raw, req := darwinSharedFixture(t)
	parent := filepath.Dir(req.Destination)
	before := darwinTreeRecord(t, parent)
	for _, selection := range []UserInstallRequest{raw, req} {
		if err := ValidateUserInstall(selection); err != nil {
			t.Fatalf("Shared selection %q refused: %v", selection.SharedPrefix, err)
		}
	}
	rawToken, rawErr := InspectUserInstall(raw)
	token, err := InspectUserInstall(req)
	if rawErr != nil || err != nil || token == "" || token != rawToken {
		t.Fatalf("raw and canonical Shared spellings must bind one selection: %q %v / %q %v", rawToken, rawErr, token, err)
	}
	preview, err := PreviewUserInstall(req, token)
	if err != nil || !strings.Contains(preview, filepath.Join(req.SharedAgent, "settings.json")) || !strings.Contains(preview, "npmCommand") {
		t.Fatalf("Shared preview = %q, %v", preview, err)
	}
	if _, err := PreviewUserInstall(req, strings.Repeat("0", 64)); err == nil {
		t.Fatal("preview accepted a stale inspection")
	}
	// Inspection binds the selected bytes: a changed agent file changes consent.
	if err := os.WriteFile(filepath.Join(req.SharedAgent, "notes.md"), []byte("later"), 0600); err != nil {
		t.Fatal(err)
	}
	if changed, err := InspectUserInstall(req); err != nil || changed == token {
		t.Fatalf("changed agent kept consent: %q %v", changed, err)
	}
	if err := os.Remove(filepath.Join(req.SharedAgent, "notes.md")); err != nil {
		t.Fatal(err)
	}
	if after := darwinTreeRecord(t, parent); after != before {
		t.Fatalf("validation, inspection or preview had effects:\n%s\n->\n%s", before, after)
	}
}

func TestDarwinSharedMissingSelectionNamesThePath(t *testing.T) {
	_, req := darwinSharedFixture(t)
	missing := req
	missing.SharedPrefix = filepath.Join(filepath.Dir(req.Destination), "absent-prefix")
	_, err := InspectUserInstall(missing)
	if err == nil || !strings.Contains(err.Error(), missing.SharedPrefix) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing Shared prefix = %v, want an error naming %q", err, missing.SharedPrefix)
	}
	// An omitted --agent refuses with the Linux message.
	unselected := req
	unselected.SharedAgent = ""
	if _, err := InspectUserInstall(unselected); err == nil || !strings.Contains(err.Error(), "shared prefix and agent must be disjoint") {
		t.Fatalf("Shared without an agent = %v", err)
	}
	if _, err := os.Lstat(missing.SharedPrefix); !os.IsNotExist(err) {
		t.Fatalf("inspection created the missing prefix: %v", err)
	}
}

// The consent stamp walks the operator's trees with the same darwin metadata
// rules as the per-entry guards: hidden grants and BSD flags refuse, while an
// operator download's quarantine is foreign and tolerated.
func TestDarwinSharedTreeStampChecksExtendedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, refusal string
		prepare       func(t *testing.T, req UserInstallRequest)
	}{
		{"allow ACL on an agent file", "extended ACL", func(t *testing.T, req UserInstallRequest) {
			darwinAllowACL(t, filepath.Join(req.SharedAgent, "settings.json"))
		}},
		{"allow ACL on a nested prefix directory", "extended ACL", func(t *testing.T, req UserInstallRequest) {
			darwinAllowACL(t, filepath.Dir(darwinSharedCLI(req)))
		}},
		{"immutable prefix file", "BSD file flags", func(t *testing.T, req UserInstallRequest) {
			cli := darwinSharedCLI(req)
			if err := unix.Chflags(cli, unix.UF_IMMUTABLE); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unix.Chflags(cli, 0) })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, req := darwinSharedFixture(t)
			tc.prepare(t, req)
			before := darwinTreeRecord(t, filepath.Dir(req.Destination))
			refused := false
			for _, root := range []string{req.SharedPrefix, req.SharedAgent} {
				_, err := userTreeStamp(root)
				refused = refused || (err != nil && strings.Contains(err.Error(), tc.refusal))
			}
			if !refused {
				t.Fatalf("tree stamps accepted %s", tc.name)
			}
			if _, err := InspectUserInstall(req); err == nil || !strings.Contains(err.Error(), tc.refusal) {
				t.Fatalf("inspection = %v, want %q", err, tc.refusal)
			}
			if after := darwinTreeRecord(t, filepath.Dir(req.Destination)); after != before {
				t.Fatal("refused inspection changed the selection")
			}
		})
	}
}

func TestDarwinSharedOperatorQuarantineIsForeign(t *testing.T) {
	_, req := darwinSharedFixture(t)
	cli := darwinSharedCLI(req)
	for _, path := range []string{cli, filepath.Join(req.SharedAgent, "settings.json"), filepath.Dir(cli)} {
		darwinQuarantined(t, path)
	}
	if err := ValidateUserInstall(req); err != nil {
		t.Fatalf("operator quarantine refused Shared validation: %v", err)
	}
	if _, err := InspectUserInstall(req); err != nil {
		t.Fatalf("operator quarantine refused Shared inspection: %v", err)
	}
	// Installer-created entries stay owned: the same quarantine refuses there.
	if _, err := privatePhysical(cli); err == nil || !strings.Contains(err.Error(), "quarantined owned entry") {
		t.Fatalf("owned check accepted a quarantined file: %v", err)
	}
	bin := filepath.Join(req.SharedAgent, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	darwinQuarantined(t, bin)
	if err := userToolBinAvailable(req.SharedAgent); err == nil || !strings.Contains(err.Error(), "agent bin") {
		t.Fatalf("quarantined AGENT/bin accepted for owned tools: %v", err)
	}
}

func TestDarwinSharedAgentToolsRefuseBeforeChanges(t *testing.T) {
	for _, source := range userToolSources {
		if !strings.HasSuffix(source.stem, "-aarch64-apple-darwin") {
			t.Fatalf("darwin Shared tool %s is not the aarch64-apple-darwin pin: %q", source.name, source.stem)
		}
	}
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, bin string)
	}{
		{"existing fd", func(t *testing.T, bin string) {
			if err := os.WriteFile(filepath.Join(bin, "fd"), []byte("personal"), 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"dangling rg link", func(t *testing.T, bin string) {
			if err := os.Symlink("missing-personal-rg", filepath.Join(bin, "rg")); err != nil {
				t.Fatal(err)
			}
		}},
		{"bin mode 0750", func(t *testing.T, bin string) {
			if err := os.Chmod(bin, 0750); err != nil {
				t.Fatal(err)
			}
		}},
		{"bin with an allow ACL", func(t *testing.T, bin string) { darwinAllowACL(t, bin) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, req := darwinSharedFixture(t)
			bin := filepath.Join(req.SharedAgent, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			tc.prepare(t, bin)
			parent := filepath.Dir(req.Destination)
			before := darwinTreeRecord(t, parent)
			for operation, call := range map[string]func() error{
				"validate": func() error { return ValidateUserInstall(req) },
				"inspect":  func() error { _, err := InspectUserInstall(req); return err },
				"run":      func() error { _, err := RunUserInstall(context.Background(), req); return err },
			} {
				var failure *PrivateRuntimeError
				if err := call(); !errors.As(err, &failure) || failure.Kind != "refused" || !strings.Contains(err.Error(), "agent bin") {
					t.Fatalf("%s admitted or misclassified the agent tool conflict: %v", operation, err)
				}
			}
			if after := darwinTreeRecord(t, parent); after != before {
				t.Fatalf("refused Shared selection changed the parent:\n%s\n->\n%s", before, after)
			}
		})
	}
}

// darwinRecoveryRoot is an installed root whose saved preimages recovery
// inspects; the live prefix and agent are its siblings.
func darwinRecoveryRoot(t *testing.T) (root string, req UserInstallRequest) {
	t.Helper()
	_, req = darwinSharedFixture(t)
	root = req.Destination
	for _, name := range []string{"", "state", "state/prefix.preimage", "state/agent.preimage"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "state/agent.preimage/settings.json"), []byte("{\"theme\":\"dark\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{"mode": "shared", "prefix": req.SharedPrefix, "agent": req.SharedAgent, "finalPrefix": req.SharedPrefix,
		"finalRoot": root, "prefixSHA": strings.Repeat("a", 64), "agentSHA": strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state/selection.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return root, req
}

func darwinRecoverInspect(t *testing.T, root string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	err := RunUserEntry(context.Background(), "", []string{"recover", root, "inspect"}, nil, &output, nil)
	fields := strings.Fields(output.String())
	if err != nil || len(fields) < 3 || fields[0] != "Recovery" {
		return output.String(), err
	}
	return fields[2], nil
}

func TestDarwinRecoverInspectBindsPreimagesWithDarwinMetadata(t *testing.T) {
	root, req := darwinRecoveryRoot(t)
	token, err := darwinRecoverInspect(t, root)
	if err != nil || len(token) != 64 {
		t.Fatalf("recover inspect = %q, %v", token, err)
	}
	// A saved preimage copies the operator's tree, so its quarantine is foreign
	// too; the copy's bytes, not its xattrs, are the recovery authority.
	saved := filepath.Join(root, "state/agent.preimage/settings.json")
	darwinQuarantined(t, saved)
	quarantined, err := darwinRecoverInspect(t, root)
	if err != nil || quarantined == token {
		t.Fatalf("recover inspect after a preimage change = %q, %v", quarantined, err)
	}
	if err := RunUserEntry(context.Background(), "", []string{"recover", root, token}, nil, &bytes.Buffer{}, nil); err == nil || !strings.Contains(err.Error(), "fresh recovery confirmation differs") {
		t.Fatalf("stale recovery confirmation reached restore: %v", err)
	}
	darwinAllowACL(t, saved)
	if _, err := darwinRecoverInspect(t, root); err == nil || !strings.Contains(err.Error(), "extended ACL") {
		t.Fatalf("recover accepted a preimage with a hidden grant: %v", err)
	}
	if err := os.Chmod(req.SharedPrefix, 0770); err != nil {
		t.Fatal(err)
	}
	if _, err := darwinRecoverInspect(t, root); err == nil {
		t.Fatal("recover accepted an unsafe selected prefix")
	}
}
