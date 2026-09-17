package sddstatus

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGrantRefusesMarkerReplacementDuringMutation(t *testing.T) {
	for _, phase := range []string{"before", "before-replay", "after", "after-replay", "publication-error"} {
		t.Run(phase, func(t *testing.T) {
			repo := t.TempDir()
			initEditAuthorityGitRepo(t, repo, true)
			const change = "grant-marker-race"
			root := seedReadyChange(t, repo, change, "- [ ] Update source\n")
			marker := filepath.Join(root, changeInstanceMarkerFile)
			a, b := "sdd-"+strings.Repeat("a", 32), "sdd-"+strings.Repeat("b", 32)
			write(t, marker, a+"\n")
			opened, err := OpenRuntimeStore(t.Context(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			store, err := opened.ForCurrentChangeInstance(a)
			if err != nil {
				t.Fatal(err)
			}
			request := GrantRootsRequest{RequestID: "race-grant", Roots: []string{repo}, Actor: "test", Reason: "marker boundary"}
			replayed := strings.HasSuffix(phase, "replay")
			before, err := store.Status()
			if err != nil {
				t.Fatal(err)
			}
			if replayed {
				before, err = store.Grant(t.Context(), request)
				if err != nil || len(before.GrantedRoots) != 1 {
					t.Fatalf("valid grant: %#v %v", before, err)
				}
				valid, err := store.Grant(t.Context(), request)
				if err != nil || !reflect.DeepEqual(valid, before) {
					t.Fatalf("valid replay changed authority: %#v %v", valid, err)
				}
			}
			receiptRevision := before.Revision
			if replayed {
				later := request
				later.RequestID, later.ExpectedRevision = "later-grant", before.Revision
				before, err = store.Grant(t.Context(), later)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := store.ensureDirectories(); err != nil {
				t.Fatal(err)
			}
			clock, sync := runtimeGrantClock, runtimeSyncDirectory
			t.Cleanup(func() { runtimeGrantClock, runtimeSyncDirectory = clock, sync })
			injected := errors.New("injected directory sync failure")
			if strings.HasPrefix(phase, "before") {
				runtimeGrantClock = func() string { write(t, marker, b+"\n"); return clock() }
			} else {
				runtimeSyncDirectory = func(dir string) error {
					if err := sync(dir); err != nil {
						return err
					}
					if dir == store.Dir {
						write(t, marker, b+"\n")
						if phase == "publication-error" {
							return injected
						}
					}
					return nil
				}
			}
			result, err := store.Grant(t.Context(), request)
			if err == nil || len(result.GrantedRoots) != 0 {
				t.Errorf("stale result usable: %#v %v", result, err)
			}
			after, readErr := store.Status()
			if readErr != nil {
				t.Fatal(readErr)
			}
			var publication *RuntimePublicationError
			committed := !strings.HasPrefix(phase, "before")
			if !replayed {
				receiptRevision = after.Revision
			}
			if errors.As(err, &publication) != committed || (committed && (!publication.Committed || publication.Revision != receiptRevision)) {
				t.Errorf("incorrect committed state: %#v %v", publication, err)
			}
			if (replayed || !committed) && !reflect.DeepEqual(after, before) {
				t.Errorf("refusal/replay changed history: before=%#v after=%#v", before, after)
			}
			if committed && (after.Revision == "" || len(after.GrantedRoots) != 1) {
				t.Error("committed historical grant lost")
			}
			if committed && phase != "publication-error" && err != nil && strings.Contains(err.Error(), "requires exact replay") {
				t.Errorf("stale receipt encouraged unsafe replay: %v", err)
			}
			if phase == "publication-error" && !errors.Is(err, injected) {
				t.Errorf("publication failure masked: %v", err)
			}
			boundB, _ := opened.ForInstance(b)
			isolated, err := boundB.Status()
			if err != nil || len(isolated.GrantedRoots) != 0 {
				t.Fatalf("replacement inherited old roots: %#v %v", isolated, err)
			}
		})
	}
}
