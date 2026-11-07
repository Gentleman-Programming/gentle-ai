package filemerge

import (
	"bytes"
	"strings"
	"testing"
)

const utf8BOM = "\xEF\xBB\xBF"

// A BOM-prefixed user config must merge exactly like the same document
// without a BOM, and keep its BOM so the file's encoding is unchanged.
func TestMergeJSONObjectsForPathKeepsBOMUserKeys(t *testing.T) {
	overlay := []byte(`{"mcp":{"engram":{"command":"engram"}},"theme":"gentleman"}`)
	for _, tc := range []struct {
		name, path, body string
		keep             []string
	}{
		{
			name: "json",
			path: "settings.json",
			body: "{\n  \"model\": \"user-model\",\n  \"mcp\": {\"mine\": {\"command\": \"mine\"}}\n}\n",
			keep: []string{`"model": "user-model"`, `"mine"`, `"engram"`, `"theme": "gentleman"`},
		},
		{
			name: "json with comments and trailing commas",
			path: "settings.json",
			body: "{\n  // user note\n  \"model\": \"user-model\",\n  \"mcp\": {\"mine\": {\"command\": \"mine\",},},\n}\n",
			keep: []string{`"model": "user-model"`, `"mine"`, `"engram"`, `"theme": "gentleman"`},
		},
		{
			name: "jsonc",
			path: "opencode.jsonc",
			body: "{\n  // user note\n  \"model\": \"user-model\",\n  \"mcp\": {\"mine\": {\"command\": \"mine\"}},\n}\n",
			keep: []string{"// user note", `"model": "user-model"`, `"mine"`, `"engram"`, `"theme": "gentleman"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MergeJSONObjectsForPath(tc.path, []byte(utf8BOM+tc.body), overlay)
			if err != nil {
				t.Fatalf("merge BOM base: %v", err)
			}
			if !bytes.HasPrefix(got, []byte(utf8BOM)) || bytes.HasPrefix(got[len(utf8BOM):], []byte(utf8BOM)) {
				t.Fatalf("output must keep exactly one leading BOM:\n%q", got)
			}
			for _, want := range tc.keep {
				if !strings.Contains(string(got), want) {
					t.Fatalf("output lost %s:\n%s", want, got)
				}
			}
			plain, err := MergeJSONObjectsForPath(tc.path, []byte(tc.body), overlay)
			if err != nil {
				t.Fatalf("merge plain base: %v", err)
			}
			if bytes.HasPrefix(plain, []byte(utf8BOM)) {
				t.Fatalf("plain base output must not gain a BOM:\n%q", plain)
			}
			if !bytes.Equal(got, append([]byte(utf8BOM), plain...)) {
				t.Fatalf("BOM output differs from plain output beyond the BOM:\nbom:   %q\nplain: %q", got, plain)
			}
		})
	}
}

func TestMergeJSONObjectsBOMOnlyBaseIsEmpty(t *testing.T) {
	got, err := MergeJSONObjects([]byte(utf8BOM), []byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if want := utf8BOM + "{\n  \"a\": 1\n}\n"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestUnmarshalJSONObjectAcceptsBOM(t *testing.T) {
	for _, raw := range []string{
		utf8BOM + `{"model":"user-model"}`,
		utf8BOM + "{\n  // note\n  \"model\": \"user-model\",\n}",
	} {
		got, err := UnmarshalJSONObject([]byte(raw))
		if err != nil {
			t.Fatalf("UnmarshalJSONObject(%q): %v", raw, err)
		}
		if got["model"] != "user-model" {
			t.Fatalf("UnmarshalJSONObject(%q) = %v", raw, got)
		}
	}
}

func TestMergeJSONDefaultsForPathKeepsBOMUserKeys(t *testing.T) {
	body := "{\n  \"model\": \"user-model\"\n}\n"
	for _, path := range []string{"settings.json", "opencode.jsonc"} {
		got, err := MergeJSONDefaultsForPath(path, []byte(utf8BOM+body), []byte(`{"model":"default","share":"disabled"}`))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		plain, err := MergeJSONDefaultsForPath(path, []byte(body), []byte(`{"model":"default","share":"disabled"}`))
		if err != nil {
			t.Fatalf("%s plain: %v", path, err)
		}
		if !bytes.Equal(got, append([]byte(utf8BOM), plain...)) || !strings.Contains(string(got), `"user-model"`) {
			t.Fatalf("%s: BOM defaults output %q, plain %q", path, got, plain)
		}
	}
}

// Only a leading BOM is encoding metadata; a malformed non-BOM base keeps the
// existing empty-base fallback exactly.
func TestMergeJSONObjectsInvalidNonBOMBaseUnchanged(t *testing.T) {
	got, err := MergeJSONObjects([]byte("a not json"), []byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if want := "{\n  \"a\": 1\n}\n"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := MergeJSONObjectsForPath("opencode.jsonc", []byte("a not json"), []byte(`{"a":1}`)); err == nil {
		t.Fatal("malformed JSONC base must still be refused")
	}
}

func TestRemoveJSONAgentToolsKeepsBOM(t *testing.T) {
	body := "{\"agent\":{\"build\":{\"tools\":{\"x\":true},\"model\":\"m\"}},\"user\":1}"
	got, err := RemoveJSONAgentTools([]byte(utf8BOM+body), "build")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	plain, err := RemoveJSONAgentTools([]byte(body), "build")
	if err != nil {
		t.Fatalf("remove plain: %v", err)
	}
	if !bytes.Equal(got, append([]byte(utf8BOM), plain...)) || strings.Contains(string(got), "tools") {
		t.Fatalf("BOM output %q, plain %q", got, plain)
	}
}
