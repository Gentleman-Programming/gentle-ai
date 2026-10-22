//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Existing destinations and Shared retain their authenticated legacy flow.
func userNativeFreshRequest(args []string) (UserInstallRequest, bool, error) {
	req, err := UserInstallFromEntry(args)
	if err != nil {
		return req, false, err
	}
	if req.Mode != "separate" {
		return req, false, nil
	}
	req, err = userCanonicalRequest(req)
	if err != nil {
		return req, true, err
	}
	reconfigure := req.Experience != nil && userReconfigureTarget() == req.Destination
	if !reconfigure {
		if _, err = os.Lstat(req.Destination); err == nil {
			return req, false, nil
		} else if !os.IsNotExist(err) {
			return req, true, err
		}
	} else if _, err = os.Lstat(req.Destination); err != nil {
		return req, false, nil
	}
	token, err := InspectUserInstall(req)
	if err != nil || token != req.Confirmation {
		return req, true, privateError("refused", errors.Join(err, errors.New("physical selection confirmation differs")))
	}
	// SDK Main + stock retains its existing authenticated Main worker.
	// Keep the original profile and token; never clear its SDK selector.
	if req.Experience != nil && (req.Experience.SDKSource == "main-preview" || req.Experience.GGA == "on") {
		return req, false, nil
	}
	return req, true, userValidateStageExperience(req.Experience)
}

func userNativeCoordinate(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) < 8 {
		return errors.New("invalid native coordinator arguments")
	}
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return err
	}
	deadline, err := userNativeDeadline(args[2], time.Now())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := userNativeCoordinatorCheck(); err != nil {
		return err
	}
	operation, err := userNativeOperation(ctx)
	if err != nil {
		return err
	}
	req, fresh, err := userNativeFreshRequest(args[3:])
	if err != nil || !fresh {
		return privateError("refused", errors.Join(err, errors.New("native coordinator requires confirmed fresh Separate selection")))
	}
	workspace, stage := args[0], args[1]
	if !userSelectionPath(workspace) || filepath.Dir(workspace) != filepath.Dir(req.Destination) ||
		!strings.HasPrefix(filepath.Base(workspace), ".gentle-native-") || stage != workspace+"-stage" {
		return privateError("destination", nil)
	}
	identity, err := privateDirectory(workspace)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		return privateError("preimage", err)
	}
	if err := privateDestination(stage); err != nil {
		return err
	}
	root := filepath.Join(workspace, "producer")
	if err := userPrepareNativeProducer(ctx, root, req.Experience); err != nil {
		return err
	}
	if err := userBootstrap(ctx, root, workspace); err != nil {
		return err
	}
	var codegraph [][]byte
	if req.Experience != nil && req.Experience.CodeGraph == "on" {
		codegraph, err = userAcquireCodeGraphArchives(ctx)
		if err != nil {
			return err
		}
	}
	worker, err := StageNativeInstallerCarrier(ctx, root)
	if err != nil {
		return err
	}
	if _, err := privateNativeFile(ctx, worker.node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		return err
	}
	mask := filepath.Join(workspace, "empty-runtime")
	if err := os.Mkdir(mask, 0700); err != nil {
		return err
	}
	if _, err := userBusEnvironment(); err != nil {
		return err
	}
	custody, err := prepareUserNativeService(ctx, root, *worker.sourceSnapshot, mask, "/run/user/"+strconv.Itoa(os.Getuid()))
	if err != nil {
		return err
	}
	plan, err := newUserNativeServicePlan(ctx, operation, custody)
	if err != nil {
		return err
	}
	entry := append([]string{"internal-native-produce", root, stage, args[2]}, args[3:]...)
	if err := userServiceWithPlan(ctx, self, entry, stdin, stdout, stderr, plan); err != nil {
		return err
	}
	// Publication runs only after the sealed producer's physical termination.
	// Keep root ownership visible; never relax supervisor provenance in a user namespace.
	if err := operation.member(ctx); err != nil {
		return err
	}
	req, fresh, err = userNativeFreshRequest(args[3:])
	if err != nil || !fresh {
		return privateError("refused", err)
	}
	native, location, err := userReadNativeReceipt(ctx, root)
	if err != nil {
		return err
	}
	installed, err := userPublishNativePNPMPrepared(ctx, root, stage, req.Destination, *location, req.Experience, codegraph, req.Mode, req.SharedAgent)
	if err != nil {
		return err
	}
	if installed.State != "ComponentInstalled" || installed.Destination == "" {
		return userNativePublicationFailure(native)
	}
	if _, err := fmt.Fprintf(stdout, "Installed %s; commands %s/bin/gentle-shell and %s/bin/pi\n", installed.Destination, installed.Destination, installed.Destination); err != nil {
		return err
	}
	// The producer's physical receipt has settled. Cleanup remains inside the
	// capped coordinator; outer cancellation bounds it with the same deadline.
	return privateCleanup(workspace, identity)
}
