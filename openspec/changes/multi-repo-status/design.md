# SDD Design: Multi-Repo Status & Registry Parsing

## Structs

```go
package repository

type Repository struct {
    GitlabPath string `json:"gitlabPath"`
    Slug       string `json:"slug"`
    Owner      string `json:"owner"`
    Type       string `json:"type"`
    Purpose    string `json:"purpose"`
    Profile    string `json:"profile"`
}

// In internal/sddstatus/status.go:
type Status struct {
    // ... existing fields ...
    TargetRepositories []repository.Repository `json:"targetRepositories,omitempty"`
}
```

## Parsing Strategy
We will use sequential line processing over `repository-registry.md`, skipping lines until we find `| Repository (gitlab_path) |`, bypassing the separator line, and then using `strings.Split(line, "|")` to map the 6 columns. `strings.TrimSpace` will clean up any backticks or padding.
