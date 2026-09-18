package organicruntime_test

import (
	"context"
	"reflect"
	"testing"
)

func TestOpenCodeTraceWrapsOnlyTopLevel(t *testing.T) {
	cmd := openCodeTraceCommand(context.Background(), "/fixture/strace", "/fixture/opencode", "/fixture/trace", "run", "--format", "json")
	want := []string{"/fixture/strace", "-ff", "-o", "/fixture/trace", "-e", "trace=connect", "/fixture/opencode", "run", "--format", "json"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("trace argv=%q want%q", cmd.Args, want)
	}
}
