package opencode

// ManagedLauncherRemovalStatus describes the outcome of an ownership-guarded
// launcher removal attempt.
type ManagedLauncherRemovalStatus string

const (
	ManagedLauncherRemovalAbsent   ManagedLauncherRemovalStatus = "absent"
	ManagedLauncherRemovalRemoved  ManagedLauncherRemovalStatus = "removed"
	ManagedLauncherRemovalNotOwned ManagedLauncherRemovalStatus = "not-owned"
	ManagedLauncherRemovalRefused  ManagedLauncherRemovalStatus = "refused"
)

// ManagedLauncherRemovalResult reports whether the named launcher was removed.
// Refused and not-owned outcomes are successful safety decisions; unexpected
// filesystem failures are returned as errors by RemoveManagedLauncher.
type ManagedLauncherRemovalResult struct {
	Status ManagedLauncherRemovalStatus
}

// Removed reports whether the managed launcher was actually removed.
func (r ManagedLauncherRemovalResult) Removed() bool {
	return r.Status == ManagedLauncherRemovalRemoved
}

// These hooks are intentionally package-private: tests use them to place
// deterministic replacements around the mutation boundary. Production leaves
// them as no-ops. They are shared mutable state, so tests replacing a hook must
// not call t.Parallel().
var (
	managedLauncherRemovalBeforeDelete = func(string) {}
	managedLauncherRemovalBeforeUnlink = func(string) {}
)
