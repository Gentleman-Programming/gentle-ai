//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"testing"
)

func TestUserPNPMInventoryWhileSyncing(t *testing.T) {
	inventoryStarted := make(chan struct{})
	syncStarted := make(chan struct{})
	inventoryFinished := false
	got, err := userPNPMInventoryWhileSyncing(context.Background(), func(context.Context) (map[string]string, error) {
		close(inventoryStarted)
		<-syncStarted
		inventoryFinished = true
		return map[string]string{"payload": "verified"}, nil
	}, func(context.Context) error {
		<-inventoryStarted
		close(syncStarted)
		return nil
	})
	if err != nil || got["payload"] != "verified" || !inventoryFinished {
		t.Fatal("operations were not joined", got, err)
	}
}

func TestUserPNPMInventoryWhileSyncingFailure(t *testing.T) {
	sentinel := errors.New("boundary failure")
	for _, name := range []string{"inventory", "sync", "already canceled"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "already canceled" {
				cancel()
			}
			inventoryFinished, syncFinished := false, false
			inventory := func(ctx context.Context) (map[string]string, error) {
				defer func() { inventoryFinished = true }()
				if name == "already canceled" {
					t.Error("started canceled inventory")
				}
				if name == "inventory" {
					return nil, sentinel
				}
				<-ctx.Done()
				return nil, ctx.Err()
			}
			synchronize := func(ctx context.Context) error {
				defer func() { syncFinished = true }()
				if name == "already canceled" {
					t.Error("started canceled synchronization")
				}
				if name == "sync" {
					return sentinel
				}
				<-ctx.Done()
				return ctx.Err()
			}
			got, err := userPNPMInventoryWhileSyncing(ctx, inventory, synchronize)
			if name == "already canceled" {
				if !errors.Is(err, context.Canceled) || inventoryFinished || syncFinished {
					t.Fatal("canceled call changed state", err)
				}
			} else if !errors.Is(err, sentinel) || !inventoryFinished || !syncFinished {
				t.Fatal("failure returned before both operations ended", err)
			}
			if got != nil {
				t.Fatal("failed inventory published", got)
			}
		})
	}
}
