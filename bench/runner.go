package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Sandbox is one journey's isolated world: its own HOME, XDG_*, throwaway git
// repository, and (when the journey needs one) a local bare remote. Nothing
// here ever touches the user's real config or repositories.
type Sandbox struct {
	Binary string
	// PathOverride is prepended to PATH for journeys that need a deterministic
	// local runtime probe without depending on the host installation.
	PathOverride             string
	Root                     string
	Home                     string
	Repo                     string
	Remote                   string
	TracePath                string
	BenchReceiptMutationPath string

	// BenchCrashAtPhase, when non-empty, is read by product binaries built
	// with `-tags bench_fixture` as GENTLE_AI_BENCH_CRASH_AT_PHASE
	// (format "<phase>:<lineage_id>"): the deterministic phase-hook
	// interruption internal/reviewtransaction's own crash-position matrix
	// uses in-process (compactReclaimPhaseHook), reachable here through the
	// real binary instead. It is read fresh from this field on every
	// invoke, so a caller sets it before the crash-inducing command and
	// clears it (empty string) before the resume command; an ordinary
	// product binary without the bench_fixture tag never reads this
	// variable at all.
	BenchCrashAtPhase string

	// NewLineageActivation opts this sandbox's whole isolated process
	// environment into GENTLE_AI_RDD_NEW_LINEAGE (Wave 3 Slice 5, task 6.7).
	// It is off by default, matching the product's own default-off
	// activation switch (design decision 5): every wave1/wave2/edge/sdd
	// journey that never sets this stays on the legacy `review start` path,
	// byte-identical to before this field existed.
	NewLineageActivation bool

	// Journey state carried between steps.
	Lineage  string
	Target   string
	Revision string
	Scratch  map[string]string
	// UnavailableProcessTemp is set by a journey after fixture setup to prove
	// product commands do not depend on the parent process temp directory.
	UnavailableProcessTemp string

	traceOffset int64
}

func newSandbox(binary, root string) (*Sandbox, error) {
	home := filepath.Join(root, "home")
	for _, dir := range []string{home, filepath.Join(root, "tmp"), filepath.Join(home, ".config"), filepath.Join(home, ".cache"), filepath.Join(home, ".local", "share"), filepath.Join(home, ".local", "state")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	sandbox := &Sandbox{
		Binary:    binary,
		Root:      root,
		Home:      home,
		Repo:      filepath.Join(home, "demo"),
		TracePath: filepath.Join(root, "git-trace.log"),
		Scratch:   map[string]string{},
	}
	return sandbox, nil
}

// env is a closed environment: only what the product legitimately needs.
// PATH is inherited because the product shells out to git.
func (s *Sandbox) env() []string {
	path := os.Getenv("PATH")
	if s.PathOverride != "" {
		path = s.PathOverride + string(os.PathListSeparator) + path
	}
	env := []string{
		"PATH=" + path,
		"HOME=" + s.Home,
		"USERPROFILE=" + s.Home,
		"XDG_CONFIG_HOME=" + filepath.Join(s.Home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(s.Home, ".cache"),
		"XDG_DATA_HOME=" + filepath.Join(s.Home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(s.Home, ".local", "state"),
		"TMP=" + filepath.Join(s.Root, "tmp"),
		"TEMP=" + filepath.Join(s.Root, "tmp"),
		"TMPDIR=" + filepath.Join(s.Root, "tmp"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_TRACE=" + s.TracePath,
		"NO_COLOR=1",
		"TERM=dumb",
		"LANG=C",
	}
	if s.BenchReceiptMutationPath != "" {
		env = append(env, "GENTLE_AI_BENCH_MUTATE_RECEIPT="+s.BenchReceiptMutationPath)
	}
	if s.BenchCrashAtPhase != "" {
		env = append(env, "GENTLE_AI_BENCH_CRASH_AT_PHASE="+s.BenchCrashAtPhase)
	}
	if s.NewLineageActivation {
		env = append(env, "GENTLE_AI_RDD_NEW_LINEAGE=1")
	}
	// Set last so a journey that poisons the process temp directory overrides
	// the sandbox's own writable TMP/TEMP/TMPDIR defaults above.
	if s.UnavailableProcessTemp != "" {
		env = append(env,
			"TEMP="+s.UnavailableProcessTemp,
			"TMP="+s.UnavailableProcessTemp,
			"TMPDIR="+s.UnavailableProcessTemp,
		)
	}
	return env
}

func unavailableProcessTemp(sandbox *Sandbox) error {
	path := filepath.Join(sandbox.Root, "unavailable-process-temp")
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("unavailable process temp path already exists: %s", path)
	}
	sandbox.UnavailableProcessTemp = path
	return nil
}

// git runs a fixture git command. Fixture commands are sandbox setup, not user
// friction, so they are never counted and never traced.
func (s *Sandbox) git(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	env := s.env()
	for index, entry := range env {
		if strings.HasPrefix(entry, "GIT_TRACE=") {
			env[index] = "GIT_TRACE="
		}
	}
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return nil
}

func (s *Sandbox) initRepo(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if err := s.git(path, "init", "-b", "main", "-q"); err != nil {
		return err
	}
	if err := s.git(path, "config", "user.email", "bench@example.invalid"); err != nil {
		return err
	}
	if err := s.git(path, "config", "user.name", "Bench"); err != nil {
		return err
	}
	if err := s.git(path, "config", "commit.gpgsign", "false"); err != nil {
		return err
	}
	// The installed agent config must never leak into a reviewed diff.
	return s.write(filepath.Join(path, ".gitignore"), ".claude/\n.gentle-ai/\n")
}

func (s *Sandbox) write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// gitCallsSince returns how many git subprocesses the last invocation spawned,
// read from the GIT_TRACE log. It returns nil when the trace is unavailable.
//
// A trace file that does not exist yet is zero, not unknown: every invocation
// runs with GIT_TRACE pointed at this path, so git would have created it had it
// run at all. Reporting that as unobservable would erase the whole corpus total
// the moment one journey legitimately spawns no git — a contract rejected at
// preflight, for instance.
func (s *Sandbox) gitCallsSince() *int {
	info, err := os.Stat(s.TracePath)
	if errors.Is(err, os.ErrNotExist) {
		zero := 0
		return &zero
	}
	if err != nil {
		return nil
	}
	file, err := os.Open(s.TracePath)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Seek(s.traceOffset, 0); err != nil {
		return nil
	}
	buffer := make([]byte, info.Size()-s.traceOffset)
	read, _ := file.Read(buffer)
	s.traceOffset = info.Size()
	count := bytes.Count(buffer[:read], []byte("trace: built-in: git "))
	count += bytes.Count(buffer[:read], []byte("trace: exec: git "))
	return &count
}

// invoke runs the product once and returns a full Observation.
func (s *Sandbox) invoke(args []string) Observation {
	return s.invokeAt(s.Repo, args)
}

func (s *Sandbox) invokeAt(dir string, args []string) Observation {
	cmd := exec.Command(s.Binary, args...)
	cmd.Dir = dir
	cmd.Env = s.env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader("")
	err := cmd.Run()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
		stderr.WriteString("\nbench: " + err.Error())
	}
	return Observation{
		Args:           args,
		ExitCode:       exitCode,
		Stdout:         stdout.String(),
		Stderr:         stderr.String(),
		StdoutCaptured: true,
		StderrCaptured: true,
	}
}

// readBack runs the product for a fixture proof, a capability probe or an
// assertion. It is benchmark instrumentation, not operator work, so it is never
// counted — and it runs with GIT_TRACE blanked, exactly like Sandbox.git, so the
// git subprocesses it spawns are never charged to the next counted invocation.
//
// It exists because an SDD fixture cannot prove its own state from git alone:
// the attempt ordinal, the review binding and the leaf/non-leaf topology live
// inside the product, and a fixture that assumed them instead of reading them
// back is the failure this corpus refuses to ship.
func (s *Sandbox) readBack(args ...string) Observation {
	cmd := exec.Command(s.Binary, args...)
	cmd.Dir = s.Repo
	env := s.env()
	for index, entry := range env {
		if strings.HasPrefix(entry, "GIT_TRACE=") {
			env[index] = "GIT_TRACE="
		}
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader("")
	err := cmd.Run()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
		stderr.WriteString("\nbench: " + err.Error())
	}
	return Observation{
		Args:           args,
		ExitCode:       exitCode,
		Stdout:         stdout.String(),
		Stderr:         stderr.String(),
		StdoutCaptured: true,
		StderrCaptured: true,
	}
}

// Capability is the CLI surface a step needs. It is probed before the step
// runs so a build without that surface records `unsupported` and never a pass.
//
// The default probe is `<verb> --help`, read for the flag names. Probe exists
// for legacy surfaces whose invocation cannot render user-facing help.
type Capability struct {
	Verb  []string
	Flags []string
	// Probe is a complete argv run INSTEAD of `<verb> --help`. The surface is
	// supported when the binary does not reject the SHAPE of that argv. Pick an
	// argv that names the flags under test and fails on state rather than on
	// shape, so a build that has them answers with a state error and a build
	// that does not answers `flag provided but not defined`.
	Probe []string
}

type capabilityProbe struct {
	sandbox *Sandbox
	cache   map[string]string
}

func newCapabilityProbe(sandbox *Sandbox) *capabilityProbe {
	return &capabilityProbe{sandbox: sandbox, cache: map[string]string{}}
}

// supported reports whether the binary has the verb and every flag. Probe
// invocations are benchmark instrumentation, not user friction, so they are
// never counted in commands_to_completion.
func (p *capabilityProbe) supported(capability *Capability) (bool, string) {
	if capability == nil {
		return true, ""
	}
	if len(capability.Probe) != 0 {
		return p.probed(capability.Probe)
	}
	key := strings.Join(capability.Verb, " ")
	help, cached := p.cache[key]
	if !cached {
		observation := p.sandbox.invoke(append(append([]string{}, capability.Verb...), "--help"))
		help = observation.Stdout + "\n" + observation.Stderr
		if IsUnsupported(observation) {
			help = "\x00unsupported"
		}
		p.cache[key] = help
	}
	if help == "\x00unsupported" {
		return false, "verb not present: " + key
	}
	for _, flag := range capability.Flags {
		if !strings.Contains(help, flag) {
			return false, "flag not present: " + key + " " + flag
		}
	}
	return true, ""
}

// probed answers a Capability that carries its own argv. The single question is
// whether the binary rejected the SHAPE of the command; anything else — a
// missing --cwd, a repository in the wrong state — means the surface is there.
func (p *capabilityProbe) probed(argv []string) (bool, string) {
	key := "\x01" + strings.Join(argv, " ")
	answer, cached := p.cache[key]
	if !cached {
		answer = ""
		if IsUnsupported(p.sandbox.readBack(argv...)) {
			answer = "\x00unsupported"
		}
		p.cache[key] = answer
	}
	if answer == "\x00unsupported" {
		return false, "surface not present: " + strings.Join(argv, " ")
	}
	return true, ""
}

// Step is one unit of a journey. Journeys are data: adding one is adding a
// Step to a slice.
type Step struct {
	Name    string
	Fixture func(*Sandbox) error
	// Skip reports why an externally-backed step cannot run in this environment.
	// The runner records the journey as unsupported rather than a false pass.
	Skip     func(*Sandbox) string
	Requires *Capability
	Args     func(*Sandbox) ([]string, error)
	// Composite drives a multi-command sub-flow (a lens loop, a rejected
	// capture and its recapture). It reports its own invocations.
	Composite func(*journeyRun) error
	After     func(*Sandbox, Observation) error
	// DeadEnd declares that no continuation exists for a block here: the flow
	// is over. It is the first of two author-declared classifier inputs; see
	// README.
	DeadEnd bool
	// ByDesign declares the opposite: a block here is a CORRECT refusal that
	// already told the operator what to do, in words no `gentle-ai` command
	// could express. It is the second author-declared input, and the more
	// expensive one — a shape from a closed vocabulary plus a quote of the
	// product's own next-action text, verified against the emitted bytes.
	// validateCorpus rejects it alongside DeadEnd, and rejects it on a step
	// that issues no invocation of its own.
	ByDesign *ByDesignDeclaration
	// ModelRun marks a step that stands in for a reviewer/lens invocation
	// even though the argv is not a capture-result.
	ModelRun bool
	// AbortOnBlock ends the journey cleanly at a terminal block instead of
	// treating later steps as reachable.
	AbortOnBlock bool
}

// Journey is one end-to-end flow through the review lifecycle.
type Journey struct {
	ID     string
	Title  string
	Source string
	Steps  []Step
	// NewLineageActivation propagates to the journey's own Sandbox (task
	// 6.7): a journey exercising the new-lineage lifecycle sets this true;
	// every other journey leaves it false and is unaffected.
	NewLineageActivation bool
}

// validateCorpus checks every author-declared classifier input in the corpus
// and fails the WHOLE run before a single journey is driven. A declaration is
// the only thing in this benchmark that can make a number look better, so a
// broken one must never be able to do so quietly: an unrecognised shape, a
// declaration with nothing to verify, a step claiming both "no next action"
// and "here is the next action", or a declaration attached where it can never
// reach the classifier are all corpus errors, not degraded passes.
//
// Note the direction of the safety net. Classify already refuses to honour an
// invalid declaration, so a corpus error can only ever make a block look worse
// than it is. This turns that silent worsening into a loud stop.
func validateCorpus(journeys []Journey) error {
	problems := []string{}
	for _, journey := range journeys {
		for _, step := range journey.Steps {
			for _, problem := range stepDeclarationProblems(step) {
				problems = append(problems, journey.ID+" / "+step.Name+": "+problem)
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New("corpus declaration errors:\n  " + strings.Join(problems, "\n  "))
}

func stepDeclarationProblems(step Step) []string {
	problems := []string{}
	if err := step.ByDesign.Validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if step.ByDesign != nil && step.DeadEnd {
		problems = append(problems,
			"declares both by_design and dead_end, which are opposite claims: dead_end says there is no next action, by_design says the product already named one")
	}
	// Only a step with its own Args reaches the classifier carrying the
	// declaration; a composite step reports its invocations through
	// journeyRun.run, which never sees Step. A declaration there would be a
	// silent no-op, which is exactly the failure mode this class must not have.
	if step.Args == nil {
		if step.ByDesign != nil {
			problems = append(problems, "declares by_design on a step that issues no invocation of its own, where the declaration never reaches the classifier")
		}
		if step.DeadEnd {
			problems = append(problems, "declares dead_end on a step that issues no invocation of its own, where the declaration never reaches the classifier")
		}
	}
	return problems
}

type journeyRun struct {
	sandbox     *Sandbox
	probe       *capabilityProbe
	accumulator *accumulator
	step        string
}

// run executes one product invocation inside a journey, folding it into the
// metrics. Composite steps call it directly.
func (r *journeyRun) run(args []string, modelRun bool) Observation {
	return r.runAt(r.sandbox.Repo, args, modelRun)
}

func (r *journeyRun) runAt(dir string, args []string, modelRun bool) Observation {
	observation := r.sandbox.invokeAt(dir, args)
	record := r.accumulator.observe(r.step, observation, r.sandbox.gitCallsSince(), modelRun)
	r.accumulator.records = append(r.accumulator.records, record)
	return observation
}

func runJourney(binary string, journey Journey) JourneyResult {
	result := JourneyResult{ID: journey.ID, Title: journey.Title, Source: journey.Source, Status: StatusCompleted}

	root, err := os.MkdirTemp("", "gentle-ai-bench-"+journey.ID+"-")
	if err != nil {
		result.Status = StatusFailed
		result.FailureReason = err.Error()
		return result
	}
	defer func() { _ = os.RemoveAll(root) }()

	// The temp root can sit behind a symlink (macOS puts /var/folders behind
	// /private/var/folders). Git canonicalizes, so every fixture that compares
	// its own path against git's answer would disagree with itself. Canonicalize
	// once, here, so the whole journey speaks one spelling of every path.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	sandbox, err := newSandbox(binary, root)
	if err != nil {
		result.Status = StatusFailed
		result.FailureReason = err.Error()
		return result
	}
	sandbox.NewLineageActivation = journey.NewLineageActivation
	accumulator := newAccumulator()
	probe := newCapabilityProbe(sandbox)
	run := &journeyRun{sandbox: sandbox, probe: probe, accumulator: accumulator}

	for _, step := range journey.Steps {
		run.step = step.Name
		if step.Skip != nil {
			if reason := step.Skip(run.sandbox); reason != "" {
				result.Status = StatusUnsupported
				result.UnsupportedSteps = append(result.UnsupportedSteps, step.Name+" ("+reason+")")
				break
			}
		}

		if step.Fixture != nil {
			if err := step.Fixture(sandbox); err != nil {
				result.Status = StatusFailed
				result.FailureReason = fmt.Sprintf("fixture %q: %v", step.Name, err)
				break
			}
		}

		if supported, reason := probe.supported(step.Requires); !supported {
			result.Status = StatusUnsupported
			result.UnsupportedSteps = append(result.UnsupportedSteps, step.Name+" ("+reason+")")
			// Continue the journey where later steps do not depend on this
			// one; abort cleanly where they do.
			if step.Args != nil || step.Composite != nil {
				break
			}
			continue
		}

		if step.Composite != nil {
			if err := step.Composite(run); err != nil {
				result.Status = StatusFailed
				result.FailureReason = fmt.Sprintf("step %q: %v", step.Name, err)
				break
			}
			continue
		}

		if step.Args == nil {
			continue
		}

		args, err := step.Args(sandbox)
		if err != nil {
			result.Status = StatusFailed
			result.FailureReason = fmt.Sprintf("step %q: %v", step.Name, err)
			break
		}

		observation := sandbox.invoke(args)
		observation.DeclaredDeadEnd = step.DeadEnd
		observation.DeclaredByDesign = step.ByDesign
		record := accumulator.observe(step.Name, observation, sandbox.gitCallsSince(), step.ModelRun)
		accumulator.records = append(accumulator.records, record)

		if record.Unsupported {
			result.Status = StatusUnsupported
			result.UnsupportedSteps = append(result.UnsupportedSteps, step.Name+" (rejected at runtime)")
			break
		}

		if step.After != nil {
			if err := step.After(sandbox, observation); err != nil {
				if errors.Is(err, errSourceCoupledFixtureUnavailable) {
					result.Status = StatusUnsupported
					result.UnsupportedSteps = append(result.UnsupportedSteps, step.Name+" (source-coupled fixture unavailable)")
					break
				}
				result.Status = StatusFailed
				result.FailureReason = fmt.Sprintf("step %q after: %v", step.Name, err)
				break
			}
		}

		if step.AbortOnBlock && record.Block != NotABlock && record.Block != BlockSelfRecovered {
			break
		}
	}

	// A product that emitted an execute transition with nothing to run fails
	// the journey outright, whatever else the journey managed to do. It is not
	// a friction number: no honest metric can be reported about a flow whose
	// stated continuation the reader cannot follow.
	if len(accumulator.deadTransitions) > 0 && result.Status != StatusFailed {
		result.Status = StatusFailed
		result.FailureReason = strings.Join(accumulator.deadTransitions, "; ")
	}

	result.Metrics = accumulator.metrics("")
	result.Commands = accumulator.records
	return result
}

// envelopeString reads one dotted path out of a JSON envelope.
func envelopeString(stdout string, path ...string) (string, bool) {
	var node any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &node); err != nil {
		return "", false
	}
	for _, key := range path {
		object, ok := node.(map[string]any)
		if !ok {
			return "", false
		}
		node, ok = object[key]
		if !ok {
			return "", false
		}
	}
	text, ok := node.(string)
	return text, ok
}
