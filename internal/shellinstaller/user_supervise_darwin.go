//go:build darwin

package shellinstaller

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Darwin has no cgroup2 or systemd user manager. The Linux unit properties in
// userServiceArgs map onto one new process group plus per-process rlimits that
// an explicit /bin/sh wrapper applies to itself before exec'ing the command,
// so the supervisor's own limits never change:
//
//	KillMode=control-group  -> kill(-pgid) on cancel/deadline, then userReapGroup
//	TasksMax=64             -> RLIMIT_NPROC = real-uid process count + 64,
//	                           capped at the inherited hard limit
//	CPUQuota=100%           -> RLIMIT_CPU = deadline seconds per process (one CPU
//	                           over the wall-clock deadline); no deadline inherits
//	UMask=0077              -> umask 077
//	LimitNOFILE 1024:524288 -> RLIMIT_NOFILE soft 1024, hard 524288 (systemd
//	                           defaults), capped at the inherited hard limit
//	(no Linux equivalent)   -> RLIMIT_FSIZE 4 GiB per written file
//
// userSupervisionLimitation states what this does not contain.
const userSupervisionLimitation = "darwin containment is one process group plus per-process rlimits, not a cgroup: " +
	"a descendant that calls setsid or setpgid leaves the owned group and is neither killed nor reaped; " +
	"RLIMIT_NPROC is headroom over every process of the real uid, not a group count; " +
	"the kernel refuses to lower RLIMIT_DATA and RLIMIT_AS, so there is no memory cap (Linux MemoryMax/MemorySwapMax); " +
	"there is no NoNewPrivileges or capability drop"

const (
	userTasksMax       = 64
	userFileBytes      = 4 << 30
	userDescriptors    = 1024
	userDescriptorsMax = 524288
	userLimitsShell    = "/bin/sh"
	userLimitsName     = "gentle-shell-limits"
)

type userLimits struct {
	CPUSeconds     uint64        // RLIMIT_CPU soft and hard; 0 inherits
	Processes      uint64        // RLIMIT_NPROC soft and hard, charged per real uid
	FileBytes      uint64        // RLIMIT_FSIZE soft and hard
	Descriptors    uint64        // RLIMIT_NOFILE soft
	DescriptorsMax uint64        // RLIMIT_NOFILE hard
	Deadline       time.Duration // wall clock for the whole group; 0 means none
}

// userUIDProcesses counts what RLIMIT_NPROC charges on darwin: every process
// whose real uid is the caller's, not only the owned group.
func userUIDProcesses() (int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.ruid", os.Getuid())
	if err != nil {
		return 0, err
	}
	return len(procs), nil
}

// userDarwinLimits snapshots the uid population for the RLIMIT_NPROC headroom.
// Other processes of the same uid consume that headroom too, so a busy session
// can see fork EAGAIN earlier than the Linux per-unit TasksMax would.
func userDarwinLimits(deadline time.Duration) (userLimits, error) {
	if deadline < 0 {
		return userLimits{}, privateError("refused", errors.New("negative supervision deadline"))
	}
	processes, err := userUIDProcesses()
	var nproc, nofile unix.Rlimit
	if err == nil {
		err = unix.Getrlimit(unix.RLIMIT_NPROC, &nproc)
	}
	if err == nil {
		err = unix.Getrlimit(unix.RLIMIT_NOFILE, &nofile)
	}
	if err != nil {
		return userLimits{}, privateError("unavailable", err)
	}
	limits := userLimits{
		Processes:      min(uint64(processes)+userTasksMax, nproc.Max),
		FileBytes:      userFileBytes,
		DescriptorsMax: min(userDescriptorsMax, nofile.Max),
		Deadline:       deadline,
	}
	limits.Descriptors = min(userDescriptors, limits.DescriptorsMax)
	if deadline > 0 {
		limits.CPUSeconds = uint64((deadline + time.Second - 1) / time.Second)
	}
	return limits, nil
}

// userLimitsScript renders only generated decimal literals; the command and its
// arguments reach the shell as "$@", never as script text. Any refused ulimit
// stops the chain before exec and exits 125 without running the command. Once
// every limit applied, the wrapper attests it on descriptor 3 and closes that
// descriptor for the command, so userLimitsWitness tells a refused limit from
// a command that itself exits 125. The close is its own exec: bash 3.2 runs
// `exec "$@" 3>&-` by saving descriptor 3 as an inheritable descriptor 10 that
// the command and its descendants would keep.
// /bin/sh is bash on stock macOS and counts -f in 1024-byte blocks. A host whose
// /private/var/select/sh is dash refuses -u (fail closed); zsh counts -f in
// 512-byte blocks, a stricter 2 GiB cap.
func userLimitsScript(limits userLimits) (string, error) {
	if limits.Processes == 0 || limits.FileBytes < 1024 || limits.FileBytes%1024 != 0 || limits.Descriptors == 0 || limits.Descriptors > limits.DescriptorsMax {
		return "", privateError("refused", errors.New("invalid owned process limits"))
	}
	decimal := func(value uint64) string { return strconv.FormatUint(value, 10) }
	var steps []string
	if limits.CPUSeconds > 0 {
		steps = append(steps, "command ulimit -t "+decimal(limits.CPUSeconds))
	}
	// Soft before hard: lowering the hard limit below an inherited soft limit is EINVAL.
	steps = append(steps, "command ulimit -u "+decimal(limits.Processes), "command ulimit -S -n "+decimal(limits.Descriptors),
		"command ulimit -H -n "+decimal(limits.DescriptorsMax), "command ulimit -f "+decimal(limits.FileBytes/1024), "command umask 077",
		"command printf "+userLimitsAttested+" >&3", "exec 3>&-", `exec "$@"`)
	return strings.Join(steps, " && ") + "; exit 125", nil
}

const userLimitsAttested = "limits-applied"

// userLimitsWitness is the attestation pipe of one limits wrapper. Attach it to
// the command before start; result is its post-Wait classification and close
// releases both ends on every path.
type userLimitsWitness struct{ read, write *os.File }

func userLimitsAttach(cmd *exec.Cmd) (*userLimitsWitness, error) {
	if len(cmd.ExtraFiles) != 0 {
		// refusal:by-design human-authority: only an installer caller passes extra descriptors; a maintainer must fix that caller
		return nil, privateError("refused", errors.New("limits wrapper owns descriptor 3"))
	}
	read, write, err := os.Pipe()
	if err != nil {
		return nil, privateError("start", err)
	}
	cmd.ExtraFiles = []*os.File{write}
	return &userLimitsWitness{read, write}, nil
}

// result maps a wrapper exit 125 without attestation to a precise refusal; any
// other status, attested or not, is returned unchanged. The wrapper wrote any
// attestation before it exited or exec'd, so a bounded read sees it without
// waiting for EOF from a descendant that might still hold the descriptor.
func (w *userLimitsWitness) result(waitErr error) error {
	w.write.Close()
	attested := make([]byte, len(userLimitsAttested))
	readErr := w.read.SetReadDeadline(time.Now().Add(time.Second))
	if readErr == nil {
		_, readErr = io.ReadFull(w.read, attested)
	}
	var exit *exec.ExitError
	if errors.As(waitErr, &exit) && exit.ExitCode() == 125 && (readErr != nil || string(attested) != userLimitsAttested) {
		// refusal:by-design world-action: the inherited hard rlimits of this login session must be raised outside gentle-ai
		return privateError("refused", errors.Join(waitErr, readErr, errors.New("the kernel refused an owned process limit before the command ran")))
	}
	return waitErr
}

func (w *userLimitsWitness) close() {
	w.read.Close()
	w.write.Close()
}

// userLimitsEnvironment refuses variables that make the wrapper shell run code
// before its limits apply: an exported function can shadow ulimit or command.
func userLimitsEnvironment(env []string) error {
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || strings.HasPrefix(name, "BASH_FUNC_") || strings.HasPrefix(name, "__BASH_FUNC") || name == "BASH_ENV" || name == "ENV" || name == "SHELLOPTS" || name == "BASHOPTS" {
			return privateError("refused", errors.New("owned process environment can alter the limits wrapper"))
		}
	}
	return nil
}

// userSupervise runs argv under limits as a new process group that owns the
// caller's foreground terminal when stdin is it. Cancellation or the deadline
// kills the group, and every started run reaps it through userReapGroup, so no
// group member survives. Descendants that left the group are not contained;
// see userSupervisionLimitation.
func userSupervise(ctx context.Context, limits userLimits, env []string, stdin io.Reader, stdout, stderr io.Writer, argv ...string) error {
	return userSuperviseIn(ctx, limits, "", env, stdin, stdout, stderr, argv...)
}

// userSuperviseIn is userSupervise in working directory dir; empty inherits
// the supervisor's.
func userSuperviseIn(ctx context.Context, limits userLimits, dir string, env []string, stdin io.Reader, stdout, stderr io.Writer, argv ...string) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	if len(argv) == 0 || !filepath.IsAbs(argv[0]) || filepath.Clean(argv[0]) != argv[0] || limits.Deadline < 0 {
		return privateError("refused", errors.New("owned command must be an absolute path with a non-negative deadline"))
	}
	if err := userLimitsEnvironment(env); err != nil {
		return err
	}
	script, err := userLimitsScript(limits)
	if err != nil {
		return err
	}
	if limits.Deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limits.Deadline)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, userLimitsShell, append([]string{"-c", script, userLimitsName}, argv...)...)
	// A nil Env would inherit the supervisor's environment; owned work gets exactly env.
	cmd.Env = append([]string{}, env...)
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = dir, stdin, stdout, stderr
	witness, err := userLimitsAttach(cmd)
	if err != nil {
		return err
	}
	defer witness.close()
	return userLaunchGroup(cmd, stdin, func(waitErr error) error {
		waitErr = witness.result(waitErr)
		switch {
		case errors.Is(waitErr, exec.ErrWaitDelay):
			// The group is empty, yet something outside it still holds the owned stdio.
			return privateError("uncertain", errors.Join(waitErr, errors.New(userSupervisionLimitation)))
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return privateError("deadline", errors.Join(context.DeadlineExceeded, waitErr))
		case ctx.Err() != nil:
			return privateError("canceled", errors.Join(ctx.Err(), waitErr))
		}
		return waitErr
	})
}
