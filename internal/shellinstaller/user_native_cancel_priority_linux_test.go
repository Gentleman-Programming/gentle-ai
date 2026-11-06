//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUserNativeRuntimeCancellationPriority(t *testing.T) {
	for _, name := range []string{"canceled", "deadline"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if name == "deadline" {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Unix(1, 0))
			} else {
				cancel()
			}
			defer cancel()
			cause := errors.New("process group settlement is unknown")
			uncertain := privateError("uncertain", cause)
			got := nativeRuntimeError(ctx, uncertain)
			var failure *PrivateRuntimeError
			if !errors.As(got, &failure) || failure.Kind != "uncertain" || !errors.Is(got, cause) {
				t.Fatal("context must not downgrade uncertain settlement", got)
			}
		})
	}
}
