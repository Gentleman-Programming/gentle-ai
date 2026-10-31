package shellinstaller

import (
	"fmt"
)

// userInstallScope captures a request's component scope: the components to
// refresh and the components reused from the existing installation. The
// supervisor is requestable, because update detection reports it after every
// gentle-ai upgrade, but never reused: every publication copies it from the
// running build. Every other component outside the scope is reused.
type userInstallScope struct {
	requested map[string]bool
	reused    map[string]bool
	full      bool
}

func userInstallScopeOf(req UserInstallRequest) (userInstallScope, error) {
	scope := userInstallScope{requested: map[string]bool{}, reused: map[string]bool{}}
	names := userScopeComponents(req.Scope)
	if len(names) == 0 {
		scope.full = true
		return scope, nil
	}
	known := map[string]bool{
		UserComponentNode:      true,
		UserComponentSDK:       true,
		UserComponentCodeGraph: true,
		UserComponentContext7:  true,
	}
	for _, name := range names {
		if !known[name] && name != UserComponentSupervisor {
			return scope, privateError("refused", fmt.Errorf("unknown update component %q", name))
		}
		if scope.requested[name] {
			return scope, privateError("refused", fmt.Errorf("duplicated update component %q", name))
		}
		scope.requested[name] = true
	}
	for name := range known {
		if !scope.requested[name] {
			scope.reused[name] = true
		}
	}
	return scope, nil
}
