package assets

import "embed"

//go:embed all:claude all:opencode all:generic all:skills all:gga all:gemini all:codex all:antigravity all:windsurf all:cursor
var FS embed.FS

// MustRead returns the content of an embedded file or panics.
func MustRead(path string) string {
	data, err := FS.ReadFile(path)
	if err != nil {
		panic("assets: " + err.Error())
	}
	return string(data)
}

// Read returns the content of an embedded file.
func Read(path string) (string, error) {
	data, err := FS.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
