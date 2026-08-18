package repository

import (
	"bufio"
	"os"
	"strings"
)

// Repository represents a row in docs/repository-registry.md
type Repository struct {
	GitlabPath string `json:"gitlabPath"`
	Slug       string `json:"slug"`
	Owner      string `json:"owner"`
	Type       string `json:"type"`
	Purpose    string `json:"purpose"`
	Profile    string `json:"profile"`
}

// ParseRegistry reads a markdown table mapping ecosystem repositories.
// It skips headers and irrelevant lines.
func ParseRegistry(path string) (map[string]Repository, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist, return empty map rather than failing.
			return map[string]Repository{}, nil
		}
		return nil, err
	}
	defer file.Close()

	registry := make(map[string]Repository)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "|") || strings.Contains(line, "|---|") || strings.Contains(line, "| Repository (gitlab_path) |") {
			continue
		}

		parts := strings.Split(line, "|")
		// parts will have length 8: empty, gitlabPath, slug, owner, type, purpose, profile, empty
		if len(parts) >= 7 {
			gitlabPath := cleanMarkdown(parts[1])
			slug := cleanMarkdown(parts[2])
			if gitlabPath == "" || slug == "" {
				continue
			}
			registry[slug] = Repository{
				GitlabPath: gitlabPath,
				Slug:       slug,
				Owner:      cleanMarkdown(parts[3]),
				Type:       cleanMarkdown(parts[4]),
				Purpose:    cleanMarkdown(parts[5]),
				Profile:    cleanMarkdown(parts[6]),
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return registry, nil
}

func cleanMarkdown(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`")
	return s
}
