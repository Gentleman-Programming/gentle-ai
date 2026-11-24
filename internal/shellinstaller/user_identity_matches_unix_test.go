//go:build linux || darwin

package shellinstaller

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestUserIdentityMatchesIgnoresDeviceNumber(t *testing.T) {
	recorded := "16777234:12345:501:drwx------"
	current := "16777232:12345:501:drwx------"
	if !userIdentityMatches(recorded, current) {
		t.Fatalf("identity differing only in device number must match: recorded=%s current=%s", recorded, current)
	}
}

func TestUserIdentityMatchesAcceptsEqualIdentities(t *testing.T) {
	identity := "16777232:12345:501:drwx------"
	if !userIdentityMatches(identity, identity) {
		t.Fatal("equal identities must match")
	}
}

func TestUserIdentityMatchesRefusesPortableMismatch(t *testing.T) {
	current := "16777232:12345:501:drwx------"
	for _, recorded := range []string{
		"16777232:54321:501:drwx------", // different inode: recreated tree
		"16777232:12345:0:drwx------",   // different uid
		"16777232:12345:501:drwxr-xr-x", // different mode
		"16777232:12345:501:drwx------:extra",
		"",
	} {
		if userIdentityMatches(recorded, current) {
			t.Fatalf("recorded=%q must not match current=%q", recorded, current)
		}
	}
}

func TestUserIdentityMatchesMalformedRecordedComparesLiterally(t *testing.T) {
	// A recorded identity missing fields is not silently portable: it only
	// matches when the whole string equals the current identity.
	current := "16777232:12345:501:drwx------"
	if userIdentityMatches("12345:501:drwx------", current) {
		t.Fatal("a truncated recorded identity must not match by portable suffix")
	}
	if !userIdentityMatches("no-colons", "no-colons") {
		t.Fatal("identical malformed identities must still match")
	}
}

func TestUserIdentityKeepsFormat(t *testing.T) {
	// The recorded format stays dev:ino:uid:mode so existing manifests and the
	// Linux Shared path keep reading it. privateDirectory only accepts paths
	// inside the private home hierarchy, so the probe lives under $HOME.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path, err := os.MkdirTemp(home, "gentle-ai-identity-probe-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(path) })
	identity, err := userIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	var dev, ino, uid uint64
	if _, err := fmt.Sscanf(identity, "%d:%d:%d", &dev, &ino, &uid); err != nil {
		t.Fatalf("identity %q does not start with dev:ino:uid: %v", identity, err)
	}
	if dev == 0 || ino == 0 {
		t.Fatalf("identity %q has implausible dev/ino values", identity)
	}
	if !strings.HasSuffix(identity, "drwx------") {
		t.Fatalf("identity %q must end with the Go FileMode text of a 0700 dir", identity)
	}
}
