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

// TestWindowsInstallScriptChecksumCatchSurfacesRealError verifies that the catch
// block around checksum download in install.ps1 includes the real exception message
// ($_.Exception.Message) so the user sees the underlying cause, not a generic
// "Could not download checksums.txt" message that hides connection errors,
// TLS failures, etc. The secure-by-default behavior (Stop-WithError when not
// -Insecure) must be preserved.
func TestWindowsInstallScriptChecksumCatchSurfacesRealError(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.ps1")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	script := string(content)

	// The catch block must surface the real error via $_.Exception.Message.
	if !strings.Contains(script, "$_.Exception.Message") {
		t.Error("scripts/install.ps1 checksum catch block must include $_.Exception.Message to surface the real error; currently hides root cause")
	}

	// The catch block must still include the checksum URL so the user knows what failed.
	if !strings.Contains(script, "$checksumsUrl") {
		t.Error("scripts/install.ps1 checksum catch block must reference $checksumsUrl in the error message")
	}

	// The insecure skip path must still exist in the catch block.
	if !strings.Contains(script, "checksum verification skipped") {
		t.Error("scripts/install.ps1 checksum catch block must retain the -Insecure skip message")
	}

	// The secure-by-default path must still call Stop-WithError (hard failure).
	// Locate the catch block and confirm Stop-WithError is present.
	catchIdx := strings.Index(script, "} catch {")
	if catchIdx < 0 {
		t.Fatal("scripts/install.ps1 missing catch block around checksum download")
	}
	catchBlock := script[catchIdx:]
	// Find the closing brace of the catch block (next standalone "}" at indent 0 relative to catch).
	closeIdx := strings.Index(catchBlock, "\n        }")
	if closeIdx < 0 {
		t.Fatal("scripts/install.ps1 could not locate end of catch block")
	}
	catchBody := catchBlock[:closeIdx]

	if !strings.Contains(catchBody, "Stop-WithError") {
		t.Error("scripts/install.ps1 catch block must call Stop-WithError when not -Insecure; secure-by-default behavior must not be changed")
	}
}

// TestWindowsInstallScriptFallbackChecksumExecution wires the PowerShell
// fallback test (scripts/test-hash-fallback.ps1) into the CI validation
// suite. If pwsh or powershell is available on the machine, it executes
// the script to ensure the .NET fallback path computes correct SHA256
// hashes and matches Get-FileHash when both are available.
func TestWindowsInstallScriptFallbackChecksumExecution(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "scripts", "test-hash-fallback.ps1")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("test-hash-fallback.ps1 script not found at %s: %v", scriptPath, err)
	}

	var shell string
	if _, err := exec.LookPath("pwsh"); err == nil {
		shell = "pwsh"
	} else if _, err := exec.LookPath("powershell"); err == nil {
		shell = "powershell"
	} else {
		t.Skip("PowerShell (pwsh or powershell) not available; skipping fallback execution test")
	}

	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive", "-File", scriptPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell fallback test failed: %v\nOutput:\n%s", err, string(out))
	}
	t.Logf("PowerShell fallback test output:\n%s", string(out))
}

func TestInstallScriptAtomicBinaryReplacementStructure(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.sh")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	script := string(content)
	for _, want := range []string{
		"local stage_file=",
		"mv -f \"$stage_file\"",
		"cleanup_stage",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("scripts/install.sh is missing %q for atomic binary replacement", want)
		}
	}
}
