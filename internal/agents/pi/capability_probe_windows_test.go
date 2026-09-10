//go:build windows

package pi

import (
	"context"
	"errors"
	"testing"
)

func TestWindowsTransportDispatchIsTypedUnsupported(t *testing.T) {
	_, err := RunBoundedModelRoutingProcess(context.Background(), validTransportPath(t), nil, validTransportOptions())
	requireTransportKind(t, err, TransportErrorUnsupportedPlatform)
	if errors.Is(err, &TransportError{Kind: TransportErrorInvalidPath}) || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("unsupported dispatch error=%v", err)
	}
}
