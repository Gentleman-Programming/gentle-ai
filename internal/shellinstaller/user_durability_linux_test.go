//go:build linux

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type userSyncWitness struct {
	name   string
	events *[]string
	fail   error
}

func (f userSyncWitness) Write(data []byte) (int, error) {
	*f.events = append(*f.events, f.name+" write")
	return len(data), nil
}
func (f userSyncWitness) Sync() error {
	*f.events = append(*f.events, f.name+" sync")
	return f.fail
}
func (f userSyncWitness) Close() error {
	*f.events = append(*f.events, f.name+" close")
	return nil
}

func TestUserPublicationSynchronizesFileBeforeDirectory(t *testing.T) {
	for _, failure := range []string{"none", "file", "directory"} {
		t.Run(failure, func(t *testing.T) {
			var events []string
			injected := errors.New("injected sync failure")
			open := func(path string, flags int, mode os.FileMode) (userSyncedFile, error) {
				name := "file"
				if path == "/owned" {
					name = "directory"
				}
				events = append(events, name+" open")
				var err error
				if name == failure {
					err = injected
				}
				return userSyncWitness{name, &events, err}, nil
			}
			err := userWriteWithSync("/owned/installation.json", []byte("manifest"), 0600, open)
			if (failure == "none" && err != nil) || (failure != "none" && !errors.Is(err, injected)) {
				t.Fatalf("durability failure lost: %v", err)
			}
			want := []string{"file open", "file write", "file sync", "file close"}
			if failure != "file" {
				want = append(want, "directory open", "directory sync", "directory close")
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("publication ordering: got %v want %v", events, want)
			}
		})
	}
}

func TestUserPublicationRefusesOccupiedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "supervisor")
	before := []byte("preserved evidence")
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	if err := userToolWrite(path, []byte("replacement"), 0700); !errors.Is(err, os.ErrExist) {
		t.Fatalf("occupied publication not refused: %v", err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
		t.Fatalf("occupied evidence changed: %v", err)
	}
}
