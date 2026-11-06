//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"testing"
)

func TestIndependentPNPMOverlapCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inventoryEntered, syncEntered := make(chan struct{}), make(chan struct{})
	inventoryCanceled, syncCanceled := make(chan struct{}), make(chan struct{})
	releaseInventory, releaseSync := make(chan struct{}), make(chan struct{})
	returned := make(chan error, 1)
	go func() {
		got, err := userPNPMInventoryWhileSyncing(ctx, func(ctx context.Context) (map[string]string, error) {
			close(inventoryEntered)
			<-ctx.Done()
			close(inventoryCanceled)
			<-releaseInventory
			return nil, ctx.Err()
		}, func(ctx context.Context) error {
			close(syncEntered)
			<-ctx.Done()
			close(syncCanceled)
			<-releaseSync
			return ctx.Err()
		})
		if got != nil {
			err = errors.New("canceled inventory escaped")
		}
		returned <- err
	}()
	<-inventoryEntered
	<-syncEntered
	cancel()
	<-inventoryCanceled
	<-syncCanceled
	select {
	case err := <-returned:
		t.Fatal("returned before joins", err)
	default:
	}
	close(releaseInventory)
	select {
	case err := <-returned:
		t.Fatal("returned before sync join", err)
	default:
	}
	close(releaseSync)
	if err := <-returned; !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation", err)
	}
}

func TestIndependentPNPMOverlapRetainsBothErrors(t *testing.T) {
	inventoryErr, syncErr := errors.New("inventory failure"), errors.New("sync failure")
	started := make(chan struct{})
	got, err := userPNPMInventoryWhileSyncing(context.Background(), func(context.Context) (map[string]string, error) { close(started); return nil, inventoryErr }, func(context.Context) error { <-started; return syncErr })
	if got != nil || !errors.Is(err, inventoryErr) || !errors.Is(err, syncErr) {
		t.Fatal("lost joined error", got, err)
	}
}
