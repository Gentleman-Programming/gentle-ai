// Package reviewerprovider defines the opaque boundary between Go-owned review
// authority and host-specific reviewer invocation.
package reviewerprovider

import "context"

// Invocation is fully materialized by the Go-owned provider. Adapters may
// deliver its opaque prompt bytes, then return the reviewer's raw output.
type Invocation struct {
	prompt []byte
}

// NewInvocation copies prompt so callers cannot alter an invocation after the
// provider has accepted the frozen request.
func NewInvocation(prompt []byte) Invocation {
	return Invocation{prompt: append([]byte(nil), prompt...)}
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
