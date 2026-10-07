package opencode

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ActivationStatus is the effective activation outcome shared by install,
// sync, reporting, and doctor. Capability readiness is not activation
// readiness: a supported runtime can still be bypassed by the PATH a new shell
// builds.
type ActivationStatus string

const (
	// ActivationStatusReady means a new login shell resolves bare `opencode`
	// to the Gentle-owned launcher.
	ActivationStatusReady ActivationStatus = "ready"
	// ActivationStatusPending means activation is not applied yet or PATH
	// persistence for new shells is missing.
	ActivationStatusPending ActivationStatus = "pending"
	// ActivationStatusShadowed means the managed directory is on PATH but
	// another OpenCode executable precedes the launcher.
	ActivationStatusShadowed    ActivationStatus = "shadowed"
	ActivationStatusUnsupported ActivationStatus = "unsupported"
	ActivationStatusUnknown     ActivationStatus = "unknown"
	ActivationStatusOff         ActivationStatus = "off"
)

// ErrManagedLauncherShadowed reports that another OpenCode executable
// precedes the managed launcher on PATH.
var ErrManagedLauncherShadowed = errors.New("managed OpenCode launcher is shadowed")

// ResolvedStatus returns the effective activation status. Reports produced by
// ActivationPlan carry an explicit status; the fallback keeps hand-built
// reports truthful by never deriving ready from capability alone.
func (r ActivationReport) ResolvedStatus() ActivationStatus {
	if r.Status != "" {
		return r.Status
	}
	if r.Action == string(activationActionOff) {
		return ActivationStatusOff
	}
	switch r.Capability.Status {
	case CapabilityUnsupported:
		return ActivationStatusUnsupported
	case CapabilityReady:
		if r.Effective {
			return ActivationStatusReady
		}
		return ActivationStatusPending
	default:
		return ActivationStatusUnknown
	}
}

// evaluate records the effective status for the plan's current stage: a
// prepared plan predicts the post-apply outcome, and an applied plan reports
// what a new shell will actually run.
func (p *ActivationPlan) evaluate() {
	p.status, p.activationReason = p.effectiveStatus()
}

func (p *ActivationPlan) effectiveStatus() (ActivationStatus, string) {
	switch {
	case p.action == activationActionOff:
		return ActivationStatusOff, "OpenCode background activation is off; only Gentle-owned launchers and profile blocks are removed"
	case p.capability.Status == CapabilityUnsupported:
		return ActivationStatusUnsupported, p.capability.Reason
	case !p.capability.Ready():
		return ActivationStatusUnknown, p.capability.Reason
	case p.goos == "windows":
		return p.windowsStatus()
	}
	var overlay map[string][]byte
	if !p.applied {
		overlay = make(map[string][]byte, len(p.profiles))
		for _, change := range p.profiles {
			if change.changed {
				overlay[change.path] = change.desired
			}
		}
	}
	resolution := resolveLoginShell(p.homeDir, p.options, overlay, !p.applied)
	switch {
	case resolution.Status == ActivationStatusReady && !p.applied:
		return ActivationStatusPending, "managed OpenCode launcher activation is prepared but not applied; once applied, " + resolution.Reason
	case resolution.Status == ActivationStatusUnknown && p.profileReason != "":
		// The launcher works once PATH reaches it; Gentle AI cannot tell.
		return ActivationStatusUnknown, p.profileReason
	case resolution.Status == ActivationStatusReady, resolution.Status == ActivationStatusShadowed, resolution.Status == ActivationStatusUnknown:
		return resolution.Status, resolution.Reason
	case p.profileReason != "":
		return ActivationStatusPending, p.profileReason
	default:
		return ActivationStatusPending, resolution.Reason
	}
}

// windowsStatus verifies the PATH a new terminal inherits (machine then user
// entries) rather than this process's PATH, which activation itself changed.
func (p *ActivationPlan) windowsStatus() (ActivationStatus, string) {
	binDir := BinDir(p.homeDir)
	if !p.applied {
		return ActivationStatusPending, fmt.Sprintf("managed OpenCode launcher activation is prepared but not applied; it adds %s to the user PATH", binDir)
	}
	pathValue, err := p.options.NewShellPath()
	if err != nil {
		return ActivationStatusUnknown, fmt.Sprintf("managed OpenCode launcher was written, but the PATH new terminals inherit could not be read: %v", err)
	}
	launcher, err := ResolveManagedLauncher(p.homeDir, pathValue, "windows")
	switch {
	case err == nil:
		return ActivationStatusReady, fmt.Sprintf("new terminals resolve opencode to the managed launcher %s; open a new terminal and restart OpenCode", launcher)
	case errors.Is(err, ErrManagedLauncherShadowed):
		return ActivationStatusShadowed, fmt.Sprintf("%v on the PATH new terminals inherit; move %s ahead of that directory in your PATH, then open a new terminal", err, binDir)
	default:
		return ActivationStatusPending, err.Error()
	}
}

// LoginShellResolution describes how a new login shell resolves bare
// `opencode` given the persisted startup files and the inherited PATH.
type LoginShellResolution struct {
	Status ActivationStatus
	// Resolved is the executable a new shell runs for `opencode`, if any.
	Resolved string
	// Source is the startup file whose PATH edit placed Resolved's directory,
	// or "the inherited PATH".
	Source string
	Reason string
}

// ResolveLoginShellActivation models a new login shell for options.Shell: it
// replays the PATH edits of the user's startup files over options.Path and
// reports whether bare `opencode` resolves to the managed launcher. Startup
// files are only read, never modified.
func ResolveLoginShellActivation(homeDir string, options ActivationOptions) LoginShellResolution {
	return resolveLoginShell(homeDir, options.normalized(), nil, false)
}

const inheritedPathSource = "the inherited PATH"

type startupPathEntry struct {
	dir    string
	source string
}

// resolveLoginShell evaluates the modeled PATH. overlay supplies prepared but
// unapplied profile bytes, and assumeLauncher treats the managed launcher as
// present so a dry run predicts the post-apply outcome.
func resolveLoginShell(homeDir string, options ActivationOptions, overlay map[string][]byte, assumeLauncher bool) LoginShellResolution {
	binDir := BinDir(homeDir)
	launcher := POSIXLauncherPath(homeDir)
	files, unmodeled := loginShellStartupFiles(homeDir, options)
	if unmodeled != "" {
		// The current PATH proves this shell, not new ones.
		return LoginShellResolution{Status: ActivationStatusUnknown, Reason: fmt.Sprintf("new login shells were not verified because %s; in a new login shell, run `command -v opencode` and confirm it prints %s, or add %s to that shell's startup file", unmodeled, launcher, ProfileExportLine(binDir))}
	}

	model := newStartupPathModel(homeDir, options.OS, overlay)
	for _, entry := range splitPath(options.Path, options.OS) {
		if filepath.IsAbs(entry) {
			model.entries = append(model.entries, startupPathEntry{dir: entry, source: inheritedPathSource})
		}
	}
	for _, file := range files {
		model.replay(file, file, 0)
	}

	resolution := model.resolve(launcher, options.OS, assumeLauncher)
	if resolution.Status != ActivationStatusReady && resolution.Status != ActivationStatusShadowed {
		return resolution
	}
	if u := model.relevantUncertainty(); u != nil {
		return LoginShellResolution{Status: ActivationStatusUnknown, Source: u.source, Reason: fmt.Sprintf("cannot verify PATH order: %s contains %s, which may change PATH after %s is added; in a new login shell, run `command -v opencode` and confirm it prints %s", u.source, u.construct, binDir, launcher)}
	}
	return resolution
}

// resolve finds the executable a new shell runs for `opencode` on the modeled
// PATH.
func (m *startupPathModel) resolve(launcher, goos string, assumeLauncher bool) LoginShellResolution {
	binDir := m.binDir
	for _, entry := range m.entries {
		candidate := filepath.Join(entry.dir, "opencode")
		managedDir := samePath(entry.dir, binDir, goos)
		if !managedDir && !pathEntryExecutable(candidate, goos) {
			continue
		}
		// A link into the managed directory runs the launcher as well.
		if managedDir || resolvesUnder(candidate, binDir, goos) {
			if assumeLauncher && managedDir {
				return readyLoginShell(launcher, entry.source)
			}
			if _, err := validateManagedLauncherCandidate(m.homeDir, launcher, goos); err != nil {
				return LoginShellResolution{Status: ActivationStatusPending, Source: entry.source, Reason: fmt.Sprintf("new login shells put %s on PATH, but %v; run gentle-ai sync", binDir, err)}
			}
			return readyLoginShell(launcher, entry.source)
		}
		result := LoginShellResolution{Resolved: candidate, Source: entry.source}
		if !m.contains(binDir, goos) {
			result.Status = ActivationStatusPending
			result.Reason = fmt.Sprintf("new login shells do not put %s on PATH, so opencode resolves to %s", binDir, candidate)
			return result
		}
		result.Status = ActivationStatusShadowed
		if entry.source == inheritedPathSource {
			result.Reason = fmt.Sprintf("new login shells resolve opencode to %s before the managed launcher %s because %s from the inherited PATH precedes %s; prepend %s with %s, then start a new login shell", candidate, launcher, entry.dir, binDir, binDir, ProfileExportLine(binDir))
			return result
		}
		result.Reason = fmt.Sprintf("new login shells resolve opencode to %s before the managed launcher %s because %s adds %s to PATH ahead of %s; in %s, remove that PATH line or add %s after it, then start a new login shell", candidate, launcher, entry.source, entry.dir, binDir, entry.source, ProfileExportLine(binDir))
		return result
	}
	return LoginShellResolution{Status: ActivationStatusPending, Reason: fmt.Sprintf("new login shells do not put %s on PATH", binDir)}
}

func readyLoginShell(launcher, source string) LoginShellResolution {
	return LoginShellResolution{
		Status:   ActivationStatusReady,
		Resolved: launcher,
		Source:   source,
		Reason:   fmt.Sprintf("new login shells resolve opencode to the managed launcher %s (PATH entry from %s); start a new login shell if this one predates activation", launcher, source),
	}
}

// loginShellStartupFiles lists, in execution order, the user startup files a
// new interactive login shell reads. A non-empty reason means the shell's
// startup cannot be modeled.
func loginShellStartupFiles(homeDir string, options ActivationOptions) ([]string, string) {
	switch shell := filepath.Base(options.Shell); shell {
	case "zsh":
		dir := homeDir
		files := []string{filepath.Join(homeDir, ".zshenv")}
		if options.ZDotDir != "" && !samePath(options.ZDotDir, homeDir, options.OS) {
			dir = options.ZDotDir
			files = append(files, filepath.Join(dir, ".zshenv"))
		}
		return append(files, filepath.Join(dir, ".zprofile"), filepath.Join(dir, ".zshrc"), filepath.Join(dir, ".zlogin")), ""
	case "bash":
		// A login shell reads only the first existing profile; with none, the
		// profile activation creates is .profile.
		profile, reason := loginProfile(homeDir, options)
		if reason != "" {
			return nil, reason
		}
		files := []string{profile}
		// macOS terminals start login shells, which read .bashrc only when the
		// profile sources it. Other terminals start interactive non-login
		// shells that read .bashrc on top of the session PATH.
		if options.OS != "darwin" {
			files = append(files, filepath.Join(homeDir, ".bashrc"))
		}
		return files, ""
	case "sh", "dash", "ksh":
		return []string{filepath.Join(homeDir, ".profile")}, ""
	default:
		_, reason := loginProfile(homeDir, options)
		if reason == "" {
			reason = fmt.Sprintf("login shell %q is not supported", shell)
		}
		return nil, reason
	}
}

// startupPathModel replays the simple PATH edits of POSIX startup files. It
// understands assignments, `export`, zsh `path=(...)` arrays, and sourcing.
// Constructs that may change PATH but cannot be replayed statically (eval,
// command substitution, function, case, and loop bodies, unresolvable
// sources) are recorded as uncertainties instead of being guessed.
type startupPathModel struct {
	homeDir string
	binDir  string
	goos    string
	overlay map[string][]byte
	entries []startupPathEntry
	vars    map[string]string
	active  []string
	blocks  []shellBlock
	// pathFunctions names functions whose bodies change PATH.
	pathFunctions map[string]bool
	visited       int
	seq           int
	// placedSeq is the last command that put the managed directory on PATH
	// from a startup file; -1 when none did.
	placedSeq     int
	uncertainties []startupUncertainty
}

type startupUncertainty struct {
	seq int
	// managedPresent records whether the managed directory was already on
	// the modeled PATH when the construct ran.
	managedPresent bool
	source         string
	construct      string
}

type shellBlock struct {
	kind string // if, else, case, loop, function, brace
	name string
	// awaitingBrace marks a function header whose body opens on a later word.
	awaitingBrace bool
}

const (
	maxStartupSourceDepth = 8
	maxStartupFiles       = 64
	maxStartupFileSize    = 1 << 20
)

func newStartupPathModel(homeDir, goos string, overlay map[string][]byte) *startupPathModel {
	return &startupPathModel{homeDir: homeDir, binDir: BinDir(homeDir), goos: goos, overlay: overlay, placedSeq: -1, pathFunctions: map[string]bool{}}
}

func (m *startupPathModel) contains(dir, goos string) bool {
	for _, entry := range m.entries {
		if samePath(entry.dir, dir, goos) {
			return true
		}
	}
	return false
}

// note records a construct in source that may change PATH unpredictably.
func (m *startupPathModel) note(source, construct string) {
	m.uncertainties = append(m.uncertainties, startupUncertainty{seq: m.seq, managedPresent: m.contains(m.binDir, m.goos), source: source, construct: construct})
}

// relevantUncertainty returns the first unmodeled construct that runs after
// the managed directory is placed on PATH. Earlier constructs cannot reorder
// it: the managed block prepends the directory after they ran.
func (m *startupPathModel) relevantUncertainty() *startupUncertainty {
	for i := range m.uncertainties {
		u := &m.uncertainties[i]
		if m.placedSeq >= 0 && u.seq >= m.placedSeq || m.placedSeq < 0 && u.managedPresent {
			return u
		}
	}
	return nil
}

// replay models sourcing path from the file from. Only bounded regular files
// are read; anything else leaves PATH unverifiable.
func (m *startupPathModel) replay(path, from string, depth int) {
	if depth > maxStartupSourceDepth {
		m.note(from, fmt.Sprintf("sources nested more than %d levels deep (%s)", maxStartupSourceDepth, path))
		return
	}
	for _, active := range m.active {
		if active == path {
			return
		}
	}
	data, ok := m.overlay[path]
	if !ok {
		var missing bool
		var reason string
		data, missing, reason = readStartupFile(path)
		if missing {
			return
		}
		if reason != "" {
			m.note(from, reason)
			return
		}
	}
	m.visited++
	if m.visited > maxStartupFiles {
		m.note(from, fmt.Sprintf("more than %d sourced startup files (%s)", maxStartupFiles, path))
		return
	}
	blocks := m.blocks
	m.blocks = nil
	m.active = append(m.active, path)
	defer func() {
		m.active = m.active[:len(m.active)-1]
		m.blocks = blocks
	}()
	for _, line := range strings.Split(string(data), "\n") {
		for _, command := range splitShellCommands(strings.TrimRight(line, "\r")) {
			m.apply(command, path, depth)
		}
	}
}

// readStartupFile reads path only when it is a regular file of bounded size.
// missing reports an absent file, which the shell skips as well.
func readStartupFile(path string) ([]byte, bool, string) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, true, ""
	}
	if err != nil {
		return nil, false, fmt.Sprintf("an unreadable source %s (%v)", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Sprintf("a source %s that is not a regular file", path)
	}
	if info.Size() > maxStartupFileSize {
		return nil, false, fmt.Sprintf("a source %s larger than %d bytes", path, maxStartupFileSize)
	}
	// Non-blocking open: a FIFO swapped in after Stat must not hang.
	file, err := os.OpenFile(path, os.O_RDONLY|startupOpenFlags, 0)
	if err != nil {
		return nil, false, fmt.Sprintf("an unreadable source %s (%v)", path, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, false, fmt.Sprintf("a source %s that changed while being read", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStartupFileSize+1))
	if err != nil {
		return nil, false, fmt.Sprintf("an unreadable source %s (%v)", path, err)
	}
	if len(data) > maxStartupFileSize {
		return nil, false, fmt.Sprintf("a source %s larger than %d bytes", path, maxStartupFileSize)
	}
	return data, false, ""
}

var (
	zshPathArrayPattern = regexp.MustCompile(`^(?:export\s+|typeset\s+(?:-U\s+)?)?path(\+?)=\((.*)\)$`)
	shellNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	functionHeader      = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_:.-]*)\(\)$`)
	// pathFreeEvalPattern matches the eval lines of the stock Debian and
	// Ubuntu .bashrc: their output sets only LS_COLORS or LESSOPEN.
	pathFreeEvalPattern = regexp.MustCompile(`^eval "?\$\((?:SHELL=\S+ )?(?:dircolors|lesspipe)(?: [^()$]*)?\)"?$`)
)

func (m *startupPathModel) apply(command, source string, depth int) {
	m.seq++
	words := m.enterBlocks(shellWords(command))
	if len(words) == 0 {
		return
	}
	command = strings.Join(words, " ")
	if context, function := m.skippedContext(); context != "" {
		if function != "" {
			// A body runs only when called; record direct PATH changes.
			if changesPath(words) {
				m.pathFunctions[function] = true
				m.note(source, fmt.Sprintf("a PATH change inside a %s (%s)", context, command))
			}
			return
		}
		if changesPath(words) || isSourceWord(words[0]) || words[0] == "eval" && !pathFreeEvalPattern.MatchString(command) || m.pathFunctions[words[0]] {
			m.note(source, fmt.Sprintf("a PATH change inside a %s (%s)", context, command))
		}
		return
	}
	if match := zshPathArrayPattern.FindStringSubmatch(command); match != nil {
		m.applyPathArray(match[1] == "+", match[2], source)
		return
	}
	switch {
	case isSourceWord(words[0]):
		if len(words) < 2 {
			return
		}
		file, unresolved := m.expand(words[1], false)
		if unresolved != "" || !filepath.IsAbs(file) {
			m.note(source, fmt.Sprintf("a source of %s whose target cannot be resolved", words[1]))
			return
		}
		m.replay(file, source, depth+1)
		return
	case words[0] == "eval":
		if !pathFreeEvalPattern.MatchString(command) {
			m.note(source, "eval ("+command+")")
		}
		return
	case m.pathFunctions[words[0]]:
		m.note(source, fmt.Sprintf("a call to %s, a function that changes PATH", words[0]))
		return
	case words[0] == "export":
		words = words[1:]
	}
	for _, word := range words {
		if name, _, ok := strings.Cut(word, "="); !ok || !shellNamePattern.MatchString(name) {
			// Assignments followed by a command only affect that command.
			if changesPath(words) {
				m.note(source, "an unmodeled PATH change ("+command+")")
			}
			return
		}
	}
	for _, word := range words {
		name, value, _ := strings.Cut(word, "=")
		if name == "PATH" {
			if unresolved := m.applyPathValue(value, source); unresolved != "" {
				m.note(source, unresolved+" in a PATH value ("+command+")")
			}
			continue
		}
		// An unexpandable value keeps its marker so later PATH segments that
		// use it are recorded as unresolved instead of guessed.
		expanded, _ := m.expand(value, false)
		m.setVar(name, expanded)
	}
}

func isSourceWord(word string) bool { return word == "." || word == `\.` || word == "source" }

// changesPath reports whether a simple command assigns PATH (or zsh's path
// array) in the current shell. A prefix assignment scoped to a command and a
// function-local PATH do not.
func changesPath(words []string) bool {
	if len(words) == 0 || words[0] == "local" {
		return false
	}
	switch words[0] {
	case "export", "declare", "typeset", "readonly":
		words = words[1:]
		for len(words) > 0 && strings.HasPrefix(words[0], "-") {
			words = words[1:]
		}
	}
	touches := false
	for _, word := range words {
		if strings.HasPrefix(word, "path=(") || strings.HasPrefix(word, "path+=(") {
			return true
		}
		name, _, ok := strings.Cut(word, "=")
		name = strings.TrimSuffix(name, "+")
		if !ok || !shellNamePattern.MatchString(name) {
			return false
		}
		touches = touches || name == "PATH"
	}
	return touches
}

// enterBlocks consumes leading compound-command keywords, tracking which
// bodies run conditionally or not at all, and returns the remaining words.
func (m *startupPathModel) enterBlocks(words []string) []string {
	for len(words) > 0 {
		top := len(m.blocks) - 1
		switch word := words[0]; {
		case word == "if":
			m.blocks = append(m.blocks, shellBlock{kind: "if"})
		case word == "else" || word == "elif":
			if top >= 0 && (m.blocks[top].kind == "if" || m.blocks[top].kind == "else") {
				m.blocks[top].kind = "else"
			}
		case word == "fi":
			m.pop("if", "else")
		case word == "case":
			m.blocks = append(m.blocks, shellBlock{kind: "case"})
			return nil
		case word == "esac":
			m.pop("case")
		case word == "for" || word == "while" || word == "until" || word == "select":
			m.blocks = append(m.blocks, shellBlock{kind: "loop"})
			return nil
		case word == "done":
			m.pop("loop")
		case word == "{":
			if top >= 0 && m.blocks[top].awaitingBrace {
				m.blocks[top].awaitingBrace = false
			} else {
				m.blocks = append(m.blocks, shellBlock{kind: "brace"})
			}
		case word == "}":
			m.pop("function", "brace")
		case word == "function" && len(words) > 1:
			m.blocks = append(m.blocks, shellBlock{kind: "function", name: strings.TrimSuffix(words[1], "()"), awaitingBrace: true})
			words = words[1:]
		case functionHeader.MatchString(word):
			m.blocks = append(m.blocks, shellBlock{kind: "function", name: strings.TrimSuffix(word, "()"), awaitingBrace: true})
		case len(words) > 1 && words[1] == "()" && shellNamePattern.MatchString(word):
			m.blocks = append(m.blocks, shellBlock{kind: "function", name: word, awaitingBrace: true})
			words = words[1:]
		case top >= 0 && m.blocks[top].kind == "case" && strings.HasSuffix(word, ")") && word != "()":
			// A case pattern label such as *":$DIR:"*).
		case word == "then" || word == "do" || word == "!":
		default:
			return words
		}
		words = words[1:]
	}
	return nil
}

func (m *startupPathModel) pop(kinds ...string) {
	if top := len(m.blocks) - 1; top >= 0 {
		for _, kind := range kinds {
			if m.blocks[top].kind == kind {
				m.blocks = m.blocks[:top]
				return
			}
		}
	}
}

// skippedContext names the innermost body whose commands may not run as
// written, and the enclosing function, if any. The then-branch of an if runs
// under its usual `[ -d dir ]` guard and is modeled as taken.
func (m *startupPathModel) skippedContext() (string, string) {
	context, function := "", ""
	for _, block := range m.blocks {
		switch block.kind {
		case "function":
			context, function = "function body", block.name
		case "case":
			context = "case body"
		case "loop":
			context = "loop body"
		case "else":
			context = "if/else branch"
		}
	}
	return context, function
}

// pathSentinel marks where the previous PATH is spliced into a new value.
const (
	pathSentinel     = "\x00"
	unresolvedMarker = "\x01"
)

// applyPathValue replays PATH=raw and returns what could not be resolved.
func (m *startupPathModel) applyPathValue(raw, source string) string {
	expanded, unresolved := m.expand(raw, true)
	var next []startupPathEntry
	for _, segment := range strings.Split(expanded, ":") {
		next = m.appendSegment(next, segment, source)
	}
	m.entries = next
	return unresolved
}

func (m *startupPathModel) applyPathArray(appendOnly bool, raw, source string) {
	var next []startupPathEntry
	if appendOnly {
		next = append(next, m.entries...)
	}
	for _, word := range shellWords(raw) {
		if word == "$path" || word == "${path}" || word == "${path[@]}" || word == `"${path[@]}"` {
			next = append(next, m.entries...)
			continue
		}
		expanded, unresolved := m.expand(word, false)
		if unresolved != "" {
			m.note(source, unresolved+" in a path array ("+word+")")
		}
		next = m.appendSegment(next, expanded, source)
	}
	m.entries = next
}

func (m *startupPathModel) appendSegment(next []startupPathEntry, segment, source string) []startupPathEntry {
	switch {
	case segment == pathSentinel:
		return append(next, m.entries...)
	case segment == "" || strings.Contains(segment, unresolvedMarker) || strings.Contains(segment, pathSentinel) || !filepath.IsAbs(segment):
		return next
	default:
		// Only a prepend places the managed directory ahead of the existing
		// PATH; an append after it leaves earlier uncertainties relevant.
		if samePath(segment, m.binDir, m.goos) && len(next) == 0 {
			m.placedSeq = m.seq
		}
		return append(next, startupPathEntry{dir: filepath.Clean(segment), source: source})
	}
}

func (m *startupPathModel) setVar(name, value string) {
	if m.vars == nil {
		m.vars = map[string]string{}
	}
	m.vars[name] = value
}

func (m *startupPathModel) lookup(name string) (string, bool) {
	switch name {
	case "HOME":
		return m.homeDir, true
	case "PATH":
		return "", false
	}
	if value, ok := m.vars[name]; ok {
		return value, !strings.Contains(value, unresolvedMarker)
	}
	return os.LookupEnv(name)
}

// expand performs the static subset of shell word expansion: quote removal,
// leading or post-colon tilde, and $NAME/${NAME}. With pathValue set, $PATH
// becomes pathSentinel; any other unexpandable piece becomes unresolvedMarker
// and the returned description names the first one.
func (m *startupPathModel) expand(word string, pathValue bool) (string, string) {
	var out strings.Builder
	unresolved := ""
	fail := func(description string) {
		out.WriteString(unresolvedMarker)
		if unresolved == "" {
			unresolved = description
		}
	}
	quote := byte(0)
	for i := 0; i < len(word); i++ {
		c := word[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				out.WriteByte(c)
			}
		case c == '\'' && quote == 0, c == '"' && quote == 0:
			quote = c
		case c == '"' && quote == '"':
			quote = 0
		case c == '\\' && i+1 < len(word):
			i++
			out.WriteByte(word[i])
		case c == '~' && quote == 0 && (i == 0 || word[i-1] == ':') && (i+1 == len(word) || word[i+1] == '/' || word[i+1] == ':'):
			out.WriteString(m.homeDir)
		case c == '`':
			fail("command substitution")
			if end := strings.IndexByte(word[i+1:], '`'); end >= 0 {
				i += end + 1
			} else {
				i = len(word)
			}
		case c == '$' && i+1 < len(word) && word[i+1] == '(':
			fail("command substitution")
			i = substitutionEnd(word, i+1) - 1
		case c == '$':
			name, end := shellVariableName(word, i+1)
			i = end - 1
			if name == "" {
				fail("parameter expansion")
				continue
			}
			if name == "PATH" && pathValue {
				out.WriteString(pathSentinel)
				continue
			}
			value, found := m.lookup(name)
			if !found {
				fail("unresolved variable $" + name)
				continue
			}
			out.WriteString(value)
		default:
			out.WriteByte(c)
		}
	}
	return out.String(), unresolved
}

// shellVariableName parses $NAME or ${NAME} starting after the dollar sign.
// It returns an empty name for command substitution, defaults, and other
// forms that cannot be expanded statically.
func shellVariableName(word string, start int) (string, int) {
	if start < len(word) && word[start] == '{' {
		end := strings.IndexByte(word[start:], '}')
		if end < 0 {
			return "", len(word)
		}
		name := word[start+1 : start+end]
		if !shellNamePattern.MatchString(name) {
			return "", start + end + 1
		}
		return name, start + end + 1
	}
	end := start
	for end < len(word) && (word[end] == '_' || word[end] >= 'a' && word[end] <= 'z' || word[end] >= 'A' && word[end] <= 'Z' || end > start && word[end] >= '0' && word[end] <= '9') {
		end++
	}
	if end == start {
		if start < len(word) && word[start] == '(' {
			return "", substitutionEnd(word, start)
		}
		return "", start
	}
	return word[start:end], end
}

// substitutionEnd returns the index just past the parenthesis that closes the
// command substitution opening at word[start].
func substitutionEnd(word string, start int) int {
	depth := 0
	for i := start; i < len(word); i++ {
		switch word[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(word)
}

// splitShellCommands splits one line at unquoted command separators and drops
// an unquoted trailing comment.
func splitShellCommands(line string) []string {
	var commands []string
	var current strings.Builder
	quote := byte(0)
	depth := 0
	flush := func() {
		if command := strings.TrimSpace(current.String()); command != "" {
			commands = append(commands, command)
		}
		current.Reset()
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' && i+1 < len(line) {
				current.WriteByte(c)
				i++
				c = line[i]
			} else if c == quote {
				quote = 0
			}
		case c == '\\' && i+1 < len(line):
			current.WriteByte(c)
			i++
			c = line[i]
		case c == '\'' || c == '"':
			quote = c
		case c == '$' && i+1 < len(line) && line[i+1] == '(':
			depth++
		case c == ')' && depth > 0:
			depth--
		case depth > 0:
		case c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			flush()
			return commands
		case c == ';' || c == '&' || c == '|':
			flush()
			continue
		}
		current.WriteByte(c)
	}
	flush()
	return commands
}

// shellWords splits a command at unquoted blanks, keeping quotes for expand.
func shellWords(command string) []string {
	var words []string
	var current strings.Builder
	quote := byte(0)
	depth := 0
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '$' && i+1 < len(command) && command[i+1] == '(':
			depth++
		case c == ')' && depth > 0:
			depth--
		case depth > 0:
		case c == '\\' && i+1 < len(command):
			current.WriteByte(c)
			i++
			c = command[i]
		case c == '\'' || c == '"':
			quote = c
		case c == ' ' || c == '\t':
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteByte(c)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

// ResolveManagedLauncher resolves the first OpenCode executable in pathValue
// with exec.LookPath's PATH semantics, without consulting the process PATH,
// and verifies it is a Gentle-owned launcher in the managed bin directory.
// A different executable first wraps ErrManagedLauncherShadowed.
func ResolveManagedLauncher(homeDir, pathValue, goos string) (string, error) {
	if goos == "" {
		goos = runtime.GOOS
	}
	return resolveManagedLauncher(homeDir, pathValue, goos, false)
}

func resolveManagedLauncher(homeDir, pathValue, goos string, assumeLauncher bool) (string, error) {
	binDir := BinDir(homeDir)
	if goos == "windows" && runtime.GOOS == "windows" {
		// Windows command lookup probes the current directory first unless
		// NoDefaultCurrentDirectoryInExePath is set.
		if _, disabled := os.LookupEnv("NoDefaultCurrentDirectoryInExePath"); !disabled {
			if candidate, ok := firstOpenCodeExecutable(".", goos); ok {
				return "", fmt.Errorf("opencode resolves through the current directory %q: %w", candidate, exec.ErrDot)
			}
		}
	}
	for _, entry := range splitPath(pathValue, goos) {
		if entry == "" {
			if goos == "windows" {
				continue
			}
			// An empty POSIX PATH entry means the current directory.
			entry = "."
		}
		if assumeLauncher && samePath(entry, binDir, goos) {
			return ManagedLauncherPaths(homeDir, goos)[0], nil
		}
		candidate, ok := firstOpenCodeExecutable(entry, goos)
		if !ok {
			continue
		}
		if !filepath.IsAbs(candidate) {
			return "", fmt.Errorf("opencode resolves through relative PATH entry %q: %w", entry, exec.ErrDot)
		}
		if !samePath(filepath.Dir(candidate), binDir, goos) {
			return "", fmt.Errorf("%w by %s", ErrManagedLauncherShadowed, candidate)
		}
		return validateManagedLauncherCandidate(homeDir, candidate, goos)
	}
	return "", fmt.Errorf("no OpenCode executable on PATH resolves to the managed launcher in %s", binDir)
}

func firstOpenCodeExecutable(entry, goos string) (string, bool) {
	for _, name := range targetNames(goos) {
		candidate := filepath.Join(entry, name)
		if pathEntryExecutable(candidate, goos) {
			return candidate, true
		}
	}
	return "", false
}

func validateManagedLauncherCandidate(homeDir, candidate, goos string) (string, error) {
	managed := false
	for _, path := range ManagedLauncherPaths(homeDir, goos) {
		if samePath(candidate, path, goos) {
			managed = true
		}
	}
	if !managed {
		return "", fmt.Errorf("%w by %s", ErrManagedLauncherShadowed, candidate)
	}
	snapshot, err := readLauncherSnapshot(candidate)
	if err != nil {
		return "", err
	}
	if !snapshot.exists {
		return "", fmt.Errorf("managed OpenCode launcher %s is missing", candidate)
	}
	if goos != "windows" && snapshot.mode&0o111 == 0 {
		return "", fmt.Errorf("managed OpenCode launcher %s is not executable", candidate)
	}
	if !snapshot.owned {
		return "", fmt.Errorf("managed OpenCode launcher %s is not Gentle-owned", candidate)
	}
	return candidate, nil
}

func pathEntryExecutable(path, goos string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return goos == "windows" || info.Mode().Perm()&0o111 != 0
}

// resolvesUnder reports whether path is a symlink into root, such as a user
// link to the managed launcher.
func resolvesUnder(path, root, goos string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && pathUnder(resolved, root, goos)
}

// ManagedLauncherTarget returns the OpenCode executable a Gentle-owned
// launcher at path delegates to. Only exact generated bytes qualify, so a file
// that merely carries the ownership marker is not a managed launcher.
func ManagedLauncherTarget(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return managedLauncherTarget(path, data)
}

// windowsTargetSafe reports whether target can be embedded in the generated
// CMD launcher without cmd.exe reinterpreting it: % expands variables, !
// expands them under delayed expansion, a quote ends the quoted argument, and
// control characters split the command.
func windowsTargetSafe(target string) bool {
	return !strings.ContainsFunc(target, func(r rune) bool {
		return r == '%' || r == '!' || r == '"' || r < 0x20 || r == 0x7f
	})
}

func windowsExecutableExtensions() []string {
	pathext := os.Getenv("PATHEXT")
	if pathext == "" {
		return []string{".com", ".exe", ".bat", ".cmd"}
	}
	var extensions []string
	for _, extension := range strings.Split(pathext, ";") {
		extension = strings.ToLower(strings.TrimSpace(extension))
		if extension == "" {
			continue
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		extensions = append(extensions, extension)
	}
	return extensions
}
