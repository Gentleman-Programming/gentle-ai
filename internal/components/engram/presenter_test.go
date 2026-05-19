package engram

import "testing"

func TestFormatDirLine_NoSize(t *testing.T) {
	got := FormatDirLine("/home/user/.engram", 0)
	if got != "/home/user/.engram" {
		t.Errorf("got %q, want path-only when dbSize=0", got)
	}
}

func TestFormatDirLine_WithSize(t *testing.T) {
	got := FormatDirLine("/home/user/.engram", 1024*1024*1024)
	want := "/home/user/.engram  (1.0 GiB used)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSpaceWarning(t *testing.T) {
	got := FormatSpaceWarning(1024*1024*1024, 300*1024*1024)
	want := "Need 1.0 GiB, only 300.0 MiB free"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
