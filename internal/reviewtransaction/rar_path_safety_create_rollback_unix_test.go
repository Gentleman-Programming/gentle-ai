//go:build !windows

package reviewtransaction

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// breakRARPrivateDirectoryMode makes every directory this package creates come
// back group- and world-readable, the way WSL DrvFS, exFAT, and SMB mounts
// without POSIX extensions behave: the mode handed to mkdir(2) is accepted and
// then ignored. No mode argument can produce this on ext4 or tmpfs, because
// mkdir only ever removes bits, so the primitive is swapped instead.
func breakRARPrivateDirectoryMode(t *testing.T) {
	t.Helper()
	previous := rarPrivateDirectoryMkdir
	rarPrivateDirectoryMkdir = func(name string, _ fs.FileMode) error {
		return os.Mkdir(name, 0o755)
	}
	t.Cleanup(func() { rarPrivateDirectoryMkdir = previous })
}

// breakRARPrivateDirectoryChmod completes the simulation for the mounts that
// also swallow chmod(2): the call reports success and changes nothing, so the
// directory can never be made safe from inside this process.
func breakRARPrivateDirectoryChmod(t *testing.T) {
	t.Helper()
	previous := rarPrivateDirectoryChmod
	rarPrivateDirectoryChmod = func(string, fs.FileMode) error { return nil }
	t.Cleanup(func() { rarPrivateDirectoryChmod = previous })
}

func TestCreatePrivateRARDirectorySecuresAModeThatDidNotStick(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, "v1")
	breakRARPrivateDirectoryMode(t)

	created, err := createPrivateRARDirectory(path)
	if err != nil {
		t.Fatalf("createPrivateRARDirectory on a filesystem that ignores the mkdir mode = %v, want the directory secured", err)
	}
	if !created {
		t.Fatalf("createPrivateRARDirectory created = false, want true for a directory it made")
	}
	info, statErr := os.Lstat(path)
	if statErr != nil {
		t.Fatalf("stat secured directory: %v", statErr)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("secured directory mode = %04o, want 0700", info.Mode().Perm())
	}
	if err := validatePrivateRARDirectory(path); err != nil {
		t.Fatalf("validate secured directory: %v", err)
	}
}

func TestCreatePrivateRARDirectoryKeepsTheUnsafePathItNames(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, "v1")
	breakRARPrivateDirectoryMode(t)
	breakRARPrivateDirectoryChmod(t)

	created, err := createPrivateRARDirectory(path)
	if created {
		t.Fatalf("createPrivateRARDirectory created = true for a directory it could not make safe")
	}
	var unsafePath *UnsafeRARPathError
	if !errors.As(err, &unsafePath) {
		t.Fatalf("createPrivateRARDirectory on an unrepairable filesystem = %v, want *UnsafeRARPathError", err)
	}
	if unsafePath.Path != path || !unsafePath.Directory {
		t.Fatalf("refusal names %q (directory=%t), want %q (directory=true)", unsafePath.Path, unsafePath.Directory, path)
	}
	// The refusal tells the operator to restore the mode on this exact path.
	// Removing it turned that instruction into "chmod: cannot access ...: No
	// such file or directory", which is the whole of issue #3285.
	if _, statErr := os.Lstat(unsafePath.Path); statErr != nil {
		t.Fatalf("refused path %q is gone, so the printed repair cannot run: %v", unsafePath.Path, statErr)
	}
	// Refusing is still the point: an unsafe path that cannot be made safe
	// must never be reported as usable.
	if validateErr := validatePrivateRARDirectory(path); validateErr == nil {
		t.Fatalf("validatePrivateRARDirectory accepted a world-readable directory")
	}
}

func TestCreatePrivateRARDirectoryNeverRepairsThroughASubstitutedSymlink(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, "v1")
	victim := filepath.Join(root, "victim")
	if err := os.Mkdir(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	// The race the no-follow walk exists to defeat, and the likeliest reason a
	// just-created 0700 directory fails on POSIX: it is swapped for a symlink
	// before validation reads it. A repair that resolved the path would chmod
	// the attacker's target instead.
	previous := rarPrivateDirectoryMkdir
	rarPrivateDirectoryMkdir = func(name string, mode fs.FileMode) error {
		if err := errors.Join(os.Mkdir(name, mode), os.Remove(name)); err != nil {
			return err
		}
		return os.Symlink(victim, name)
	}
	t.Cleanup(func() { rarPrivateDirectoryMkdir = previous })

	created, err := createPrivateRARDirectory(path)
	if created || err == nil {
		t.Fatalf("createPrivateRARDirectory over a substituted symlink = (%t, %v), want (false, refusal)", created, err)
	}
	if info, statErr := os.Lstat(victim); statErr != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("substituted target = %v (%v), want mode 0755; the repair followed the symlink", info, statErr)
	}
	// Refusing has to be stable: the wrong entry is the operator's to remove,
	// never this product's, so every later invocation refuses it untouched.
	if created, err := createPrivateRARDirectory(path); created || err == nil {
		t.Fatalf("second attempt over a substituted symlink = (%t, %v), want (false, refusal)", created, err)
	}
	if info, statErr := os.Lstat(victim); statErr != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("substituted target = %v (%v), want mode 0755 after the second refusal", info, statErr)
	}
}

func TestCreatePrivateRARDirectoryRepairsWhatAnInterruptedRunLeftBehind(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, "v1")
	// What a run killed between mkdir(2) and its repair leaves behind -- a CI
	// timeout, an OOM kill, Ctrl-C, a transient EPERM. mkdir answers every
	// later attempt with EEXIST, so a `created` gate never repairs it.
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	created, err := createPrivateRARDirectory(path)
	if err != nil || created {
		t.Fatalf("invocation after an interrupted run = (%t, %v), want (false, nil)", created, err)
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("directory left by the interrupted run = %v (%v), want it repaired to 0700", info, statErr)
	}
}

func TestCreatePrivateRARDirectoryNeverRepairsADirectoryHoldingState(t *testing.T) {
	root := resolvedTempDir(t)
	path := filepath.Join(root, "v1")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	// Content is the line between an interrupted creation and an authority
	// directory somebody weakened. Re-tightening the second one silently would
	// hide from the operator that anyone could write what is already inside it.
	if err := os.WriteFile(filepath.Join(path, "state.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	created, err := createPrivateRARDirectory(path)
	if created || err == nil {
		t.Fatalf("createPrivateRARDirectory over a populated unsafe directory = (%t, %v), want (false, refusal)", created, err)
	}
	if info, statErr := os.Lstat(path); statErr != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("populated directory = %v (%v), want it left at 0755", info, statErr)
	}
}

// TestEnsureRARDirectoryChainRepairsAPreExistingPrivateDirectory proves #3416:
// when the private tier of the RAR authority chain already exists (an older
// release, an interrupted run, or an operator's own `mkdir`), the walk must
// route it through the same repair helper createPrivateRARDirectory uses for
// a freshly created directory, not refuse it outright the first time
// validation fails. Before the fix, ensureRARDirectoryChain's existing-entry
// branch (os.Lstat succeeds, so the create branch is skipped entirely) fell
// straight through to validatePrivateRARDirectory and returned its error
// unrepaired -- exactly the "documented repair is ineffective" symptom the
// issue reports after a successful out-of-band chmod/chown that a later
// process restart could not see.
func TestEnsureRARDirectoryChainRepairsAPreExistingPrivateDirectory(t *testing.T) {
	commonDir := resolvedTempDir(t)
	privateParent := filepath.Join(commonDir, "gentle-ai", "review-transactions", rarAuthorityDirectory)
	root := filepath.Join(privateParent, rarAuthorityVersion)
	// The parent is pre-created already owner-only, exactly as a prior,
	// successful run of this same chain would leave it -- an intermediate
	// private directory that already holds a child is never repairable
	// in-place (repair only ever tightens an EMPTY directory; see
	// TestCreatePrivateRARDirectoryNeverRepairsADirectoryHoldingState), so
	// widening it here would test a scenario this design deliberately never
	// promises to fix. What #3416 reports, and what this test exercises, is
	// the empty leaf: an older release or an interrupted run left it
	// world- and group-readable instead of owner-only, but still owned by
	// this process and holding nothing.
	if err := os.MkdirAll(privateParent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ensureRARRepositoryRoot(commonDir, root, true); err != nil {
		t.Fatalf("ensureRARRepositoryRoot over a pre-existing wrong-mode authority directory = %v, want it repaired instead of refused", err)
	}
	info, statErr := os.Lstat(root)
	if statErr != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("pre-existing private directory %q = %v (%v), want it repaired to 0700", root, info, statErr)
	}
	if err := validatePrivateRARDirectory(root); err != nil {
		t.Fatalf("validate repaired authority directory: %v", err)
	}
}

// TestEnsureRARDirectoryChainStillRefusesAnUnrepairablePreExistingDirectory
// pins the other half: a pre-existing private directory whose mode chmod(2)
// cannot fix (breakRARPrivateDirectoryChmod) must still be refused, not
// silently accepted, so the repair route added for #3416 never widens into a
// trust bypass.
func TestEnsureRARDirectoryChainStillRefusesAnUnrepairablePreExistingDirectory(t *testing.T) {
	commonDir := resolvedTempDir(t)
	privateParent := filepath.Join(commonDir, "gentle-ai", "review-transactions", rarAuthorityDirectory)
	root := filepath.Join(privateParent, rarAuthorityVersion)
	if err := os.MkdirAll(privateParent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	breakRARPrivateDirectoryChmod(t)

	if err := ensureRARRepositoryRoot(commonDir, root, true); err == nil {
		t.Fatal("ensureRARRepositoryRoot accepted a pre-existing directory chmod(2) cannot repair")
	}
	info, statErr := os.Lstat(root)
	if statErr != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("unrepairable directory = %v (%v), want it left untouched at 0755", info, statErr)
	}
}

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	// The private-path walk opens every ancestor with O_NOFOLLOW, so a
	// symlinked temporary root would fail for a reason unrelated to the mode.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
