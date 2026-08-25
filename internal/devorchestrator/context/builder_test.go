package context

import "testing"

// TestBuild_PopulatesDesignRef covers design decision D-D: a canonical
// design reference string flows from BuildRequest.DesignRef into
// Package.DesignRef unchanged, mirroring how DBImpact is threaded through
// Build (see BuildRequest.DBImpact / Package.DBImpact above).
func TestBuild_PopulatesDesignRef(t *testing.T) {
	req := BuildRequest{
		ExecutionID: "exec-1",
		AgentName:   "frontend-implementer",
		DesignRef:   "https://www.figma.com/design/ABC12345XY",
	}

	pkg := Build(req)

	if pkg.DesignRef != req.DesignRef {
		t.Errorf("Build(req).DesignRef = %q, want %q", pkg.DesignRef, req.DesignRef)
	}
}

// TestBuild_OmitsDesignRefWhenEmpty covers the negative case: an empty
// BuildRequest.DesignRef must produce an empty Package.DesignRef, never a
// synthesized value.
func TestBuild_OmitsDesignRefWhenEmpty(t *testing.T) {
	req := BuildRequest{
		ExecutionID: "exec-2",
		AgentName:   "backend-implementer",
	}

	pkg := Build(req)

	if pkg.DesignRef != "" {
		t.Errorf("Build(req).DesignRef = %q, want empty string", pkg.DesignRef)
	}
}
