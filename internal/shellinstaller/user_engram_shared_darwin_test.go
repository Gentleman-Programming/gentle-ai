//go:build darwin

package shellinstaller

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func userEngramEntryEnv(ctx context.Context, root string, args []string) []string {
	return userEngramContextEnv(userEngramLaunchScope(ctx, root, args))
}

// The supervisor that verifies a darwin launch decides for its Pi arguments:
// a version or help run never reaches the decision, so it never probes or
// starts a server, and gets memory disabled without a notice.
func TestUserEngramDarwinVersionAndHelpLaunchesNeverDecide(t *testing.T) {
	roots := userEngramObserveDecisions(t)
	for _, args := range userEngramQuietArgs {
		if env := userEngramEntryEnv(context.Background(), "/owned", args); !reflect.DeepEqual(env, userEngramQuiet) || len(*roots) != 0 {
			t.Fatalf("%q forwarded %q after deciding for %q", args, env, *roots)
		}
	}
	for i, args := range userEngramDecidingArgs {
		if env := userEngramEntryEnv(context.Background(), "/owned", args); !reflect.DeepEqual(env, []string{"ENGRAM_URL=decided"}) || len(*roots) != i+1 {
			t.Fatalf("%q forwarded %q after deciding for %q", args, env, *roots)
		}
	}
}

func TestUserEngramDarwinLaunchScopeDecidesLazilyForItsRoot(t *testing.T) {
	roots := userEngramObserveDecisions(t)
	ctx := userEngramLaunchScope(context.Background(), "/owned", []string{"--flag"})
	if len(*roots) != 0 {
		t.Fatal("the decision ran before the launch environment was built")
	}
	if env := userEngramContextEnv(ctx); !reflect.DeepEqual(env, []string{"ENGRAM_URL=decided"}) || !reflect.DeepEqual(*roots, []string{"/owned"}) {
		t.Fatalf("darwin launch forwarded %q for roots %q", env, *roots)
	}
}

func TestUserEngramDarwinOnlyVerifiedLaunchesDecide(t *testing.T) {
	roots := userEngramObserveDecisions(t)
	original := userKernelGate
	t.Cleanup(func() { userKernelGate = original })
	userKernelGate = func() error { return nil }
	missing := filepath.Join(t.TempDir(), "missing")
	for _, args := range [][]string{{"check"}, {"internal-check"}, {"install", missing, "separate", "", "", "token"}, {"recover", missing}, {"unknown"}} {
		_ = RunUserEntry(context.Background(), "", args, nil, nil, nil)
	}
	if len(*roots) != 0 {
		t.Fatalf("non-launch entries ran the decision for %q", *roots)
	}
	// An installation that fails launch verification starts no server.
	if err := RunUserEntry(context.Background(), "", []string{"launch", missing}, nil, nil, nil); err == nil || len(*roots) != 0 {
		t.Fatalf("unverified launch: err %v decisions %q", err, *roots)
	}
}
