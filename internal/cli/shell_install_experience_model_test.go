package cli

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestShellInstallEmbeddedExperienceHandoff(t *testing.T) {
	profile := shellinstaller.UserExperience{Persona: "neutral", BackgroundSubagents: "off", StrictTDD: "on", GlobalRDD: "on", CodeGraph: "on", Context7: "on", GGA: "on", GGAProvider: "codex"}
	want := profile
	model := NewShellInstallModelWithExperience(nil, profile)
	profile.Persona = "gentleman"
	if model.Init() != nil {
		t.Fatal("configure handoff starts a worker")
	}
	state, ok := ShellInstallPresentationOf(model)
	if !ok || state.Review || state.Request.Experience == nil || *state.Request.Experience != want {
		t.Fatalf("destination lost the complete selection: %+v", state)
	}
	state.Request.Experience.CodeGraph = "off"
	selection, err := ShellInstallOutcome(model)
	if err != nil || selection.Confirmed || *selection.Request.Experience != want {
		t.Fatalf("presentation changed the selected profile: %+v, %v", selection, err)
	}
	selection.Request.Experience.Persona = "unmanaged"
	again, err := ShellInstallOutcome(model)
	if err != nil || !reflect.DeepEqual(again.Request.Experience, &want) {
		t.Fatal("outcome readback mutates the selection")
	}
	if args := ShellInstallArguments(again); len(args) != 0 {
		t.Fatal("unconfirmed experience creates worker arguments")
	}
}

func TestShellInstallEmbeddedExperienceRefusalAndCancel(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		t.Run(tea.KeyMsg{Type: key}.String(), func(t *testing.T) {
			profile := shellinstaller.DefaultUserExperience()
			profile.CodeGraph = "on"
			cancelled := false
			model := NewShellInstallModelWithExperience(func() { cancelled = true }, profile)
			model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(t.TempDir() + "/target")})
			if cmd != nil {
				t.Fatal("typing destination starts a worker")
			}
			model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			state, ok := ShellInstallPresentationOf(model)
			if !ok || state.Error == nil || state.Review || cmd != nil {
				t.Fatal("unsupported effects admitted or dropped during inspection")
			}
			model, cmd = model.Update(tea.KeyMsg{Type: key})
			if !cancelled || cmd == nil {
				t.Fatal("profile selector cannot cancel")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("cancel starts a worker")
			}
			selection, _ := ShellInstallOutcome(model)
			if selection.Confirmed || len(ShellInstallArguments(selection)) != 0 {
				t.Fatal("cancelled profile executes")
			}
		})
	}
}

func TestShellInstallEmbeddedPresentation(t *testing.T) {
	if _, ok := ShellInstallPresentationOf(nil); ok {
		t.Fatal("unrelated model accepted")
	}
	model := NewShellInstallModel(nil).(shellInstallModel)
	model.review, model.field = true, 1
	model.req.Confirmation = "model-only-review-token"
	state, ok := ShellInstallPresentationOf(model)
	if !ok || !state.Review || state.Field != 1 || state.Request != model.req {
		t.Fatalf("legacy presentation changed: %+v", state)
	}
	bad := shellinstaller.DefaultUserExperience()
	bad.CodeGraph = "invalid"
	state, ok = ShellInstallPresentationOf(NewShellInstallModelWithExperience(nil, bad))
	if !ok || state.Error == nil || state.Review {
		t.Fatal("invalid configuration has no visible refusal")
	}
}
