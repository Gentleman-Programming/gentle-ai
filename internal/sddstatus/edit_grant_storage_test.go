package sddstatus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func grantTestStore(t *testing.T) (RuntimeStore, GrantRootsRequest) {
	t.Helper()
	opened := mustRuntimeStore(t, initRuntimeLedgerRepo(t), "grant-storage")
	store, err := opened.ForInstance("grant-storage-instance")
	if err != nil {
		t.Fatal(err)
	}
	return store, GrantRootsRequest{RequestID: "grant-one", Roots: []string{t.TempDir()}, Actor: "maintainer", Reason: "authorized test edit root"}
}

func TestEditGrantCASAndLockPermitOnlyOneConcurrentWriter(t *testing.T) {
	store, request := grantTestStore(t)
	// Keep the #1850 first-use population: do not pre-create the store or lock.
	const writers = 24
	start := make(chan struct{})
	outcomes := make(chan error, writers)
	var workers sync.WaitGroup
	for index := 0; index < writers; index++ {
		workers.Add(1)
		go func(id string) {
			defer workers.Done()
			<-start
			attempt := request
			attempt.RequestID = id
			_, err := store.Grant(context.Background(), attempt)
			outcomes <- err
		}(fmt.Sprintf("grant-%d", index))
	}
	close(start)
	workers.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrRuntimeRevisionConflict) && !errors.Is(err, ErrRuntimeConcurrentUpdate) {
			t.Fatalf("unexpected conflict: %v", err)
		}
	}
	if successes != 1 || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("successes=%d, records=%d", successes, countRuntimeRecords(t, store.Dir))
	}
}

func TestEditGrantRejectsStaleCASWithoutPublishing(t *testing.T) {
	store, request := grantTestStore(t)
	first, err := store.Grant(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.RequestID = "grant-two"
	if _, err := store.Grant(context.Background(), request); !errors.Is(err, ErrRuntimeRevisionConflict) {
		t.Fatalf("stale CAS: %v", err)
	}
	after, err := store.Status()
	if err != nil || after.Revision != first.Revision || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("stale CAS changed history: %#v %v", after, err)
	}
}

func TestEditGrantCannotFollowAuthoritySymlinks(t *testing.T) {
	for _, target := range []string{"store", "lock"} {
		t.Run(target, func(t *testing.T) {
			store, request := grantTestStore(t)
			if err := store.ensureDirectories(); err != nil {
				t.Fatal(err)
			}
			foreign := t.TempDir()
			link := store.Dir
			if target == "store" {
				if err := os.Remove(filepath.Join(store.Dir, "records")); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(store.Dir); err != nil {
					t.Fatal(err)
				}
			} else {
				foreign = filepath.Join(foreign, "LOCK")
				if err := os.WriteFile(foreign, []byte("untouched"), 0600); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(store.Dir, "LOCK")
			}
			if err := os.Symlink(foreign, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := store.Grant(context.Background(), request); err == nil {
				t.Fatal("followed authority symlink")
			}
			if target == "lock" {
				content, err := os.ReadFile(foreign)
				if err != nil || string(content) != "untouched" {
					t.Fatalf("modified foreign lock: %q %v", content, err)
				}
			}
		})
	}
}

func TestEditGrantPublicationFailureCanReplayWithoutDuplicateAuthority(t *testing.T) {
	store, request := grantTestStore(t)
	original := runtimeReplaceHead
	defer func() { runtimeReplaceHead = original }()
	runtimeReplaceHead = func(source, target string) error { return errors.New("simulated head publication failure") }
	if _, err := store.Grant(context.Background(), request); err == nil {
		t.Fatal("publication failure hidden")
	}
	if _, exists, err := readRuntimeHead(filepath.Join(store.Dir, "HEAD")); err != nil || exists {
		t.Fatalf("failed publication advanced HEAD: %v %v", exists, err)
	}
	runtimeReplaceHead = original
	granted, err := store.Grant(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.Grant(context.Background(), request)
	if err != nil || replayed.Revision != granted.Revision || len(replayed.GrantedRoots) != 1 {
		t.Fatalf("replay duplicated or lost grant: %#v %v", replayed, err)
	}
}
