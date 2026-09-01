package sddstatus

import (
	"context"
	"testing"
)

func mustRuntimeStore(t *testing.T, repo, change string) RuntimeStore {
	t.Helper()
	store, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
