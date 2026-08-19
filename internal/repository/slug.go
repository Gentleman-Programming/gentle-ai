// Package repository provides the stable repo-slug key derivation used to
// scope per-repository artifacts (e.g. sdd/{change}/apply-progress/{repo-slug})
// across the ecosystem's repository registry.
package repository

import "strings"

// Slugify maps a GitLab namespace path to a stable repo-slug.
//
// "gp-apps-cross/Pagos" -> "gp-apps-cross-pagos". Idempotent: Slugify(Slugify(s)) == Slugify(s).
//
// Rules: trim surrounding whitespace, lowercase, collapse '/' and any run of
// non-[a-z0-9] characters to a single '-', then trim leading/trailing '-'.
// Empty or separator-only input returns "" — the caller treats the repo as
// unresolved (SPEC-005).
func Slugify(gitlabPath string) string {
	trimmed := strings.TrimSpace(gitlabPath)
	lowered := strings.ToLower(trimmed)

	var b strings.Builder
	b.Grow(len(lowered))
	prevDash := false
	for _, r := range lowered {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}
