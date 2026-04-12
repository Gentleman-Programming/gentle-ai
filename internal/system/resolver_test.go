package system

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestDefaultResolver_Resolve(t *testing.T) {
	resolver := &DefaultResolver{}
	
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "absolute path remains same", path: "/usr/bin/bash", want: "/usr/bin/bash"},
		{name: "relative path remains same", path: "bin/bash", want: "bin/bash"},
		{name: "empty path", path: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filepath.ToSlash(resolver.Resolve(tc.path))
			if got != tc.want {
				t.Fatalf("Resolve(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestTermuxResolver_Resolve(t *testing.T) {
	prefix := "/data/data/com.termux/files/usr"
	resolver := &TermuxResolver{Prefix: prefix}
	
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "resolves /usr path to prefix",
			path: "/usr/bin/bash",
			want: "/data/data/com.termux/files/usr/bin/bash",
		},
		{
			name: "resolves /bin path to prefix",
			path: "/bin/sh",
			want: "/data/data/com.termux/files/usr/bin/sh",
		},
		{
			name: "resolves /etc path to prefix",
			path: "/etc/os-release",
			want: "/data/data/com.termux/files/usr/etc/os-release",
		},
		{
			name: "resolves /tmp path to prefix",
			path: "/tmp/some-file",
			want: "/data/data/com.termux/files/usr/tmp/some-file",
		},
		{
			name: "leaves non-standard paths alone",
			path: "/home/user/test.txt",
			want: "/home/user/test.txt",
		},
		{
			name: "handles relative paths",
			path: "local/bin/myapp",
			want: "local/bin/myapp",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filepath.ToSlash(resolver.Resolve(tc.path))
			if got != tc.want {
				t.Fatalf("Resolve(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestNewResolverForDistro(t *testing.T) {
	prefix := "/data/data/com.termux/files/usr"
	t.Setenv("PREFIX", prefix)
	
	tests := []struct {
		name   string
		distro string
		want   string // Type name as string for comparison
	}{
		{name: "termux returns TermuxResolver", distro: LinuxDistroTermux, want: "*system.TermuxResolver"},
		{name: "ubuntu returns DefaultResolver", distro: LinuxDistroUbuntu, want: "*system.DefaultResolver"},
		{name: "unknown returns DefaultResolver", distro: LinuxDistroUnknown, want: "*system.DefaultResolver"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := NewResolverForDistro(tc.distro)
			typeName := fmt.Sprintf("%T", resolver)
			if typeName != tc.want {
				t.Fatalf("NewResolverForDistro(%q) returned %s, want %s", tc.distro, typeName, tc.want)
			}
			
			if tc.distro == LinuxDistroTermux {
				tr := resolver.(*TermuxResolver)
				if tr.Prefix != prefix {
					t.Fatalf("TermuxResolver.Prefix = %q, want %q", tr.Prefix, prefix)
				}
			}
		})
	}
}
