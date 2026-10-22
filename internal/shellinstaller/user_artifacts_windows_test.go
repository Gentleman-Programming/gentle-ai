//go:build windows

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func userWindowsZipFixture(t *testing.T, names ...string) ([]byte, userWindowsArtifact) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte("fixture-only, never executed")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	return data, userWindowsArtifact{SHA: userWindowsSHA(data), Prefix: "fixture", Archive: "fixture.zip", Bound: 32 << 20}
}

func TestUserWindowsArchivesRefuseUnsafeNamesBeforeEffects(t *testing.T) {
	for _, names := range [][]string{
		{"fixture/good.txt", "fixture/../escape"},
		{"fixture/good.txt", "fixture//double.txt"},
		{"fixture/good.txt", "fixture"},
		{"fixture/good.txt", "foreign/executable.exe"},
		{"fixture/good.txt", "fixture/alias:stream"},
		{"fixture/good.txt", "fixture/NUL.txt"},
		{"fixture/good.txt", "fixture/dot./entry"},
		{"fixture/good.txt", "fixture/SAME.txt", "fixture/same.txt"},
	} {
		root := t.TempDir()
		data, artifact := userWindowsZipFixture(t, names...)
		if _, err := userWindowsZIP(context.Background(), data, artifact, root, ""); err == nil {
			t.Fatalf("unsafe Windows ZIP admitted: %v", names)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("refusal had extraction effects: %v %v", entries, err)
		}
	}
}

func TestUserWindowsArchiveAuthorityAndOwnedMember(t *testing.T) {
	data, artifact := userWindowsZipFixture(t, "fixture/fd.exe")
	member, err := userWindowsZIP(context.Background(), data, artifact, "", "fixture/fd.exe")
	if err != nil || string(member) != "fixture-only, never executed" {
		t.Fatalf("selected bytes differ: %q %v", member, err)
	}
	artifact.SHA = "wrong"
	if _, err := userWindowsZIP(context.Background(), data, artifact, filepath.Join(t.TempDir(), "unowned"), "fixture/fd.exe"); err == nil {
		t.Fatal("archive not bound to acquisition admitted")
	}
	artifact.SHA = userWindowsSHA(data)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := userWindowsZIP(ctx, data, artifact, "", "fixture/fd.exe"); err == nil {
		t.Fatal("canceled archive admission")
	}
}
