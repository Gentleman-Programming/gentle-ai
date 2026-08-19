package sddstatus

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/repository"
)

// RepoProgress is present exactly when tasks declares MORE THAN ONE
// repository slug; a change that declares zero or exactly one repo-slug
// keeps this nil (structural absence). Exactly-one is deliberately treated
// the same as zero here (design.md speaks of "one or more", but the apply
// step for this slice tightened it to ">1"): a single declared repo is
// indistinguishable from today's single-repo flow, which still uses the
// flat apply-progress.md path and never gains a repo-scoped
// apply-progress/{slug}.md file. Keeping RepoProgress nil for that case is
// what makes every single-repo Status byte-identical to before this field
// existed.
type RepoProgress struct {
	Repos       []RepoProgressEntry `json:"repos"`
	AllComplete bool                `json:"allComplete"`
}

type RepoProgressEntry struct {
	Slug          string        `json:"slug"`
	ApplyProgress ArtifactState `json:"applyProgress"`
}

// repositoryFieldPattern matches the load-bearing `repository:` field of a
// tasks.md/tasks-artifact task line (`dev-task-planner`'s contract: "Every
// task's repository field is load-bearing"), e.g.
// "- [ ] 4a.1 repository: gentle-ai | depends_on: ...". Captures everything
// up to the next `|` field separator or end of line.
var repositoryFieldPattern = regexp.MustCompile(`(?im)\brepository:\s*([^|\r\n]+)`)

// declaredRepoSlugs parses every `repository:` field out of tasksText,
// Slugifies each, dedupes, and returns them sorted. Malformed or
// repository-less lines are simply not matched -- there is no error case,
// only "found none". Zero matches returns nil.
func declaredRepoSlugs(tasksText string) []string {
	if strings.TrimSpace(tasksText) == "" {
		return nil
	}
	matches := repositoryFieldPattern.FindAllStringSubmatch(tasksText, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, match := range matches {
		slug := repository.Slugify(match[1])
		if slug == "" {
			continue
		}
		seen[slug] = true
	}
	if len(seen) == 0 {
		return nil
	}
	slugs := make([]string, 0, len(seen))
	for slug := range seen {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return slugs
}

// buildRepoProgress composes the per-repo apply-progress block from the
// declared slugs (declaredRepoSlugs) and whichever store-specific resolver
// found apply-progress state per slug. Fewer than two declared slugs returns
// nil -- see RepoProgress's doc comment for why.
func buildRepoProgress(declaredSlugs []string, stateBySlug map[string]ArtifactState) *RepoProgress {
	if len(declaredSlugs) < 2 {
		return nil
	}
	entries := make([]RepoProgressEntry, 0, len(declaredSlugs))
	allComplete := true
	for _, slug := range declaredSlugs {
		state, ok := stateBySlug[slug]
		if !ok {
			state = ArtifactMissing
		}
		if state != ArtifactDone {
			allComplete = false
		}
		entries = append(entries, RepoProgressEntry{Slug: slug, ApplyProgress: state})
	}
	return &RepoProgress{Repos: entries, AllComplete: allComplete}
}

// applyMultiRepoApplyGate narrows an already-computed route (the house
// pattern also used by applyReviewGate/applyReviewOfferRouting/
// applyTargetedReVerifyRouting): when progress is non-nil and not every
// declared repo-slug has completed apply-progress, Verify and Archive are
// forced blocked and nextRecommended stays "apply" regardless of what the
// single/flat apply-progress artifact otherwise concluded. Nil progress (no
// declared repos, or exactly one) is a no-op -- legacy single-repo behavior
// is untouched.
func applyMultiRepoApplyGate(dependencies *Dependencies, nextRecommended *string, reasons *blockerReasons, progress *RepoProgress) {
	if progress == nil || progress.AllComplete {
		return
	}
	dependencies.Verify = DependencyBlocked
	dependencies.Archive = DependencyBlocked
	*nextRecommended = "apply"
	incomplete := make([]string, 0, len(progress.Repos))
	for _, entry := range progress.Repos {
		if entry.ApplyProgress != ArtifactDone {
			incomplete = append(incomplete, entry.Slug)
		}
	}
	reasons.genuine = append(reasons.genuine, "apply-progress incomplete for repo(s): "+strings.Join(incomplete, ", "))
}

// applyProgressStateBySlug maps each apply-progress file's base name (sans
// extension) to its artifact state. Used for the OpenSpec store, where
// per-repo files live at changeRoot/apply-progress/{slug}.md. Harmless for
// the legacy flat changeRoot/apply-progress.md case too -- its base name
// ("apply-progress") is not a real repo-slug, so it simply never matches a
// declared slug.
func applyProgressStateBySlug(paths []string) map[string]ArtifactState {
	states := make(map[string]ArtifactState, len(paths))
	for _, path := range paths {
		slug := strings.TrimSuffix(filepath.Base(path), ".md")
		if hasContent(path) {
			states[slug] = ArtifactDone
		} else {
			states[slug] = ArtifactPartial
		}
	}
	return states
}

// engramApplyProgressStateBySlug is applyProgressStateBySlug's Engram-store
// counterpart: it reads the scoped `apply-progress/{slug}` keys
// engramArtifactsForChange produces (see engramTitlePattern) rather than
// filesystem paths.
func engramApplyProgressStateBySlug(artifactsByType map[string]engramObservation) map[string]ArtifactState {
	const prefix = "apply-progress/"
	states := make(map[string]ArtifactState)
	for key, observation := range artifactsByType {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		slug := strings.TrimPrefix(key, prefix)
		states[slug] = engramArtifactState(observation)
	}
	return states
}
