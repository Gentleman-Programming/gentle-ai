package update

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWindowsInstallScriptHasNoUTF8BOM(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.ps1")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("scripts/install.ps1 starts with UTF-8 BOM; PowerShell irm | iex treats BOM+#Requires as an invalid command")
	}
}

// TestWindowsInstallScriptHasNoUnsafeStringSubexpression guards against the
// PowerShell 5.1 parser failure reported in issue #849. Patterns like
// "($fileSize bytes)" inside a double-quoted string are read by Windows
// PowerShell 5.1 as an invalid subexpression and abort parsing before any code
// runs. Use the -f format operator instead, e.g. ("... {0} bytes" -f $fileSize).
func TestInstallScriptsRepairLegacyPathShadowing(t *testing.T) {
	checks := []struct {
		name     string
		path     string
		required []string
	}{
		{
			name: "unix",
			path: filepath.Join("..", "..", "scripts", "install.sh"),
			required: []string{
				"ensure_launcher_first_in_path",
				"persist_launcher_path_first",
				"Legacy installs can shadow the launcher",
				"This session now uses the Gentle AI launcher first",
			},
		},
		{
			name: "windows",
			path: filepath.Join("..", "..", "scripts", "install.ps1"),
			required: []string{
				"Ensure-LauncherFirstInPath",
				"SetEnvironmentVariable(\"PATH\", $newUserPath, \"User\")",
				"Legacy installs can shadow the launcher",
				"Moved the Gentle AI launcher to the front of your User PATH",
			},
		},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			content, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", tc.path, err)
			}
			for _, required := range tc.required {
				if !bytes.Contains(content, []byte(required)) {
					t.Fatalf("%s missing legacy PATH repair marker %q", tc.path, required)
				}
			}
		})
	}
}

func TestInstallScriptsActivateBetaThroughLauncher(t *testing.T) {
	checks := []struct {
		name      string
		path      string
		forbidden []string
		required  []string
	}{
		{
			name: "unix",
			path: filepath.Join("..", "..", "scripts", "install.sh"),
			forbidden: []string{
				"--channel beta installs Gentle AI from main and only supports --method go",
				"GENTLE_AI_CHANNEL=beta ${BINARY_NAME} install",
			},
			required: []string{
				"activate_requested_channel",
				"$launcher\" channel beta",
				"cmd/${BINARY_NAME}@latest",
				"install_channel_capable_launcher_from_main",
				"cmd/${BINARY_NAME}@main",
				"bootstrapping from main",
			},
		},
		{
			name: "windows",
			path: filepath.Join("..", "..", "scripts", "install.ps1"),
			forbidden: []string{
				"-Channel beta installs Gentle AI from main and only supports -Method go",
				"Install-ViaGo -Channel $Channel",
			},
			required: []string{
				"Activate-RequestedChannel",
				"& $launcher channel beta",
				"cmd/$BINARY_NAME@latest",
				"Install-ChannelCapableLauncherFromMain",
				"cmd/$BINARY_NAME@main",
				"bootstrapping from main",
			},
		},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			content, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", tc.path, err)
			}
			text := string(content)
			for _, forbidden := range tc.forbidden {
				if bytes.Contains(content, []byte(forbidden)) {
					t.Fatalf("%s still contains obsolete beta install behavior %q", tc.path, forbidden)
				}
			}
			for _, required := range tc.required {
				if !bytes.Contains(content, []byte(required)) {
					t.Fatalf("%s missing required launcher beta activation marker %q", tc.path, required)
				}
			}
			if bytes.Contains(content, []byte("@main")) && !bytes.Contains(content, []byte("channel beta")) {
				t.Fatalf("%s references @main without launcher channel activation", tc.path)
			}
			_ = text
		})
	}
}

func TestWindowsInstallScriptHasNoUnsafeStringSubexpression(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.ps1")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	// Match double-quoted strings, then flag any "($identifier <word>" inside
	// them. Scoping to quoted strings avoids false positives on real code such
	// as `foreach ($loc in $locations)`.
	stringLiteral := regexp.MustCompile(`"[^"]*"`)
	unsafeSubexpr := regexp.MustCompile(`\(\$[A-Za-z_][A-Za-z0-9_]*\s+[A-Za-z]`)

	for _, line := range bytes.Split(content, []byte("\n")) {
		for _, str := range stringLiteral.FindAll(line, -1) {
			if unsafeSubexpr.Match(str) {
				t.Errorf("scripts/install.ps1 contains an unsafe ($var word) string subexpression that breaks PowerShell 5.1 parsing: %s\nUse the -f format operator instead.", str)
			}
		}
	}
}

func TestInstallScriptBetaGoInstallBypassesPublicGoProxy(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.sh")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	script := string(content)
	for _, want := range []string{
		"prepend_go_env_pattern GONOSUMDB github.com/gentleman-programming/gentle-ai",
		"prepend_go_env_pattern GOPRIVATE github.com/gentleman-programming/gentle-ai",
		"prepend_go_env_pattern GONOPROXY github.com/gentleman-programming/gentle-ai",
		"go install \"$go_package\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.sh is missing %q in beta go install proxy-bypass path", want)
		}
	}

	for _, clobber := range []string{
		"GONOSUMDB=github.com/gentleman-programming/gentle-ai \\",
		"GOPRIVATE=github.com/gentleman-programming/gentle-ai \\",
		"GONOPROXY=github.com/gentleman-programming/gentle-ai \\",
	} {
		if strings.Contains(script, clobber) {
			t.Fatalf("scripts/install.sh clobbers existing user env with %q; beta proxy bypass must preserve existing patterns", clobber)
		}
	}

	start := strings.Index(script, "prepend_go_env_pattern() {")
	if start == -1 {
		t.Fatal("scripts/install.sh is missing prepend_go_env_pattern function")
	}
	endMarker := "\n}\n\n# ============================================================================\n# Install via binary download"
	end := strings.Index(script[start:], endMarker)
	if end == -1 {
		t.Fatal("could not locate end of prepend_go_env_pattern function")
	}
	function := script[start : start+end+3]

	cmd := exec.Command("bash", "-c", function+`
GONOSUMDB=example.com/private
GOPRIVATE=github.com/acme/*
GONOPROXY=github.com/gentleman-programming/gentle-ai
prepend_go_env_pattern GONOSUMDB github.com/gentleman-programming/gentle-ai
prepend_go_env_pattern GOPRIVATE github.com/gentleman-programming/gentle-ai
prepend_go_env_pattern GONOPROXY github.com/gentleman-programming/gentle-ai
printf '%s\n%s\n%s\n' "$GONOSUMDB" "$GOPRIVATE" "$GONOPROXY"
`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run prepend_go_env_pattern fixture: %v\noutput: %s", err, out)
	}

	got := strings.TrimSpace(string(out))
	want := strings.Join([]string{
		"github.com/gentleman-programming/gentle-ai,example.com/private",
		"github.com/gentleman-programming/gentle-ai,github.com/acme/*",
		"github.com/gentleman-programming/gentle-ai",
	}, "\n")
	if got != want {
		t.Fatalf("prepend_go_env_pattern output = %q, want %q", got, want)
	}
}

func TestWindowsInstallScriptBetaGoInstallPreservesGoProxyBypassEnv(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.ps1")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	script := string(content)
	for _, want := range []string{
		"Add-GoEnvPattern -Name \"GONOSUMDB\" -Pattern \"github.com/gentleman-programming/gentle-ai\"",
		"Add-GoEnvPattern -Name \"GOPRIVATE\" -Pattern \"github.com/gentleman-programming/gentle-ai\"",
		"Add-GoEnvPattern -Name \"GONOPROXY\" -Pattern \"github.com/gentleman-programming/gentle-ai\"",
		"& go install $goPackage",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.ps1 is missing %q in beta go install proxy-bypass path", want)
		}
	}

	for _, clobber := range []string{
		"$env:GONOSUMDB = \"github.com/gentleman-programming/gentle-ai\"",
		"$env:GOPRIVATE = \"github.com/gentleman-programming/gentle-ai\"",
		"$env:GONOPROXY = \"github.com/gentleman-programming/gentle-ai\"",
	} {
		if strings.Contains(script, clobber) {
			t.Fatalf("scripts/install.ps1 clobbers existing user env with %q; beta proxy bypass must preserve existing patterns", clobber)
		}
	}

	start := strings.Index(script, "function Add-GoEnvPattern {")
	if start == -1 {
		t.Fatal("scripts/install.ps1 is missing Add-GoEnvPattern function")
	}
	endMarker := "\n}\n\n# ============================================================================\n# Install via binary download"
	end := strings.Index(script[start:], endMarker)
	if end == -1 {
		t.Fatal("could not locate end of Add-GoEnvPattern function")
	}
	function := script[start : start+end+3]

	for _, want := range []string{
		"$current = [Environment]::GetEnvironmentVariable($Name, \"Process\")",
		"Set-Item -Path \"Env:$Name\" -Value $Pattern",
		"Set-Item -Path \"Env:$Name\" -Value (\"{0},{1}\" -f $Pattern, $current)",
		"if ($patterns -contains $Pattern) { return }",
	} {
		if !strings.Contains(function, want) {
			t.Fatalf("Add-GoEnvPattern does not preserve existing env patterns; missing %q", want)
		}
	}
}
