package model

// PlatformProfile records OS, distribution, and package manager details.
type PlatformProfile struct {
	OS             string
	LinuxDistro    string
	PackageManager string
	NpmWritable    bool // true when npm global prefix is user-writable (nvm/fnm/volta)
	GoAvailable    bool // true when `go` is found on PATH (used for auto-detect: brew → go-install → binary)
	Supported      bool
}
