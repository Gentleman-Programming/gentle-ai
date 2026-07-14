package kimi

import (
	"os"
)

// StatResult mirrors the unexported statResult for use in test constructors.
type StatResult struct {
	IsDir bool
	Err   error
}

// TestAdapterOption configures a test Adapter.
type TestAdapterOption func(*Adapter)

// WithStatPath sets a custom statPath function for test adapters.
func WithStatPath(fn func(string) StatResult) TestAdapterOption {
	return func(a *Adapter) {
		a.statPath = func(path string) statResult {
			r := fn(path)
			return statResult{isDir: r.IsDir, err: r.Err}
		}
	}
}

// WithPathExists sets a custom pathExists function for test adapters.
func WithPathExists(fn func(string) bool) TestAdapterOption {
	return func(a *Adapter) {
		a.pathExists = fn
	}
}

// NewTestAdapter creates an Adapter configured for testing with the given options.
// By default it uses the real lookPath and userHomeDir.
func NewTestAdapter(opts ...TestAdapterOption) *Adapter {
	a := &Adapter{
		lookPath:    LookPathOverride,
		statPath:    defaultStat,
		pathExists:  defaultPathExists,
		userHomeDir: os.UserHomeDir,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}
