//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestUserNativeReplaceOwner(t *testing.T) {
	parent := filepath.Dir(installStagePath(t))
	req := UserInstallRequest{Destination: filepath.Join(parent, "final"), Mode: "separate", Experience: &UserExperience{Persona: "neutral", BackgroundSubagents: "off"}}
	encoded, err := req.Experience.Encode()
	if err != nil {
		t.Fatal(err)
	}
	token, err := InspectUserInstall(req)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"install", req.Destination, "separate", "", "", token, "--experience", encoded}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	before := nativeChildFixtureStamp(t, parent)
	sentinel := errors.New("exec failure")
	calls := 0
	err = userReplaceOwner(ctx, self, args, os.Stdin, os.Stdout, os.Stderr, func(path string, argv, env []string) error {
		calls++
		if path != "/proc/self/exe" || len(argv) != len(args)+3 || argv[0] != self || argv[1] != "shell" || argv[2] != "internal-native-own" || !slices.Equal(argv[4:], args[1:]) {
			t.Fatal("changed executable or selection", path, argv)
		}
		if argv[3] != strconv.FormatInt(deadline.UnixNano(), 10) {
			t.Fatal("extended parent deadline", argv[3])
		}
		count := 0
		for _, e := range env {
			if strings.HasPrefix(e, "GOMAXPROCS=") {
				count++
				if e != "GOMAXPROCS=1" {
					t.Fatal(e)
				}
			}
		}
		if count != 1 {
			t.Fatal("missing unique control", env)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || calls != 1 || before != nativeChildFixtureStamp(t, parent) {
		t.Fatal("replacement failure changed state", err, calls)
	}
}

func TestUserNativeReplaceOwnerRefusals(t *testing.T) {
	for _, name := range []string{"canceled", "unconfirmed", "stale", "wrong executable", "custom streams"} {
		t.Run(name, func(t *testing.T) {
			parent := filepath.Dir(installStagePath(t))
			req := UserInstallRequest{Destination: parent + "/final", Mode: "separate"}
			token, err := InspectUserInstall(req)
			if err != nil {
				t.Fatal(err)
			}
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			args := []string{"install", req.Destination, "separate", "", "", token}
			var out bytes.Buffer
			switch name {
			case "canceled", "custom streams":
				cancel()
			case "unconfirmed":
				args[5] = ""
			case "stale":
				args[5] = strings.Repeat("0", 64)
			case "wrong executable":
				self = "/not/the/current/binary"
			}
			before := nativeChildFixtureStamp(t, parent)
			exec := func(string, []string, []string) error { t.Fatal("unsafe replacement"); return nil }
			if name == "custom streams" {
				err = userReplaceOwner(ctx, self, args, nil, &out, &out, exec)
			} else {
				err = userReplaceOwner(ctx, self, args, os.Stdin, os.Stdout, os.Stderr, exec)
			}
			if err == nil || out.Len() != 0 || before != nativeChildFixtureStamp(t, parent) {
				t.Fatal("refusal changed state", err, out.String())
			}
		})
	}
}

func TestUserNativeOwnerReentryRefusals(t *testing.T) {
	for _, name := range []string{"missing", "expired", "extended", "stale", "existing"} {
		t.Run(name, func(t *testing.T) {
			parent := filepath.Dir(installStagePath(t))
			req := UserInstallRequest{Destination: parent + "/final", Mode: "separate"}
			token, err := InspectUserInstall(req)
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Minute)
			if name == "expired" {
				deadline = time.Now().Add(-time.Second)
			}
			if name == "extended" {
				deadline = time.Now().Add(141 * time.Second)
			}
			if name == "stale" {
				token = strings.Repeat("0", 64)
			}
			if name == "existing" {
				if err := os.Mkdir(req.Destination, 0700); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"internal-native-own", strconv.FormatInt(deadline.UnixNano(), 10), req.Destination, "separate", "", "", token}
			if name == "missing" {
				args = args[:1]
			}
			before := nativeChildFixtureStamp(t, parent)
			var out bytes.Buffer
			err = RunUserEntry(context.Background(), "", args, nil, &out, &out)
			if err == nil || out.Len() != 0 || before != nativeChildFixtureStamp(t, parent) {
				t.Fatal("unsafe reentry changed state", err, out.String())
			}
		})
	}
}

func TestUserNativeOwnerEnvironment(t *testing.T) {
	env := []string{"HOME=/private", "GOMAXPROCS=8", "TERM=xterm", "GOMAXPROCS=2"}
	before := slices.Clone(env)
	got := userNativeOwnerEnvironment(env)
	if !slices.Equal(env, before) || !slices.Equal(got, []string{"HOME=/private", "TERM=xterm", "GOMAXPROCS=1"}) {
		t.Fatal("environment changed", env, got)
	}
}
