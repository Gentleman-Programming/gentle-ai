//go:build !linux

package privatefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteNewReportUnsupportedDoesNotWrite(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "report.json")
	err := WriteNewReport(destination, []byte("private payload"))
	if err != ErrUnsupported || err.Error() != "private report storage is unsupported" {
		t.Fatalf("error = %v, want fixed unsupported error", err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("unsupported writer created a destination: %v", err)
	}
}
