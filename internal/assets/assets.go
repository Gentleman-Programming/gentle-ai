package assets

import "embed"

//go:embed all:claude all:opencode all:generic all:skills all:gga all:gemini all:codex all:antigravity all:windsurf all:cursor all:kimi all:qwen all:kiro all:hermes all:engram
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

// SharedSkillFileNames returns the list of all file basenames in skills/_shared/ sorted alphabetically.
func SharedSkillFileNames() ([]string, error) {
	entries, err := FS.ReadDir("skills/_shared")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}
