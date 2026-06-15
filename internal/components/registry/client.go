// Package registry provides a YAML-based client for the community
// profile registry. It supports listing, installing, and searching
// profiles from a remote registry server.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// RegistryEntry is a single profile in the registry index.
type RegistryEntry struct {
	ID          string   `json:"id"          yaml:"id"`
	Name        string   `json:"name"        yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Author      string   `json:"author"      yaml:"author"`
	Version     string   `json:"version"     yaml:"version"`
	Tags        []string `json:"tags"        yaml:"tags"`
	Quality     float64  `json:"quality_score" yaml:"quality_score"`
	URL         string   `json:"url"         yaml:"url"`
}

// RegistryClient interacts with the community profile registry.
// It fetches a JSON index of available profiles and can download
// individual profiles by ID.
type RegistryClient struct {
	BaseURL    string
	CacheDir   string
	HTTPClient *http.Client
}

// NewRegistryClient returns a client targeting the given registry URL.
func NewRegistryClient(baseURL, cacheDir string) *RegistryClient {
	return &RegistryClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		CacheDir:   cacheDir,
		HTTPClient: &http.Client{},
	}
}

// List fetches the registry index and returns all available profiles.
func (c *RegistryClient) List(ctx context.Context) ([]RegistryEntry, error) {
	url := c.BaseURL + "/profiles.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registry request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var entries []RegistryEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse registry index: %w", err)
	}

	return entries, nil
}

// Install downloads and installs a profile by ID. It first fetches
// the registry index to find the profile's download URL, then
// downloads and parses the profile JSON.
func (c *RegistryClient) Install(ctx context.Context, id string) (*model.RoleProfile, error) {
	entries, err := c.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry: %w", err)
	}

	var target RegistryEntry
	found := false
	for _, e := range entries {
		if e.ID == id {
			target = e
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("profile %q not found in registry", id)
	}

	return c.downloadProfile(ctx, target.URL)
}

// Search filters the registry by role and/or tags. An empty role
// matches all entries. An empty tags slice matches all entries.
func (c *RegistryClient) Search(ctx context.Context, role model.RoleEnum, tags []string) ([]RegistryEntry, error) {
	entries, err := c.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry: %w", err)
	}

	var results []RegistryEntry
	for _, e := range entries {
		if !matchTags(e.Tags, tags) {
			continue
		}
		results = append(results, e)
	}

	return results, nil
}

// downloadProfile fetches a profile JSON from the given URL path
// (relative to BaseURL) and parses it into a RoleProfile.
func (c *RegistryClient) downloadProfile(ctx context.Context, path string) (*model.RoleProfile, error) {
	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile download returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile: %w", err)
	}

	var p model.RoleProfile
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("failed to parse profile: %w", err)
	}

	return &p, nil
}

// matchTags reports whether all required tags are present in available.
// If required is empty, it matches everything.
func matchTags(available, required []string) bool {
	if len(required) == 0 {
		return true
	}

	availSet := make(map[string]bool, len(available))
	for _, t := range available {
		availSet[t] = true
	}

	for _, t := range required {
		if !availSet[t] {
			return false
		}
	}
	return true
}
