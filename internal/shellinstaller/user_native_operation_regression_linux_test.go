//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIndependentNativePlanBounds(t *testing.T) {
	root := t.TempDir()
	op := &userNativeSlice{name: "gentleshell0123456789abcdef.slice", group: userUnitEvidence{Path: root}}
	before, e := userTreeStamp(root)
	if e != nil {
		t.Fatal(e)
	}
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	extended, cancelExtended := context.WithTimeout(context.Background(), 141*time.Second)
	defer cancelExtended()
	canceled, cancelCanceled := context.WithTimeout(context.Background(), time.Second)
	cancelCanceled()
	for name, ctx := range map[string]context.Context{"nil": nil, "unbounded": context.Background(), "expired": expired, "extended": extended, "canceled": canceled} {
		t.Run(name, func(t *testing.T) {
			if p, e := newUserNativeServicePlan(ctx, op, nil); e == nil || p != nil {
				t.Fatal("invalid deadline accepted", e)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p, e := newUserNativeServicePlan(ctx, op, nil)
	if e != nil || p == nil || p.cancel != nil {
		t.Fatal("read-only plan differs", e)
	}
	if args, e := p.arguments("probe", false, "/probe/self", "/fixture", nil, nil); e == nil || args != nil {
		t.Fatal("unbound operation reached service", args, e)
	}
	if e := op.stop(ctx); e == nil {
		t.Fatal("read-only stop accepted")
	}
	after, e := userTreeStamp(root)
	if e != nil || before != after {
		t.Fatal("refusal changed state", e)
	}
}
func TestIndependentNativePlanPrivate(t *testing.T) {
	if os.Getenv("GENTLE_NATIVE_STOP_TEST") != "parent" {
		t.Skip("requires isolated private manager")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e := userNativeCoordinatorCheck(); e != nil {
		t.Fatal(e)
	}
	root := "/fixture/native-worker"
	if e := userPrepareInstallStage(ctx, root, "separate", nil); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(root+"/runtime", 0700); e != nil {
		t.Fatal(e)
	}
	w, e := StageNativeInstallerCarrier(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	s, e := newUserNativeSlice(ctx)
	if e != nil {
		t.Fatal(e)
	}
	c, e := prepareUserNativeService(ctx, root, *w.sourceSnapshot, "/fixture/empty-runtime", "/run/user/1000")
	if e != nil {
		t.Fatal(e)
	}
	p, e := newUserNativeServicePlan(ctx, s, c)
	if e != nil {
		t.Fatal(e)
	}
	args, e := p.arguments("independent-native", false, "/probe/self", "/fixture", []string{"internal-native-produce"}, s.env)
	if e != nil {
		t.Fatal(e)
	}
	sealed, e := c.args(ctx, "independent-native", false, "/probe/self", "/fixture", []string{"internal-native-produce"}, append(append([]string(nil), s.env...), "GOMAXPROCS=1"))
	if e != nil {
		t.Fatal(e)
	}
	slice, e := s.serviceArg(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(args, append([]string{slice}, sealed...)) {
		t.Fatal("sealed arguments differ", args)
	}
	joined := strings.Join(args, "\n")
	for _, expected := range []string{"--property=PrivateUsers=yes", "--property=ReadOnlyPaths=" + root + "/runtime/main " + root + "/runtime/main-source /sys/fs/cgroup", "--property=BindReadOnlyPaths=/fixture/empty-runtime:/run/user/1000"} {
		if !strings.Contains(joined, expected+"\n") {
			t.Fatal("missing sealed property", expected)
		}
	}
	originalIdentity := s.runtimeIdentity
	s.runtimeIdentity = "changed"
	if e := p.cancel(); e == nil || !strings.Contains(e.Error(), "uncertain") {
		t.Fatal("manager preimage drift not uncertain", e)
	}
	s.runtimeIdentity = originalIdentity
	if e := s.read(); e != nil {
		t.Fatal("rejected stop changed aggregate", e)
	}
	if e := s.stop(ctx); e != nil {
		t.Fatal(e)
	}
	t.Log("AUTHENTIC_SEALED_ARGUMENTS_AND_PREIMAGE_REFUSAL")
}
