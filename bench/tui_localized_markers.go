package main

import "strings"

// The fork localized the TUI to Spanish (INC-16) while upstream ships English.
// These journeys drive the real screens through a PTY and wait for on-screen
// text, so a single-language literal makes them wait forever and die with
// "read /dev/ptmx: input/output error" when the process exits.
//
// internal/tui/model_test.go already matches both spellings; the benchmark
// corpus was never updated with it. Accepting both keeps these journeys honest
// against either language without asserting which one ships.
// Each entry pairs the English marker the corpus asks for with the spelling
// the fork actually renders. Only screens the fork localized appear here; a
// marker whose English text still ships is matched literally and must not be
// added, or the map would claim a translation that does not exist.
var localizedTUIMarkers = map[string][]string{
	"Start installation":      {"Start installation", "Iniciar instalación"},
	"q: quit":                 {"q: quit", "q: salir"},
	"Choose your Persona":     {"Choose your Persona", "Elige tu Persona"},
	"Detected Configs":        {"Detected Configs", "Configuraciones Detectadas"},
	"Select Ecosystem Preset": {"Select Ecosystem Preset", "Seleccionar Preset del Ecosistema"},
	"Continue":                {"Continue", "Continuar"},
}

// screenShows reports whether the captured screen contains marker in any
// language the product may render it in. An unknown marker is matched
// literally, so callers can pass screen text that was never localized.
func screenShows(screen, marker string) bool {
	for _, spelling := range localizedTUIMarkers[marker] {
		if strings.Contains(screen, spelling) {
			return true
		}
	}
	if _, localized := localizedTUIMarkers[marker]; localized {
		return false
	}
	return strings.Contains(screen, marker)
}
