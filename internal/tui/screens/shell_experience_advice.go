package screens

// Advice is display-only. The current value always comes from the entry's
// UserExperience projection, never from this recommendation or keyboard focus.
func shellExperienceRecommendation(entry ShellExperienceEntry) (string, string) {
	switch entry.Control {
	case ShellExperiencePersona:
		switch entry.Choice {
		case "neutral":
			return "Gentleman", "Use Neutral when you prefer neutral conversational wording."
		case "unmanaged":
			return "Gentleman", "Use Custom when you already manage your own persona."
		default:
			return "Gentleman", "It provides the complete Gentle-Shell ODD experience."
		}
	case ShellExperienceBackground:
		return "ON", "Run independent work without blocking your terminal."
	case ShellExperienceTDD:
		return "ON", "Prove behavior with a failing test before implementation."
	case ShellExperienceRDD:
		return "ON", "Independent review preserves evidence; it applies globally and costs extra reviewer-agent tokens."
	case ShellExperienceCodeGraph:
		return "ON", "Supported on Linux amd64 and Windows x64; the runtime stays private to this install. Project overrides remain."
	case ShellExperienceContext7:
		return "ON", "Use current library references through MCP."
	case ShellExperienceInstallTarget:
		if entry.Choice == "existing" {
			return "Configure existing", "Prefills this install's selections; continuing replaces only its target and keeps the previous tree."
		}
		if entry.Choice == "agent" {
			return "On my existing Pi", "Reuses the detected agent's settings and MCP with a private prefix; every selection is staged privately and the Pi prefix is never written."
		}
		if entry.Choice == "shared" {
			return "Separate install", "Reuse a detected Pi prefix only when you want the defaults applied over it; Main is available with On my existing Pi or Separate, not the retained Shared prefix."
		}
		return "Separate install", "Keeps your personal Pi untouched and installs its own prefix, agent and launchers."
	case ShellExperienceUpdate:
		return "Check", "Looks only at this installation's installed channel. Stable and Main are never mixed."
	case ShellExperienceUninstall:
		return "Uninstall", "Moves this managed installation to a timestamped backup and removes it from detection. Your personal Pi is untouched."
	case ShellExperienceSDK:
		if entry.Choice == "main-preview" {
			return "Stable", "Choose Main for On my existing Pi or a Separate install; provider and native pins stay stock. Linux amd64 is qualified today; Windows and macOS support is being completed."
		}
		return "Stable", "Keep the stock SDK source, provider and native pins."
	default:
		return "", ""
	}
}
