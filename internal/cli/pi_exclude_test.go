package cli

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestParseInstallFlagsPiExcludePackageAcceptsRepeatedAndCommaSeparatedValues(t *testing.T) {
	flags, err := ParseInstallFlags([]string{
		"--pi-exclude-package", "npm:gentle-pi, npm:pi-btw",
		"--pi-exclude-package=npm:pi-web-access",
		"--pi-background-subagents=off",
	})
	if err != nil {
		t.Fatalf("ParseInstallFlags() error = %v", err)
	}
	want := []string{"npm:gentle-pi", "npm:pi-btw", "npm:pi-web-access"}
	if !reflect.DeepEqual(flags.PiExcludePackages, want) {
		t.Fatalf("PiExcludePackages = %v, want %v", flags.PiExcludePackages, want)
	}
	if !flags.PiBackgroundSubagentsSet || flags.PiBackgroundSubagents != "off" {
		t.Fatalf("pi background flag = %q set=%v, want off set", flags.PiBackgroundSubagents, flags.PiBackgroundSubagentsSet)
	}
}

func TestResolvePiExcludePackages(t *testing.T) {
	for _, tt := range []struct {
		name     string
		flag     []string
		envValue string
		envSet   bool
		want     []string
	}{
		{name: "nothing set"},
		{name: "flag only", flag: []string{"npm:gentle-pi"}, want: []string{"npm:gentle-pi"}},
		{name: "env applies when flag is absent", envValue: "npm:gentle-pi, npm:pi-btw", envSet: true, want: []string{"npm:gentle-pi", "npm:pi-btw"}},
		{name: "flag wins over env", flag: []string{"npm:pi-btw"}, envValue: "npm:gentle-pi", envSet: true, want: []string{"npm:pi-btw"}},
		{name: "empty env is unset", envValue: "", envSet: true},
		{name: "separator-only env is unset", envValue: " , ", envSet: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePiExcludePackages(tt.flag, tt.envValue, tt.envSet)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resolvePiExcludePackages() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunInstallRejectsInvalidPiExcludePackagesBeforeWrites(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		env  string
		want string
	}{
		{"unknown value", []string{"--agent", "pi", "--pi-exclude-package", "npm:unknown"}, "", `--pi-exclude-package: unknown Pi package "npm:unknown"; allowed values: npm:gentle-pi, npm:pi-web-access, npm:pi-btw`},
		{"unknown env value", []string{"--agent", "pi"}, "npm:unknown", `GENTLE_AI_PI_EXCLUDE_PACKAGES: unknown Pi package "npm:unknown"`},
		{"engram source", []string{"--agent", "pi", "--pi-exclude-package", "npm:gentle-engram"}, "", `--pi-exclude-package: package "npm:gentle-engram" cannot be excluded`},
		{"orphan flag", []string{"--agent", "claude-code", "--pi-exclude-package", "npm:gentle-pi"}, "", "--pi-exclude-package requires the pi agent in the resolved install plan; rerun gentle-ai install with pi added to your existing agent list, or remove --pi-exclude-package"},
		{"orphan env", []string{"--agent", "claude-code"}, "npm:gentle-pi", "GENTLE_AI_PI_EXCLUDE_PACKAGES requires the pi agent in the resolved install plan; rerun gentle-ai install with pi added to your existing agent list, or unset GENTLE_AI_PI_EXCLUDE_PACKAGES"},
	} {
		for _, dryRun := range []bool{true, false} {
			name := tc.name + "/real"
			if dryRun {
				name = tc.name + "/dry-run"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv(PiExcludePackagesEnv, tc.env)
				home := t.TempDir()
				previous := osUserHomeDir
				osUserHomeDir = func() (string, error) { return home, nil }
				t.Cleanup(func() { osUserHomeDir = previous })
				args := append([]string{"--component", "theme"}, tc.args...)
				if dryRun {
					args = append(args, "--dry-run")
				}
				_, err := RunInstall(args, system.DetectionResult{})
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("RunInstall() error = %v, want %q", err, tc.want)
				}
				entries, err := os.ReadDir(home)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 0 {
					t.Fatalf("rejected install wrote home entries: %v", entries)
				}
			})
		}
	}
}

func TestRenderDryRunShowsPiExcludedPackages(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		env      string
		wantLine string
	}{
		{name: "no exclusion", args: []string{}},
		{name: "flag", args: []string{"--pi-exclude-package", "npm:gentle-pi"}, wantLine: "Pi excluded packages: npm:gentle-pi"},
		{name: "env", env: "npm:gentle-pi,npm:pi-btw", wantLine: "Pi excluded packages: npm:gentle-pi,npm:pi-btw"},
		{name: "flag wins over env", args: []string{"--pi-exclude-package", "npm:pi-btw"}, env: "npm:gentle-pi", wantLine: "Pi excluded packages: npm:pi-btw"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(PiExcludePackagesEnv, tc.env)
			home := t.TempDir()
			previous := osUserHomeDir
			osUserHomeDir = func() (string, error) { return home, nil }
			t.Cleanup(func() { osUserHomeDir = previous })
			args := append([]string{"--agent", "pi", "--component", "theme", "--dry-run"}, tc.args...)
			result, err := RunInstall(args, system.DetectionResult{})
			if err != nil {
				t.Fatalf("RunInstall() error = %v", err)
			}
			out := RenderDryRun(result)
			if !strings.Contains(out, "Pi background intent:") {
				t.Fatalf("dry-run lost the Pi background line:\n%s", out)
			}
			if tc.wantLine == "" {
				if strings.Contains(out, "Pi excluded packages") {
					t.Fatalf("dry-run without exclusion shows an exclusion line:\n%s", out)
				}
				return
			}
			if !strings.Contains(out, tc.wantLine+"\n") && !strings.HasSuffix(out, tc.wantLine) {
				t.Fatalf("dry-run missing %q:\n%s", tc.wantLine, out)
			}
		})
	}
}

func TestPrintInstallHelpDocumentsPiExcludePackage(t *testing.T) {
	var out bytes.Buffer
	PrintInstallHelp(&out)
	help := out.String()
	for _, want := range []string{"--pi-exclude-package <list>", "env: GENTLE_AI_PI_EXCLUDE_PACKAGES"} {
		if !strings.Contains(help, want) {
			t.Fatalf("install help missing %q:\n%s", want, help)
		}
	}
}
