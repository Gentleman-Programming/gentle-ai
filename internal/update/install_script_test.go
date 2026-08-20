package update

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

func TestWindowsInstallScriptIsASCIIOnly(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.ps1")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	for i, b := range content {
		if b >= 0x80 {
			line := 1 + bytes.Count(content[:i], []byte("\n"))
			t.Fatalf("scripts/install.ps1 contains non-ASCII byte 0x%X at byte offset %d, line %d; Windows PowerShell 5.1 can misdecode UTF-8 without BOM when running powershell -File", b, i, line)
		}
	}
}

// TestWindowsInstallScriptHasNoUnsafeStringSubexpression guards against the
// PowerShell 5.1 parser failure reported in issue #849. Patterns like
// "($fileSize bytes)" inside a double-quoted string are read by Windows
// PowerShell 5.1 as an invalid subexpression and abort parsing before any code
// runs. Use the -f format operator instead, e.g. ("... {0} bytes" -f $fileSize).
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
		"prepend_go_env_pattern GONOSUMDB github.com/gentleman-programming/gentle-ai/v2",
		"prepend_go_env_pattern GOPRIVATE github.com/gentleman-programming/gentle-ai/v2",
		"prepend_go_env_pattern GONOPROXY github.com/gentleman-programming/gentle-ai/v2",
		"go install \"$go_package\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.sh is missing %q in beta go install proxy-bypass path", want)
		}
	}

	for _, clobber := range []string{
		"GONOSUMDB=github.com/gentleman-programming/gentle-ai/v2 \\",
		"GOPRIVATE=github.com/gentleman-programming/gentle-ai/v2 \\",
		"GONOPROXY=github.com/gentleman-programming/gentle-ai/v2 \\",
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
GONOPROXY=github.com/gentleman-programming/gentle-ai/v2
prepend_go_env_pattern GONOSUMDB github.com/gentleman-programming/gentle-ai/v2
prepend_go_env_pattern GOPRIVATE github.com/gentleman-programming/gentle-ai/v2
prepend_go_env_pattern GONOPROXY github.com/gentleman-programming/gentle-ai/v2
printf '%s\n%s\n%s\n' "$GONOSUMDB" "$GOPRIVATE" "$GONOPROXY"
`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run prepend_go_env_pattern fixture: %v\noutput: %s", err, out)
	}

	got := strings.TrimSpace(string(out))
	want := strings.Join([]string{
		"github.com/gentleman-programming/gentle-ai/v2,example.com/private",
		"github.com/gentleman-programming/gentle-ai/v2,github.com/acme/*",
		"github.com/gentleman-programming/gentle-ai/v2",
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
		"Add-GoEnvPattern -Name \"GONOSUMDB\" -Pattern \"github.com/gentleman-programming/gentle-ai/v2\"",
		"Add-GoEnvPattern -Name \"GOPRIVATE\" -Pattern \"github.com/gentleman-programming/gentle-ai/v2\"",
		"Add-GoEnvPattern -Name \"GONOPROXY\" -Pattern \"github.com/gentleman-programming/gentle-ai/v2\"",
		"& go install $goPackage",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.ps1 is missing %q in beta go install proxy-bypass path", want)
		}
	}

	for _, clobber := range []string{
		"$env:GONOSUMDB = \"github.com/gentleman-programming/gentle-ai/v2\"",
		"$env:GOPRIVATE = \"github.com/gentleman-programming/gentle-ai/v2\"",
		"$env:GONOPROXY = \"github.com/gentleman-programming/gentle-ai/v2\"",
	} {
		if strings.Contains(script, clobber) {
			t.Fatalf("scripts/install.ps1 clobbers existing user env with %q; beta proxy bypass must preserve existing patterns", clobber)
		}
	}

	start := strings.Index(script, "function Add-GoEnvPattern {")
	if start == -1 {
		t.Fatal("scripts/install.ps1 is missing Add-GoEnvPattern function")
	}
	endMarker := "\n}\n\nfunction Test-Installation"
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

func TestInstallScriptAtomicBinaryReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping Unix bash install script test on Windows")
	}

	path := filepath.Join("..", "..", "scripts", "install.sh")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	script := string(content)
	start := strings.Index(script, "    # Install binary\n")
	if start < 0 {
		t.Fatal("could not locate binary replacement block")
	}
	end := strings.Index(script[start:], "    success \"Installed ${BINARY_NAME}")
	if end < 0 {
		t.Fatal("could not locate end of binary replacement block")
	}
	replacement := script[start : start+end]

	tests := []struct {
		name        string
		failCommand string
		existing    bool
		fakeSudo    bool
		wantSuccess bool
	}{
		{name: "first install", wantSuccess: true},
		{name: "replacement", existing: true, wantSuccess: true},
		{name: "mktemp failure", failCommand: "mktemp", existing: true},
		{name: "cp failure", failCommand: "cp", existing: true},
		{name: "chmod failure", failCommand: "chmod", existing: true},
		{name: "mv failure", failCommand: "mv", existing: true},
		{name: "sudo fallback", failCommand: "cp", existing: true, fakeSudo: true, wantSuccess: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			installDir := filepath.Join(root, "install")
			tmpDir := filepath.Join(root, "download")
			fakeBin := filepath.Join(root, "bin")
			for _, dir := range []string{installDir, tmpDir, fakeBin} {
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			binary := filepath.Join(installDir, "gentle-ai")
			if tt.existing {
				if err := os.WriteFile(binary, []byte("old"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(tmpDir, "gentle-ai"), []byte("new"), 0o644); err != nil {
				t.Fatal(err)
			}

			for _, command := range []string{"cp", "chmod", "mv", "mktemp", "rm"} {
				realCommand, err := exec.LookPath(command)
				if err != nil {
					t.Fatal(err)
				}
				wrapper := fmt.Sprintf("#!/bin/sh\nif [ -z \"${INSTALL_TEST_PRIVILEGED:-}\" ] && [ \"$FAIL_COMMAND\" = %q ]; then exit 1; fi\nexec %q \"$@\"\n", command, realCommand)
				if err := os.WriteFile(filepath.Join(fakeBin, command), []byte(wrapper), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			sudoLog := filepath.Join(root, "sudo.log")
			if tt.fakeSudo {
				sudoScript := `#!/bin/sh
printf '%s\n' "$1" >> "$SUDO_LOG"
export INSTALL_TEST_PRIVILEGED=1
exec "$@"
`
				if err := os.WriteFile(filepath.Join(fakeBin, "sudo"), []byte(sudoScript), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			fixture := "set -eu\n" +
				"BINARY_NAME=gentle-ai\ninstall_dir=$1\ntmpdir=$2\nstage_file=\nsudo_stage_file=\n" +
				"info() { :; }\nwarn() { :; }\nfatal() { return 1; }\n" +
				"cleanup_install() { rm -f \"${stage_file:-}\" 2>/dev/null; [ -n \"${tmpdir:-}\" ] && rm -rf \"$tmpdir\"; }\n" +
				"trap cleanup_install EXIT\nreplace_binary() {\n" + replacement + "}\nreplace_binary\n"
			cmd := exec.Command("bash", "-c", fixture, "bash", installDir, tmpDir)
			env := append(os.Environ(), "PATH="+fakeBin, "FAIL_COMMAND="+tt.failCommand, "INSTALL_TEST_PRIVILEGED=")
			if tt.fakeSudo {
				env = append(env, "SUDO_LOG="+sudoLog)
			}
			cmd.Env = env
			out, runErr := cmd.CombinedOutput()
			if tt.wantSuccess && runErr != nil {
				t.Fatalf("replacement failed: %v\n%s", runErr, out)
			}
			if !tt.wantSuccess && runErr == nil {
				t.Fatalf("replacement unexpectedly succeeded when %s failed", tt.failCommand)
			}
			if tt.fakeSudo {
				delegated, err := os.ReadFile(sudoLog)
				if err != nil {
					t.Fatalf("read sudo command log: %v", err)
				}
				wantDelegated := "mktemp\ncp\nchmod\nmv"
				if got := strings.TrimSpace(string(delegated)); got != wantDelegated {
					t.Fatalf("sudo delegated commands = %q, want %q", got, wantDelegated)
				}
				sudoResidue, err := filepath.Glob(filepath.Join(installDir, ".gentle-ai.tmp.sudo.*"))
				if err != nil {
					t.Fatal(err)
				}
				if len(sudoResidue) != 0 {
					t.Fatalf("sudo staging residue remains: %v", sudoResidue)
				}
			}

			got, readErr := os.ReadFile(binary)
			if tt.wantSuccess {
				if readErr != nil {
					t.Fatal(readErr)
				}
				if string(got) != "new" {
					t.Fatalf("installed binary = %q, want %q", got, "new")
				}
				info, statErr := os.Stat(binary)
				if statErr != nil {
					t.Fatal(statErr)
				}
				if info.Mode()&0o111 == 0 {
					t.Fatal("installed binary is not executable")
				}
			} else {
				if !tt.existing {
					if !os.IsNotExist(readErr) {
						t.Fatalf("failed first install left binary error = %v", readErr)
					}
				} else {
					if readErr != nil {
						t.Fatal(readErr)
					}
					if string(got) != "old" {
						t.Fatalf("installed binary = %q, want old content preserved", got)
					}
				}
			}

			residue, globErr := filepath.Glob(filepath.Join(installDir, ".gentle-ai.tmp.*"))
			if globErr != nil {
				t.Fatal(globErr)
			}
			if len(residue) != 0 {
				t.Fatalf("staging residue remains: %v", residue)
			}
			if _, statErr := os.Stat(tmpDir); !os.IsNotExist(statErr) {
				t.Fatalf("download temp directory remains: %v", statErr)
			}
		})
	}
}
