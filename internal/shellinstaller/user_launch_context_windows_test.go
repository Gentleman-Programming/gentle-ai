//go:build windows

package shellinstaller

import (
	"context"
	"errors"
	"testing"
)

func TestUserWindowsEntryLifetime(t *testing.T) {
	for _, tt := range []struct {
		action   string
		deadline bool
	}{{"install", true}, {"check", true}, {"launch", false}} {
		t.Run(tt.action, func(t *testing.T) {
			ctx, cancel := userWindowsEntryContext(context.Background(), tt.action)
			defer cancel()
			if _, bounded := ctx.Deadline(); bounded != tt.deadline {
				t.Fatalf("%s deadline=%t; want %t", tt.action, bounded, tt.deadline)
			}
			cancel()
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatal("entry no longer honors cancellation")
			}
		})
	}
}
