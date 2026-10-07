package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// reviewStartPreflightOptionNames are the START options STATUS preflights
// (rdd-risk-gated S14/S17). START stays the only owner of their frozen state.
var reviewStartPreflightOptionNames = []string{"request-context", "escalate-item", "escalate-reason", "lenses", "lenses-reason"}

// reviewStartPreflightOptions are the START options a negotiated STATUS
// validated and renders into its fresh review.start vector. The zero value
// renders nothing, so a STATUS without them is unchanged.
type reviewStartPreflightOptions struct {
	// RequestContextPath is absolute, so the rendered START reads the same
	// file whatever process cwd runs it.
	RequestContextPath string
	Escalation         *reviewtransaction.CompactAgentEscalation
	// LensSelection is the agent's --lenses choice (verify-always-rdd-high S8),
	// rendered in canonical lens names.
	LensSelection *reviewLensSelection
}

func (options reviewStartPreflightOptions) declared() bool {
	return options.RequestContextPath != "" || options.Escalation != nil || options.LensSelection != nil
}

// arguments renders the options after every other START argument, in the
// order START documents them.
func (options reviewStartPreflightOptions) arguments() []ReviewTransitionArgument {
	var arguments []ReviewTransitionArgument
	if options.RequestContextPath != "" {
		arguments = append(arguments, ReviewTransitionArgument{Name: "request-context", Value: options.RequestContextPath})
	}
	if options.Escalation != nil {
		arguments = append(arguments,
			ReviewTransitionArgument{Name: "escalate-item", Value: strconv.Itoa(options.Escalation.Item)},
			ReviewTransitionArgument{Name: "escalate-reason", Value: options.Escalation.Reason})
	}
	if options.LensSelection != nil {
		arguments = append(arguments,
			ReviewTransitionArgument{Name: "lenses", Value: strings.Join(options.LensSelection.Lenses, ",")},
			ReviewTransitionArgument{Name: "lenses-reason", Value: options.LensSelection.Reason})
	}
	return arguments
}

// reviewStatusStartOptionsDeclared reports whether a STATUS invocation names
// any START option, including an empty or repeated one.
func reviewStatusStartOptionsDeclared(args []string) bool {
	for _, count := range reviewStatusStartOptionCounts(args) {
		if count != 0 {
			return true
		}
	}
	return false
}

func reviewStatusStartOptionCounts(args []string) map[string]int {
	counts := make(map[string]int, len(reviewStartPreflightOptionNames))
	shape := reviewIntegrationOperationFlagShape("review.status")
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" || argument == "" || argument == "-" || argument[0] != '-' {
			break
		}
		nameValue := strings.TrimPrefix(strings.TrimPrefix(argument, "-"), "-")
		name, hasValue := nameValue, false
		if separator := strings.IndexByte(nameValue, '='); separator >= 0 {
			name, hasValue = nameValue[:separator], true
		}
		kind, known := shape[name]
		if !known {
			continue
		}
		for _, option := range reviewStartPreflightOptionNames {
			if name == option {
				counts[name]++
			}
		}
		if kind != reviewIntegrationBoolFlag && !hasValue {
			index++
		}
	}
	return counts
}

// parseReviewStatusStartOptions validates the START options exactly as START
// does, without reading the repository or creating authority. They only make
// sense for the fresh START a --next-transition STATUS renders.
func parseReviewStatusStartOptions(args []string, nextTransition bool, contract, requestContextSource, item, reason, lenses, lensesReason string) (reviewStartPreflightOptions, error) {
	counts := reviewStatusStartOptionCounts(args)
	if !reviewStatusStartOptionsDeclared(args) {
		return reviewStartPreflightOptions{}, nil
	}
	if !nextTransition {
		return reviewStartPreflightOptions{}, fmt.Errorf("review status --request-context, --escalate-item, --escalate-reason, --lenses, and --lenses-reason preflight the fresh START a next transition renders, so they require --next-transition; rerun `gentle-ai review status --contract %s --next-transition` with them", contract)
	}
	for _, name := range reviewStartPreflightOptionNames {
		if counts[name] > 1 {
			return reviewStartPreflightOptions{}, fmt.Errorf("review status repeats --%s", name) // refusal:by-design operator-knowledge: only the caller knows the single value START should freeze
		}
	}
	var options reviewStartPreflightOptions
	if counts["request-context"] != 0 {
		path := strings.TrimSpace(requestContextSource)
		if path == "" {
			return reviewStartPreflightOptions{}, errors.New("review status --request-context requires the path of a request file") // refusal:by-design operator-knowledge: only the caller can name the request file the candidate was built for
		}
		if _, err := reviewRequestContextContent(path); err != nil {
			return reviewStartPreflightOptions{}, fmt.Errorf("review status --request-context names a file START cannot freeze: %w", err)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return reviewStartPreflightOptions{}, fmt.Errorf("resolve review status --request-context: %w", err)
		}
		options.RequestContextPath = absolute
	}
	escalation, fault := reviewEscalationFromFlags(args, item, reason)
	switch fault {
	case reviewEscalationUnpaired:
		return reviewStartPreflightOptions{}, fmt.Errorf("review status --escalate-item and --escalate-reason must be passed together; rerun `gentle-ai review status --contract %s --next-transition --escalate-item <1-6> --escalate-reason <text>`", contract)
	case reviewEscalationBadItem:
		return reviewStartPreflightOptions{}, fmt.Errorf("review status --escalate-item %q must be an integer from 1 to 6 naming a high-risk item; rerun `gentle-ai review status --contract %s --next-transition --escalate-item <1-6> --escalate-reason <text>`", item, contract)
	case reviewEscalationBadReason:
		return reviewStartPreflightOptions{}, fmt.Errorf("review status --escalate-reason must be non-empty and at most %d characters; rerun `gentle-ai review status --contract %s --next-transition --escalate-item <1-6> --escalate-reason <text>` with a one-line reason", reviewtransaction.AgentEscalationReasonMax, contract)
	}
	options.Escalation = escalation
	selection, err := parseReviewStartLensSelection(args, lenses, lensesReason)
	if err != nil {
		// Same validation as START, worded for the STATUS that preflights it.
		message := strings.Replace(err.Error(), "review start", "review status", 1)
		message = strings.Replace(message, "`gentle-ai review start --lenses", "`gentle-ai review status --contract "+contract+" --next-transition --lenses", 1)
		return reviewStartPreflightOptions{}, errors.New(message)
	}
	options.LensSelection = selection
	return options, nil
}

// reviewStatusStartOptionsUnapplied refuses START options when the next
// transition is not the fresh START they preflight: an existing lineage
// already froze its own options, and every other route runs no START. Ignoring
// them with success would report options nothing applies.
func reviewStatusStartOptionsUnapplied(options reviewStartPreflightOptions, transition *ReviewNextTransition, contract string) error {
	if !options.declared() {
		return nil
	}
	if transition != nil && transition.Kind == reviewNextTransitionExecute && transition.Execute != nil &&
		transition.Execute.Operation == "review.start" && transition.ReasonCode == "fresh_target_ready" {
		return nil
	}
	kind, reason := "none", "none"
	if transition != nil {
		kind, reason = transition.Kind, transition.ReasonCode
	}
	return fmt.Errorf("review status --request-context, --escalate-item, and --escalate-reason only preflight a fresh START, but the next transition is %s %q; rerun `gentle-ai review status --contract %s --next-transition` without them and follow its transition, then pass them again to the STATUS that offers review.start", kind, reason, contract)
}
