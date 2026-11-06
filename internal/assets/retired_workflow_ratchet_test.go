package assets

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// retiredWorkflowReferenceAllowlist lists embedded files that may still name
// SDD or OpenSpec, mapped to the reason. Only retirement inventory qualifies:
// content whose sole purpose is to recognize and clean up retired SDD assets.
// Prose offered to an agent never qualifies. Every entry must still match, so
// the list can only shrink.
var retiredWorkflowReferenceAllowlist = map[string]string{}

// TestEmbeddedAssetsHaveNoRetiredWorkflowReferences is a ratchet over every
// embedded asset Gentle AI can install or render into a runtime. SDD and
// OpenSpec were retired in v4.0.0; their names must not reappear in shipped
// paths or content.
func TestEmbeddedAssetsHaveNoRetiredWorkflowReferences(t *testing.T) {
	retired := regexp.MustCompile(`(?i)sdd|openspec`)
	allowlisted := map[string]bool{}
	scanned := 0
	err := fs.WalkDir(FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if retired.MatchString(entry.Name()) {
			if _, ok := retiredWorkflowReferenceAllowlist[path]; ok {
				allowlisted[path] = true
			} else {
				t.Errorf("%s: embedded path names a retired workflow", path)
			}
		}
		if entry.IsDir() {
			return nil
		}
		scanned++
		for index, line := range strings.Split(MustRead(path), "\n") {
			if !retired.MatchString(line) {
				continue
			}
			if _, ok := retiredWorkflowReferenceAllowlist[path]; ok {
				allowlisted[path] = true
				continue
			}
			t.Errorf("%s:%d: retired workflow reference: %s", path, index+1, strings.TrimSpace(line))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("no embedded assets scanned")
	}
	for path := range retiredWorkflowReferenceAllowlist {
		if !allowlisted[path] {
			t.Errorf("%s is allowlisted but no longer names a retired workflow; remove the entry", path)
		}
	}
}
