package system

import (
	"strings"
	"testing"
)

func TestSanitizeCommandEnvironment_Deduplication(t *testing.T) {
	base := []string{
		"FOO=first",
		"foo=second",
		"BAR=val1",
		"Bar=val2",
	}
	got := SanitizeCommandEnvironment(base, nil, nil)
	envMap := envSliceToMap(got)

	if len(envMap) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(envMap), got)
	}
	if envMap["FOO"] != "second" {
		t.Errorf("expected FOO to be 'second', got %q", envMap["FOO"])
	}
	if envMap["BAR"] != "val2" {
		t.Errorf("expected BAR to be 'val2', got %q", envMap["BAR"])
	}
}

func TestSanitizeCommandEnvironment_HazardousKeysStripped(t *testing.T) {
	base := []string{
		"SAFE_VAR=ok",
		"NODE_OPTIONS=--inspect",
		"node_path=/usr/lib/node_modules",
		"GIT_DIR=.git",
		"git_work_tree=/tmp",
		"Git_Index_File=/tmp/index",
	}
	passThrough := []string{"NODE_OPTIONS", "GIT_DIR", "SAFE_VAR"}
	overrides := map[string]string{
		"NODE_PATH": "/injected/node_modules",
	}

	got := SanitizeCommandEnvironment(base, passThrough, overrides)
	envMap := envSliceToMap(got)

	hazardous := []string{"NODE_OPTIONS", "NODE_PATH", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"}
	for _, h := range hazardous {
		if _, exists := envMap[h]; exists {
			t.Errorf("hazardous key %s was not stripped: %v", h, got)
		}
	}
	if envMap["SAFE_VAR"] != "ok" {
		t.Errorf("SAFE_VAR should be preserved, got %q", envMap["SAFE_VAR"])
	}
}

func TestSanitizeCommandEnvironment_Overrides(t *testing.T) {
	base := []string{
		"FOO=original",
		"BAR=keep",
	}
	overrides := map[string]string{
		"foo": "overridden",
		"BAZ": "new_val",
	}

	got := SanitizeCommandEnvironment(base, nil, overrides)
	envMap := envSliceToMap(got)

	if envMap["FOO"] != "overridden" {
		t.Errorf("expected FOO to be 'overridden', got %q", envMap["FOO"])
	}
	if envMap["BAR"] != "keep" {
		t.Errorf("expected BAR to be 'keep', got %q", envMap["BAR"])
	}
	if envMap["BAZ"] != "new_val" {
		t.Errorf("expected BAZ to be 'new_val', got %q", envMap["BAZ"])
	}
}

func TestSanitizeCommandEnvironment_PassThroughFiltering(t *testing.T) {
	base := []string{
		"KEEP_ME=yes",
		"DROP_ME=no",
		"Case_Insensitive=yes",
	}
	passThrough := []string{"keep_me", "CASE_INSENSITIVE"}

	got := SanitizeCommandEnvironmentForOS(base, passThrough, nil, "linux")
	envMap := envSliceToMap(got)

	if envMap["KEEP_ME"] != "yes" {
		t.Errorf("expected KEEP_ME to be preserved, got %q", envMap["KEEP_ME"])
	}
	if envMap["CASE_INSENSITIVE"] != "yes" {
		t.Errorf("expected CASE_INSENSITIVE to be preserved, got %q", envMap["CASE_INSENSITIVE"])
	}
	if _, exists := envMap["DROP_ME"]; exists {
		t.Errorf("expected DROP_ME to be filtered out, got %v", got)
	}

	// Empty non-nil passThrough slice filters out all non-essential keys
	gotEmpty := SanitizeCommandEnvironmentForOS(base, []string{}, nil, "linux")
	if len(gotEmpty) != 0 {
		t.Errorf("expected empty result for empty passThrough slice on linux, got %v", gotEmpty)
	}
}

func TestSanitizeCommandEnvironment_WindowsEssentialRetention(t *testing.T) {
	base := []string{
		"COMSPEC=C:\\Windows\\system32\\cmd.exe",
		"PATH=C:\\Windows;C:\\Windows\\system32",
		"PathExt=.COM;.EXE;.BAT;.CMD",
		"SystemDrive=C:",
		"SystemRoot=C:\\Windows",
		"TEMP=C:\\Users\\user\\AppData\\Local\\Temp",
		"TMP=C:\\Users\\user\\AppData\\Local\\Temp",
		"TMPDIR=C:\\Users\\user\\AppData\\Local\\Temp",
		"windir=C:\\Windows",
		"USERPROFILE=C:\\Users\\user",
		"APPDATA=C:\\Users\\user\\AppData\\Roaming",
		"LOCALAPPDATA=C:\\Users\\user\\AppData\\Local",
		"ALLUSERSPROFILE=C:\\ProgramData",
		"ProgramData=C:\\ProgramData",
		"ProgramFiles=C:\\Program Files",
		"ProgramFiles(x86)=C:\\Program Files (x86)",
		"UNWANTED_VAR=discard",
		"CUSTOM_ALLOWED=keep",
	}

	passThrough := []string{"CUSTOM_ALLOWED"}
	got := SanitizeCommandEnvironmentForOS(base, passThrough, nil, "windows")
	envMap := envSliceToMap(got)

	windowsEssentials := []string{
		"COMSPEC", "PATH", "PATHEXT", "SYSTEMDRIVE", "SYSTEMROOT",
		"TEMP", "TMP", "TMPDIR", "WINDIR", "USERPROFILE",
		"APPDATA", "LOCALAPPDATA", "ALLUSERSPROFILE", "PROGRAMDATA",
		"PROGRAMFILES", "PROGRAMFILES(X86)",
	}

	for _, k := range windowsEssentials {
		if _, exists := envMap[k]; !exists {
			t.Errorf("Windows essential key %s missing from environment: %v", k, got)
		}
	}

	if envMap["CUSTOM_ALLOWED"] != "keep" {
		t.Errorf("CUSTOM_ALLOWED should be kept, got %q", envMap["CUSTOM_ALLOWED"])
	}
	if _, exists := envMap["UNWANTED_VAR"]; exists {
		t.Errorf("UNWANTED_VAR should be filtered out, got %v", got)
	}
}

func TestSanitizeCommandEnvironment_BidirectionalHomeUserProfileSync(t *testing.T) {
	t.Run("Windows syncs HOME from USERPROFILE", func(t *testing.T) {
		base := []string{"USERPROFILE=C:\\Users\\alice"}
		got := SanitizeCommandEnvironmentForOS(base, nil, nil, "windows")
		envMap := envSliceToMap(got)
		if envMap["HOME"] != "C:\\Users\\alice" {
			t.Errorf("expected HOME to be synced from USERPROFILE, got %q", envMap["HOME"])
		}
		if envMap["USERPROFILE"] != "C:\\Users\\alice" {
			t.Errorf("expected USERPROFILE to remain, got %q", envMap["USERPROFILE"])
		}
	})

	t.Run("Windows syncs USERPROFILE from HOME", func(t *testing.T) {
		base := []string{"HOME=C:\\Users\\bob"}
		got := SanitizeCommandEnvironmentForOS(base, nil, nil, "windows")
		envMap := envSliceToMap(got)
		if envMap["USERPROFILE"] != "C:\\Users\\bob" {
			t.Errorf("expected USERPROFILE to be synced from HOME, got %q", envMap["USERPROFILE"])
		}
		if envMap["HOME"] != "C:\\Users\\bob" {
			t.Errorf("expected HOME to remain, got %q", envMap["HOME"])
		}
	})

	t.Run("Windows keeps distinct values if both present", func(t *testing.T) {
		base := []string{"HOME=C:\\Users\\home", "USERPROFILE=C:\\Users\\profile"}
		got := SanitizeCommandEnvironmentForOS(base, nil, nil, "windows")
		envMap := envSliceToMap(got)
		if envMap["HOME"] != "C:\\Users\\home" {
			t.Errorf("expected HOME to remain unchanged, got %q", envMap["HOME"])
		}
		if envMap["USERPROFILE"] != "C:\\Users\\profile" {
			t.Errorf("expected USERPROFILE to remain unchanged, got %q", envMap["USERPROFILE"])
		}
	})

	t.Run("POSIX does not sync HOME or USERPROFILE", func(t *testing.T) {
		base := []string{"USERPROFILE=/tmp/profile"}
		got := SanitizeCommandEnvironmentForOS(base, nil, nil, "linux")
		envMap := envSliceToMap(got)
		if _, exists := envMap["HOME"]; exists {
			t.Errorf("POSIX should not inject HOME from USERPROFILE")
		}

		base2 := []string{"HOME=/home/user"}
		got2 := SanitizeCommandEnvironmentForOS(base2, nil, nil, "linux")
		envMap2 := envSliceToMap(got2)
		if _, exists := envMap2["USERPROFILE"]; exists {
			t.Errorf("POSIX should not inject USERPROFILE from HOME")
		}
	})
}

func envSliceToMap(env []string) map[string]string {
	m := make(map[string]string)
	for _, entry := range env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			m[strings.ToUpper(parts[0])] = parts[1]
		}
	}
	return m
}
