//go:build linux

package shellinstaller

import (
	"os"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

// openTestTerminal returns the subordinate side of a fresh pseudo-terminal
// that is not the test's controlling terminal.
func openTestTerminal(t *testing.T) *os.File {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("pseudo-terminal unavailable: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	fd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := os.OpenFile("/dev/pts/"+strconv.Itoa(number), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = terminal.Close() })
	return terminal
}

func TestLinuxPathHooksKeepExactBehavior(t *testing.T) {
	if got, err := userCanonicalPath("/tmp/Gentle"); err != nil || got != "/tmp/Gentle" {
		t.Fatalf("canonical = %q, %v; Linux must check the operator path as given", got, err)
	}
	for _, pair := range [][2]string{{"/a/b", "/a/b"}, {"/a/b", "/a/b/c"}, {"/a/b/c", "/a/b"}} {
		if !userPathsOverlap(pair[0], pair[1]) {
			t.Fatalf("overlap not detected for %q and %q", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{{"/a/b", "/a/B"}, {"/a/b", "/a/bc"}, {"/a/b", "/a/c"}} {
		if userPathsOverlap(pair[0], pair[1]) {
			t.Fatalf("case-sensitive Linux paths %q and %q reported as overlapping", pair[0], pair[1])
		}
	}
	if err := privateExtendedMetadata("/owned", nil, true); err != nil {
		t.Fatalf("Linux extended metadata hook = %v, want no-op", err)
	}
}
