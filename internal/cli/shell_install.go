package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

var (
	shellInstallInspect = shellinstaller.InspectUserInstall
	shellInstallEntry   = shellinstaller.RunUserEntry
)

// Platform contracts live in shell_install_platform_{windows,other}.go:
// shellInstallHelp, shellInstallPlatformFlags, shellEntryValues,
// shellInstallInspected and runShellInstallTUI.

func parseShellInstall(args []string, stdout io.Writer) (shellinstaller.UserInstallRequest, bool, error) {
	var req shellinstaller.UserInstallRequest
	flags := flag.NewFlagSet("shell install", flag.ContinueOnError)
	flags.SetOutput(stdout)
	flags.StringVar(&req.Destination, "target", "", "owned installation target")
	flags.StringVar(&req.Mode, "mode", "separate", "separate or shared")
	flags.StringVar(&req.SharedPrefix, "prefix", "", "selected existing global Pi prefix")
	flags.StringVar(&req.SharedAgent, "agent", "", "selected existing Pi configuration")
	flags.StringVar(&req.Confirmation, "confirm", "", "physical selection SHA256")
	inspect := flags.Bool("inspect", false, "inspect without installation")
	finish := shellInstallPlatformFlags(flags, &req)
	flags.Usage = func() { _, _ = io.WriteString(stdout, shellInstallHelp) }
	if err := flags.Parse(args); err != nil {
		return shellinstaller.UserInstallRequest{}, false, err
	}
	if flags.NArg() != 0 {
		return shellinstaller.UserInstallRequest{}, false, errors.New("unexpected shell install positional arguments; run gentle-ai shell install --help for supported flags")
	}
	if err := finish(); err != nil {
		return shellinstaller.UserInstallRequest{}, false, err
	}
	return req, *inspect, nil
}

// Dedicated early route, deliberately independent of the generic installer,
// profile detector, self-update, gate and ordinary Gentle AI startup TUI.
func RunShell(args []string, stdout io.Writer) (resultErr error) {
	defer func() { resultErr = preserveShellFailure(resultErr) }()
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := io.WriteString(stdout, shellInstallHelp)
		return err
	}
	switch args[0] {
	case "install", "launch-pnpm", "internal-launch-pnpm", "launch", "recover", "internal-check", "internal-install", "internal-install-direct", "internal-launch", "internal-recover", "internal-native-produce", "internal-native-coordinate", "internal-native-own":
		// The delegated supervisor or worker re-enters here; internal selectors
		// still repeat kernel qualification and never grant execution authority.
		// Each backend refuses the operations its platform does not provide.
	default:
		return fmt.Errorf("unknown shell command %q; run gentle-ai shell --help", args[0])
	}
	ctx, cancel, self, err := shellExecutionContext()
	if err != nil {
		return err
	}
	defer cancel()
	if args[0] != "install" {
		return shellinstaller.RunUserEntry(ctx, self, args, os.Stdin, stdout, os.Stderr)
	}
	if len(args) == 1 {
		return runShellInstallTUI(ctx, cancel, self, stdout)
	}
	req, inspect, err := parseShellInstall(args[1:], stdout)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	return runShellInstallRequest(ctx, self, req, inspect, stdout)
}

type shellEntryRunner func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error

func runShellInstallRequest(ctx context.Context, self string, req shellinstaller.UserInstallRequest, inspect bool, stdout io.Writer) error {
	return runShellInstallRequestWithEntry(ctx, self, req, inspect, stdout, shellInstallEntry)
}

func runShellInstallRequestWithEntry(ctx context.Context, self string, req shellinstaller.UserInstallRequest, inspect bool, stdout io.Writer, entry shellEntryRunner) error {
	return runShellInstallRequestWithStreams(ctx, self, req, inspect, os.Stdin, stdout, os.Stderr, entry)
}

func runShellInstallRequestWithStreams(ctx context.Context, self string, req shellinstaller.UserInstallRequest, inspect bool, stdin io.Reader, stdout, stderr io.Writer, entry shellEntryRunner) error {
	token, err := shellInstallInspect(req)
	if err != nil {
		return err
	}
	if inspect {
		return shellInstallInspected(stdout, req, token)
	}
	// guard:population windows-separate-confirmation fail-closed: legitimate explicit installations carry the current token from the owned physical selection; missing or mismatched confirmations remain excluded without starting a worker
	if req.Confirmation != token {
		return errors.New("inspect the physical selection first with --inspect, then pass its --confirm SHA256; run gentle-ai shell install --help for selection flags or gentle-ai shell install for interactive review")
	}
	args := ShellInstallArguments(ShellInstallSelection{Request: req, Confirmed: true})
	if len(args) == 0 {
		return errors.New("invalid or oversized experience selection; run gentle-ai shell install to review it again")
	}
	return entry(ctx, self, args, stdin, stdout, stderr)
}

// RunShellInstallInline executes a confirmed selection without replacing the
// process or restoring the terminal. Ordered progress is reported through the
// installer context; the caller keeps ownership of its UI.
func RunShellInstallInline(ctx context.Context, selection ShellInstallSelection, report func(shellinstaller.UserInstallProgress), _ io.Writer) error {
	if !selection.Confirmed {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	token, err := shellInstallInspect(selection.Request)
	if err != nil {
		return err
	}
	if selection.Request.Confirmation != token {
		return errors.New("inspect the physical selection first with --inspect, then pass its --confirm SHA256; run gentle-ai shell install --help for selection flags or gentle-ai shell install for interactive review")
	}
	args := ShellInstallArguments(selection)
	if len(args) < 2 {
		return errors.New("invalid or oversized experience selection; run gentle-ai shell install to review it again")
	}
	if runtime.GOOS != "linux" {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		workerLog := &boundedWorkerLog{limit: 8192}
		progress := &shellProgressWriter{log: workerLog, report: report}
		if report != nil {
			report(shellinstaller.UserInstallProgress{Index: 1, Total: 5, State: "done"})
			report(shellinstaller.UserInstallProgress{Index: 2, Total: 5, State: "running"})
		}
		runErr := shellinstaller.RunUserEntry(ctx, self, args, nil, progress, progress)
		if runErr != nil {
			if output := strings.TrimSpace(workerLog.String()); output != "" {
				return fmt.Errorf("%w; worker output: %s", runErr, output)
			}
		}
		return runErr
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	workerLog := &boundedWorkerLog{limit: 8192}
	entry := append([]string{"--no-new-privs", "--inh-caps=-all", "--ambient-caps=-all", "--", self, "shell", "internal-install-direct"}, args[1:]...)
	cmd := exec.CommandContext(ctx, "/usr/bin/setpriv", entry...)
	cmd.Env = append(os.Environ(), "GENTLE_AI_SHELL_STEPS=1")
	cmd.Stderr = workerLog
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	scan := make(chan struct{})
	go func() {
		defer close(scan)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			_, _ = workerLog.Write([]byte(line + "\n"))
			if index, total, label, ok := shellStepLine(line); ok && report != nil {
				report(shellinstaller.UserInstallProgress{Index: index, Total: total, Label: label, State: "done"})
			} else if index, total, label, ok := shellDetailLine(line); ok && report != nil {
				report(shellinstaller.UserInstallProgress{Index: index, Total: total, Label: label, State: "detail", Detail: label})
			}
		}
	}()
	waitErr := cmd.Wait()
	<-scan
	if waitErr != nil {
		if output := strings.TrimSpace(workerLog.String()); output != "" {
			waitErr = fmt.Errorf("%w; worker output: %s", waitErr, output)
		}
	}
	return preserveShellFailure(waitErr)
}

type boundedWorkerLog struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func (w *boundedWorkerLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	if len(w.data) > w.limit {
		w.data = append([]byte(nil), w.data[len(w.data)-w.limit:]...)
	}
	return len(p), nil
}

func (w *boundedWorkerLog) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.data)
}

type shellProgressWriter struct {
	log    *boundedWorkerLog
	report func(shellinstaller.UserInstallProgress)
	buffer []byte
	step   int
}

func (w *shellProgressWriter) Write(p []byte) (int, error) {
	_, _ = w.log.Write(p)
	if w.report == nil {
		return len(p), nil
	}
	w.buffer = append(w.buffer, p...)
	for {
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			break
		}
		line := string(w.buffer[:index])
		w.buffer = w.buffer[index+1:]
		w.observe(line)
	}
	return len(p), nil
}

func (w *shellProgressWriter) observe(line string) {
	if !strings.HasPrefix(line, "Windows install phase: ") {
		return
	}
	phase := strings.TrimSpace(strings.TrimPrefix(line, "Windows install phase: "))
	if w.step < 2 {
		w.step = 2
	}
	switch {
	case strings.HasPrefix(phase, "acquire"), strings.HasPrefix(phase, "unpack"):
		if w.step == 2 {
			w.report(shellinstaller.UserInstallProgress{Index: 2, Total: 5, State: "done"})
			w.step = 3
			w.report(shellinstaller.UserInstallProgress{Index: 3, Total: 5, State: "running"})
		}
		w.report(shellinstaller.UserInstallProgress{Index: 3, Total: 5, State: "detail", Detail: phase})
	case strings.HasPrefix(phase, "runtimes-ready"):
		if w.step == 3 {
			w.report(shellinstaller.UserInstallProgress{Index: 3, Total: 5, State: "done"})
			w.step = 4
			w.report(shellinstaller.UserInstallProgress{Index: 4, Total: 5, State: "running"})
		}
		w.report(shellinstaller.UserInstallProgress{Index: 4, Total: 5, State: "detail", Detail: phase})
	case strings.HasPrefix(phase, "provision-ready"):
		if w.step <= 4 {
			w.report(shellinstaller.UserInstallProgress{Index: 4, Total: 5, State: "done"})
			w.step = 5
			w.report(shellinstaller.UserInstallProgress{Index: 5, Total: 5, State: "running"})
		}
		w.report(shellinstaller.UserInstallProgress{Index: 5, Total: 5, State: "detail", Detail: phase})
	case strings.HasPrefix(phase, "provision"):
		if w.step == 3 {
			w.report(shellinstaller.UserInstallProgress{Index: 3, Total: 5, State: "done"})
			w.step = 4
			w.report(shellinstaller.UserInstallProgress{Index: 4, Total: 5, State: "running"})
		}
		w.report(shellinstaller.UserInstallProgress{Index: 4, Total: 5, State: "detail", Detail: phase})
	case strings.HasPrefix(phase, "publish"), strings.HasPrefix(phase, "verify"), strings.HasPrefix(phase, "overlay"):
		if w.step < 5 {
			w.report(shellinstaller.UserInstallProgress{Index: 4, Total: 5, State: "done"})
			w.step = 5
			w.report(shellinstaller.UserInstallProgress{Index: 5, Total: 5, State: "running"})
		}
		w.report(shellinstaller.UserInstallProgress{Index: 5, Total: 5, State: "detail", Detail: phase})
	case strings.HasPrefix(phase, "codegraph-ready"):
		if w.step == 2 {
			w.report(shellinstaller.UserInstallProgress{Index: 2, Total: 5, State: "done"})
			w.step = 3
			w.report(shellinstaller.UserInstallProgress{Index: 3, Total: 5, State: "running"})
		}
		w.report(shellinstaller.UserInstallProgress{Index: 3, Total: 5, State: "detail", Detail: phase})
	default:
		w.report(shellinstaller.UserInstallProgress{Index: w.step, Total: 5, State: "detail", Detail: phase})
	}
}

// shellDetailLine parses the worker's ordered internal task marker: "~ i/n label".
func shellDetailLine(line string) (int, int, string, bool) {
	rest, ok := strings.CutPrefix(line, "~ ")
	if !ok {
		return 0, 0, "", false
	}
	counts, label, ok := strings.Cut(rest, " ")
	if !ok {
		return 0, 0, "", false
	}
	left, right, ok := strings.Cut(counts, "/")
	if !ok {
		return 0, 0, "", false
	}
	index, leftErr := strconv.Atoi(left)
	total, rightErr := strconv.Atoi(right)
	if leftErr != nil || rightErr != nil || index < 1 || total < 1 || index > total {
		return 0, 0, "", false
	}
	return index, total, label, true
}

// shellStepLine parses the worker's fixed ordered marker: "✓ i/n label".
func shellStepLine(line string) (int, int, string, bool) {
	rest, ok := strings.CutPrefix(line, "✓ ")
	if !ok {
		return 0, 0, "", false
	}
	counts, label, ok := strings.Cut(rest, " ")
	if !ok {
		return 0, 0, "", false
	}
	left, right, ok := strings.Cut(counts, "/")
	if !ok {
		return 0, 0, "", false
	}
	index, leftErr := strconv.Atoi(left)
	total, rightErr := strconv.Atoi(right)
	if leftErr != nil || rightErr != nil || index < 1 || total < 1 || index > total {
		return 0, 0, "", false
	}
	return index, total, label, true
}

func shellExecutionContext() (context.Context, context.CancelFunc, string, error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	self, err := os.Executable()
	if err != nil {
		cancel()
	}
	return ctx, cancel, self, err
}

func preserveShellFailure(err error) error {
	var failure *shellinstaller.PrivateRuntimeError
	if errors.As(err, &failure) && (failure.Workspace != "" || failure.Destination != "") {
		return fmt.Errorf("%w\nPreserve evidence: workspace=%q destination/unit=%q\nFor shared installation recovery, inspect ROOT=workspace/installed or published destination with gentle-ai shell recover ROOT inspect", err, failure.Workspace, failure.Destination)
	}
	return err
}
