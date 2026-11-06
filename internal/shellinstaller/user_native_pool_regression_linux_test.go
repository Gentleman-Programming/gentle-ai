//go:build linux && amd64

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIndependentNativeGenericPoolOutput(t *testing.T) {
	env := []string{"HOME=/fixture/home", "GOMAXPROCS=8", "TOKEN=GOMAXPROCS=keep"}
	original := append([]string(nil), env...)
	signatures := map[string]string{}
	for _, interactive := range []bool{false, true} {
		args := userServiceArgs("fixture-unit", interactive, "/probe/self", "/fixture", []string{"internal-check"}, env)
		if !reflect.DeepEqual(env, original) {
			t.Fatal("generic caller environment changed")
		}
		if strings.Count(strings.Join(args, "\n"), "GOMAXPROCS=8\n") != 1 {
			t.Fatal("generic pool assignment changed")
		}
		data, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		signatures[fmt.Sprint(interactive)] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	data, err := json.Marshal(signatures)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("GENERIC_ARGUMENT_SIGNATURE", string(data))
}

func TestIndependentNativePoolPrivate(t *testing.T) {
	if os.Getenv("GENTLE_PRIVATE_POOL_TEST") != "parent" {
		t.Skip("requires isolated private manager")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := userNativeCoordinatorCheck(); err != nil {
		t.Fatal(err)
	}
	root := "/fixture/native-worker"
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
	operation, err := newUserNativeSlice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sealed := range []bool{false, true} {
		t.Run(fmt.Sprint("sealed=", sealed), func(t *testing.T) {
			selected := append(append([]string{}, operation.env...), "GOMAXPROCS=8", "GOMAXPROCS=", "TOKEN=GOMAXPROCS=keep", "GOMAXPROCS=2")
			backing := make([]string, len(selected)+3)
			copy(backing, selected)
			copy(backing[len(selected):], []string{"spare-one", "spare-two", "spare-three"})
			env := backing[:len(selected)]
			before := append([]string{}, backing...)
			wantedEnv := make([]string, 0, len(env))
			for _, value := range env {
				if !strings.HasPrefix(value, "GOMAXPROCS=") {
					wantedEnv = append(wantedEnv, value)
				}
			}
			wantedEnv = append(wantedEnv, "GOMAXPROCS=1")
			var selectedCustody *userNativeServiceCustody
			if sealed {
				selectedCustody = custody
			}
			plan, err := newUserNativeServicePlan(ctx, operation, selectedCustody)
			if err != nil {
				t.Fatal(err)
			}
			transport := []string{"internal-check", "GOMAXPROCS=opaque-argument"}
			actual, err := plan.arguments("pool-probe", sealed, "/probe/self", "/fixture", transport, env)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(backing, before) {
				t.Fatal("caller slice or spare capacity changed")
			}
			start, end := -1, -1
			for index, value := range actual {
				if value == "/usr/bin/env" && index+1 < len(actual) && actual[index+1] == "-i" {
					start = index + 2
				}
				if start >= 0 && value == userCapabilityDrop {
					end = index
					break
				}
			}
			if start < 0 || end < start || !reflect.DeepEqual(actual[start:end], wantedEnv) {
				t.Fatal("sanitized service environment differs", actual)
			}
			fixed := 0
			for _, value := range actual[start:end] {
				if strings.HasPrefix(value, "GOMAXPROCS=") {
					fixed++
					if value != "GOMAXPROCS=1" {
						t.Fatal("caller pool survived", value)
					}
				}
			}
			if fixed != 1 {
				t.Fatal("pool assignment is not unique", fixed)
			}
			slice, err := operation.serviceArg(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var wanted []string
			if sealed {
				wanted, err = custody.args(ctx, "pool-probe", sealed, "/probe/self", "/fixture", transport, wantedEnv)
				wanted = append([]string{slice}, wanted...)
			} else {
				wanted = append([]string{slice, "--property=CapabilityBoundingSet=", "--property=RuntimeMaxSec=300s"}, userServiceArgs("pool-probe", sealed, "/probe/self", "/fixture", transport, wantedEnv)...)
			}
			if err != nil || !reflect.DeepEqual(actual, wanted) {
				t.Fatal("non-pool service arguments changed", err)
			}
			t.Log("UNIQUE_FIXED_POOL_AFTER_ENV_I_CALLER_UNCHANGED", sealed)
		})
	}
	if err := operation.settle(ctx); err != nil {
		t.Fatal(err)
	}
}
