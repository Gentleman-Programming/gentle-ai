package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ReleaseManifest holds the parsed, validated journey ID list.
type ReleaseManifest struct {
	Journeys []string `yaml:"journeys"`
}

// LoadReleaseManifest reads the manifest file, validates it is non-empty,
// sorted, and de-duplicated, then returns the parsed manifest.
func LoadReleaseManifest(path string) (ReleaseManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ReleaseManifest{}, fmt.Errorf("read manifest %q: %w", path, err)
	}

	var m ReleaseManifest
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return ReleaseManifest{}, fmt.Errorf("parse manifest %q: %w", path, err)
	}

	if len(m.Journeys) == 0 {
		return ReleaseManifest{}, fmt.Errorf("manifest %q is empty: at least one journey ID is required", path)
	}

	// Check for duplicates using a map.
	seen := make(map[string]bool)
	for _, id := range m.Journeys {
		if seen[id] {
			return ReleaseManifest{}, fmt.Errorf("manifest %q contains duplicate ID %q", path, id)
		}
		seen[id] = true
	}

	// Check sorted.
	for i := 1; i < len(m.Journeys); i++ {
		if m.Journeys[i-1] >= m.Journeys[i] {
			return ReleaseManifest{}, fmt.Errorf("manifest %q is not sorted: %q >= %q",
				path, m.Journeys[i-1], m.Journeys[i])
		}
	}

	return m, nil
}

// SortedJourneys returns a sorted, de-duplicated copy of ids.
func SortedJourneys(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
