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

	// Handle /usr, /bin, /etc prefixes for Termux layout.
	if strings.HasPrefix(path, "/usr") {
		return filepath.Join(r.Prefix, strings.TrimPrefix(path, "/usr"))
	}
	if strings.HasPrefix(path, "/bin") {
		return filepath.Join(r.Prefix, "bin", strings.TrimPrefix(path, "/bin"))
	}
	if strings.HasPrefix(path, "/etc") {
		return filepath.Join(r.Prefix, "etc", strings.TrimPrefix(path, "/etc"))
	}
	if strings.HasPrefix(path, "/tmp") {
		return filepath.Join(r.Prefix, "tmp", strings.TrimPrefix(path, "/tmp"))
	}

	return path
}

// NewResolverForDistro returns the appropriate PathResolver for the given distro.
func NewResolverForDistro(distro string) PathResolver {
	if distro == LinuxDistroTermux {
		prefix := os.Getenv("PREFIX")
		if prefix == "" {
			// Fallback to default Termux prefix if env var is missing.
			prefix = "/data/data/com.termux/files/usr"
		}
		return &TermuxResolver{Prefix: prefix}
	}
	return &DefaultResolver{}
}
