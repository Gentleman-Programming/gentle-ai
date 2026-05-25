package engram

import (
	"path/filepath"
	"testing"
)

func TestDataDirRef_Resolve(t *testing.T) {
	home := t.TempDir()
	custom := filepath.Join(home, "custom-data")

	tests := []struct {
		ref  DataDirRef
		want string
	}{
		{"", DefaultDir(home)},
		{DataDirRef(DefaultDir(home)), DefaultDir(home)},
		{DataDirRef(custom), custom},
		{DataDirRef(custom + string(filepath.Separator)), custom},
		{DataDirRef("relative-data"), filepath.Join(home, "relative-data")},
	}
	for _, tt := range tests {
		got := tt.ref.Resolve(home)
		if got != tt.want {
			t.Errorf("DataDirRef(%q).Resolve(%q) = %q, want %q", tt.ref, home, got, tt.want)
		}
	}
}

func TestDefaultDir(t *testing.T) {
	got := DefaultDir("/home/user")
	want := filepath.Join("/home/user", ".engram")
	if got != want {
		t.Errorf("DefaultDir = %q, want %q", got, want)
	}
}

func TestDBPath(t *testing.T) {
	got := DBPath("/home/user/.engram")
	want := filepath.Join("/home/user/.engram", "engram.db")
	if got != want {
		t.Errorf("DBPath = %q, want %q", got, want)
	}
}
