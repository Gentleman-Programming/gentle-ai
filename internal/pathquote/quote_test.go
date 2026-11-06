package pathquote

import "testing"

func TestQuote(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "windows path keeps single backslashes",
			path: `C:\Users\dev\repo`,
			want: `"C:\Users\dev\repo"`,
		},
		{
			name: "posix path is wrapped unchanged",
			path: "/home/dev/repo",
			want: `"/home/dev/repo"`,
		},
		{
			name: "path with spaces stays one quoted token",
			path: `C:\Users\dev name\repo`,
			want: `"C:\Users\dev name\repo"`,
		},
		{
			name: "embedded double quote is escaped so the token survives a shell",
			path: `/home/dev/"odd"/repo`,
			want: `"/home/dev/\"odd\"/repo"`,
		},
		{
			name: "empty path renders as an empty quoted token",
			path: "",
			want: `""`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Quote(tt.path); got != tt.want {
				t.Fatalf("Quote(%s) = %s, want %s", tt.path, got, tt.want)
			}
		})
	}
}

func TestShellWord(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "safe value stays unquoted", value: "sha256:abc-123", want: "sha256:abc-123"},
		{name: "spaces are quoted", value: "two words", want: "'two words'"},
		{name: "single quotes use POSIX splice", value: "o'brien", want: "'o'\\''brien'"},
		{name: "command substitution is quoted", value: "$(touch marker)", want: "'$(touch marker)'"},
		{name: "empty value is quoted", value: "", want: "''"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShellWord(tt.value); got != tt.want {
				t.Fatalf("ShellWord(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
