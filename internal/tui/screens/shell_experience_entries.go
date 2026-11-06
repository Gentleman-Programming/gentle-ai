package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// ShellExperienceControl identifies an existing preference or navigation action,
// independently of the restored radio rows and their visual cursor positions.
type ShellExperienceControl int

const (
	ShellExperienceFoundation ShellExperienceControl = -1
	ShellExperiencePersona    ShellExperienceControl = iota - 1
	ShellExperienceBackground
	ShellExperienceTDD
	ShellExperienceRDD
	ShellExperienceCodeGraph
	ShellExperienceContext7
	ShellExperienceSDK
	ShellExperienceContinue
	ShellExperienceCancel
	ShellExperienceInstallTarget
	ShellExperienceUninstall
	ShellExperienceUpdate
)

// ShellExperienceEntry is an inert projection, never a second preference store.
type ShellExperienceEntry struct {
	Category, Kind, Label, Description, Value, Choice string
	Control                                           ShellExperienceControl
	Cursor                                            int
	Current, Checked, Selectable, Locked              bool
	SeparatorBefore                                   bool
}

func shellExperienceCategories() []string {
	return []string{"Required foundation", "Persona", "Recommended", "SDK source", "Install target", "Actions"}
}

// shellExperienceEntries restores the original grouping using only preferences
// carried by UserExperience. Retired SDD and whole-Shell channel rows stay absent.
// ShellInstallTargetEntries projects the install-target choice. It is not part
// of UserExperience, so the transport DTO stays unchanged; the caller owns the
// selection and the detected Pi data.
func ShellInstallTargetEntries(mode string, piFound bool, piNote string, agent string, existing []shellinstaller.UserShellInstall, locked bool) []ShellExperienceEntry {
	_ = existing // Existing-install configuration is not a target choice.
	agentDescription := "Install privately but reuse " + agent + " for settings and MCP; its prefix (for example Homebrew) is never written. Pi's own fd/rg in its bin are set aside inside the installation and restored on uninstall."
	if agent == "" {
		agentDescription = "No Pi agent was detected; choose Separate install."
		if piNote != "" && piNote != "missing" && piNote != "ambiguous" {
			agentDescription = piNote
		}
	}
	return []ShellExperienceEntry{
		{Control: ShellExperienceInstallTarget, Category: "Install target", Kind: "radio", Label: "On my existing Pi", Description: agentDescription, Choice: "agent", Value: "On my existing Pi", Current: mode == "agent" || mode == "shared", Locked: locked},
		{Control: ShellExperienceInstallTarget, Category: "Install target", Kind: "radio", Label: "Separate install (Gentle-Shell)", Description: "Own target: private prefix, agent, runtime and launchers; your personal Pi is untouched.", Choice: "separate", Value: "Separate install (Gentle-Shell)", Current: mode == "separate", Locked: locked},
	}
}

func shellExperienceEntries(p shellinstaller.UserExperience, uninstall bool, continueLabel string, targets ...ShellExperienceEntry) []ShellExperienceEntry {
	var entries []ShellExperienceEntry
	cursor := 0
	add := func(e ShellExperienceEntry) {
		e.Cursor = -1
		if e.Control != ShellExperienceFoundation && !e.Locked {
			e.Cursor, e.Selectable = cursor, true
			cursor++
		}
		entries = append(entries, e)
	}
	for _, item := range [][2]string{
		{"Gentle-Shell", "The managed Gentle-Shell foundation provides ODD + core skills."},
		{"Persistent memory + MCP", "Persistent memory and MCP support are included."},
		{"ODD + core skills", "Organic Driven Development and the core skill set are included."},
		{"Subagent/interaction stack", "Subagent delegation and human interaction support are included."},
		{"Security-first Permissions", "Security-first permission guardrails are included."},
	} {
		add(ShellExperienceEntry{Control: ShellExperienceFoundation, Category: "Required foundation", Kind: "foundation", Label: item[0], Description: item[1], Value: "required"})
	}
	for _, choice := range [][3]string{
		{"gentleman", "Gentleman", "Natural voseo in Spanish + English technical artifacts"},
		{"neutral", "Neutral", "Neutral conversational tone + English technical artifacts"},
		{"unmanaged", "Custom", "Keep your own persona; Gentle AI won't manage it."},
	} {
		add(ShellExperienceEntry{Control: ShellExperiencePersona, Category: "Persona", Kind: "persona", Label: choice[1], Description: choice[2], Choice: choice[0], Value: p.Persona, Current: p.Persona == choice[0]})
	}
	addToggle := func(control ShellExperienceControl, category, label, description, value string) {
		caption := strings.ToUpper(value)
		if value == "" {
			caption = "Inherited"
		}
		add(ShellExperienceEntry{Control: control, Category: category, Kind: "toggle", Label: label, Description: description, Value: caption, Checked: value == "on"})
	}
	addToggle(ShellExperienceBackground, "Recommended", "Background subagents", "Independent work without blocking terminal", p.BackgroundSubagents)
	addToggle(ShellExperienceTDD, "Recommended", "Strict TDD", "Prove behavior with a failing test before implementation", p.StrictTDD)
	addToggle(ShellExperienceRDD, "Recommended", "Global RDD", "Independent reviewers preserve evidence; extra reviewer-agent tokens; applies globally", p.GlobalRDD)
	addToggle(ShellExperienceCodeGraph, "Recommended", "CodeGraph", "Structural indexing/navigation; project overrides and explicit qualification remain.", p.CodeGraph)
	addToggle(ShellExperienceContext7, "Recommended", "Context7", "Current library docs through MCP", p.Context7)
	for _, choice := range [][3]string{
		{"stable", "Stable", "Stock SDK source; native installer and pins remain unchanged. Backspace inherits the stock default."},
		{"main-preview", "Main preview", "SDK Main + stock provider. Works with On my existing Pi or Separate installs; native installer and pins remain stock. Linux amd64 is qualified today; Windows and macOS support is being completed."},
	} {
		add(ShellExperienceEntry{Control: ShellExperienceSDK, Category: "SDK source", Kind: "radio", Label: choice[1], Description: choice[2], Choice: choice[0], Value: shellSDKSourceLabel(&p), Current: p.SDKSource == choice[0] || p.SDKSource == "" && choice[0] == "stable"})
	}
	for _, target := range targets {
		add(target)
	}
	if uninstall {
		add(ShellExperienceEntry{Control: ShellExperienceUpdate, Category: "Actions", Kind: "action", Label: "Check for updates", Description: "Checks this installation's Stable or Main channel for component updates."})
	}
	if continueLabel == "" {
		continueLabel = "Continue"
	}
	continueDescription := "Continue with this Gentle-Shell profile. Installation still requires physical confirmation."
	if continueLabel != "Continue" {
		continueDescription = "Review and apply the selected changes to this installation."
	}
	add(ShellExperienceEntry{Control: ShellExperienceContinue, Category: "Actions", Kind: "action", Label: continueLabel, Description: continueDescription})
	add(ShellExperienceEntry{Control: ShellExperienceCancel, Category: "Actions", Kind: "action", Label: "Cancel", Description: "Cancel configuration without applying changes."})
	if uninstall {
		add(ShellExperienceEntry{Control: ShellExperienceUninstall, Category: "Actions", Kind: "action", Label: "Uninstall this installation", Description: "Moves this managed installation to a timestamped backup and removes it from detection. Your personal Pi is untouched.", SeparatorBefore: true})
	}
	return entries
}

func ShellExperienceOptionsWithUninstall(p shellinstaller.UserExperience, uninstall bool, targets ...ShellExperienceEntry) []ShellExperienceEntry {
	return ShellExperienceOptionsWithUninstallAction(p, uninstall, "Continue", targets...)
}

func ShellExperienceOptionsWithUninstallAction(p shellinstaller.UserExperience, uninstall bool, continueLabel string, targets ...ShellExperienceEntry) []ShellExperienceEntry {
	var options []ShellExperienceEntry
	for _, entry := range shellExperienceEntries(p, uninstall, continueLabel, targets...) {
		if entry.Selectable {
			options = append(options, entry)
		}
	}
	return options
}
