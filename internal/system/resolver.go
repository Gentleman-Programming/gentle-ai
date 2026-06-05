package system

import (
	"os"
	"path/filepath"
	"strings"
)

// PathResolver defines the contract for resolving system paths
// across different platform layouts (Standard Unix vs Termux).
type PathResolver interface {
	Resolve(path string) string
}

// DefaultResolver provides standard Unix path resolution (pass-through).
type DefaultResolver struct{}

func (r *DefaultResolver) Resolve(path string) string {
	return path
}

// TermuxResolver resolves paths by prepending the Termux $PREFIX.
type TermuxResolver struct {
	Prefix string
}

func (r *TermuxResolver) Resolve(path string) string {
	if path == "" {
		return ""
	}

	// Only resolve absolute paths that target standard Unix hierarchy.
	// In Termux, even on Android, we check for leading slash to identify
	// Unix-style absolute paths, regardless of host OS (for testing portability).
	if !strings.HasPrefix(path, "/") {
		return path
	}

	// Handle /usr, /bin, /etc, /tmp prefixes for Termux layout.
	// Match exact directory boundaries to avoid rewriting unrelated paths
	// like "/usrbin" or "/etcetera" (requires trailing slash or exact match).
	if path == "/usr" || strings.HasPrefix(path, "/usr/") {
		return filepath.Join(r.Prefix, strings.TrimPrefix(path, "/usr"))
	}
	if path == "/bin" || strings.HasPrefix(path, "/bin/") {
		return filepath.Join(r.Prefix, "bin", strings.TrimPrefix(path, "/bin"))
	}
	if path == "/etc" || strings.HasPrefix(path, "/etc/") {
		return filepath.Join(r.Prefix, "etc", strings.TrimPrefix(path, "/etc"))
	}
	if path == "/tmp" || strings.HasPrefix(path, "/tmp/") {
		return filepath.Join(r.Prefix, "tmp", strings.TrimPrefix(path, "/tmp"))
	}

	return path
}

// NewResolverForProfile returns the appropriate PathResolver for a resolved
// platform profile. Termux is a first-class Android OS profile, not a Linux
// distro detected through /etc/os-release.
func NewResolverForProfile(profile PlatformProfile) PathResolver {
	if profile.OS == "android" {
		return newTermuxResolver()
	}
	return &DefaultResolver{}
}

// NewResolverForDistro returns the appropriate PathResolver for the given distro.
// Prefer NewResolverForProfile at platform call sites. Termux is intentionally
// not resolved from a Linux distro string because Android is the canonical
// Termux platform boundary.
func NewResolverForDistro(distro string) PathResolver {
	return &DefaultResolver{}
}

func newTermuxResolver() PathResolver {
	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		// Fallback to default Termux prefix if env var is missing.
		prefix = "/data/data/com.termux/files/usr"
	}
	return &TermuxResolver{Prefix: prefix}
}
