package system

import (
	"os/exec"
	"testing"
)

func TestIsWindowsBatchFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{`C:\Program Files\nodejs\pi.cmd`, true},
		{`C:\tools\pi.CMD`, true},
		{`C:\tools\script.bat`, true},
		{`C:\tools\script.BAT`, true},
		{`C:\tools\pi.exe`, false},
		{`/usr/local/bin/pi`, false},
		{`pi`, false},
		{``, false},
	}
	for _, tt := range tests {
		if got := IsWindowsBatchFile(tt.path); got != tt.want {
			t.Errorf("IsWindowsBatchFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestQuoteWindowsArgument(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"--mode", "--mode"},
		{"", `""`},
		{"hello world", `"hello world"`},
		{"tab\there", "\"tab\there\""},
		{`say "hello"`, `"say \"hello\""`},
		{`plain\backslash`, `plain\backslash`},
		{`c:\path with space\`, `"c:\path with space\\"`},
		{`c:\path with space\"file"`, `"c:\path with space\\\"file\""`},
	}
	for _, tt := range tests {
		if got := QuoteWindowsArgument(tt.arg); got != tt.want {
			t.Errorf("QuoteWindowsArgument(%q) = %q, want %q", tt.arg, got, tt.want)
		}
	}
}

func TestWindowsBatchCommandLine(t *testing.T) {
	tests := []struct {
		binary string
		args   []string
		want   string
	}{
		{
			binary: `C:\Program Files\nodejs\pi.cmd`,
			args:   []string{"--print", "--tools", ""},
			want:   `""C:\Program Files\nodejs\pi.cmd" --print --tools """`,
		},
		{
			binary: `C:\Program Files\run.bat`,
			args:   []string{"arg with space", `quote"inside`},
			want:   `""C:\Program Files\run.bat" "arg with space" "quote\"inside""`,
		},
		{
			binary: `simple.cmd`,
			args:   nil,
			want:   `"simple.cmd"`,
		},
	}
	for _, tt := range tests {
		got := WindowsBatchCommandLine(tt.binary, tt.args)
		if got != tt.want {
			t.Errorf("WindowsBatchCommandLine(%q, %v) = %q, want %q", tt.binary, tt.args, got, tt.want)
		}
	}
}

func TestConfigureCommandProcess(t *testing.T) {
	// Should not panic on nil command
	ConfigureCommandProcess(nil, "pi.cmd", []string{"--version"})

	cmd := exec.Command("echo", "hello")
	ConfigureCommandProcess(cmd, "echo", []string{"hello"})
	// Inherited healthy directory leaves cmd.Dir empty
	if cmd.Dir != "" {
		t.Errorf("ConfigureCommandProcess modified cmd.Dir for healthy directory: %q", cmd.Dir)
	}
}
