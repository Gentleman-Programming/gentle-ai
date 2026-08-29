package cli

import (
	"bytes"
	"strings"
	"testing"
)

// #3818: every runtime except OpenCode received the phase result contract as
// prose with nothing enforcing it. This command is the enforcement surface they
// were missing, and it renders the same typed terminal failure the OpenCode
// transport already emits.

func runTaskResult(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	err := runSDDTaskResult(args, strings.NewReader(stdin), &stdout)
	return stdout.String(), err
}

func TestSDDTaskResultAdmitsAUsableResult(t *testing.T) {
	out, err := runTaskResult(t, "a real phase result", "--phase", "sdd-apply", "--cwd", "/repo", "--input", "-")
	if err != nil {
		t.Fatalf("admitted result returned error: %v", err)
	}
	if !strings.Contains(out, `"status":"ok"`) {
		t.Errorf("stdout = %q, want an ok status", out)
	}
}

func TestSDDTaskResultRefusesAnEmptyResultWithTheTypedHandoff(t *testing.T) {
	out, err := runTaskResult(t, "   ", "--phase", "sdd-apply", "--cwd", "/repo", "--input", "-")
	if err == nil {
		t.Fatal("empty result was admitted")
	}
	if !strings.HasPrefix(err.Error(), "GENTLE_AI_SDD_FAILURE ") {
		t.Errorf("error is not the typed handoff: %q", err.Error())
	}
	if !strings.Contains(err.Error(), `"code":"sdd_task_result_empty"`) {
		t.Errorf("handoff carries the wrong code: %q", err.Error())
	}
	if out != "" {
		t.Errorf("a refused result wrote to stdout: %q", out)
	}
}

func TestSDDTaskResultRefusesANestedEnvelope(t *testing.T) {
	nested := "<task id=\"phase\" state=\"completed\">\n<task_result>\n<task id=\"nested\" state=\"completed\">\nresult\n</task>\n</task_result>\n</task>"
	_, err := runTaskResult(t, nested, "--phase", "sdd-verify", "--cwd", "/repo", "--input", "-")
	if err == nil || !strings.Contains(err.Error(), `"code":"sdd_task_result_malformed"`) {
		t.Fatalf("nested envelope was not refused as malformed: %v", err)
	}
}

func TestSDDTaskResultRendersTheDispatchLatch(t *testing.T) {
	_, err := runTaskResult(t, "anything", "--phase", "sdd-verify", "--cwd", "/repo", "--input", "-",
		"--latched-phase", "sdd-apply", "--latched-code", "sdd_task_result_empty")
	if err == nil {
		t.Fatal("a latched session dispatched anyway")
	}
	for _, want := range []string{`"code":"sdd_task_dispatch_latched"`, `"latchedPhase":"sdd-apply"`, `"phase":"sdd-verify"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("latched handoff missing %s: %q", want, err.Error())
		}
	}
}

func TestSDDTaskResultRequiresPhaseAndCwd(t *testing.T) {
	for _, args := range [][]string{
		{"--cwd", "/repo", "--input", "-"},
		{"--phase", "sdd-apply", "--input", "-"},
	} {
		if _, err := runTaskResult(t, "result", args...); err == nil {
			t.Errorf("missing required flag was accepted: %v", args)
		}
	}
}
