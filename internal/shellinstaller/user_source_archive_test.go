package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

// ZIP fixture bytes are DATA, never a publisher/TLS provenance or executable.
func TestUserMainSourceArchiveInventory(t *testing.T) {
	commit := strings.Repeat("a", 40)
	prefix := "gentle-shell-" + commit
	for _, name := range []string{"valid", "native custody", "dependency custody", "case duplicate", "escape", "symlink", "missing package", "parent file", "case parent alias", "wrong descriptor", "bad package CRC", "canceled", "wrong digest"} {
		t.Run(name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			members := []string{"package.json", "bin/entry.mjs"}
			switch name {
			case "native custody":
				members = append(members, ".gentle-ai/bin")
			case "dependency custody":
				members = append(members, "node_modules/pkg/index.js")
			case "case duplicate":
				members = append(members, "PACKAGE.JSON")
			case "escape":
				members = append(members, "../escaped")
			case "symlink":
				members = append(members, "alias")
			case "missing package":
				members = members[1:]
			case "parent file":
				members = append(members, "bin")
			case "case parent alias":
				members = append(members, "BIN")
			}
			for _, member := range members {
				header := &zip.FileHeader{Name: prefix + "/" + member, Method: zip.Store}
				header.SetMode(0600)
				if name == "symlink" && member == "alias" {
					header.SetMode(os.ModeSymlink | 0600)
				}
				stream, err := writer.CreateHeader(header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := stream.Write([]byte("source DATA " + member)); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			data := buffer.Bytes()
			if name == "bad package CRC" {
				index := bytes.Index(data, []byte("source DATA package.json"))
				if index < 0 {
					t.Fatal("package DATA not found")
				}
				data[index] ^= 1
			}
			artifact, err := userMainDescriptor(commit, fmt.Sprintf("%x", sha256.Sum256(data)))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "canceled" {
				cancel()
			}
			if name == "wrong digest" {
				artifact.SHA = strings.Repeat("0", 64)
			}
			if name == "wrong descriptor" {
				artifact.Archive = "node.zip"
			}
			files, err := userMainSourceFiles(ctx, data, artifact)
			if name != "valid" {
				if err == nil || files != nil {
					t.Fatal("unusable Main source returned inventory", name, files, err)
				}
				return
			}
			if err != nil || len(files) != 3 || files["package.json"] == nil || files["bin/entry.mjs"] == nil || files["bin"] != nil {
				t.Fatal("exact source/directory inventory differs", files, err)
			}
		})
	}
}
