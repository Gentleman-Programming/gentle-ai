package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// RunSDDAttempt retains only the existing grant verb for explicit edit consent.
// Runtime attempt admission, settlement and budgets are retired.
func RunSDDAttempt(args []string, stdout io.Writer) error {
	return runSDDAttempt(context.Background(), args, stdout)
}

func runSDDAttempt(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("sdd-attempt requires grant; run `gentle-ai sdd-attempt grant --help` for edit-authority inputs")
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(stdout, "Usage: gentle-ai sdd-attempt grant [flags]\nRecord explicit per-change edit authority. Runtime attempt operations are retired.")
		return err
	}
	if args[0] != "grant" {
		return fmt.Errorf("unknown sdd-attempt operation %q; only grant remains; use `gentle-ai sdd-status --cwd <repo> --json` for SDD progress", args[0])
	}
	flags := flag.NewFlagSet("sdd-attempt grant", flag.ContinueOnError)
	flags.SetOutput(stdout)
	flags.Usage = func() { fmt.Fprintln(stdout, "Usage: gentle-ai sdd-attempt grant [flags]"); flags.PrintDefaults() }
	cwd := flags.String("cwd", "", "required; repository working directory")
	change := flags.String("change", "", "required; SDD change identifier")
	expected := flags.String("expected-revision", "", "empty initially, otherwise exact current grant-chain revision")
	instance := flags.String("change-instance", "", "required; current change-instance identity")
	requestID := flags.String("request-id", "", "required; unique idempotency key")
	actor := flags.String("actor", "", "required; authorizing actor")
	reason := flags.String("reason", "", "required; scope authorization reason")
	var roots sddAttemptRootList
	flags.Var(&roots, "root", "required and repeatable; authorized edit root")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected sdd-attempt argument %q; use `gentle-ai sdd-attempt grant --help`", flags.Arg(0))
	}
	missing := []string{}
	for _, field := range []struct{ name, value string }{{"cwd", *cwd}, {"change", *change}, {"change-instance", *instance}, {"request-id", *requestID}, {"actor", *actor}, {"reason", *reason}} {
		if strings.TrimSpace(field.value) == "" {
			missing = append(missing, "--"+field.name)
		}
	}
	if len(roots) == 0 {
		missing = append(missing, "--root")
	}
	if len(missing) > 0 {
		return fmt.Errorf("sdd-attempt grant requires %s; rerun `gentle-ai sdd-attempt grant` with those missing flags", strings.Join(missing, ", "))
	}
	store, err := sddstatus.OpenRuntimeStore(ctx, *cwd, *change)
	if err != nil {
		return fmt.Errorf("open SDD edit authority: %w", err)
	}
	store, err = store.ForCurrentChangeInstance(strings.TrimSpace(*instance))
	if err != nil {
		return fmt.Errorf("sdd-attempt grant: %w", err)
	}
	result, err := store.Grant(ctx, sddstatus.GrantRootsRequest{ExpectedRevision: strings.TrimSpace(*expected), RequestID: *requestID, Roots: roots, Reason: *reason, Actor: *actor})
	if err != nil {
		return fmt.Errorf("sdd-attempt grant: %w", err)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

type sddAttemptRootList []string

func (roots *sddAttemptRootList) String() string         { return strings.Join(*roots, ", ") }
func (roots *sddAttemptRootList) Set(value string) error { *roots = append(*roots, value); return nil }
