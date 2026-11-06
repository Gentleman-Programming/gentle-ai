package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func mustReplacePathTestScript(t *testing.T, script, anchor, replacement string) string {
	t.Helper()
	if !strings.Contains(script, anchor) {
		t.Fatalf("injection anchor %q not found in script", anchor)
	}
	return strings.ReplaceAll(script, anchor, replacement)
}

func TestPrioritizeUserPathPreservesLiteralSemantics(t *testing.T) {
	if runtime.GOOS != "windows" || testing.Short() {
		t.Skip("requires Windows registry and PowerShell")
	}
	host, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	const target = `C:\GentleAI-Literal-Test\bin`
	const other = `%GENTLE_AI_LITERAL_UNDEFINED%\tools`
	for _, kind := range []string{"String", "ExpandString"} {
		for _, tt := range []struct{ name, initial, want string }{
			{"padded", " " + target + " ;" + other, target + "; " + target + " ;" + other},
			{"quoted padded", ` "` + target + `" ;` + other, target + `; "` + target + `" ;` + other},
			{"quoted", `"` + target + `";` + other, target + ";" + other},
			{"already front", target + ";" + other, target + ";" + other},
		} {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				t.Setenv("PATH", `C:\GentleAI-Literal-Other`)
				key := fmt.Sprintf(`Software\GentleAI-PathTest-%d-%d`, os.Getpid(), time.Now().UnixNano())
				useWindowsUserPathSeam(t, userPathRunnerFunc(func(ctx context.Context, args ...string) ([]byte, error) {
					script := mustReplacePathTestScript(t, args[len(args)-1], `CreateSubKey('Environment')`, `CreateSubKey('`+key+`')`)
					if strings.Contains(script, `SubKey('Environment'`) {
						t.Fatal("unsafe registry script")
					}
					// Count only production writes; stop notification before compilation/broadcast.
					script = mustReplacePathTestScript(t, script, "$pathKey.SetValue", "$global:writes++; $pathKey.SetValue")
					script = mustReplacePathTestScript(t, script, "Add-Type -TypeDefinition", "$global:notifications++; throw 'injected notification failure'; Add-Type -TypeDefinition")
					wrapped := fmt.Sprintf(`$ErrorActionPreference = 'Stop'; $global:writes = 0; $global:notifications = 0; $fixture = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('%s'); try { $fixture.SetValue('Path', '%s', [Microsoft.Win32.RegistryValueKind]::%s); & { %s }; $actual = $fixture.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); if ($actual -cne '%s') { throw "raw PATH mismatch: $actual" }; if ($fixture.GetValueKind('Path').ToString() -ne '%s') { throw 'kind mismatch' }; if ($global:writes -ne 1 -or $global:notifications -ne 1) { throw "side effects: writes=$global:writes notifications=$global:notifications" } } finally { $fixture.Close(); [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree('%s') }`, key, escapePowerShellString(tt.initial), kind, script, escapePowerShellString(tt.want), kind, key)
					cmd := exec.CommandContext(ctx, host, "-NoProfile", "-NonInteractive", "-Command", wrapped)
					var stdout, stderr strings.Builder
					cmd.Stdout, cmd.Stderr = &stdout, &stderr
					err := cmd.Run()
					if err != nil || cmd.ProcessState.ExitCode() != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
						t.Fatalf("PowerShell: error=%v exit=%d stdout=%q stderr=%q", err, cmd.ProcessState.ExitCode(), stdout.String(), stderr.String())
					}
					return []byte(stdout.String()), nil
				}))
				if err := PrioritizeUserPath(target); err != nil {
					t.Fatal(err)
				}
				if got, want := os.Getenv("PATH"), target+`;C:\GentleAI-Literal-Other`; got != want {
					t.Fatalf("process PATH = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestUserPathMatchesExpandedEntries(t *testing.T) {
	if runtime.GOOS != "windows" || testing.Short() {
		t.Skip("requires Windows registry and PowerShell")
	}
	host, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	const target = `C:\GentleAI-Expanded-Test\bin`
	const entry = `%GENTLE_AI_PATH_EXPANDED_TEST%\bin`
	const other = `%GENTLE_AI_PATH_UNDEFINED_TEST%\tools; C:\Other\bin `
	t.Setenv("GENTLE_AI_PATH_EXPANDED_TEST", `C:\GentleAI-Expanded-Test`)
	for _, kind := range []string{"String", "ExpandString"} {
		for _, operation := range []string{"add", "remove", "prioritize"} {
			t.Run(kind+"/"+operation, func(t *testing.T) {
				t.Setenv("PATH", `C:\GentleAI-Expanded-Other`)
				initial, want := other+";"+entry, other+";"+entry
				if operation == "remove" {
					want = other
				} else if operation == "prioritize" {
					want = entry + ";" + other
				}
				key := fmt.Sprintf(`Software\GentleAI-PathTest-%d-%d`, os.Getpid(), time.Now().UnixNano())
				useWindowsUserPathSeam(t, userPathRunnerFunc(func(ctx context.Context, args ...string) ([]byte, error) {
					script := mustReplacePathTestScript(t, args[len(args)-1], `CreateSubKey('Environment')`, `CreateSubKey('`+key+`')`)
					if strings.Contains(script, `SubKey('Environment'`) {
						t.Fatal("unsafe registry script")
					}
					if operation == "add" {
						script = mustReplacePathTestScript(t, script, "$pathKey.SetValue", "throw 'no-op wrote'; $pathKey.SetValue")
						script = mustReplacePathTestScript(t, script, "Add-Type -TypeDefinition", "$global:notificationInvoked = $true; throw 'notification invoked'; Add-Type -TypeDefinition")
					}
					wrapped := fmt.Sprintf(`$ErrorActionPreference = 'Stop'; $fixture = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('%s'); try { $fixture.SetValue('Path', '%s', [Microsoft.Win32.RegistryValueKind]::%s); & { %s }; $actual = $fixture.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); if ($actual -cne '%s') { throw "raw PATH mismatch: $actual" }; if ($fixture.GetValueKind('Path').ToString() -ne '%s') { throw 'kind mismatch' }; if ($global:notificationInvoked) { throw 'no-op notified' } } finally { $fixture.Close(); [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree('%s') }`, key, initial, kind, script, want, kind, key)
					out, err := exec.CommandContext(ctx, host, "-NoProfile", "-NonInteractive", "-Command", wrapped).CombinedOutput()
					if err != nil {
						return out, fmt.Errorf("registry probe: %w: %s", err, out)
					}
					wantOutput := ""
					if operation == "add" {
						wantOutput = "unchanged"
					}
					if strings.TrimSpace(string(out)) != wantOutput {
						t.Fatalf("output = %q, want %q", out, wantOutput)
					}
					return out, nil
				}))
				var err error
				switch operation {
				case "add":
					var addition UserPathAddition
					addition, err = AddToUserPathWithResult(target)
					if err == nil && (addition.PersistentAdded || !addition.ProcessAdded) {
						t.Fatalf("unexpected ownership: %+v", addition)
					}
				case "remove":
					err = RemoveFromUserPath(target)
				case "prioritize":
					err = PrioritizeUserPath(target)
				}
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestUserPathScriptInjectionRequiresAnchor(t *testing.T) {
	const childFlag = "GENTLE_AI_PATH_MISSING_ANCHOR_TEST"
	if os.Getenv(childFlag) == "1" {
		mustReplacePathTestScript(t, "unchanged script", "missing anchor", "injected failure")
		return
	}
	if got := mustReplacePathTestScript(t, "before anchor after", "anchor", "injected"); got != "before injected after" {
		t.Fatalf("replacement = %q", got)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestUserPathScriptInjectionRequiresAnchor$", "-test.count=1")
	cmd.Env = append(os.Environ(), childFlag+"=1")
	output, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("missing anchor: error = %v, output = %s; want exit 1", err, output)
	}
	if !strings.Contains(string(output), `injection anchor "missing anchor" not found in script`) {
		t.Fatalf("missing anchor diagnostic not found: %s", output)
	}
}

func TestUserPathNotificationFailureRetainsRollbackOwnership(t *testing.T) {
	if runtime.GOOS != "windows" || testing.Short() {
		t.Skip("requires Windows registry and PowerShell")
	}
	host, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []string{"Add-Type", "invocation"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("PATH", os.Getenv("PATH"))
			const target = `C:\GentleAI-5581\bin`
			const raw = `%USERPROFILE%\bin;%UNDEFINED_5581%\tools`
			key := fmt.Sprintf(`Software\GentleAI-PathTest-%d-%d`, os.Getpid(), time.Now().UnixNano())
			run := func(script string) ([]byte, error) {
				return exec.Command(host, "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
			}
			t.Cleanup(func() {
				out, err := run(fmt.Sprintf(`try {} finally { [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree('%s', $false) }`, key))
				if err != nil {
					t.Errorf("fixture cleanup: %v: %s", err, out)
				}
			})
			calls := 0
			useWindowsUserPathSeam(t, userPathRunnerFunc(func(ctx context.Context, args ...string) ([]byte, error) {
				calls++
				script := strings.ReplaceAll(args[len(args)-1], `CreateSubKey('Environment')`, `CreateSubKey('`+key+`')`)
				if strings.Contains(script, `SubKey('Environment'`) {
					t.Fatal("unsafe registry script")
				}
				if failure == "Add-Type" {
					script = mustReplacePathTestScript(t, script, "Add-Type -TypeDefinition", "throw 'injected Add-Type failure'; Add-Type -TypeDefinition")
				} else {
					script = mustReplacePathTestScript(t, script, `[void][GentleAIPathNotification]::SendMessageTimeout`, `throw 'injected notification failure'; [void][GentleAIPathNotification]::SendMessageTimeout`)
				}
				setup, want, output := "", raw, ""
				if calls == 1 {
					setup = fmt.Sprintf(`$fixture.SetValue('Path', '%s', [Microsoft.Win32.RegistryValueKind]::ExpandString); `, raw)
					want, output = target+";"+raw, "changed"
				}
				wrapped := fmt.Sprintf(`$ErrorActionPreference = 'Stop'; $fixture = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('%s'); try { %s & { %s }; if ($fixture.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) -cne '%s') { throw 'unexpected persisted PATH' }; if ($fixture.GetValueKind('Path').ToString() -ne 'ExpandString') { throw 'unexpected kind' } } finally { $fixture.Close() }`, key, setup, script, want)
				out, err := exec.CommandContext(ctx, host, "-NoProfile", "-NonInteractive", "-Command", wrapped).CombinedOutput()
				if err != nil {
					return out, fmt.Errorf("registry probe: %w: %s", err, out)
				}
				if strings.TrimSpace(string(out)) != output {
					t.Fatalf("output = %q, want %q", out, output)
				}
				return out, nil
			}))
			addition, err := AddToUserPathWithResult(target)
			if err != nil {
				t.Fatal(err)
			}
			if !addition.PersistentAdded {
				t.Fatalf("lost ownership: %+v", addition)
			}
			if err := RollbackUserPathAddition(target, addition); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("calls = %d, want add and rollback", calls)
			}
		})
	}
}

// Execute the public operations' actual PowerShell against an isolated registry
// key. Legacy Environment calls are redirected too, so RED never edits real PATH.
func TestUserPathPreservesRegistryValue(t *testing.T) {
	if runtime.GOOS != "windows" || testing.Short() {
		t.Skip("requires Windows registry and PowerShell")
	}
	host, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	const target = `C:\O'Brien\bin`
	const raw = `%USERPROFILE%\bin;%GENTLE_AI_UNDEFINED_5581%\tools`
	for _, kind := range []string{"ExpandString", "String", "missing", "empty"} {
		for _, operation := range []string{"add", "add existing", "remove", "remove absent", "prioritize", "rollback"} {
			t.Run(kind+"/"+operation, func(t *testing.T) {
				t.Setenv("PATH", os.Getenv("PATH"))
				initial, want := raw, target+";"+raw
				if operation != "add" {
					initial = raw + ";" + target
				}
				if operation == "add existing" {
					want = initial
				}
				if operation == "remove" || operation == "rollback" {
					want = raw
				}
				if operation == "remove absent" {
					initial, want = raw, raw
				}
				wantKind := kind
				if kind == "missing" || kind == "empty" {
					initial = ""
					wantKind = "ExpandString"
					want = target
					if operation == "remove" || operation == "remove absent" || operation == "rollback" {
						want = ""
					}
				}
				key := fmt.Sprintf(`Software\GentleAI-PathTest-%d-%d`, os.Getpid(), time.Now().UnixNano())
				useWindowsUserPathSeam(t, userPathRunnerFunc(func(ctx context.Context, args ...string) ([]byte, error) {
					script := args[len(args)-1]
					// Redirect the old API for the regression's RED run.
					script = strings.ReplaceAll(script, `[Environment]::GetEnvironmentVariable('PATH', 'User')`, `$fixture.GetValue('Path')`)
					script = strings.ReplaceAll(script, `[Environment]::SetEnvironmentVariable('PATH', `, `$fixture.SetValue('Path', `)
					script = strings.ReplaceAll(script, `, 'User')`, `, [Microsoft.Win32.RegistryValueKind]::String)`)
					// Redirect the fixed registry implementation, not the real Environment key.
					script = strings.ReplaceAll(script, `CreateSubKey('Environment')`, `CreateSubKey('`+key+`')`)
					if strings.Contains(script, `GetEnvironmentVariable`) || strings.Contains(script, `SetEnvironmentVariable`) || strings.Contains(script, `SubKey('Environment'`) {
						t.Fatal("unsafe registry script: fixture redirection incomplete")
					}
					setup := ""
					if kind != "missing" {
						setup = fmt.Sprintf(`$fixture.SetValue('Path', '%s', [Microsoft.Win32.RegistryValueKind]::%s); `, escapePowerShellString(initial), wantKind)
					}
					check := fmt.Sprintf(`if ($actual -cne '%s') { throw "raw PATH mismatch: $actual" }; if ($fixture.GetValueKind('Path').ToString() -ne '%s') { throw "kind mismatch" }`, escapePowerShellString(want), wantKind)
					noRemoval := operation == "remove absent" || ((kind == "missing" || kind == "empty") && (operation == "remove" || operation == "rollback"))
					if kind == "missing" && noRemoval {
						check = `if ($fixture.GetValueNames() -contains 'Path') { throw 'absent Path was created' }`
					}
					if noRemoval {
						script = mustReplacePathTestScript(t, script, "Add-Type -TypeDefinition", "$global:notificationInvoked = $true; throw 'notification invoked'; Add-Type -TypeDefinition")
						check += `; if ($global:notificationInvoked) { throw 'no-op notified' }`
					}
					wrapped := fmt.Sprintf(`$ErrorActionPreference = 'Stop'; $fixture = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('%s'); try { %s & { %s }; $actual = $fixture.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); %s } finally { $fixture.Close(); [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree('%s') }`, key, setup, script, check, key)
					cmd := exec.CommandContext(ctx, host, "-NoProfile", "-NonInteractive", "-Command", wrapped)
					out, err := cmd.CombinedOutput()
					if err != nil {
						return out, fmt.Errorf("registry probe: %w: %s", err, out)
					}
					wantOutput := ""
					if operation == "add" || operation == "add existing" {
						wantOutput = "changed"
						if operation == "add existing" && kind != "missing" && kind != "empty" {
							wantOutput = "unchanged"
						}
					}
					if strings.TrimSpace(string(out)) != wantOutput {
						t.Fatalf("PowerShell output = %q, want %q", out, wantOutput)
					}
					return out, nil
				}))
				var err error
				switch operation {
				case "add", "add existing":
					var addition UserPathAddition
					addition, err = AddToUserPathWithResult(target)
					if err == nil && addition.PersistentAdded != (operation == "add" || kind == "missing" || kind == "empty") {
						t.Fatalf("unexpected ownership: %+v", addition)
					}
				case "remove", "remove absent":
					err = RemoveFromUserPath(target)
				case "prioritize":
					err = PrioritizeUserPath(target)
				case "rollback":
					err = RollbackUserPathAddition(target, UserPathAddition{PersistentAdded: true})
				}
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
