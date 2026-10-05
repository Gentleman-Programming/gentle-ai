// Package reviewerprovider defines the opaque boundary between Go-owned review
// authority and host-specific reviewer invocation.
package reviewerprovider

import "context"

// Invocation is fully materialized by the Go-owned provider. Adapters may
// deliver its opaque prompt bytes, then return the reviewer's raw output.
type Invocation struct {
	prompt []byte
	probe  ProbeWorkspace
}

// ProbeWorkspace writes the frozen candidate tree into dir, an empty directory
// an adapter created under the system temp dir. Only an adapter that can
// confine a reproducing command to that copy with no network uses it (S11);
// the adapter owns the directory and removes it after the reviewer returns.
type ProbeWorkspace func(ctx context.Context, dir string) error

// NewInvocation copies prompt so callers cannot alter an invocation after the
// provider has accepted the frozen request.
func NewInvocation(prompt []byte) Invocation {
	return Invocation{prompt: append([]byte(nil), prompt...)}
}

// WithProbeWorkspace returns the same invocation offering a probe copy of the
// candidate. The prompt bytes are unchanged; the provider already rendered the
// runtime's probe paragraph into them.
func (invocation Invocation) WithProbeWorkspace(workspace ProbeWorkspace) Invocation {
	invocation.prompt = append([]byte(nil), invocation.prompt...)
	invocation.probe = workspace
	return invocation
}

// ProbeWorkspace returns the candidate materializer, or nil when the provider
// offered no probe for this invocation.
func (invocation Invocation) ProbeWorkspace() ProbeWorkspace {
	return invocation.probe
}

// Prompt returns a copy so one adapter cannot mutate bytes another invocation
// would deliver.
func (invocation Invocation) Prompt() []byte {
	return append([]byte(nil), invocation.prompt...)
}

// Adapter is the entire host boundary. It may invoke a reviewer and return its
// untouched raw final output, or return a transport error.
type Adapter interface {
	Review(context.Context, Invocation) ([]byte, error)
}
