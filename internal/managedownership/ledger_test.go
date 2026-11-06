package managedownership_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/managedownership"
)

const abcHash = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

func emptyLedger() managedownership.Ledger {
	return managedownership.Ledger{Version: managedownership.Version, Files: map[string]string{}}
}

func writeLedger(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHashIsLowercaseSHA256(t *testing.T) {
	if got := managedownership.Hash([]byte("abc")); got != abcHash {
		t.Fatalf("Hash(abc) = %s", got)
	}
}

func TestLedgerEncodingKeepsFieldOrder(t *testing.T) {
	encoded, err := json.MarshalIndent(managedownership.Ledger{Version: 1, Files: map[string]string{"a.md": abcHash}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"version\": 1,\n  \"files\": {\n    \"a.md\": \"" + abcHash + "\"\n  }\n}"
	if string(encoded) != want {
		t.Fatalf("encoding changed:\n%s", encoded)
	}
}

func TestReadMissingLedgerIsEmptyAndAbsent(t *testing.T) {
	ledger, exists, err := managedownership.Read[managedownership.Ledger](filepath.Join(t.TempDir(), "absent.json"), []string{"a.md"})
	if err != nil || exists || !reflect.DeepEqual(ledger, emptyLedger()) {
		t.Fatalf("Read(missing) = %+v, %v, %v", ledger, exists, err)
	}
}

func TestReadAcceptsAllowedEntriesWithoutRewriting(t *testing.T) {
	// Unknown top-level fields stay accepted, as they always were.
	body := `{"version":1,"files":{"a.md":"` + abcHash + `"},"extra":true}`
	path := writeLedger(t, body)
	ledger, exists, err := managedownership.Read[managedownership.Ledger](path, []string{"a.md", "b.md"})
	want := managedownership.Ledger{Version: 1, Files: map[string]string{"a.md": abcHash}}
	if err != nil || !exists || !reflect.DeepEqual(ledger, want) {
		t.Fatalf("Read = %+v, %v, %v", ledger, exists, err)
	}
	if data, _ := os.ReadFile(path); string(data) != body {
		t.Fatalf("Read rewrote the ledger: %s", data)
	}
}

func TestReadRejectsInvalidLedgers(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{name: "malformed", body: "{", want: "decode ownership ledger: unexpected end of JSON input"},
		{name: "unsupported version", body: `{"version":2,"files":{}}`, want: "unsupported ownership ledger version or missing files"},
		{name: "missing files", body: `{"version":1}`, want: "unsupported ownership ledger version or missing files"},
		{name: "version string", body: `{"version":"1","files":{}}`, want: "decode ownership ledger: json: cannot unmarshal string into Go struct field Ledger.version of type int"},
		{name: "unlisted name", body: `{"version":1,"files":{"c.md":"` + abcHash + `"}}`, want: `invalid ownership ledger entry "c.md"`},
		{name: "uppercase hash", body: `{"version":1,"files":{"a.md":"` + strings.ToUpper(abcHash) + `"}}`, want: `invalid ownership ledger entry "a.md"`},
		{name: "non-hex hash", body: `{"version":1,"files":{"a.md":"` + strings.Repeat("z", 64) + `"}}`, want: `invalid ownership hash for "a.md": encoding/hex: invalid byte: U+007A 'z'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger, exists, err := managedownership.Read[managedownership.Ledger](writeLedger(t, tc.body), []string{"a.md"})
			if err == nil || err.Error() != tc.want || exists || !reflect.DeepEqual(ledger, emptyLedger()) {
				t.Fatalf("Read = %+v, %v, %v; want error %q", ledger, exists, err, tc.want)
			}
		})
	}
}

func TestReadRejectsNonRegularLedger(t *testing.T) {
	dir := t.TempDir()
	target := writeLedger(t, `{"version":1,"files":{}}`)
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, link} {
		ledger, exists, err := managedownership.Read[managedownership.Ledger](path, nil)
		if err == nil || err.Error() != "ownership ledger is not a regular file: "+path || exists || !reflect.DeepEqual(ledger, emptyLedger()) {
			t.Fatalf("Read(%s) = %+v, %v, %v", path, ledger, exists, err)
		}
	}
}
