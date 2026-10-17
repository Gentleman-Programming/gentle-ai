package shellinstaller

import "strings"

// This is bounded, inert selection data for GGA v2.10.1, not a provider
// installation, authentication or review-health check. Never infer Claude
// from the selected Pi runtime. Project hooks require separate consent.
func userValidateGGASelection(choice, provider string) error {
	if choice != "" && choice != "on" && choice != "off" {
		return experienceError("choose on or off for optional GGA, or omit it")
	}
	if choice != "on" {
		if provider != "" {
			return experienceError("a GGA provider requires explicit GGA ON")
		}
		return nil
	}
	if provider == "" || len(provider) > 64 || strings.Trim(provider, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./:-") != "" {
		return experienceError("GGA ON requires an explicit bounded documented provider")
	}
	base, model, inline := strings.Cut(provider, ":")
	if inline && model == "" {
		return experienceError("GGA inline model must not be empty")
	}
	switch base {
	case "claude", "gemini", "codex", "opencode", "cursor", "kilo", "lmstudio", "minimax":
	case "kiro":
		if inline {
			return experienceError("GGA Kiro configures its model through kiro-cli settings")
		}
	case "ollama", "github":
		if !inline {
			return experienceError("GGA Ollama/GitHub requires an explicit model")
		}
	default:
		return experienceError("GGA requires a documented provider; Pi has no GGA provider binding")
	}
	return nil
}
