package pi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveReviewRouting(t *testing.T) {
	for _, tc := range []struct {
		name, config, model, thinking string
		invalid                       bool
	}{
		{"string", `{"review-refuter":"provider/model"}`, "provider/model", "", false},
		{"model", `{"review-refuter":{"model":" provider/model "}}`, "provider/model", "", false},
		{"independent", `{"review-refuter":{"model":"a","thinking":"max"},"review-validator":{"model":"b","thinking":"off"}}`, "a", "max", false},
		{"empty", `{"review-refuter":{}}`, "", "", false},
		{"missing", `{"unrelated":false}`, "", "", false},
		{"syntax", `{`, "", "", true},
		{"array", `[]`, "", "", true},
		{"null", `null`, "", "", true},
		{"bad selected", `{"review-refuter":false}`, "", "", true},
		{"bad model", `{"review-refuter":{"model":"bad model","thinking":"high"}}`, "", "", true},
		{"bad thinking", `{"review-refuter":{"thinking":"huge"}}`, "", "", true},
		{"unknown field", `{"review-refuter":{"typo":"a"}}`, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GENTLE_PI_CONFIG_HOME", t.TempDir())
			path := filepath.Join(os.Getenv("GENTLE_PI_CONFIG_HOME"), "models.json")
			if err := os.WriteFile(path, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := ResolveReviewRouting(t.TempDir(), "review-refuter")
			if (err != nil) != tc.invalid {
				t.Fatalf("routing error = %v", err)
			}
			if !tc.invalid && (got.Model != tc.model || got.Thinking != tc.thinking) {
				t.Fatalf("routing = %+v", got)
			}
		})
	}
}

func TestResolveReviewRoutingPrecedenceAndThinking(t *testing.T) {
	home, repo, custom := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GENTLE_PI_CONFIG_HOME", custom)
	write := func(dir, value string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(model, thinking string) {
		t.Helper()
		got, err := ResolveReviewRouting(repo, "review-validator")
		if err != nil || got.Model != model || got.Thinking != thinking {
			t.Fatalf("routing = %+v, %v", got, err)
		}
	}
	check("", "")
	write(filepath.Join(repo, ".pi", "gentle-ai"), `{"review-validator":"project"}`)
	check("project", "")
	for _, level := range []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"} {
		write(custom, `{"review-validator":{"thinking":"`+level+`"}}`)
		check("", level)
	}
	write(custom, `{}`)
	check("", "") // An existing global file never merges project entries.
	if err := os.Unsetenv("GENTLE_PI_CONFIG_HOME"); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(home, ".pi", "gentle-ai"), `{"review-validator":"home"}`)
	check("home", "")
}
