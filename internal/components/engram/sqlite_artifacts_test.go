package engram

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiskSpaceOKForSQLiteArtifacts_noArtifacts(t *testing.T) {
	dir := t.TempDir()
	ok, needed, avail, err := DiskSpaceOKForSQLiteArtifacts(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || needed != 0 || avail != 0 {
		t.Fatalf("got ok=%v needed=%d avail=%d, want ok=true needed=0 avail=0", ok, needed, avail)
	}
}

func TestDiskSpaceOKForSQLiteArtifacts_withTinyDB(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db := DBPath(dir)
	if err := os.WriteFile(db, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, needed, avail, err := DiskSpaceOKForSQLiteArtifacts(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	if needed != 1 {
		t.Fatalf("needed = %d, want 1", needed)
	}
	if avail <= 0 {
		t.Fatalf("avail = %d, want > 0", avail)
	}
	if !ok {
		t.Fatal("expected ok=true for one-byte DB on same volume")
	}
}
