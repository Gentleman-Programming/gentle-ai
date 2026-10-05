package opencode

import (
	"errors"
	"fmt"
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
	case resolution.Status == ActivationStatusReady, resolution.Status == ActivationStatusShadowed:
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
		// The startup files cannot be replayed, so the current PATH is the only
		// evidence; it proves this shell, not every new one.
		if _, err := resolveManagedLauncher(homeDir, options.Path, options.OS, assumeLauncher); err == nil {
			return LoginShellResolution{Status: ActivationStatusReady, Resolved: launcher, Source: inheritedPathSource, Reason: fmt.Sprintf("the current PATH resolves opencode to the managed launcher %s; new shells were not verified because %s, so keep %s first on PATH there", launcher, unmodeled, binDir)}
		}
		return LoginShellResolution{Status: ActivationStatusPending, Reason: unmodeled}
	}

	model := startupPathModel{homeDir: homeDir, overlay: overlay}
	for _, entry := range splitPath(options.Path, options.OS) {
		if filepath.IsAbs(entry) {
			model.entries = append(model.entries, startupPathEntry{dir: entry, source: inheritedPathSource})
		}
	}
	for _, file := range files {
		model.replay(file, 0)
	}

	for _, entry := range model.entries {
		candidate := filepath.Join(entry.dir, "opencode")
		managedDir := samePath(entry.dir, binDir, options.OS)
		if !managedDir && !pathEntryExecutable(candidate, options.OS) {
			continue
		}
		// A link into the managed directory runs the launcher as well.
		if managedDir || resolvesUnder(candidate, binDir, options.OS) {
			if assumeLauncher && managedDir {
				return readyLoginShell(launcher, entry.source)
			}
			if _, err := validateManagedLauncherCandidate(homeDir, launcher, options.OS); err != nil {
				return LoginShellResolution{Status: ActivationStatusPending, Source: entry.source, Reason: fmt.Sprintf("new login shells put %s on PATH, but %v; run gentle-ai sync", binDir, err)}
			}
			return readyLoginShell(launcher, entry.source)
		}
		result := LoginShellResolution{Resolved: candidate, Source: entry.source}
		if !model.contains(binDir, options.OS) {
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
// understands assignments, `export`, zsh `path=(...)` arrays, and sourcing;
// anything it cannot expand statically is ignored rather than guessed.
type startupPathModel struct {
	homeDir string
	overlay map[string][]byte
	entries []startupPathEntry
	vars    map[string]string
	active  []string
}

const maxStartupSourceDepth = 8

func (m *startupPathModel) contains(dir, goos string) bool {
	for _, entry := range m.entries {
		if samePath(entry.dir, dir, goos) {
			return true
		}
	}
	return false
}

func (m *startupPathModel) replay(path string, depth int) {
	if depth > maxStartupSourceDepth {
		return
	}
	for _, active := range m.active {
		if active == path {
			return
		}
	}
	data, ok := m.overlay[path]
	if !ok {
		var err error
		if data, err = os.ReadFile(path); err != nil {
			return
		}
	}
	m.active = append(m.active, path)
	defer func() { m.active = m.active[:len(m.active)-1] }()
	for _, line := range strings.Split(string(data), "\n") {
		for _, command := range splitShellCommands(strings.TrimRight(line, "\r")) {
			m.apply(command, path, depth)
		}
	}
}

var (
	zshPathArrayPattern = regexp.MustCompile(`^(?:export\s+|typeset\s+(?:-U\s+)?)?path(\+?)=\((.*)\)$`)
	shellNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func (m *startupPathModel) apply(command, source string, depth int) {
	words := shellWords(command)
	for len(words) > 0 {
		switch words[0] {
		case "if", "then", "else", "elif", "do", "{", "!":
			words = words[1:]
			continue
		}
		break
	}
	if len(words) == 0 {
		return
	}
	if match := zshPathArrayPattern.FindStringSubmatch(strings.Join(words, " ")); match != nil {
		m.applyPathArray(match[1] == "+", match[2], source)
		return
	}
	switch words[0] {
	case ".", "source":
		if len(words) > 1 {
			if file, ok := m.expand(words[1], false); ok && filepath.IsAbs(file) {
				m.replay(file, depth+1)
			}
		}
		return
	case "export":
		words = words[1:]
	}
	for _, word := range words {
		if name, _, ok := strings.Cut(word, "="); !ok || !shellNamePattern.MatchString(name) {
			// Assignments followed by a command only affect that command.
			return
		}
	}
	for _, word := range words {
		name, value, _ := strings.Cut(word, "=")
		if name == "PATH" {
			m.applyPathValue(value, source)
			continue
		}
		// An unexpandable value keeps its marker so later PATH segments that
		// use it are dropped instead of guessed.
		expanded, _ := m.expand(value, false)
		m.setVar(name, expanded)
	}
}

// pathSentinel marks where the previous PATH is spliced into a new value.
const (
	pathSentinel     = "\x00"
	unresolvedMarker = "\x01"
)

func (m *startupPathModel) applyPathValue(raw, source string) {
	expanded, _ := m.expand(raw, true)
	var next []startupPathEntry
	for _, segment := range strings.Split(expanded, ":") {
		next = m.appendSegment(next, segment, source)
	}
	m.entries = next
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
		expanded, _ := m.expand(word, false)
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
		return value, true
	}
	return os.LookupEnv(name)
}

// expand performs the static subset of shell word expansion: quote removal,
// leading or post-colon tilde, and $NAME/${NAME}. With pathValue set, $PATH
// becomes pathSentinel; any other unexpandable piece becomes unresolvedMarker.
func (m *startupPathModel) expand(word string, pathValue bool) (string, bool) {
	var out strings.Builder
	ok := true
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
			out.WriteString(unresolvedMarker)
			ok = false
		case c == '$':
			name, end := shellVariableName(word, i+1)
			if name == "" {
				out.WriteString(unresolvedMarker)
				ok = false
				i = end - 1
				continue
			}
			i = end - 1
			if name == "PATH" && pathValue {
				out.WriteString(pathSentinel)
				continue
			}
			value, found := m.lookup(name)
			if !found {
				out.WriteString(unresolvedMarker)
				ok = false
				continue
			}
			out.WriteString(value)
		default:
			out.WriteByte(c)
		}
	}
	return out.String(), ok
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
