//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func receiptPrivateService(ctx context.Context, plan *userServicePlan, self, mode string) (string, error) {
	original := plan.arguments
	plan.arguments = func(unit string, interactive bool, self, cwd string, args, env []string) ([]string, error) {
		env = append(append([]string{}, env...), "GENTLE_NATIVE_RECEIPT_PRIVATE="+mode)
		values, err := original(unit, interactive, self, cwd, args, env)
		if err != nil {
			return nil, err
		}
		result := make([]string, 0, len(values))
		removed := false
		for _, value := range values {
			if value == "shell" && !removed {
				removed = true
				continue
			}
			result = append(result, value)
		}
		return result, nil
	}
	var out bytes.Buffer
	err := userServiceWithPlan(ctx, self, []string{"-test.run=^TestIndependentNativeReceiptPrivate$", "-test.timeout=110s", "-test.v"}, nil, &out, &out, plan)
	return out.String(), err
}
func TestIndependentNativeReceiptPrivate(t *testing.T) {
	mode := os.Getenv("GENTLE_NATIVE_RECEIPT_PRIVATE")
	if mode == "" {
		t.Skip("requires isolated private manager")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := "/fixture/native-worker"
	if mode != "parent" && mode != "coordinator" {
		if err := userNativeExecutionCheck(root, "/run/user/1000"); err != nil {
			t.Fatal(err)
		}
		if _, err := userNativeOperation(ctx); err != nil {
			t.Fatal(err)
		}
		if mode != "uncertain" {
			if err := userRecordUnit(); err != nil {
				t.Fatal(err)
			}
		}
		info, err := os.Stat(self)
		if err != nil || info.Sys().(*syscall.Stat_t).Uid != 65534 {
			t.Fatal("root-owned executable not unmapped", info, err)
		}
		if _, err := userSupervisorSHA(ctx, self); err == nil {
			t.Fatal("unmapped supervisor ownership accepted")
		}
		native := NativeInstallerResult{Details: json.RawMessage(`{"outcome":"terminal-action-required","action":"open-new-terminal","completed":["synthetic-no-original"]}`)}
		loc := NativeInstallationLocation{PackageRoot: root + "/pnpm/store/v11/links/package/node_modules/gentle-pi", PNPMHome: root + "/pnpm", GlobalBin: root + "/pnpm/bin", ShellVersion: "4.3.0", PiVersion: "1.3.0"}
		if err := userWriteNativeReceipt(root, native, &loc); err != nil {
			t.Fatal(err)
		}
		if mode == "failed" {
			t.Fatal("intentional producer failure after synthetic receipt")
		}
		t.Log("SEALED_RECEIPT_WITH_STRICT_SUPERVISOR_REFUSAL")
		return
	}
	if err := userNativeCoordinatorCheck(); err != nil {
		t.Fatal(err)
	}
	if mode == "parent" {
		operation, err := newUserNativeSlice(ctx)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := newUserNativeServicePlan(ctx, operation, nil)
		if err != nil {
			t.Fatal(err)
		}
		output, err := receiptPrivateService(ctx, plan, self, "coordinator")
		t.Log(output)
		if err != nil || !strings.Contains(output, "FULL_MAP_SETTLED_RECEIPT_BOUNDARY") {
			t.Fatal("actual coordinator failed", err)
		}
		if err := operation.settle(ctx); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := userRecordUnit(); err != nil {
		t.Fatal(err)
	}
	operation, err := userNativeOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root+"/runtime", 0700); err != nil {
		t.Fatal(err)
	}
	worker, err := StageNativeInstallerCarrier(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	custody, err := prepareUserNativeService(ctx, root, *worker.sourceSnapshot, "/fixture/empty-runtime", "/run/user/1000")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("/fixture/final-parent", 0700); err != nil {
		t.Fatal(err)
	}
	destination := "/fixture/final-parent/destination"
	for _, scenario := range []string{"success", "failed", "uncertain"} {
		t.Run(scenario, func(t *testing.T) {
			path := root + "/native-installation.json"
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			plan, err := newUserNativeServicePlan(ctx, operation, custody)
			if err != nil {
				t.Fatal(err)
			}
			output, runErr := receiptPrivateService(ctx, plan, self, scenario)
			t.Log(output)
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatal("destination appeared before settled publication", err)
			}
			if scenario != "success" {
				if runErr == nil {
					t.Fatal("failed or uncertain producer authorized publication")
				}
				var failure *PrivateRuntimeError
				if scenario == "uncertain" && (!errors.As(runErr, &failure) || failure.Kind != "uncertain") {
					t.Fatal("missing physical receipt not uncertain", runErr)
				}
			} else {
				if runErr != nil {
					t.Fatal(runErr)
				}
				if err := operation.member(ctx); err != nil {
					t.Fatal(err)
				}
				native, loc, err := userReadNativeReceipt(ctx, root)
				if err != nil || loc == nil || len(native.Completed) != 1 {
					t.Fatal("settled receipt rejected", err)
				}
				info, err := os.Stat(self)
				if err != nil || info.Sys().(*syscall.Stat_t).Uid != 0 {
					t.Fatal("coordinator does not see trusted root owner", info, err)
				}
				copied := "/fixture/copied-supervisor"
				expected, err := userSupervisorSHA(ctx, self)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := userCopySupervisor(ctx, self, copied)
				if err != nil || actual != expected {
					t.Fatal("full-map supervisor copy failed", err)
				}
				if _, err := privateNativeFile(ctx, copied, 0700, -1, expected); err != nil {
					t.Fatal("copied supervisor provenance lost", err)
				}
				before := nativeChildFixtureStamp(t, root)
				req := []string{destination, "separate", "", "", strings.Repeat("0", 64)}
				if _, fresh, err := userNativeFreshRequest(req); err == nil || !fresh {
					t.Fatal("stale confirmation accepted", err)
				}
				if before != nativeChildFixtureStamp(t, root) {
					t.Fatal("stale refusal changed producer")
				}
			}
			if err := userCompareMainSource(ctx, root, *worker.sourceSnapshot); err != nil {
				t.Fatal("producer changed immutable source", err)
			}
		})
	}
	t.Log("FULL_MAP_SETTLED_RECEIPT_BOUNDARY")
}
