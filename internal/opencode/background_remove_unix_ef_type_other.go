//go:build unix && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package opencode

func isManagedLauncherEFTYPE(error) bool { return false }
