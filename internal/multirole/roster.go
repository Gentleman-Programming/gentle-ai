package multirole

import (
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// RosterSource identifies which layer produced a resolved roster (D-06):
// the sealed kickoff configuration, an explicit design.md declaration, or
// DetectRoles' own single-role compatibility fallback.
type RosterSource string

const (
	// RosterSourceKickoff means the roster came from a sealed kickoff.yaml
	// (internal/kickoff). It always wins over design.md when both exist.
	RosterSourceKickoff RosterSource = "kickoff"
	// RosterSourceDesign means no kickoff is sealed and design.md declared
	// an explicit "roles:" block that DetectRoles parsed successfully.
	RosterSourceDesign RosterSource = "design"
	// RosterSourceCompatibility means no kickoff is sealed and design.md
	// declared no roles at all: DetectRoles' own single-role fallback
	// produced the roster.
	RosterSourceCompatibility RosterSource = "fallback"
)

// RosterConflict reports a non-blocking discrepancy between a sealed
// kickoff roster and what design.md separately declares. Its existence
// never changes which roster wins (the sealed one always does, D-06 rule
// 1) — it only makes the discrepancy observable instead of resolving it in
// silence.
type RosterConflict struct {
	KickoffRoles []string
	DesignRoles  []string
	Detail       string
}

// Roster is the single reconciled view of "who implements this change",
// returned by ResolveRoster: the authoritative role list, which layer it
// came from, and any conflict found against a layer that did not win.
type Roster struct {
	Roles    []RoleAssignment
	Source   RosterSource
	Conflict *RosterConflict
}

// ResolveRoster is the single authority for a change's role roster (D-06).
// It stratifies three layers with strict precedence instead of unifying or
// duplicating them:
//
//  1. sealed (non-empty): the roster a kickoff.yaml already sealed always
//     wins. If design.md separately declares different roles, that
//     discrepancy is surfaced as a non-blocking Conflict — never resolved
//     in silence, and never allowed to override the sealed roster.
//  2. sealed empty: ResolveRoster delegates entirely to
//     DetectRoles(designPath, wsConfig), UNMODIFIED — REQ-1.1 of
//     multi-role-fan-out-engine is not touched by this function. The
//     result is classified as RosterSourceDesign when design.md declared
//     an explicit "roles:" block, or RosterSourceCompatibility when
//     DetectRoles' own single-role fallback produced it.
//
// ResolveRoster never imports internal/kickoff: the edge this package
// participates in runs kickoff -> multirole, never the reverse (D-06,
// avoiding the cycle the alternative "unify in DetectRoles" would create).
func ResolveRoster(sealed []RoleAssignment, designPath string, wsConfig *workspace.WorkspaceConfig) (Roster, error) {
	if len(sealed) > 0 {
		roster := Roster{Roles: sealed, Source: RosterSourceKickoff}
		if designRoles, err := designDeclaredRoles(designPath); err == nil && len(designRoles) > 0 {
			if conflict := detectRosterConflict(sealed, designRoles); conflict != nil {
				roster.Conflict = conflict
			}
		}
		return roster, nil
	}

	detected, err := DetectRoles(designPath, wsConfig)
	if err != nil {
		return Roster{}, err
	}
	source := RosterSourceCompatibility
	if designRoles, designErr := designDeclaredRoles(designPath); designErr == nil && len(designRoles) > 0 {
		source = RosterSourceDesign
	}
	return Roster{Roles: detected, Source: source}, nil
}

// designDeclaredRoles reads designPath and parses ONLY its explicit
// "roles:" declaration (ParseRolesMarkdown), with no axiom.yaml
// cross-validation and no single-role fallback. It exists purely to
// classify RosterSourceDesign vs RosterSourceCompatibility and to detect a
// sealed/design conflict — never as a second source of truth for the
// actual roster, which DetectRoles alone still produces. A missing or
// unreadable design.md is reported as an error here (never panics) and
// simply means "nothing to classify or compare against", handled by both
// call sites above via their own err == nil guard.
func designDeclaredRoles(designPath string) ([]RoleAssignment, error) {
	data, err := os.ReadFile(designPath)
	if err != nil {
		return nil, fmt.Errorf("leer %q para clasificar el origen del roster: %w", designPath, err)
	}
	return ParseRolesMarkdown(string(data))
}

// detectRosterConflict reports whether design declares a different set of
// role identifiers than sealed, comparing case-insensitively (role
// identifiers are already treated case-insensitively everywhere else this
// package validates them, e.g. roleExists).
func detectRosterConflict(sealed, design []RoleAssignment) *RosterConflict {
	if sameRoleNameSet(sealed, design) {
		return nil
	}
	sealedNames := roleNames(sealed)
	designNames := roleNames(design)
	return &RosterConflict{
		KickoffRoles: sealedNames,
		DesignRoles:  designNames,
		Detail: fmt.Sprintf(
			"el kickoff sellado declara los roles %s pero design.md declara %s; el roster sellado sigue mandando",
			strings.Join(sealedNames, ", "), strings.Join(designNames, ", "),
		),
	}
}

func sameRoleNameSet(a, b []RoleAssignment) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, r := range a {
		seen[strings.ToLower(r.Role)] = true
	}
	for _, r := range b {
		if !seen[strings.ToLower(r.Role)] {
			return false
		}
	}
	return true
}

func roleNames(roles []RoleAssignment) []string {
	names := make([]string, 0, len(roles))
	for _, r := range roles {
		names = append(names, r.Role)
	}
	return names
}
