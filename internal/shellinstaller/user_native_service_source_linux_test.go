//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// These fixtures capture real file identities, not process admission. The
// production wrappers must check the kernel before using these same leaves.
func userNativeServiceFileFixture(ctx context.Context, root string, artifact userSnapshotArtifact, mask, managerRuntime string) (*userNativeServiceCustody, error) {
	custody := &userNativeServiceCustody{root: root, artifact: artifact, mask: mask, managerRuntime: managerRuntime}
	identities, err := custody.captureFiles(ctx)
	if err != nil {
		return nil, err
	}
	custody.identities = identities
	return custody, nil
}

func userNativeServiceFileArguments(custody *userNativeServiceCustody, ctx context.Context, unit string, interactive bool, self, cwd string, args, env []string) ([]string, error) {
	identities, err := custody.captureFiles(ctx)
	if err != nil {
		return nil, err
	}
	return custody.argsForIdentities(identities, unit, interactive, self, cwd, args, env)
}

func TestUserNativeServiceRequiresProcessAdmission(t *testing.T) {
	ctx := context.Background()
	root := userMainCustodyStage(t)
	artifact, data := userNativeSourceFixture(t)
	if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
		t.Fatal(err)
	}
	if err := userStageMainSource(ctx, root, artifact); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	mask, runtime := filepath.Join(parent, "empty"), filepath.Join(parent, "manager")
	for _, path := range []string{mask, runtime} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"HOME=" + parent, "XDG_RUNTIME_DIR=" + runtime, "DBUS_SESSION_BUS_ADDRESS=unix:path=" + runtime + "/bus", "PATH=/usr/bin:/bin"}
	args := []string{"internal-install", "selected"}
	before, err := userTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	files, err := userNativeServiceFileFixture(ctx, root, artifact, mask, runtime)
	if err != nil {
		t.Fatal("file-custody baseline must work independently of process admission", err)
	}
	kernelErr := UserKernelCheck()
	prepared, prepareErr := prepareUserNativeService(ctx, root, artifact, mask, runtime)
	built, buildErr := files.args(ctx, "owned-unit", false, "/qualified/self", parent, args, env)
	if kernelErr != nil {
		if prepareErr == nil || prepareErr.Error() != kernelErr.Error() || prepared != nil {
			t.Fatal("preparation bypassed or replaced kernel refusal", prepared, prepareErr, kernelErr)
		}
		if buildErr == nil || buildErr.Error() != kernelErr.Error() || built != nil {
			t.Fatal("argument builder treated file identities as process admission", built, buildErr, kernelErr)
		}
	} else {
		if prepareErr != nil || prepared == nil || buildErr != nil {
			t.Fatal("qualified public boundary refused valid custody", prepareErr, buildErr)
		}
		wanted, err := userNativeServiceFileArguments(files, ctx, "owned-unit", false, "/qualified/self", parent, args, env)
		if err != nil || !reflect.DeepEqual(built, wanted) || !reflect.DeepEqual(prepared.identities, files.identities) {
			t.Fatal("qualified wrapper changed the file/argument contract", built, wanted, err)
		}
		fromPrepared, err := prepared.args(ctx, "owned-unit", false, "/qualified/self", parent, args, env)
		if err != nil || !reflect.DeepEqual(fromPrepared, wanted) {
			t.Fatal("prepared public custody changed the argument contract", fromPrepared, wanted, err)
		}
	}
	if after, err := userTreeStamp(root); err != nil || before != after {
		t.Fatal("admission or argument checking changed source", err)
	}
}
