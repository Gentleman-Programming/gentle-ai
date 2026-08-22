//go:build windows

package pi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeWindowsTransportInterpreter(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("interpreter"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWindowsTransportDispatchIsTypedUnsupported(t *testing.T) {
	_, err := RunBoundedModelRoutingProcess(context.Background(), validTransportPath(t), nil, validTransportOptions())
	requireTransportKind(t, err, TransportErrorUnsupportedPlatform)
	if errors.Is(err, &TransportError{Kind: TransportErrorInvalidPath}) || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("unsupported dispatch error=%v", err)
	}
}

func TestWindowsTransportCommandPlanNativeIsDirectAndCopySafe(t *testing.T) {
	executable := `C:\Program Files\safe & model.exe`
	environment := []string{"pAtH=C:\\first", "PATH=C:\\second", "AWS_SECRET=must-not-cross"}
	plan, err := newWindowsTransportCommandPlan(executable, environment)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable() != executable || len(plan.Arguments()) != 0 || plan.WorkingDirectory() != `C:\` || !reflect.DeepEqual(plan.Environment(), []string{"PATH=C:\\second"}) {
		t.Fatalf("plan=%+v; want direct executable, zero arguments, drive-root cwd, and filtered environment", plan)
	}
}

func TestWindowsTransportCommandPlanBatchUsesValidatedComSpec(t *testing.T) {
	root := t.TempDir()
	comspec := writeWindowsTransportInterpreter(t, filepath.Join(root, "preferred", "cmd.exe"))
	candidate := filepath.Join(root, "script path with spaces.CmD")
	plan, err := newWindowsTransportCommandPlan(candidate, []string{"sYsTeMrOoT=C:\\fallback", "cOmSpEc=" + comspec, "PATH=C:\\bin"})
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"/d", "/s", "/c", `""` + candidate + `""`}
	if plan.Executable() != comspec || !reflect.DeepEqual(plan.Arguments(), wantArgs) || plan.WorkingDirectory() != filepath.VolumeName(candidate)+string(filepath.Separator) {
		t.Fatalf("plan=%+v; want ComSpec, /d /s /c, preserved path, and volume-root cwd", plan)
	}
	args, env := plan.Arguments(), plan.Environment()
	args[3], env[0] = "mutated", "COMSPEC=mutated"
	if !reflect.DeepEqual(plan.Arguments(), wantArgs) || plan.Environment()[0] != "COMSPEC="+comspec {
		t.Fatal("plan accessors exposed mutable slices")
	}
}

func TestWindowsTransportCommandPlanRejectsUnsafeBatchPaths(t *testing.T) {
	base := filepath.Join(t.TempDir(), "candidate")
	for _, tc := range []struct{ name, value string }{
		{"percent", "%"}, {"exclamation", "!"}, {"ampersand", "&"}, {"pipe", "|"}, {"less-than", "<"}, {"greater-than", ">"},
		{"caret", "^"}, {"left-paren", "("}, {"right-paren", ")"}, {"double-quote", `"`}, {"single-quote", "'"},
		{"carriage-return", "\r"}, {"line-feed", "\n"}, {"nul", "\x00"}, {"tab-control", "\t"}, {"control", "\x01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newWindowsTransportCommandPlan(base+tc.value+".cmd", nil)
			requireTransportKind(t, err, TransportErrorInvalidPath)
		})
	}
}

func TestWindowsTransportCommandPlanUsesComSpecPrecedenceAndFallback(t *testing.T) {
	root := t.TempDir()
	preferred := writeWindowsTransportInterpreter(t, filepath.Join(root, "preferred", "cmd.exe"))
	fallbackRoot := t.TempDir()
	fallback := writeWindowsTransportInterpreter(t, filepath.Join(fallbackRoot, "System32", "cmd.exe"))
	candidate := filepath.Join(root, "script.cmd")
	for _, tc := range []struct {
		name, want string
		env        []string
	}{
		{"case-insensitive precedence", preferred, []string{"sYsTeMrOoT=" + fallbackRoot, "cOmSpEc=" + preferred}},
		{"validated SystemRoot fallback", fallback, []string{"sYsTeMrOoT=" + fallbackRoot}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := newWindowsTransportCommandPlan(candidate, tc.env)
			if err != nil || plan.Executable() != tc.want {
				t.Fatalf("plan=%+v error=%v; want interpreter %q", plan, err, tc.want)
			}
		})
	}
}

func TestWindowsTransportCommandPlanFailsClosedForInvalidInterpreters(t *testing.T) {
	root := t.TempDir()
	fallbackRoot := t.TempDir()
	writeWindowsTransportInterpreter(t, filepath.Join(fallbackRoot, "System32", "cmd.exe"))
	valid := writeWindowsTransportInterpreter(t, filepath.Join(root, "cmd.exe"))
	directory := filepath.Join(root, "directory", "cmd.exe")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, comspec string }{
		{"relative", "cmd.exe"}, {"empty", ""}, {"missing", filepath.Join(root, "missing", "cmd.exe")},
		{"unclean", valid + string(filepath.Separator) + "."}, {"wrong basename", filepath.Join(root, "powershell.exe")}, {"directory", directory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newWindowsTransportCommandPlan(filepath.Join(root, "script.bat"), []string{"SYSTEMROOT=" + fallbackRoot, "COMSPEC=" + tc.comspec})
			requireTransportKind(t, err, TransportErrorInvalidPath)
		})
	}
	for _, tc := range []struct {
		name, systemRoot string
		present          bool
	}{
		{"missing fallback", "", false}, {"relative fallback", "relative", true}, {"empty fallback", "", true}, {"missing cmd", t.TempDir(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			environment := []string(nil)
			if tc.present {
				environment = []string{"SYSTEMROOT=" + tc.systemRoot}
			}
			_, err := newWindowsTransportCommandPlan(filepath.Join(root, "script.bat"), environment)
			requireTransportKind(t, err, TransportErrorInvalidPath)
		})
	}
}

func TestWindowsTransportEnvironmentIsCanonicalAndSecretFree(t *testing.T) {
	entries := []string{"path=C:\\first", "PATH=C:\\second", "lAnG=en_US", "LANGUAGE=en", "comspec=C:\\one", "ComSpec=C:\\two", "SYSTEMROOT=C:\\Windows", "AWS_SECRET=hidden", "HOME=private", "malformed"}
	want := []string{"COMSPEC=C:\\two", "LANG=en_US", "LANGUAGE=en", "PATH=C:\\second", "SYSTEMROOT=C:\\Windows"}
	got := windowsTransportEnvironment(entries)
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(got, windowsTransportEnvironment(entries)) {
		t.Fatalf("environment=%q; want deterministic %q", got, want)
	}
	for _, tc := range []struct {
		name          string
		entries, want []string
	}{
		{"empty PATH", []string{"SYSTEMROOT=C:\\Windows", "PATH="}, []string{"PATH=C:\\Windows\\System32;C:\\Windows", "SYSTEMROOT=C:\\Windows"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := windowsTransportEnvironment(tc.entries); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("environment=%q; want %q", got, tc.want)
			}
		})
	}
}
