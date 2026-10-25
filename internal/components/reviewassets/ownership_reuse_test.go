package reviewassets_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// These fixtures pin the public native-agent ownership behavior so the ledger
// can move to a shared package without changing bytes, errors, or the files a
// rejected install leaves behind.

func kiroOwnershipDir(t *testing.T) (string, agents.Adapter, string) {
	t.Helper()
	adapter, err := agents.NewAdapter(model.AgentKiroIDE)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return home, adapter, dir
}

// snapshotTree records every entry under root: type, permissions, and either
// its bytes or its symlink target.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		state := info.Mode().String()
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			state += " -> " + target
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			state += " " + string(data)
		}
		tree[strings.TrimPrefix(path, root)] = state
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func sha(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

func TestOwnershipReuseFreshLedgerBytes(t *testing.T) {
	home, adapter, dir := kiroOwnershipDir(t)
	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	names := append([]string(nil), reviewassets.NativeAgentManifest[model.AgentKiroIDE]...)
	sort.Strings(names)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, "    \""+name+"\": \""+sha(string(data))+"\"")
	}
	want := "{\n  \"version\": 1,\n  \"files\": {\n" + strings.Join(entries, ",\n") + "\n  }\n}\n"
	ledger := filepath.Join(dir, reviewassets.OwnershipLedgerFilename)
	got, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("ledger bytes changed:\n%s\nwant:\n%s", got, want)
	}
	wantMode := os.FileMode(0o644)
	if runtime.GOOS == "windows" {
		// Windows exposes writable files as 0666 rather than POSIX permission bits.
		wantMode = 0o666
	}
	if info, err := os.Lstat(ledger); err != nil || info.Mode().Perm() != wantMode {
		t.Fatalf("ledger mode: %v %v, want %04o", info, err, wantMode)
	}
}

func TestOwnershipReuseRejectsLedgerWithoutMutation(t *testing.T) {
	valid := sha("x")
	// JSON type errors keep the baseline 9e metadata: the native struct name,
	// the root type and its package, and the decoder offsets.
	root := `reviewassets.ownershipLedger "github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"`
	for _, tc := range []struct {
		name, body, want, typeErr string
		setup                     func(t *testing.T, ledger string)
		wantFor                   func(ledger string) string
	}{
		{name: "malformed", body: "{", want: "decode ownership ledger: unexpected end of JSON input"},
		{name: "unsupported version", body: `{"version":2,"files":{}}`, want: "unsupported ownership ledger version or missing files"},
		{name: "missing files", body: `{"version":1}`, want: "unsupported ownership ledger version or missing files"},
		{name: "null files", body: `{"version":1,"files":null}`, want: "unsupported ownership ledger version or missing files"},
		{name: "unmanaged name", body: `{"version":1,"files":{"notes.md":"` + valid + `"}}`, want: `invalid ownership ledger entry "notes.md"`},
		{name: "uppercase hash", body: `{"version":1,"files":{"jd-judge-a.md":"` + strings.ToUpper(valid) + `"}}`, want: `invalid ownership ledger entry "jd-judge-a.md"`},
		{name: "short hash", body: `{"version":1,"files":{"jd-judge-a.md":"` + valid[:63] + `"}}`, want: `invalid ownership ledger entry "jd-judge-a.md"`},
		{name: "non-hex hash", body: `{"version":1,"files":{"jd-judge-a.md":"` + strings.Repeat("z", 64) + `"}}`, want: `invalid ownership hash for "jd-judge-a.md": encoding/hex: invalid byte: U+007A 'z'`},
		{name: "version string", body: `{"version":"1","files":{}}`, want: "decode ownership ledger: json: cannot unmarshal string into Go struct field ownershipLedger.version of type int", typeErr: `string int "" 14 ownershipLedger.version`},
		{name: "version array", body: `{"version":[],"files":{}}`, want: "decode ownership ledger: json: cannot unmarshal array into Go struct field ownershipLedger.version of type int", typeErr: `array int "" 12 ownershipLedger.version`},
		{name: "files string", body: `{"version":1,"files":"x"}`, want: "decode ownership ledger: json: cannot unmarshal string into Go struct field ownershipLedger.files of type map[string]string", typeErr: `string map[string]string "" 24 ownershipLedger.files`},
		{name: "hash number", body: `{"version":1,"files":{"jd-judge-a.md":7}}`, want: "decode ownership ledger: json: cannot unmarshal number into Go struct field ownershipLedger.files of type string", typeErr: `number string "" 39 ownershipLedger.files`},
		{name: "root array", body: `[]`, want: "decode ownership ledger: json: cannot unmarshal array into Go value of type reviewassets.ownershipLedger", typeErr: "array " + root + " 1 ."},
		{name: "root string", body: `"x"`, want: "decode ownership ledger: json: cannot unmarshal string into Go value of type reviewassets.ownershipLedger", typeErr: "string " + root + " 3 ."},
		{name: "root number", body: `7`, want: "decode ownership ledger: json: cannot unmarshal number into Go value of type reviewassets.ownershipLedger", typeErr: "number " + root + " 1 ."},
		{name: "root bool", body: `true`, want: "decode ownership ledger: json: cannot unmarshal bool into Go value of type reviewassets.ownershipLedger", typeErr: "bool " + root + " 4 ."},
		{name: "directory", setup: func(t *testing.T, ledger string) {
			if err := os.Mkdir(ledger, 0o755); err != nil {
				t.Fatal(err)
			}
		}, wantFor: func(ledger string) string {
			// Pin the wrapper while preserving the host OS's directory-read error.
			_, readErr := os.ReadFile(ledger)
			return fmt.Sprintf("capture ownership ledger: read before-image %q: %v", ledger, readErr)
		}},
		{name: "symlink", setup: func(t *testing.T, ledger string) {
			target := filepath.Join(filepath.Dir(ledger), "ledger-target")
			if err := os.WriteFile(target, []byte(`{"version":1,"files":{}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, ledger); err != nil {
				t.Fatal(err)
			}
		}, wantFor: func(ledger string) string {
			return fmt.Sprintf("capture ownership ledger: refuse symlink mutation journal path %q", ledger)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, adapter, dir := kiroOwnershipDir(t)
			ledger := filepath.Join(dir, reviewassets.OwnershipLedgerFilename)
			// A user file and a managed-looking retired agent must both survive.
			for name, body := range map[string]string{"jd-judge-b.md": "user content\n", "review-risk.md": "retired agent\n"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			if tc.setup != nil {
				tc.setup(t, ledger)
			} else if err := os.WriteFile(ledger, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			// The mutation journal guards a non-regular ledger before it is read.
			want := tc.want
			if tc.wantFor != nil {
				want = tc.wantFor(ledger)
			}
			before := snapshotTree(t, home)
			result, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
			var typeErr *json.UnmarshalTypeError
			if tc.typeErr != "" && (!errors.As(err, &typeErr) || fmt.Sprintf("%s %v %q %d %s.%s", typeErr.Value, typeErr.Type, typeErr.Type.PkgPath(), typeErr.Offset, typeErr.Struct, typeErr.Field) != tc.typeErr) {
				t.Fatalf("type error metadata = %+v, want %s", typeErr, tc.typeErr)
			}
			if !reflect.DeepEqual(result, reviewassets.InstallResult{}) {
				t.Fatalf("rejected install reported effects: %+v", result)
			}
			if after := snapshotTree(t, home); !reflect.DeepEqual(after, before) {
				t.Fatalf("rejected install mutated files:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func TestOwnershipReuseKeepsCurrentLedgerAcceptance(t *testing.T) {
	home, adapter, dir := kiroOwnershipDir(t)
	modified := filepath.Join(dir, "jd-judge-a.md")
	if err := os.WriteFile(modified, []byte("user modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := sha("an earlier render\n")
	// Unknown top-level fields and entries for retired names stay accepted.
	body := `{"version":1,"files":{"jd-judge-a.md":"` + stale + `","review-risk.md":"` + stale + `"},"extra":true}`
	ledger := filepath.Join(dir, reviewassets.OwnershipLedgerFilename)
	if err := os.WriteFile(ledger, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(modified); string(got) != "user modified\n" || !reflect.DeepEqual(result.Skipped, []string{modified}) {
		t.Fatalf("user-modified agent not preserved and skipped: %+v %q", result, got)
	}
	recorded := readOwnershipLedger(t, ledger).Files
	if recorded["jd-judge-a.md"] != stale {
		t.Fatalf("skipped agent lost its recorded hash: %v", recorded)
	}
	if _, ok := recorded["review-risk.md"]; ok {
		t.Fatalf("retired entry kept: %v", recorded)
	}
	if data, _ := os.ReadFile(ledger); strings.Contains(string(data), "extra") {
		t.Fatalf("rewritten ledger kept unknown field: %s", data)
	}
}
