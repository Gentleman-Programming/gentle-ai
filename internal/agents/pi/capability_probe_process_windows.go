//go:build windows

package pi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

func runModelRoutingProcess(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
	return ModelRoutingProcessResult{}, transportError(TransportErrorUnsupportedPlatform, ErrTransportUnsupportedPlatform)
}

type windowsTransportCommandPlan struct {
	executable string
	args       []string
	env        []string
	cwd        string
}

func (p windowsTransportCommandPlan) Executable() string       { return p.executable }
func (p windowsTransportCommandPlan) Arguments() []string      { return append([]string(nil), p.args...) }
func (p windowsTransportCommandPlan) Environment() []string    { return append([]string(nil), p.env...) }
func (p windowsTransportCommandPlan) WorkingDirectory() string { return p.cwd }

func newWindowsTransportCommandPlan(executable string, environment []string) (windowsTransportCommandPlan, error) {
	if err := validateWindowsTransportPath(executable); err != nil {
		return windowsTransportCommandPlan{}, windowsTransportPlanError(err)
	}
	plan := windowsTransportCommandPlan{
		executable: executable,
		env:        windowsTransportEnvironment(environment),
		cwd:        filepath.VolumeName(executable) + string(filepath.Separator),
	}
	if ext := strings.ToLower(filepath.Ext(executable)); ext == ".cmd" || ext == ".bat" {
		if err := validateWindowsTransportBatchPath(executable); err != nil {
			return windowsTransportCommandPlan{}, windowsTransportPlanError(err)
		}
		comspec, err := validatedWindowsTransportComSpec(environment)
		if err != nil {
			return windowsTransportCommandPlan{}, windowsTransportPlanError(err)
		}
		plan.executable = comspec
		plan.args = []string{"/d", "/s", "/c", `""` + executable + `""`}
	}
	return plan, nil
}

func windowsTransportPlanError(cause error) error {
	return transportError(TransportErrorInvalidPath, errors.Join(ErrTransportInvalidPath, cause))
}

func validateWindowsTransportPath(path string) error {
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.VolumeName(path) == "" {
		return ErrTransportInvalidPath
	}
	return nil
}

func validateWindowsTransportBatchPath(path string) error {
	if strings.ContainsAny(path, "%!&|<>^()\"'") {
		return ErrTransportInvalidPath
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return ErrTransportInvalidPath
		}
	}
	return nil
}

func validatedWindowsTransportComSpec(environment []string) (string, error) {
	values := windowsTransportEnvironmentValues(environment)
	if comspec, present := values["COMSPEC"]; present {
		if err := validateWindowsTransportInterpreter(comspec); err != nil {
			return "", err
		}
		return comspec, nil
	}
	root, present := values["SYSTEMROOT"]
	if !present {
		return "", ErrTransportInvalidPath
	}
	if err := validateWindowsTransportPath(root); err != nil {
		return "", err
	}
	interpreter := filepath.Join(root, "System32", "cmd.exe")
	if err := validateWindowsTransportInterpreter(interpreter); err != nil {
		return "", err
	}
	return interpreter, nil
}

func validateWindowsTransportInterpreter(path string) error {
	if err := validateWindowsTransportPath(path); err != nil || !strings.EqualFold(filepath.Base(path), "cmd.exe") {
		return ErrTransportInvalidPath
	}
	info, err := os.Stat(path)
	if err != nil {
		return errors.Join(ErrTransportInvalidPath, err)
	}
	if !info.Mode().IsRegular() {
		return ErrTransportInvalidPath
	}
	return nil
}

var windowsTransportEnvironmentKeys = [...]string{
	"COMSPEC", "LANG", "LANGUAGE", "LC_ALL", "LC_COLLATE", "LC_CTYPE", "LC_MESSAGES", "LC_MONETARY", "LC_NUMERIC", "LC_TIME", "PATH", "PATHEXT", "SYSTEMDRIVE", "SYSTEMROOT", "TEMP", "TMP", "TMPDIR", "WINDIR",
}

func windowsTransportEnvironmentValues(entries []string) map[string]string {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			key = strings.ToUpper(key)
			if slices.Contains(windowsTransportEnvironmentKeys[:], key) {
				values[key] = value
			}
		}
	}
	return values
}

func windowsTransportEnvironment(entries []string) []string {
	values := windowsTransportEnvironmentValues(entries)
	if values["PATH"] == "" {
		root := values["SYSTEMROOT"]
		if root == "" {
			root = values["WINDIR"]
		}
		if root == "" {
			root = `C:\Windows`
		}
		values["PATH"] = filepath.Join(root, "System32") + ";" + root
	}
	env := make([]string, 0, len(windowsTransportEnvironmentKeys))
	for _, key := range windowsTransportEnvironmentKeys {
		if value, present := values[key]; present {
			env = append(env, key+"="+value)
		}
	}
	return env
}
