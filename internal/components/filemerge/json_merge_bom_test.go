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

// Repeated install/sync runs over a BOM file must neither stack BOMs nor lose
// user keys, whatever precedes the root object.
func TestBOMSurvivesRepeatedMergesWithoutStacking(t *testing.T) {
	for _, tc := range []struct{ name, path, body string }{
		{"json", "settings.json", `{"user":"kept"}`},
		{"json leading whitespace", "settings.json", "\n\t {\"user\":\"kept\"}\n"},
		{"json leading comment", "settings.json", "// note\n{\"user\":\"kept\",}\n"},
		{"json empty body", "settings.json", ""},
		{"jsonc", "opencode.jsonc", `{"user":"kept"}`},
		{"jsonc leading whitespace", "opencode.jsonc", "\n\t {\"user\":\"kept\"}\n"},
		{"jsonc leading comment", "opencode.jsonc", "// note\n{\"user\":\"kept\",}\n"},
		{"jsonc empty body", "opencode.jsonc", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := []byte(utf8BOM + tc.body)
			for run := 1; run <= 3; run++ {
				merged, err := MergeJSONObjectsForPath(tc.path, current, []byte(`{"theme":"gentleman"}`))
				if err != nil {
					t.Fatalf("run %d merge: %v", run, err)
				}
				current, err = MergeJSONDefaultsForPath(tc.path, merged, []byte(`{"share":"disabled"}`))
				if err != nil {
					t.Fatalf("run %d defaults: %v", run, err)
				}
				if !bytes.HasPrefix(current, []byte(utf8BOM)) || bytes.Count(current, []byte(utf8BOM)) != 1 {
					t.Fatalf("run %d: want exactly one leading BOM:\n%q", run, current)
				}
				root, err := UnmarshalJSONObject(current)
				if err != nil {
					t.Fatalf("run %d: output does not parse: %v\n%q", run, err, current)
				}
				if tc.body != "" && root["user"] != "kept" {
					t.Fatalf("run %d: user key lost:\n%q", run, current)
				}
				if root["theme"] != "gentleman" || root["share"] != "disabled" {
					t.Fatalf("run %d: managed keys missing:\n%q", run, current)
				}
			}
		})
	}
}

// Writers prepend the BOM to a fresh slice: mutating their output must never
// reach the caller's base, even when the base has spare capacity, nor the
// package BOM.
func TestBOMOutputDoesNotAliasBaseOrBOM(t *testing.T) {
	body := `{"user":"kept"}`
	spare := make([]byte, 0, 4096)
	spare = append(spare, utf8BOM+body...)
	for _, tc := range []struct {
		name string
		base []byte
		run  func([]byte) ([]byte, error)
	}{
		{"json", []byte(utf8BOM + body), func(b []byte) ([]byte, error) {
			return MergeJSONObjectsForPath("settings.json", b, []byte(`{"theme":"x"}`))
		}},
		{"json spare capacity", spare, func(b []byte) ([]byte, error) {
			return MergeJSONObjectsForPath("settings.json", b, []byte(`{"theme":"x"}`))
		}},
		{"jsonc spare capacity", spare, func(b []byte) ([]byte, error) {
			return MergeJSONObjectsForPath("opencode.jsonc", b, []byte(`{"theme":"x"}`))
		}},
		{"defaults", []byte(utf8BOM + body), func(b []byte) ([]byte, error) {
			return MergeJSONDefaultsForPath("settings.json", b, []byte(`{"theme":"x"}`))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]byte{}, tc.base...)
			got, err := tc.run(tc.base)
			if err != nil {
				t.Fatalf("merge: %v", err)
			}
			for i := range got {
				got[i] = 'X'
			}
			_ = append(got[:0], "overwrite"...)
			if !bytes.Equal(tc.base, before) {
				t.Fatalf("base changed through output:\n%q", tc.base)
			}
			if string(jsonBOM) != utf8BOM {
				t.Fatalf("package BOM changed through output: %q", jsonBOM)
			}
		})
	}
}

// Every JSONC refusal returns the original document, BOM included, so a
// caller that writes the returned bytes leaves the user's file unchanged.
func TestMergeOpenCodeJSONCObjectsRefusalReturnsBOMBase(t *testing.T) {
	for _, tc := range []struct{ name, body, overlay string }{
		{"duplicate keys", "{\"a\":1,\"a\":2}", `{"theme":"x"}`},
		{"malformed", "{\"a\":", `{"theme":"x"}`},
		{"nested comments", "{\"theme\":{// note\n\"a\":1}}", `{"theme":{"b":2}}`},
		{"escaped key", "{\"\\u0074heme\":\"old\"}", `{"theme":"x"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := []byte(utf8BOM + tc.body)
			got, err := MergeOpenCodeJSONCObjects(base, []byte(tc.overlay))
			if err == nil {
				t.Fatalf("want refusal, got %q", got)
			}
			if !bytes.Equal(got, []byte(utf8BOM+tc.body)) {
				t.Fatalf("refusal returned %q, want original %q", got, utf8BOM+tc.body)
			}
		})
	}
}
