//go:build linux && amd64

package shellinstaller

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type independentOwnerProof struct {
	PID      int
	Deadline string
	Profile  string
	Input    string
	Pool     []string
}

func init() {
	if os.Getenv("INDEPENDENT_OWNER_ROLE") != "launch" || len(os.Args) < 4 || os.Args[1] != "shell" {
		return
	}
	fail := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(81) }
	if os.Args[2] != "internal-native-own" {
		fail(fmt.Errorf("wrong selector"))
	}
	pid, err := strconv.Atoi(os.Getenv("INDEPENDENT_OWNER_PID"))
	if err != nil || pid != os.Getpid() {
		fail(fmt.Errorf("PID changed"))
	}
	if os.Args[3] != os.Getenv("INDEPENDENT_OWNER_DEADLINE") {
		fail(fmt.Errorf("deadline changed"))
	}
	if _, err = userNativeDeadline(os.Args[3], time.Now()); err != nil {
		fail(err)
	}
	req, err := UserInstallFromEntry(os.Args[4:])
	if err != nil {
		fail(err)
	}
	token, err := InspectUserInstall(req)
	if err != nil || token != req.Confirmation {
		fail(fmt.Errorf("physical confirmation changed: %v", err))
	}
	if req.Experience == nil || req.Experience.Persona != "neutral" || req.Experience.BackgroundSubagents != "off" {
		fail(fmt.Errorf("profile changed"))
	}
	pools := []string{}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOMAXPROCS=") {
			pools = append(pools, e)
		}
	}
	if len(pools) != 1 || pools[0] != "GOMAXPROCS=1" {
		fail(fmt.Errorf("pool changed: %v", pools))
	}
	input, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil || input != "inherited-input\n" {
		fail(fmt.Errorf("stdin changed: %q %v", input, err))
	}
	proof := independentOwnerProof{PID: os.Getpid(), Deadline: os.Args[3], Profile: req.Experience.Persona + "/" + req.Experience.BackgroundSubagents, Input: input, Pool: pools}
	// Register before readiness so the parent can send a deterministic signal.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	if err = json.NewEncoder(os.Stdout).Encode(proof); err != nil {
		fail(err)
	}
	fmt.Fprintln(os.Stderr, "INHERITED_STDERR")
	if os.Getenv("INDEPENDENT_OWNER_SIGNAL") == "yes" {
		<-signals
		fmt.Fprintln(os.Stderr, "SIGNAL_RECEIVED")
		os.Exit(43)
	}
	os.Exit(0)
}

func TestIndependentNativeOwnerTransport(t *testing.T) {
	if os.Getenv("INDEPENDENT_OWNER_ROLE") == "launch" {
		deadline, err := strconv.ParseInt(os.Getenv("INDEPENDENT_OWNER_DEADLINE"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, deadline))
		defer cancel()
		req := UserInstallRequest{Destination: filepath.Join(os.Getenv("INDEPENDENT_OWNER_PARENT"), "final"), Mode: "separate", Experience: &UserExperience{Persona: "neutral", BackgroundSubagents: "off"}}
		token, err := InspectUserInstall(req)
		if err != nil {
			t.Fatal(err)
		}
		profile, err := req.Experience.Encode()
		if err != nil {
			t.Fatal(err)
		}
		self, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Setenv("INDEPENDENT_OWNER_PID", strconv.Itoa(os.Getpid())); err != nil {
			t.Fatal(err)
		}
		args := []string{"install", req.Destination, "separate", "", "", token, "--experience", profile}
		t.Fatal("replacement unexpectedly returned", RunUserEntryReplacingOwner(ctx, self, args, os.Stdin, os.Stdout, os.Stderr))
	}
	for _, name := range []string{"roundtrip", "signal"} {
		t.Run(name, func(t *testing.T) {
			parent := filepath.Dir(installStagePath(t))
			before := nativeChildFixtureStamp(t, parent)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			deadline := strconv.FormatInt(time.Now().Add(10*time.Second).UnixNano(), 10)
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIndependentNativeOwnerTransport$")
			cmd.Env = append(os.Environ(), "INDEPENDENT_OWNER_ROLE=launch", "INDEPENDENT_OWNER_PARENT="+parent, "INDEPENDENT_OWNER_DEADLINE="+deadline, "GOMAXPROCS=6")
			if name == "signal" {
				cmd.Env = append(cmd.Env, "INDEPENDENT_OWNER_SIGNAL=yes")
			}
			cmd.Stdin = strings.NewReader("inherited-input\n")
			out, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr strings.Builder
			cmd.Stderr = &stderr
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			line, readErr := bufio.NewReader(out).ReadString('\n')
			var proof independentOwnerProof
			decodeErr := json.Unmarshal([]byte(line), &proof)
			if readErr != nil || decodeErr != nil || proof.PID != cmd.Process.Pid || proof.Deadline != deadline || proof.Profile != "neutral/off" || proof.Input != "inherited-input\n" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatalf("actual exec proof: %q %v %v stderr=%s", line, readErr, decodeErr, stderr.String())
			}
			if name == "signal" {
				if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
			}
			err = cmd.Wait()
			if name == "roundtrip" && err != nil {
				t.Fatal(err, stderr.String())
			}
			if name == "signal" {
				exit, ok := err.(*exec.ExitError)
				if !ok || exit.ExitCode() != 43 || !strings.Contains(stderr.String(), "SIGNAL_RECEIVED") {
					t.Fatal(err, stderr.String())
				}
			}
			if !strings.Contains(stderr.String(), "INHERITED_STDERR") || before != nativeChildFixtureStamp(t, parent) {
				t.Fatal("stderr or preimage changed", stderr.String())
			}
			t.Logf("ACTUAL_EXEC samePID=%d profile=%s deadline=%s pool=%v signal=%s preimage=unchanged", proof.PID, proof.Profile, proof.Deadline, proof.Pool, name)
		})
	}
}
