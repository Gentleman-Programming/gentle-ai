package assets

import (
	"testing"
	"testing/fstest"
)

func TestManagedDigestIsStableAcrossCalls(t *testing.T) {
	first, err := ManagedDigest()
	if err != nil {
		t.Fatalf("ManagedDigest() error = %v", err)
	}
	second, err := ManagedDigest()
	if err != nil {
		t.Fatalf("ManagedDigest() error = %v", err)
	}
	if first != second {
		t.Fatalf("ManagedDigest() is not stable across calls: %q != %q", first, second)
	}
	if len(first) != len("sha256:")+64 {
		t.Fatalf("ManagedDigest() = %q, want sha256:<64 hex chars>", first)
	}
}

func TestManagedDigestChangesWithContent(t *testing.T) {
	base := fstest.MapFS{
		"a.txt":        {Data: []byte("hello")},
		"dir/b.txt":    {Data: []byte("world")},
		"dir/c/d.json": {Data: []byte(`{"k":"v"}`)},
	}
	baseDigest, err := managedDigest(base)
	if err != nil {
		t.Fatalf("managedDigest(base) error = %v", err)
	}

	// Reordering the map (Go map iteration order is randomized) must not
	// change the digest: the walk sorts paths before hashing.
	reordered := fstest.MapFS{
		"dir/c/d.json": {Data: []byte(`{"k":"v"}`)},
		"a.txt":        {Data: []byte("hello")},
		"dir/b.txt":    {Data: []byte("world")},
	}
	reorderedDigest, err := managedDigest(reordered)
	if err != nil {
		t.Fatalf("managedDigest(reordered) error = %v", err)
	}
	if baseDigest != reorderedDigest {
		t.Fatalf("managedDigest() depends on iteration order: %q != %q", baseDigest, reorderedDigest)
	}

	changedContent := fstest.MapFS{
		"a.txt":        {Data: []byte("hello")},
		"dir/b.txt":    {Data: []byte("WORLD")}, // content changed
		"dir/c/d.json": {Data: []byte(`{"k":"v"}`)},
	}
	if digest, err := managedDigest(changedContent); err != nil {
		t.Fatalf("managedDigest(changedContent) error = %v", err)
	} else if digest == baseDigest {
		t.Fatal("managedDigest() did not change when file content changed")
	}

	changedPath := fstest.MapFS{
		"a.txt":        {Data: []byte("hello")},
		"dir/renamed":  {Data: []byte("world")}, // path changed, content same
		"dir/c/d.json": {Data: []byte(`{"k":"v"}`)},
	}
	if digest, err := managedDigest(changedPath); err != nil {
		t.Fatalf("managedDigest(changedPath) error = %v", err)
	} else if digest == baseDigest {
		t.Fatal("managedDigest() did not change when a file path changed")
	}

	addedFile := fstest.MapFS{
		"a.txt":        {Data: []byte("hello")},
		"dir/b.txt":    {Data: []byte("world")},
		"dir/c/d.json": {Data: []byte(`{"k":"v"}`)},
		"dir/e.txt":    {Data: []byte("extra")},
	}
	if digest, err := managedDigest(addedFile); err != nil {
		t.Fatalf("managedDigest(addedFile) error = %v", err)
	} else if digest == baseDigest {
		t.Fatal("managedDigest() did not change when a file was added")
	}
}
