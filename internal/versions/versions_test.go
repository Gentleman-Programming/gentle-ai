package versions

import "testing"

func TestPinnedVersionsAreDefined(t *testing.T) {
	for name, val := range map[string]string{
		"OpenCode":    OpenCode,
		"Context7MCP": Context7MCP,
		"GGAVersion":  GGAVersion,
	} {
		if val == "" {
			t.Errorf("%s must not be empty", name)
		}
	}
}
