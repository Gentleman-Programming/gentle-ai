package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Observation is everything the benchmark can see about ONE gentle-ai
// invocation, from either the driven runner or a recorded agent session.
// It is deliberately the process boundary and nothing more: the classifier
// must work against any build of the product, including old releases.
type Observation struct {
	Args     []string `json:"args"`
	ExitCode int      `json:"exit_code"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`

	// StdoutCaptured / StderrCaptured are false when the stream was a
	// terminal and the recording shim passed it through untouched rather
	// than teeing it (teeing a TTY would change the product's own
	// interactivity detection and void every measurement).
	StdoutCaptured bool `json:"stdout_captured"`
	StderrCaptured bool `json:"stderr_captured"`

	// DeclaredDeadEnd is the first of two author-declared inputs to
	// classification. "No continuation exists at all" cannot be decided from
	// outside the binary, so the journey corpus states it explicitly and the
	// README says so. Everything else below is mechanical.
	DeclaredDeadEnd bool `json:"declared_dead_end,omitempty"`

	// DeclaredByDesign is the second, and it is deliberately more expensive
	// to make: it names one of three shapes from a closed vocabulary AND
	// quotes the product's own next-action text, which the classifier then
	// verifies is really in the bytes below. See ByDesignDeclaration.
	DeclaredByDesign *ByDesignDeclaration `json:"declared_by_design,omitempty"`

	// SelfRecovered marks a block after which the flow continued with no
	// extra command. The driven runner sets it when a composite step
	// absorbed the failure internally without issuing a recovery command.
	SelfRecovered bool `json:"self_recovered,omitempty"`
}

// Block classes. Exactly one applies to any block.
const (
	BlockSelfRecovered = "self_recovered"
	BlockInBand        = "in_band"
	BlockOutOfBand     = "out_of_band"
	BlockByDesign      = "by_design"
	BlockDeadEnd       = "dead_end"
	NotABlock          = ""
)

// The three shapes a by-design refusal can take. This vocabulary is CLOSED:
// anything outside it is a corpus error that fails the run, never a shape the
// benchmark quietly accepts as free text. Each one answers the same question —
// why can no runnable command honestly exist here? — and if a block's answer
// is not one of these three, the honest class is out_of_band.
const (
	// ByDesignOperatorKnowledge: the product would have to know a value only
	// the operator has. `review start` in a bare repository can only name
	// `--cwd <path-to-a-checkout>`, because it cannot know where the
	// operator's checkout is, and naming a guess would be naming a dead end.
	ByDesignOperatorKnowledge = "operator-knowledge"
	// ByDesignWorldAction: the exit is an action, not a command. Edit the
	// code, free some disk space, plug the network mount back in.
	ByDesignWorldAction = "world-action"
	// ByDesignHumanAuthority: the block IS a human decision by design. If a
	// command could produce the authorization, the gate would be theatre.
	ByDesignHumanAuthority = "human-authority"
)

var byDesignShapes = []struct {
	Name    string
	Meaning string
}{
	{ByDesignOperatorKnowledge, "the product cannot know a value only the operator has"},
	{ByDesignWorldAction, "the exit is an action in the world, not a command"},
	{ByDesignHumanAuthority, "the block is a human decision by design"},
}

// ByDesignDeclaration is the corpus claiming that a block is a CORRECT refusal
// for which no runnable command can honestly exist — that naming one would be
// naming a dead end, which this benchmark treats as worse than naming nothing.
//
// It is expensive on purpose, because it is the one annotation that can make
// the out_of_band number smaller. It costs two things:
//
//   - Shape, from the closed vocabulary above. Not free text: an unrecognised
//     shape is a corpus error and fails the run.
//   - NextAction, the EXACT substring of the product's own output that states
//     what the operator does next. The classifier verifies that substring is
//     really in the emitted bytes. This is the load-bearing half. "No command
//     can exist" never excuses "the message says nothing", so `Error: no.`
//     cannot be declared by-design: there is no next-action text to quote, and
//     a quote that is not in the output is not a quote.
type ByDesignDeclaration struct {
	Shape      string `json:"shape"`
	NextAction string `json:"next_action"`
}

// Validate reports why a declaration is unusable, or nil. It checks only what
// is decidable from the declaration itself; whether the quote is TRUE is
// decided against the emitted bytes, by verified.
func (d *ByDesignDeclaration) Validate() error {
	if d == nil {
		return nil
	}
	known := false
	names := make([]string, 0, len(byDesignShapes))
	for _, shape := range byDesignShapes {
		names = append(names, shape.Name)
		if shape.Name == d.Shape {
			known = true
		}
	}
	if !known {
		return fmt.Errorf("by_design shape %q is not one of %s", d.Shape, strings.Join(names, ", "))
	}
	if strings.TrimSpace(d.NextAction) == "" {
		return errors.New("by_design declares no next-action text to verify: quote the exact words the product prints, or the block is out_of_band")
	}
	return nil
}

// verified reports whether the declaration is usable AND its quoted
// next-action text really is in what the product printed.
func (d *ByDesignDeclaration) verified(emitted string) bool {
	if d == nil || d.Validate() != nil {
		return false
	}
	return strings.Contains(emitted, d.NextAction)
}

// ByDesignOutcome is what the classifier did with a declaration. It is
// recorded even when the exemption did NOT apply, so a declaration that has
// gone stale — the product now names a runnable command, or no longer prints
// the quoted words — is visible in the report instead of silently ignored.
type ByDesignOutcome struct {
	Shape      string `json:"shape"`
	NextAction string `json:"next_action"`
	Applied    bool   `json:"applied"`
	Reason     string `json:"reason,omitempty"`
}

// DeadEndOutcome is the same accounting for the other author declaration.
// `dead_end` is the most expensive claim the corpus allows — it says no
// continuation exists at all — so a declaration the product has since outgrown
// must be visible rather than quietly outranked. Silently ignoring it lets a
// closed gap keep advertising itself as open.
type DeadEndOutcome struct {
	Applied bool   `json:"applied"`
	Reason  string `json:"reason,omitempty"`
}

// deadEndOutcome explains what became of a declared dead end. It returns nil
// when the corpus declared nothing, which is the ordinary case.
func deadEndOutcome(o Observation, class string) *DeadEndOutcome {
	if !o.DeclaredDeadEnd {
		return nil
	}
	switch {
	case class == BlockDeadEnd:
		return &DeadEndOutcome{Applied: true}
	case class == NotABlock:
		return &DeadEndOutcome{Reason: "the invocation did not block, so there was no dead end to declare"}
	case class == BlockInBand:
		return &DeadEndOutcome{Reason: "the product named a runnable continuation, so the declaration is stale and should be removed"}
	case class == BlockSelfRecovered:
		return &DeadEndOutcome{Reason: "the flow continued with no extra command"}
	default:
		return &DeadEndOutcome{Reason: "the block was classified " + class}
	}
}

// byDesignOutcome explains, for one already-classified observation, what
// became of its by-design declaration. It returns nil when the corpus
// declared nothing, which is the ordinary case.
func byDesignOutcome(o Observation, class string) *ByDesignOutcome {
	if o.DeclaredByDesign == nil {
		return nil
	}
	outcome := &ByDesignOutcome{Shape: o.DeclaredByDesign.Shape, NextAction: o.DeclaredByDesign.NextAction}
	switch {
	case class == BlockByDesign:
		outcome.Applied = true
	case class == NotABlock:
		outcome.Reason = "the invocation did not block, so there was nothing to exempt"
	case class == BlockInBand:
		outcome.Reason = "the product named a runnable continuation, and mechanical evidence outranks the declaration"
	case class == BlockSelfRecovered:
		outcome.Reason = "the flow continued with no extra command"
	case o.DeclaredByDesign.Validate() != nil:
		outcome.Reason = o.DeclaredByDesign.Validate().Error()
	default:
		outcome.Reason = "the declared next action is not in the emitted output, so the quote is not a quote"
	}
	return outcome
}

// productName is the literal a suggested command must begin with.
const productName = "gentle-ai"

// commandStart matches the product name where a command could begin: at the
// start of a line, or after whitespace or an opening quote/bracket, and
// followed by a delimiter rather than more word characters.
var commandStart = regexp.MustCompile("(?:^|[\\s'\"`(\\[])gentle-ai(?:[\\s'\"`)\\]]|$)")

// placeholderRun matches an unfilled template argument such as <gate>.
// A command the user still has to fill in is not runnable, so it does not
// make a block in-band on its own.
var placeholderRun = regexp.MustCompile(`<[^>\s]*>`)

// continuationKeys are the JSON envelope keys that name a runnable follow-up
// operation. They are looked up recursively so the rule survives envelope
// nesting changes across versions.
var continuationKeys = map[string]bool{
	"next_action":        true,
	"recovery_operation": true,
	"capture_operation":  true,
}

// nonContinuationValues are values that explicitly mean "there is nothing to
// run next".
var nonContinuationValues = map[string]bool{
	"":     true,
	"stop": true,
	"none": true,
	"n/a":  true,
}

// HasRunnableCommand reports whether the emitted text names a literal,
// immediately runnable gentle-ai command: the product name, at least one
// argument after it, and no unfilled <placeholder>.
//
// Each occurrence of the product name opens a candidate command that ends at
// the end of the line, at a closing quote or bracket, or where the next
// occurrence begins. That keeps two suggestions on one line independent, so a
// line offering both a clean command and a templated one still counts as
// in-band on the strength of the clean one.
func HasRunnableCommand(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		positions := commandStart.FindAllStringIndex(line, -1)
		for index, position := range positions {
			end := len(line)
			if index+1 < len(positions) {
				end = positions[index+1][0]
			}
			// The match includes the surrounding delimiters; the argument
			// list starts immediately after the product name itself.
			offset := strings.Index(line[position[0]:position[1]], productName)
			tailStart := position[0] + offset + len(productName)
			if commandTailIsRunnable(line[tailStart:end]) {
				return true
			}
		}
	}
	return false
}

// commandTailIsRunnable reports whether what follows the product name is a
// usable argument list.
func commandTailIsRunnable(tail string) bool {
	arguments := 0
	for _, token := range strings.Fields(tail) {
		if cut := strings.IndexAny(token, "'\"`)]"); cut >= 0 {
			token = token[:cut]
			if token == "" {
				break
			}
			if placeholderRun.MatchString(token) {
				return false
			}
			arguments++
			break
		}
		if placeholderRun.MatchString(token) {
			return false
		}
		arguments++
	}
	return arguments > 0
}

// HasEnvelopeContinuation reports whether a JSON envelope on stdout carries a
// next_action / recovery_operation / collect.capture_operation naming a real
// operation. next_transition.execute.operation is treated the same way: it is
// the execute-shaped sibling of collect.capture_operation.
func HasEnvelopeContinuation(stdout string) bool {
	var envelope any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &envelope); err != nil {
		return false
	}
	return walkContinuation(envelope, "")
}

func walkContinuation(node any, parentKey string) bool {
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			if continuationKeys[key] || (parentKey == "execute" && key == "operation") {
				if text, ok := value.(string); ok && !nonContinuationValues[strings.ToLower(strings.TrimSpace(text))] {
					return true
				}
			}
			if walkContinuation(value, key) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if walkContinuation(item, parentKey) {
				return true
			}
		}
	}
	return false
}

// denialResults are gate results that deny delivery. A denial is a block even
// when the process exits 0, because the flow cannot proceed.
var denialResults = map[string]bool{
	"invalidated":   true,
	"scope-changed": true,
	"deny":          true,
	"denied":        true,
	"stop":          true,
	"blocked":       true,
	"corrupted":     true,
}

// deliveryDisabledUnmanaged is what a lifecycle gate reports when the kill
// switch is off and no receipt governs the candidate. It is the one delivery
// disposition that is not an opinion about the work: ordinary repository policy
// governs and the commit or push proceeds.
const deliveryDisabledUnmanaged = "disabled/unmanaged"

// IsBlock reports whether an invocation stopped the flow: a non-zero exit, or
// an envelope that denies.
func IsBlock(o Observation) bool {
	if IsUnsupported(o) {
		// A CLI surface this build does not have is recorded as
		// `unsupported`, never as a block and never as a pass.
		return false
	}
	if o.ExitCode != 0 {
		return true
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(o.Stdout)), &envelope); err != nil {
		return false
	}
	// A gate that hands delivery back to ordinary repository policy stopped
	// nothing, so it is not a block, and counting it as one would report the
	// kill switch working as friction it caused. It carries `allowed: false`
	// and `result: invalidated` because review-driven development is refusing
	// to express an opinion, not refusing the delivery -- and refusing to
	// fabricate an approval it did not earn is the whole point.
	//
	// This is exempted by the typed delivery disposition alone. The sibling
	// `unmanaged`, which is the switch ON with no receipt yet, stays a block:
	// there the operator really is stopped, and the gate names `review start`.
	if delivery, ok := envelope["delivery"].(string); ok &&
		strings.EqualFold(strings.TrimSpace(delivery), deliveryDisabledUnmanaged) {
		return false
	}
	if allowed, ok := envelope["allowed"].(bool); ok && !allowed {
		return true
	}
	if result, ok := envelope["result"].(string); ok && denialResults[strings.ToLower(strings.TrimSpace(result))] {
		return true
	}
	if action, ok := envelope["action"].(string); ok && strings.EqualFold(strings.TrimSpace(action), "stop") {
		return true
	}
	return false
}

// unsupportedPatterns are the shapes a gentle-ai build emits when the CLI
// surface a step needs does not exist in that build. Matching on output rather
// than on the exit code alone is deliberate: exit codes for "I do not have
// that flag" are not guaranteed to differ from ordinary state failures, and
// counting a missing surface as a state failure would be a flattering lie.
var unsupportedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)unknown [a-z-]+ command "`),
	regexp.MustCompile(`(?i)flag provided but not defined`),
	regexp.MustCompile(`(?i)unknown (flag|shorthand flag|option)`),
	regexp.MustCompile(`(?i)unrecognized (flag|option|argument)`),
	regexp.MustCompile(`(?i)unexpected [a-z ]+ argument "`),
	regexp.MustCompile(`(?i)unknown [a-z-]+ "--`),
	regexp.MustCompile(`(?i)^Error: unknown command`),
}

// IsUnsupported reports whether the binary rejected the SHAPE of the command
// (verb or flag it does not have) rather than the state of the repository.
func IsUnsupported(o Observation) bool {
	text := o.Stdout + "\n" + o.Stderr
	for _, pattern := range unsupportedPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// Classify assigns exactly one class to a block. It is the load-bearing
// function of this benchmark, and it is mechanical on purpose: given the same
// bytes it always returns the same class, so the in_band / out_of_band split
// cannot drift into opinion between two runs or two reviewers.
//
// Order:
//  1. not a block            -> NotABlock
//  2. flow continued anyway  -> self_recovered
//  3. text names a runnable `gentle-ai <verb> ...`  -> in_band
//  4. envelope names an operation                   -> in_band
//  5. corpus declares by-design AND the quoted next
//     action is verified in the emitted bytes       -> by_design
//  6. corpus declares no continuation exists        -> dead_end
//  7. otherwise                                     -> out_of_band
//
// The order is a ranking by how much evidence each rule carries. Rules 3 and 4
// read the product's own bytes and outrank both annotations. Rule 5 is an
// annotation that must survive a check against those same bytes. Rule 6 is a
// bare bool nobody can falsify from outside. More evidence wins.
//
// by_design and dead_end are neighbours and are NOT the same claim, so reach
// for them by what they assert about the operator's next action:
//
//   - dead_end says there is NO next action. The flow is over; the product owes
//     the reader nothing further because there is nothing further. It is the
//     worst outcome in this benchmark and it is still counted as one.
//   - by_design says there IS a next action, the product already stated it, and
//     it simply is not expressible as a `gentle-ai` command — because it needs
//     a value only the operator has, because it is an act in the world, or
//     because it is a human decision the whole gate exists to require.
//
// So neither subsumes the other: they are opposite answers to "is there
// anything to do next?". A step declaring both is contradicting itself, and
// validateCorpus rejects that combination before any journey runs, which is why
// the relative order of rules 5 and 6 never actually decides a real case. It is
// written this way anyway so that if the guard were ever removed, the class
// backed by verified evidence would win over the one backed by none.
func Classify(o Observation) string {
	if !IsBlock(o) {
		return NotABlock
	}
	if o.SelfRecovered {
		return BlockSelfRecovered
	}
	if HasRunnableCommand(o.Stdout + "\n" + o.Stderr) {
		return BlockInBand
	}
	if HasEnvelopeContinuation(o.Stdout) {
		return BlockInBand
	}
	if o.DeclaredByDesign.verified(o.Stdout + "\n" + o.Stderr) {
		return BlockByDesign
	}
	if o.DeclaredDeadEnd {
		return BlockDeadEnd
	}
	return BlockOutOfBand
}

// hasManualAuthorization reports whether an argv carried a hand-assembled
// --maintainer-authorization value. Both `--flag value` and `--flag=value`
// spellings count; an empty value does not.
func hasManualAuthorization(args []string) bool {
	const flag = "--maintainer-authorization"
	for index, arg := range args {
		trimmed := strings.TrimPrefix(arg, "-")
		trimmed = "--" + strings.TrimPrefix(trimmed, "-")
		if strings.HasPrefix(trimmed, flag+"=") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, flag+"=")) != ""
		}
		if trimmed == flag && index+1 < len(args) {
			return strings.TrimSpace(args[index+1]) != ""
		}
	}
	return false
}

// consentNotices are the exact stderr strings gentle-ai emits when it would
// have asked the human but had no terminal to ask on. Counting them is how a
// non-TTY run measures "times the flow would stop to ask a human".
var consentNotices = []string{
	"without asking, because this session has no terminal to answer on",
	"could not read an answer, so it reviewed this change",
	"did not recognize that answer, so it reviewed this change",
}

func countConsentNotices(stderr string) int {
	total := 0
	for _, notice := range consentNotices {
		total += strings.Count(stderr, notice)
	}
	return total
}

// isCaptureResult reports whether an argv is a reviewer-result capture that
// actually consumes reviewer output. `--preflight` is excluded: it verifies a
// binding and reads no result, so it is not a model run.
func isCaptureResult(args []string) bool {
	if len(args) < 2 || args[0] != "review" || args[1] != "capture-result" {
		return false
	}
	for _, arg := range args {
		if arg == "--preflight" || arg == "--preflight=true" {
			return false
		}
	}
	return true
}
