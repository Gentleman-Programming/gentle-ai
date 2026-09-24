//go:build linux

package privatefile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteNewReportCreatesOwnerOnlyFile(t *testing.T) {
	dir := privateTempDir(t)
	destination := filepath.Join(dir, "report.json")
	payload := []byte("private report contents")

	if err := WriteNewReport(destination, payload); err != nil {
		t.Fatalf("WriteNewReport() error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("report bytes = %q, want %q", got, payload)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("stat report: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("report permissions = %04o, want 0600", got)
	}
}

func TestWriteNewReportAcceptsMaximumPayload(t *testing.T) {
	destination := filepath.Join(privateTempDir(t), "report.bin")
	payload := bytes.Repeat([]byte{'x'}, maxReportSize)
	if err := WriteNewReport(destination, payload); err != nil {
		t.Fatalf("WriteNewReport() at limit error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("report bytes differ from payload")
	}
}

func TestWriteNewReportRefusesExistingEntries(t *testing.T) {
	t.Run("file is unchanged", func(t *testing.T) {
		destination := filepath.Join(privateTempDir(t), "report.json")
		original := []byte("do not replace")
		if err := os.WriteFile(destination, original, 0600); err != nil {
			t.Fatal(err)
		}
		err := WriteNewReport(destination, []byte("new private report"))
		assertFixedError(t, err, ErrDestinationExists, destination, "new private report")
		got, readErr := os.ReadFile(destination)
		if readErr != nil || !bytes.Equal(got, original) {
			t.Fatalf("existing file changed: bytes=%q error=%v", got, readErr)
		}
	})

	t.Run("final symlink is refused", func(t *testing.T) {
		dir := privateTempDir(t)
		target := filepath.Join(dir, "target")
		if err := os.WriteFile(target, []byte("target stays private"), 0600); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(dir, "report.json")
		if err := os.Symlink(target, destination); err != nil {
			t.Fatal(err)
		}
		err := WriteNewReport(destination, []byte("secret payload"))
		assertFixedError(t, err, ErrDestinationExists, destination, "secret payload")
		got, readErr := os.ReadFile(target)
		if readErr != nil || string(got) != "target stays private" {
			t.Fatalf("symlink target changed: bytes=%q error=%v", got, readErr)
		}
	})
}

func TestWriteNewReportRejectsSymlinkParent(t *testing.T) {
	root := privateTempDir(t)
	realParent := filepath.Join(root, "real")
	if err := os.Mkdir(realParent, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(realParent, link); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(link, "report.json")
	err := WriteNewReport(destination, []byte("secret payload"))
	assertFixedError(t, err, ErrUnsafeParent, destination, "secret payload")
	if _, err := os.Lstat(filepath.Join(realParent, "report.json")); !os.IsNotExist(err) {
		t.Fatalf("report created through parent symlink: %v", err)
	}
}

func TestWriteNewReportRejectsUnsafeParent(t *testing.T) {
	dir := privateTempDir(t)
	if err := os.Chmod(dir, 0770); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "report.json")
	err := WriteNewReport(destination, []byte("secret payload"))
	assertFixedError(t, err, ErrUnsafeParent, destination, "secret payload")
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("report created in unsafe parent: %v", err)
	}
}

func TestWriteNewReportReturnsFixedWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may bypass directory write permissions")
	}
	dir := privateTempDir(t)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "report.json")
	payload := []byte("sensitive-payload")
	err := WriteNewReport(destination, payload)
	assertFixedError(t, err, ErrWriteFailed, destination, string(payload))
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("write failure created a destination: %v", err)
	}
}

func TestWriteNewReportRejectsInvalidInputsWithoutCreatingParents(t *testing.T) {
	root := privateTempDir(t)
	missingParent := filepath.Join(root, "not-created")
	cases := []struct {
		name        string
		destination string
		payload     []byte
		want        error
	}{
		{name: "empty payload", destination: filepath.Join(root, "empty"), want: ErrInvalidPayload},
		{name: "oversized payload", destination: filepath.Join(root, "large"), payload: bytes.Repeat([]byte("sensitive"), maxReportSize/9+1), want: ErrInvalidPayload},
		{name: "relative path", destination: "relative-report.json", payload: []byte("sensitive-payload"), want: ErrInvalidDestination},
		{name: "unsafe basename", destination: filepath.Join(root, "report name.json"), payload: []byte("sensitive-payload"), want: ErrInvalidDestination},
		{name: "missing parent", destination: filepath.Join(missingParent, "report.json"), payload: []byte("sensitive-payload"), want: ErrUnsafeParent},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteNewReport(tt.destination, tt.payload)
			assertFixedError(t, err, tt.want, tt.destination, string(tt.payload))
		})
	}
	if _, err := os.Lstat(missingParent); !os.IsNotExist(err) {
		t.Fatalf("missing parent was created: %v", err)
	}
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertFixedError(t *testing.T, got, want error, destination, payload string) {
	t.Helper()
	if got != want {
		t.Fatalf("error = %v, want fixed error %v", got, want)
	}
	for _, private := range []string{destination, payload} {
		if private != "" && strings.Contains(got.Error(), private) {
			t.Errorf("fixed error contains private input %q", private)
		}
	}
}
