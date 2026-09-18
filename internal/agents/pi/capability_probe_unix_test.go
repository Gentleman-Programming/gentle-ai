//go:build !windows

package pi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUnixTransportDispatchIsTypedUnsupported(t *testing.T) {
	_, err := RunBoundedModelRoutingProcess(context.Background(), validTransportPath(t), nil, validTransportOptions())
	requireTransportKind(t, err, TransportErrorUnsupportedPlatform)
	if errors.Is(err, &TransportError{Kind: TransportErrorInvalidPath}) || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("unsupported dispatch error=%v", err)
	}
}

func TestUnixBoundedTransportOutputRetainsOnlyLimitPlusOne(t *testing.T) {
	for _, tc := range []struct {
		name, input, retained string
		limit, count          int
		overflow              bool
	}{
		{"exact", "abc", "abc", 3, 3, false}, {"one-over", "abcd", "abcd", 3, 4, true},
		{"many-over", "abcdef", "abcd", 3, 6, true}, {"zero-limit", "a", "a", 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := &boundedTransportOutput{limit: tc.limit}
			n, err := output.Write([]byte(tc.input))
			if err != nil || n != len(tc.input) || string(output.buf.Bytes()) != tc.retained || output.count != tc.count || output.overflow != tc.overflow {
				t.Fatalf("Write()=(%d,%v), output=%q count=%d overflow=%v", n, err, output.buf.Bytes(), output.count, output.overflow)
			}
		})
	}
}

func TestUnixDrainTransportOutputConsumesReader(t *testing.T) {
	output := &boundedTransportOutput{limit: 3}
	done := drainTransportOutput(strings.NewReader("drained"), output)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("drain did not finish")
	}
	if got := string(output.buf.Bytes()); got != "drai" || !output.overflow || len(output.buf.Bytes()) > output.limit+1 {
		t.Fatalf("drained output=%q overflow=%v", got, output.overflow)
	}
}
