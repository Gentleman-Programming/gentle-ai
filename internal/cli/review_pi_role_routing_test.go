package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
)

func TestPiCaptureWindowsFallbackPreservesParentTest(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only fallback; requires Windows execution")
	}
	for _, role := range []reviewerprovider.Role{reviewerprovider.RoleRefuter, reviewerprovider.RoleTargetedValidator} {
		continued := false
		t.Run(string(role), func(t *testing.T) {
			raw := []byte("original fake adapter result")
			assertRoutedPiCapture(t, "", role, raw, nil)
			continued = true
			adapter, err := reviewProviderRoleHostAdapter(role, "")
			if err != nil {
				t.Fatal(err)
			}
			got, err := adapter.Review(t.Context(), reviewerprovider.NewInvocation(nil))
			if err != nil || !bytes.Equal(got, raw) {
				t.Fatalf("fallback result = %q, %v", got, err)
			}
		})
		if !continued {
			t.Errorf("%s helper skipped the parent capture assertions", role)
		}
	}
}

// Keep the real resolver and transport where the POSIX fixture is supported.
func assertRoutedPiCapture(t *testing.T, repo string, wantRole reviewerprovider.Role, raw []byte, execute []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Retain the platform-neutral capture/admission assertions in callers.
		overrideProviderRoleHostAdapter(t, providerTestAdapter{raw: raw})
		return
	}
	dir := t.TempDir()
	t.Setenv("GENTLE_PI_CONFIG_HOME", dir)
	config := filepath.Join(dir, "models.json")
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	saved := []byte(`{"review-refuter":{"model":"provider/refuter","thinking":"max"},"review-validator":{"model":"provider/validator","thinking":"off"}}`)
	write(config, saved)
	executable, argv, response := filepath.Join(dir, "pi"), filepath.Join(dir, "argv"), filepath.Join(dir, "response")
	write(response, raw)
	write(executable, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+strconv.Quote(argv)+"\ncat > "+strconv.Quote(filepath.Join(dir, "stdin"))+"\ncat "+strconv.Quote(response)+"\n"))
	previous := reviewProviderRoleHostAdapter
	t.Cleanup(func() { reviewProviderRoleHostAdapter = previous })
	reviewProviderRoleHostAdapter = func(role reviewerprovider.Role, root string) (reviewerprovider.Adapter, error) {
		if role != wantRole || root != repo {
			t.Fatalf("routing context = %s %s", role, root)
		}
		adapter, err := previous(role, root)
		if err != nil {
			return nil, err
		}
		adapter.(*reviewerprovider.PiAdapter).LookPath = func(string) (string, error) { return executable, nil }
		return adapter, nil
	}
	materialize := slices.Clone(execute)
	materialize[slices.Index(materialize, "--execute=true")] = "--materialize=true"
	var prompt bytes.Buffer
	if err := RunReview(materialize, &prompt); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{`, `{"review-refuter":{"thinking":"invalid"},"review-validator":{"model":false}}`} {
		write(config, []byte(invalid))
		if err := RunReview(execute, io.Discard); err == nil || !strings.Contains(err.Error(), "invalid Pi routing") {
			t.Fatalf("malformed routing capture = %v", err)
		}
		if _, err := os.Stat(argv); !os.IsNotExist(err) {
			t.Fatalf("malformed routing spawned Pi: %v", err)
		}
	}
	baseArgs := "--print\n--mode\ntext\n--no-session\n--no-tools\n--no-extensions\n--no-skills\n--no-prompt-templates\n--no-themes\n--no-context-files\n--no-approve\n"
	for _, tc := range []struct{ assignment, flags string }{
		{`"provider/only"`, "--model\nprovider/only\n"},
		{`{"thinking":"off"}`, "--thinking\noff\n"},
		{`{}`, ""},
		{"", ""},
	} {
		data := `{}`
		if tc.assignment != "" {
			data = `{"review-refuter":` + tc.assignment + `,"review-validator":` + tc.assignment + `}`
		}
		write(config, []byte(data))
		adapter, err := reviewProviderRoleHostAdapter(wantRole, repo)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := adapter.Review(t.Context(), reviewerprovider.NewInvocation(prompt.Bytes())); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(argv)
		if err != nil || string(got) != baseArgs+tc.flags {
			t.Fatalf("assignment %s argv = %q, %v", data, got, err)
		}
	}
	write(config, saved)
	t.Cleanup(func() {
		stdin, err := os.ReadFile(filepath.Join(dir, "stdin"))
		if err != nil || !bytes.Equal(stdin, prompt.Bytes()) {
			t.Errorf("capture changed materialized stdin: %v", err)
		}
		got, err := os.ReadFile(argv)
		suffix := "--model\nprovider/refuter\n--thinking\nmax\n"
		if wantRole == reviewerprovider.RoleTargetedValidator {
			suffix = "--model\nprovider/validator\n--thinking\noff\n"
		}
		if err != nil || string(got) != baseArgs+suffix {
			t.Errorf("subprocess argv = %q, %v; want %q", got, err, baseArgs+suffix)
		}
		unchanged, err := os.ReadFile(config)
		if err != nil || !bytes.Equal(unchanged, saved) {
			t.Errorf("capture changed routing config: %v", err)
		}
	})
}
