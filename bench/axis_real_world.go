package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file is ONE opt-in axis, and deleting it is a supported operation:
// `rm bench/axis_real_world*.go` removes the axis whole, and the core corpus
// still builds, tests and reports exactly as it did.
//
// ---------------------------------------------------------------------------
// Why the axis exists: the corpus's repositories are sterile
// ---------------------------------------------------------------------------
//
// A community tester ran the RC on a real production repository and was walled
// by a shape no fixture had: a nested git worktree inside the repository, not
// gitignored (`review start` -> `logical path is not canonical: ".wt/test/"`,
// exit 1, empty stdout — issue #1881). The same tester later published the
// full composite that walled them: ~90 changed files at high risk, a nested
// worktree, three stale lineages left in `reviewing` from previous sessions,
// and ignored directories holding a 15MB binary. The detection-gap audit had
// named two blind spots — unvisited paths, unconstructable states — and this
// is the third: every repository in the corpus is minutes old, minimal, and
// touched by nothing but the fixture that built it. Real repositories carry
// tool residue — worktrees, dependency trees, secrets files, hooks that
// rewrite bytes, dirty submodules, shallow history, fork remotes, review
// state accumulated across days of sessions. And real operators interleave
// the review lifecycle with ordinary git life — rebase, amend, pull — instead
// of running it back to back.
//
// Two families follow from that, and every journey names its family:
//
//	family A  ecosystem clutter       repository shapes produced by tools and
//	                                  by accumulated sessions, not by the git
//	                                  operations the corpus runs
//	family B  life between commands   the review lifecycle interleaved with
//	                                  ordinary git operations
//
// The corpus rule stands unchanged: a journey that stresses no distinct
// product path is not added. The rejected candidates, and why, are in the
// README next to the accepted table.
//
// ---------------------------------------------------------------------------
// This axis IS black-box, and it is opt-in anyway
// ---------------------------------------------------------------------------
//
// Unlike damaged-store, nothing here reads or writes anything the product
// owns. Every fixture is built with git, the filesystem and the product's own
// CLI; every state is proven through git or the product before a counted
// command runs; every measurement crosses the process boundary. These
// journeys are portable across builds the same way the core is.
//
// It is an axis anyway, for two reasons. First, "43 journeys" is a number
// people compare across time, and growing it silently would make every old
// results file read as a regression. Second, this axis carries a standing
// rule the core does not: community-reported shapes become journeys — the
// reporter's fixture is the finding — so it is expected to keep growing at
// the pace of community reports, not at the pace of releases. rw01, rw08 and
// rw09 are the first entries under that rule, and rw01 is measured honestly:
// a product fix for #1881 is in flight, so the journey may block today and
// clear tomorrow, and the axis records the truth either way. Once fixed, it
// is the permanent pin.

func init() {
	RegisterAxis(Axis{
		Name:     realWorldAxis,
		Title:    "Repositories with tool residue, and review lifecycles interleaved with ordinary git life",
		BlackBox: true,
		Properties: []string{
			"Black-box, unlike damaged-store: every fixture is built with git, the filesystem and the product's own CLI, every state is proven through git or the product before a counted command runs, and nothing product-owned is read or written. These journeys stay portable across builds the way the core does.",
			"Opt-in anyway, because it is a different population: the core's repositories are sterile and its sequences contiguous; these repositories carry tool residue and accumulated review state, and their lifecycles are interleaved with rebase, amend and pull. \"43 core journeys\" and \"43 plus real-world\" must never look alike.",
			"Community-reported shapes become journeys: the reporter's fixture is the finding. rw01 is issue #1881 verbatim (nested worktree, not gitignored); rw08 and rw09 are the two elements of the same reporter's production composite the corpus could not have built. A product fix for #1881 is in flight, so rw01 may block today and clear tomorrow; the axis records the truth either way.",
			"rw03 asserts what the emitted bytes QUOTE: a sentinel secret value in an untracked .env must not appear in any counted command's stdout or stderr, and rw09 asserts the same for an ignored binary's path. The journey FAILS, naming the echo, if it ever does. This proves absence from the driven surfaces only — not that the product never read the file, which no black-box harness can see.",
			"rw02 writes a node_modules-scale tree (3,000 files) and rw09 writes a 15MB binary on every run; the cost of surviving them is part of what is measured (`git_subprocesses`, bytes).",
			"rw08's three stale lineages are built through the product's own CLI in prior-session style; those setup commands run uncounted, exactly as damaged-store's do, because the operator whose friction is measured did not run them — they opened a repository that already held the residue.",
		},
		Journeys: realWorldJourneys,
	})
}

const realWorldAxis = "real-world"

// benchSecretValue is the sentinel planted in rw03's untracked .env. It is
// distinctive enough that it cannot appear in the product's output by chance:
// if it is ever found in a counted command's emitted bytes, the product quoted
// secret content out of an untracked file, and the journey fails naming it.
const benchSecretValue = "sk_live_bench_4f1e9c2ab7d85630"

// ignoredBinaryName is the basename of rw09's gitignored 15MB binary. Ignored
// content is outside every scope the product freezes, so its PATH appearing in
// a counted command's emitted bytes would mean discovery walked into content
// git told it to skip.
const ignoredBinaryName = "build-cache.bin"

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

// stageDocsOnly writes one passive documentation file and stages exactly that
// path. The core's stageProse uses `git add -A`, which is correct in a sterile
// repository and wrong in every journey here: a cluttered repository's whole
// point is that the clutter stays UNTRACKED, and `-A` would quietly sweep a
// nested worktree, a dependency tree or a secrets file into the candidate and
// the journey would measure a different shape than it claims.
func stageDocsOnly(name string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error {
		rel := filepath.Join("docs", name+".md")
		if err := sandbox.write(filepath.Join(sandbox.Repo, rel),
			"# "+name+"\n\nplain prose, no executable content.\n"); err != nil {
			return err
		}
		return sandbox.git(sandbox.Repo, "add", "--", rel)
	}
}

// requireUntrackedNotIgnored proves the two halves of "clutter": the path is
// really there as far as git is concerned (`status --porcelain` lists it
// untracked) and nothing ignores it (`check-ignore` finds no rule). Either
// half alone can be true by accident — an ignored tree is invisible to
// discovery, and an ignore rule with a typo leaves the tree untracked — so
// both are checked.
func requireUntrackedNotIgnored(sandbox *Sandbox, path string) error {
	if _, err := gitOut(sandbox, sandbox.Repo, "check-ignore", "-q", "--", path); err == nil {
		return fmt.Errorf("fixture claims %q is not gitignored but check-ignore matches it", path)
	}
	status, err := gitOut(sandbox, sandbox.Repo, "status", "--porcelain")
	if err != nil {
		return err
	}
	if !strings.Contains(status, "?? "+path) {
		return fmt.Errorf("fixture claims %q is untracked but the porcelain status is %q", path, status)
	}
	return nil
}

// afterEach chains After hooks so a step can both remember its lineage and run
// an assertion against the same observation.
func afterEach(hooks ...func(*Sandbox, Observation) error) func(*Sandbox, Observation) error {
	return func(sandbox *Sandbox, observation Observation) error {
		for _, hook := range hooks {
			if err := hook(sandbox, observation); err != nil {
				return err
			}
		}
		return nil
	}
}

// mustNotEcho builds an After assertion that fails the journey the moment a
// counted command's emitted bytes contain the given sentinel. The failure
// reason names the invocation, because the finding IS the journey's result.
func mustNotEcho(sentinel, meaning string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		if strings.Contains(observation.Stdout, sentinel) || strings.Contains(observation.Stderr, sentinel) {
			return fmt.Errorf("the product echoed %q into its emitted bytes (args %v): %s", sentinel, observation.Args, meaning)
		}
		return nil
	}
}

// rememberStagedTree records the exact tree the review is about to freeze, so
// a later fixture can prove a hook or a detour moved bytes relative to it.
func rememberStagedTree(key string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error {
		tree, err := gitOut(sandbox, sandbox.Repo, "write-tree")
		if err != nil {
			return err
		}
		sandbox.Scratch[key] = tree
		return nil
	}
}

// ---------------------------------------------------------------------------
// Family A fixtures — ecosystem clutter
// ---------------------------------------------------------------------------

// nestedWorktreeClutter is issue #1881 verbatim: a linked worktree at
// `.wt/test` INSIDE the repository's own working tree, not gitignored. j15
// proved a linked worktree beside the repository; the community repository had
// it inside, where untracked-scope discovery walks straight into it.
func nestedWorktreeClutter(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	nested := filepath.Join(sandbox.Repo, ".wt", "test")
	if err := sandbox.git(sandbox.Repo, "worktree", "add", "-q", "-b", "wt-test", nested); err != nil {
		return err
	}

	// Proof 1: it is a linked worktree — `.git` is a file holding a gitdir
	// pointer, and its common dir is the outer repository's git dir.
	info, err := os.Stat(filepath.Join(nested, ".git"))
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("fixture claims a nested linked worktree but .git is a directory")
	}
	common, err := gitCommonDir(sandbox, nested)
	if err != nil {
		return err
	}
	outerGitDir, err := gitDir(sandbox, sandbox.Repo)
	if err != nil {
		return err
	}
	if common != outerGitDir {
		return fmt.Errorf("fixture claims a nested worktree of this repository but its common dir %q is not the outer git dir %q", common, outerGitDir)
	}

	// Proof 2: the outer repository sees it as untracked clutter, exactly as
	// the reporter's did — nothing ignores `.wt/`.
	if err := requireUntrackedNotIgnored(sandbox, ".wt/"); err != nil {
		return err
	}
	return stageDocsOnly("clutter")(sandbox)
}

const largeTreeDirs = 120
const largeTreeFilesPerDir = 25

// largeUntrackedTree drops a node_modules-scale dependency tree — 3,000 files
// — into the repository, untracked and not ignored. The question is whether
// untracked-scope discovery survives it and at what cost; `git_subprocesses`
// and the byte counts carry the answer.
func largeUntrackedTree(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	root := filepath.Join(sandbox.Repo, "node_modules")
	for dir := 0; dir < largeTreeDirs; dir++ {
		parent := filepath.Join(root, fmt.Sprintf("pkg-%03d", dir))
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
		for file := 0; file < largeTreeFilesPerDir; file++ {
			content := fmt.Sprintf("module.exports = %d;\n", dir*largeTreeFilesPerDir+file)
			if err := os.WriteFile(filepath.Join(parent, fmt.Sprintf("mod-%02d.js", file)), []byte(content), 0o644); err != nil {
				return err
			}
		}
	}

	// Proof 1: git itself enumerates exactly the claimed number of untracked
	// files under the tree. Counting on disk would prove the filesystem, not
	// what discovery has to walk.
	listed, err := gitOut(sandbox, sandbox.Repo, "ls-files", "--others", "--exclude-standard", "--", "node_modules")
	if err != nil {
		return err
	}
	want := largeTreeDirs * largeTreeFilesPerDir
	if got := len(strings.Split(listed, "\n")); got != want {
		return fmt.Errorf("fixture claims %d untracked files but git enumerates %d", want, got)
	}
	// Proof 2: untracked and not ignored, so it is really in discovery's path.
	if err := requireUntrackedNotIgnored(sandbox, "node_modules/"); err != nil {
		return err
	}
	return stageDocsOnly("deps-note")(sandbox)
}

// secretEnvUntracked plants an untracked .env whose values look exactly like
// what they are. The candidate is an ordinary docs change; the .env is the
// clutter beside it. What matters is not whether the flow completes but what
// the product's emitted bytes QUOTE along the way — the After assertion on
// every counted step in the journey checks.
func secretEnvUntracked(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	env := strings.Join([]string{
		"DATABASE_URL=postgres://svc:" + benchSecretValue + "@db.internal:5432/prod",
		"AWS_SECRET_ACCESS_KEY=" + benchSecretValue,
		"STRIPE_API_KEY=" + benchSecretValue,
		"",
	}, "\n")
	if err := sandbox.write(filepath.Join(sandbox.Repo, ".env"), env); err != nil {
		return err
	}

	// Proof 1: the sentinel is really in the file on disk, so a passing
	// journey proved absence of an echo and not absence of a secret.
	content, err := os.ReadFile(filepath.Join(sandbox.Repo, ".env"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(content), benchSecretValue) {
		return errors.New("fixture claims a planted secret but the sentinel is not in .env")
	}
	// Proof 2: untracked and not ignored, so discovery meets it.
	if err := requireUntrackedNotIgnored(sandbox, ".env"); err != nil {
		return err
	}
	return stageDocsOnly("config-note")(sandbox)
}

// secretMustNotEcho fails the journey the moment any counted command's
// emitted bytes contain the planted secret VALUE. The .env's path appearing is
// fine — the file is in the untracked scope and naming it is honest. Its
// content appearing would mean a classifier quoted secret material into an
// envelope, a finding of the first order.
var secretMustNotEcho = mustNotEcho(benchSecretValue,
	"quoting secret content out of an untracked file into an envelope is a first-order finding, and this journey exists to catch exactly that")

// ignoredMustNotBeCited fails rw09 the moment a counted command quotes the
// ignored binary's path. Ignored content is outside every scope the product
// freezes; citing it means discovery read what git said to skip.
var ignoredMustNotBeCited = mustNotEcho(ignoredBinaryName,
	"the file is gitignored and outside every frozen scope; citing it means discovery walked into content git told it to skip")

// lensLoopCheckingEchoes is captureAllLenses with rw03's assertion folded in:
// every counted observation in the collect loop — the status envelopes that
// carry the changed-path manifest, and the capture acknowledgements — is
// checked for the planted secret before the loop trusts it. The untracked
// .env escalates the review to a lens on the current build, and the collect
// envelope is exactly the surface most likely to quote content, so the loop
// that drives it must not be the one gap in the assertion.
func lensLoopCheckingEchoes(r *journeyRun) error {
	for round := 0; round < 8; round++ {
		observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo, "--contract", reviewContract, "--next-transition"}, false)
		if err := secretMustNotEcho(r.sandbox, observation); err != nil {
			return err
		}
		var envelope statusEnvelope
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &envelope); err != nil {
			return fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
		}
		if envelope.NextTransition.Kind != "collect" ||
			len(envelope.NextTransition.Collect.Inputs) == 0 ||
			envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
			return nil
		}
		result, err := synthesizeReviewerResult(
			envelope.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash, envelope.paths())
		if err != nil {
			return err
		}
		path, err := writeScratch(r.sandbox, fmt.Sprintf("reviewer-checked-%d.json", round), result)
		if err != nil {
			return err
		}
		captured := r.run([]string{
			"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", envelope.argument("lineage"),
			"--target", envelope.argument("target"),
			"--expected-revision", envelope.argument("expected-revision"),
			"--lens", envelope.argument("lens"),
			"--order", envelope.argument("order"),
			"--input", path,
		}, true)
		if err := secretMustNotEcho(r.sandbox, captured); err != nil {
			return err
		}
	}
	return errors.New("lens capture loop did not converge")
}

// evidenceCheckingEchoes is captureFinalEvidence with the same assertion.
func evidenceCheckingEchoes(r *journeyRun) error {
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo, "--contract", reviewContract, "--next-transition"}, false)
	if err := secretMustNotEcho(r.sandbox, observation); err != nil {
		return err
	}
	var envelope statusEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &envelope); err != nil {
		return fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if envelope.NextTransition.Kind != "collect" {
		return nil
	}
	path, err := writeScratch(r.sandbox, "final-evidence-checked.txt",
		[]byte("go build ./... ok\ngo test ./... ok\nall packages passed\n"))
	if err != nil {
		return err
	}
	captured := r.run([]string{
		"review", "capture-evidence", "--cwd", r.sandbox.Repo,
		"--lineage", envelope.argument("lineage"),
		"--target", envelope.argument("target"),
		"--expected-revision", envelope.argument("expected-revision"),
		"--outcome", "passed",
		"--input", path,
	}, false)
	return secretMustNotEcho(r.sandbox, captured)
}

// mutatingPreCommitHook installs a husky-style pre-commit hook that appends a
// stamp to a tracked VERSION file and re-stages it, so the commit's bytes are
// not the bytes the review approved. The branch's normalization rule says a
// mutating hook is allowed only when convergent; this hook is deliberately
// not, and the journey measures what actually happens to the receipt/gate
// chain when bytes move between review and commit.
func mutatingPreCommitHook(sandbox *Sandbox) error {
	if err := baseRepoWithRemote(sandbox); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "VERSION"), "1.0.0\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "--", "VERSION"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "add VERSION"); err != nil {
		return err
	}

	hook := filepath.Join(sandbox.Repo, ".git", "hooks", "pre-commit")
	script := "#!/bin/sh\nprintf 'stamped-by-hook\\n' >> VERSION\ngit add VERSION\n"
	if err := sandbox.write(hook, script); err != nil {
		return err
	}
	if err := os.Chmod(hook, 0o755); err != nil {
		return err
	}
	// Proof: the hook is installed where git will run it, and is executable.
	info, err := os.Stat(hook)
	if err != nil {
		return err
	}
	if info.Mode()&0o111 == 0 {
		return errors.New("fixture claims a pre-commit hook but the file is not executable")
	}
	return stageDocsOnly("release-note")(sandbox)
}

// commitThroughMutatingHook commits the reviewed candidate and then PROVES the
// hook fired and moved bytes: the stamp is in the committed VERSION, and the
// committed tree is not the tree the review froze. A hook that silently did
// not run would leave this journey measuring an ordinary happy path.
func commitThroughMutatingHook(sandbox *Sandbox) error {
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "docs: release note"); err != nil {
		return err
	}
	stamped, err := gitOut(sandbox, sandbox.Repo, "show", "HEAD:VERSION")
	if err != nil {
		return err
	}
	if !strings.Contains(stamped, "stamped-by-hook") {
		return fmt.Errorf("fixture claims the hook rewrote VERSION during commit but HEAD:VERSION is %q", stamped)
	}
	committedTree, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	reviewed := sandbox.Scratch["reviewed-tree"]
	if reviewed == "" {
		return errors.New("fixture ordering error: no reviewed tree was recorded before the commit")
	}
	if committedTree == reviewed {
		return errors.New("fixture claims the hook moved bytes between review and commit but the committed tree equals the reviewed tree")
	}
	return nil
}

// dirtySubmodule is j19's real-world sibling. j19 proved a clean gitlink being
// ADDED; here the submodule is already part of the repository, its pinned
// commit is being bumped, and its working tree carries uncommitted edits on
// top — the state a contributor's checkout is actually in.
func dirtySubmodule(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	source := filepath.Join(sandbox.Home, "sub")
	if err := sandbox.initRepo(source); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(source, "sub.txt"), "sub\n"); err != nil {
		return err
	}
	if err := sandbox.git(source, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(source, "commit", "-qm", "sub initial"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "-c", "protocol.file.allow=always",
		"submodule", "add", "-q", source, "vendor/sub"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "add submodule"); err != nil {
		return err
	}

	// Advance the submodule's checkout and stage the gitlink bump. The
	// checkout is a fresh clone, so it needs its own committer identity.
	checkout := filepath.Join(sandbox.Repo, "vendor", "sub")
	for _, config := range [][]string{
		{"config", "user.email", "bench@example.invalid"},
		{"config", "user.name", "Bench"},
		{"config", "commit.gpgsign", "false"},
	} {
		if err := sandbox.git(checkout, config...); err != nil {
			return err
		}
	}
	if err := sandbox.write(filepath.Join(checkout, "feature.txt"), "feature\n"); err != nil {
		return err
	}
	if err := sandbox.git(checkout, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(checkout, "commit", "-qm", "sub feature"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "--", "vendor/sub"); err != nil {
		return err
	}
	// Then dirty the submodule's working tree with an edit nobody committed.
	if err := sandbox.write(filepath.Join(checkout, "sub.txt"), "sub, edited and not committed\n"); err != nil {
		return err
	}

	// Proof 1: the staged entry and the committed entry are both gitlinks and
	// they differ — the bump is really the candidate.
	staged, err := gitOut(sandbox, sandbox.Repo, "ls-files", "-s", "vendor/sub")
	if err != nil {
		return err
	}
	committed, err := gitOut(sandbox, sandbox.Repo, "ls-tree", "HEAD", "vendor/sub")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(staged, "160000 ") || !strings.Contains(committed, "160000 ") {
		return fmt.Errorf("fixture claims gitlinks but the entries are %q and %q", staged, committed)
	}
	if strings.Fields(staged)[1] == strings.Fields(committed)[2] {
		return errors.New("fixture claims a staged gitlink bump but the staged and committed commits are identical")
	}
	// Proof 2: the submodule's own working tree is really dirty.
	dirty, err := gitOut(sandbox, checkout, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(dirty) == "" {
		return errors.New("fixture claims a dirty submodule but its working tree is clean")
	}
	return nil
}

// shallowClone builds a `--depth 1` clone: one commit of history in a
// repository whose remote has three. Publication-boundary derivation has to
// answer against history that is not there.
func shallowClone(sandbox *Sandbox) error {
	seed := filepath.Join(sandbox.Home, "seed")
	if err := sandbox.initRepo(seed); err != nil {
		return err
	}
	for index := 0; index < 3; index++ {
		if err := sandbox.write(filepath.Join(seed, "README.md"), fmt.Sprintf("# seed\n\nrevision %d\n", index)); err != nil {
			return err
		}
		if err := sandbox.git(seed, "add", "-A"); err != nil {
			return err
		}
		if err := sandbox.git(seed, "commit", "-qm", fmt.Sprintf("revision %d", index)); err != nil {
			return err
		}
	}
	remote := filepath.Join(sandbox.Home, "seed-remote.git")
	if err := sandbox.git(sandbox.Home, "clone", "-q", "--bare", seed, remote); err != nil {
		return err
	}
	shallow := filepath.Join(sandbox.Home, "shallow")
	// file:// forces the non-local transport; a plain path would silently
	// ignore --depth and clone full history.
	if err := sandbox.git(sandbox.Home, "clone", "-q", "--depth", "1", "file://"+remote, shallow); err != nil {
		return err
	}
	for _, config := range [][]string{
		{"config", "user.email", "bench@example.invalid"},
		{"config", "user.name", "Bench"},
		{"config", "commit.gpgsign", "false"},
	} {
		if err := sandbox.git(shallow, config...); err != nil {
			return err
		}
	}

	// Proof: git itself says the clone is shallow, and exactly one commit of
	// the seed's three is reachable.
	isShallow, err := gitOut(sandbox, shallow, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return err
	}
	if isShallow != "true" {
		return fmt.Errorf("fixture claims a shallow clone but --is-shallow-repository says %q", isShallow)
	}
	count, err := gitOut(sandbox, shallow, "rev-list", "--count", "HEAD")
	if err != nil {
		return err
	}
	if count != "1" {
		return fmt.Errorf("fixture claims depth 1 but %s commits are reachable", count)
	}
	sandbox.Repo = shallow
	return stageDocsOnly("shallow-note")(sandbox)
}

// forkTopology is the fork shape every open-source contributor works in:
// `origin` is the fork, `upstream` is the source of truth, and the local
// branch tracks UPSTREAM. The cross-remote publication logic was recently
// narrowed; this is the real-world shape it has to answer in.
func forkTopology(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	upstream := filepath.Join(sandbox.Home, "upstream.git")
	fork := filepath.Join(sandbox.Home, "fork.git")
	if err := sandbox.git(sandbox.Home, "init", "--bare", "-q", upstream); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Home, "init", "--bare", "-q", fork); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "remote", "add", "upstream", upstream); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "remote", "add", "origin", fork); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "push", "-q", "-u", "upstream", "main"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "push", "-q", "origin", "main"); err != nil {
		return err
	}

	// Proof: both remotes exist and the branch really tracks upstream, not
	// origin — the half of the shape that makes it a fork and not a mirror.
	remotes, err := gitOut(sandbox, sandbox.Repo, "remote")
	if err != nil {
		return err
	}
	if !strings.Contains(remotes, "origin") || !strings.Contains(remotes, "upstream") {
		return fmt.Errorf("fixture claims a fork topology but the remotes are %q", remotes)
	}
	tracking, err := gitOut(sandbox, sandbox.Repo, "config", "branch.main.remote")
	if err != nil {
		return err
	}
	if tracking != "upstream" {
		return fmt.Errorf("fixture claims the branch tracks upstream but branch.main.remote is %q", tracking)
	}
	return stageDocsOnly("fork-note")(sandbox)
}

// staleReviewingLineages rebuilds the first production-composite element the
// corpus could not have: review state accumulated across days of sessions.
// Three candidates are started and never finalized — prior-session style,
// through the product's own CLI, uncounted — and each is then committed so the
// next candidate is distinct. The operator being measured arrives afterwards,
// with a fourth candidate staged and three lineages sitting in `reviewing`.
func staleReviewingLineages(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	for index := 1; index <= 3; index++ {
		name := fmt.Sprintf("stale-%d", index)
		if err := stageDocsOnly(name)(sandbox); err != nil {
			return err
		}
		started := sandbox.readBack("review", "start", "--cwd", sandbox.Repo)
		if started.ExitCode != 0 {
			return fmt.Errorf("stale-lineage setup: prior-session review start %d exited %d: %s",
				index, started.ExitCode, firstLine(started.Stderr, started.Stdout))
		}
		if err := sandbox.git(sandbox.Repo, "commit", "-qm", "docs: "+name); err != nil {
			return err
		}
	}

	// Proof: the product's own inventory reports at least three DISTINCT
	// lineages in `reviewing` before a single counted command runs. Anything
	// less means the residue was not built and the journey would measure a
	// sterile store while claiming an accumulated one.
	inventory := sandbox.readBack("review", "status", "--cwd", sandbox.Repo)
	var head authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(inventory.Stdout)), &head); err != nil {
		return fmt.Errorf("stale-lineage proof: parse review status: %w (stderr: %s)", err, firstLine(inventory.Stderr))
	}
	reviewing := map[string]bool{}
	for _, entry := range head.Entries {
		if entry.State == "reviewing" {
			reviewing[entry.LineageID] = true
		}
	}
	if len(reviewing) < 3 {
		return fmt.Errorf("fixture claims three stale reviewing lineages but the product inventories %d", len(reviewing))
	}
	return stageDocsOnly("fresh")(sandbox)
}

// ignoredLargeBinary rebuilds the second production-composite element: a 15MB
// binary inside a gitignored directory. Ignored content should cost nothing —
// `git_subprocesses` and the byte counts say whether discovery honors that,
// and the After assertion says whether anything ever cites it.
func ignoredLargeBinary(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	// The ignore rule is committed history, as it would be in a real
	// repository — not a staged change that would contaminate the candidate.
	if err := sandbox.write(filepath.Join(sandbox.Repo, ".gitignore"), ".claude/\n.gentle-ai/\ndist/\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "--", ".gitignore"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "ignore dist"); err != nil {
		return err
	}
	// 15MB of deterministic non-text bytes: no randomness, so two runs write
	// identical files.
	pattern := append([]byte{0x00, 0x01, 0xfe, 0xff}, []byte("gentle-ai-bench-binary-block")...)
	binary := bytes.Repeat(pattern, 15*1024*1024/len(pattern)+1)[:15*1024*1024]
	path := filepath.Join(sandbox.Repo, "dist", ignoredBinaryName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, binary, 0o644); err != nil {
		return err
	}

	// Proof 1: the binary is really 15MB on disk.
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() != 15*1024*1024 {
		return fmt.Errorf("fixture claims a 15MB binary but it holds %d bytes", info.Size())
	}
	// Proof 2: git really ignores it — the rule matches, and neither status
	// nor the untracked listing shows it. All three, because a typo in the
	// rule would leave it untracked and this journey would silently become
	// rw02's shape with one large file.
	if _, err := gitOut(sandbox, sandbox.Repo, "check-ignore", "-q", "--", "dist/"+ignoredBinaryName); err != nil {
		return fmt.Errorf("fixture claims the binary is gitignored but check-ignore does not match it: %w", err)
	}
	status, err := gitOut(sandbox, sandbox.Repo, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.Contains(status, "dist") {
		return fmt.Errorf("fixture claims the binary is invisible to status but the porcelain status is %q", status)
	}
	others, err := gitOut(sandbox, sandbox.Repo, "ls-files", "--others", "--exclude-standard", "--", "dist")
	if err != nil {
		return err
	}
	if strings.TrimSpace(others) != "" {
		return fmt.Errorf("fixture claims the binary is excluded from the untracked listing but git enumerates %q", others)
	}
	return stageDocsOnly("dist-note")(sandbox)
}

// ---------------------------------------------------------------------------
// Family B fixtures — life between commands
// ---------------------------------------------------------------------------

// featureBranchWithRemote starts family B's rebase journey: main is pushed, a
// feature branch is cut, PUBLISHED so it has a real upstream, and the
// candidate is staged there. The publication matters: without it the pre-push
// gate answers about a missing upstream instead of about the rebase, and the
// journey would pin a different (sterile, core-shaped) denial than it claims.
func featureBranchWithRemote(sandbox *Sandbox) error {
	if err := baseRepoWithRemote(sandbox); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "-b", "feature"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "push", "-q", "-u", "origin", "feature"); err != nil {
		return err
	}
	// Proof: the branch really tracks origin/feature, so the gate's
	// publication boundary is derivable.
	upstream, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "--abbrev-ref", "feature@{upstream}")
	if err != nil {
		return err
	}
	if upstream != "origin/feature" {
		return fmt.Errorf("fixture claims the feature branch tracks origin/feature but the upstream is %q", upstream)
	}
	return stageDocsOnly("feature")(sandbox)
}

// commitReviewedCandidate commits the approved candidate and records where it
// landed, so the detour fixtures can prove what moved.
func commitReviewedCandidate(message string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error {
		if err := sandbox.git(sandbox.Repo, "commit", "-qm", message); err != nil {
			return err
		}
		head, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		tree, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^{tree}")
		if err != nil {
			return err
		}
		sandbox.Scratch["reviewed-commit"] = head
		sandbox.Scratch["reviewed-commit-tree"] = tree
		return nil
	}
}

// rebaseOntoMovedMain advances main under the reviewed branch and rebases the
// reviewed commit onto it — the most ordinary thing an operator does between
// approval and push. The files are disjoint on purpose: a conflict would turn
// this into a different journey.
func rebaseOntoMovedMain(sandbox *Sandbox) error {
	reviewed := sandbox.Scratch["reviewed-commit"]
	if reviewed == "" {
		return errors.New("fixture ordering error: no reviewed commit recorded before the rebase")
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "main"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "upstream.md"),
		"# upstream\n\nmain moved while the review was open.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "--", "docs/upstream.md"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "docs: upstream moved"); err != nil {
		return err
	}
	mainHead, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "feature"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "rebase", "-q", "main"); err != nil {
		return err
	}

	// Proof 1: the rebase really replaced the reviewed commit — new id, and
	// its parent is the moved main.
	head, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head == reviewed {
		return errors.New("fixture claims a rebase but HEAD is still the reviewed commit")
	}
	parent, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^")
	if err != nil {
		return err
	}
	if parent != mainHead {
		return fmt.Errorf("fixture claims a rebase onto moved main but HEAD^ is %q, not %q", parent, mainHead)
	}
	// Proof 2: the tree moved too — the rebased commit now contains main's
	// file — so this is NOT the amend journey, where the tree is identical.
	tree, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	if tree == sandbox.Scratch["reviewed-commit-tree"] {
		return errors.New("fixture claims the rebase changed the tree but it is identical; this would silently be the amend journey")
	}
	return nil
}

// amendKeepTree rewrites the reviewed commit's id while keeping its tree
// byte-identical — `git commit --amend` with only the message changed. It is
// the way a human actually trips the tree-not-commit identity principle.
func amendKeepTree(sandbox *Sandbox) error {
	reviewed := sandbox.Scratch["reviewed-commit"]
	if reviewed == "" {
		return errors.New("fixture ordering error: no reviewed commit recorded before the amend")
	}
	if err := sandbox.git(sandbox.Repo, "commit", "--amend", "-qm", "docs: amended message, same bytes"); err != nil {
		return err
	}
	head, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	// Proof: the commit id moved and the tree did not. Either half failing
	// means this is measuring a different journey than it claims.
	if head == reviewed {
		return errors.New("fixture claims an amend but the commit id did not change")
	}
	tree, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	if tree != sandbox.Scratch["reviewed-commit-tree"] {
		return errors.New("fixture claims a content-identical amend but the tree changed")
	}
	return nil
}

// seedRemoteAhead makes origin/main move under the local branch: a colleague's
// clone commits and pushes while the local review is open.
func seedRemoteAhead(sandbox *Sandbox) error {
	colleague := filepath.Join(sandbox.Home, "colleague")
	// `-b main` matters: the bare remote's default HEAD may name an unborn
	// branch, and a clone without it checks out nothing — the colleague's
	// commit would then land on a DIFFERENT branch and the pull under test
	// would be a no-op. The first draft of this fixture had exactly that bug,
	// and the proofs below are what caught it.
	if err := sandbox.git(sandbox.Home, "clone", "-q", "-b", "main", sandbox.Remote, colleague); err != nil {
		return err
	}
	for _, config := range [][]string{
		{"config", "user.email", "colleague@example.invalid"},
		{"config", "user.name", "Colleague"},
		{"config", "commit.gpgsign", "false"},
	} {
		if err := sandbox.git(colleague, config...); err != nil {
			return err
		}
	}
	if err := sandbox.write(filepath.Join(colleague, "docs", "colleague.md"),
		"# colleague\n\nlanded while the review was open.\n"); err != nil {
		return err
	}
	if err := sandbox.git(colleague, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(colleague, "commit", "-qm", "docs: colleague"); err != nil {
		return err
	}
	if err := sandbox.git(colleague, "push", "-q", "origin", "HEAD"); err != nil {
		return err
	}

	// Proof 1: the colleague's commit landed on MAIN of the shared remote —
	// the ref the pull under test will fetch — not on some other branch.
	remoteHead, err := gitOut(sandbox, colleague, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	remoteRef, err := gitOut(sandbox, colleague, "ls-remote", sandbox.Remote, "refs/heads/main")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(remoteRef, remoteHead) {
		return fmt.Errorf("fixture claims the remote's main moved to %q but the remote reports %q", remoteHead, remoteRef)
	}
	// Proof 2: it moved AHEAD — the local head is a strict ancestor of it, so
	// the pull will genuinely bring in a commit the branch does not have.
	localHead, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if remoteHead == localHead {
		return errors.New("fixture claims the remote moved ahead but both heads are identical")
	}
	if _, err := gitOut(sandbox, colleague, "merge-base", "--is-ancestor", localHead, remoteHead); err != nil {
		return fmt.Errorf("fixture claims the remote moved ahead of the local head but the local head is not its ancestor: %w", err)
	}
	sandbox.Scratch["remote-head"] = remoteHead
	return nil
}

// pullMergeIntoReviewedBranch pulls the moved remote into the branch holding
// the reviewed commit. The result is a merge commit: HEAD is no longer the
// commit the receipt was issued against, but it contains it.
func pullMergeIntoReviewedBranch(sandbox *Sandbox) error {
	reviewed := sandbox.Scratch["reviewed-commit"]
	if reviewed == "" {
		return errors.New("fixture ordering error: no reviewed commit recorded before the pull")
	}
	if err := sandbox.git(sandbox.Repo, "-c", "pull.rebase=false", "pull", "-q"); err != nil {
		return err
	}

	// Proof 1: HEAD is a real merge commit — a second parent exists and it is
	// the colleague's commit.
	secondParent, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "--verify", "HEAD^2")
	if err != nil {
		return fmt.Errorf("fixture claims a merge commit but HEAD has no second parent: %w", err)
	}
	if secondParent != sandbox.Scratch["remote-head"] {
		return fmt.Errorf("fixture claims the merge brought in the remote's commit but HEAD^2 is %q", secondParent)
	}
	// Proof 2: the reviewed commit is still reachable — the pull wrapped it,
	// it did not replace it.
	if _, err := gitOut(sandbox, sandbox.Repo, "merge-base", "--is-ancestor", reviewed, "HEAD"); err != nil {
		return fmt.Errorf("fixture claims the reviewed commit survived the pull but it is not an ancestor of HEAD: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Corpus
// ---------------------------------------------------------------------------

// realWorldJourneys is the axis corpus: family A is ecosystem clutter (rw01 to
// rw09), family B is life between commands (rw10 to rw12). Each journey names
// the distinct product path it pins; a candidate that stressed none was
// rejected, and the README lists the rejects next to this table.
func realWorldJourneys() []Journey {
	return []Journey{
		// ------------------------------------------------------------ family A
		{
			ID:     "rw01-nested-worktree-not-ignored",
			Title:  "Nested worktree inside the repository, not gitignored: issue #1881 verbatim",
			Source: "family A (ecosystem clutter): community production repo, issue #1881",
			// Distinct path: untracked-scope discovery walking into a linked
			// worktree UNDER the repository root. j15's worktree lives beside
			// the repository, where discovery never meets it. A product fix is
			// in flight; this journey records whatever HEAD does today, and
			// once the fix lands it is the permanent pin.
			Steps: []Step{
				{Name: "fixture: nested worktree proven linked, untracked and not ignored", Fixture: nestedWorktreeClutter},
				{Name: "review start beside the nested worktree", Requires: startCapability,
					Args: productArgs("review", "start"), After: rememberLineage, AbortOnBlock: true},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "rw02-node-modules-scale-untracked-tree",
			Title:  "3,000 untracked files beside the candidate: discovery must survive, and the cost is measured",
			Source: "family A (ecosystem clutter): dependency trees tools leave behind",
			// Distinct path: untracked-scope freeze over thousands of paths.
			// The corpus's largest candidate (j04) is 1,200 lines in four
			// files; nothing has ever made discovery walk a tree this size.
			// `git_subprocesses` and the byte counts carry the price — and on
			// the current build the price includes an escalation: the same
			// staged docs change that reviews at tier 0 in j01 demands a
			// reviewer lens here, purely because of what sits UNTRACKED
			// beside it. The lens flow below is what the product dictated.
			Steps: []Step{
				{Name: "fixture: 3,000 untracked files proven enumerated by git", Fixture: largeUntrackedTree},
				{Name: "review start beside the tree", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens the tree escalated to", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "rw03-untracked-env-with-secrets",
			Title:  "Untracked .env with secret-looking content: nothing counted may quote the value",
			Source: "family A (ecosystem clutter): secrets files beside every real candidate",
			// Distinct path: what risk evidence and scope envelopes QUOTE.
			// Naming the .env's PATH is honest — it is in the untracked scope.
			// Echoing its VALUE would be a first-order finding, and every
			// counted observation in the journey is checked for it — the lens
			// flow (which the untracked .env itself escalates the review
			// into) through the checked composites, everything else through
			// After assertions.
			Steps: []Step{
				{Name: "fixture: sentinel secret proven planted, untracked and not ignored", Fixture: secretEnvUntracked},
				{Name: "review start beside the .env", Requires: startCapability,
					Args: productArgs("review", "start"), After: afterEach(rememberLineage, secretMustNotEcho)},
				{Name: "review status beside the .env", Requires: statusOnlyCapability,
					Args: productArgs("review", "status"), After: secretMustNotEcho},
				{Name: "capture every lens, checking each envelope for the secret", Requires: captureResultCapability, Composite: lensLoopCheckingEchoes},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability,
					Args: productArgs("review", "finalize", "--captured-results=true"), After: secretMustNotEcho},
				{Name: "capture final evidence, checking for the secret", Requires: captureEvidenceCapability, Composite: evidenceCheckingEchoes},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability,
					Args: productArgs("review", "finalize", "--captured-evidence=true"), After: afterEach(rememberLineage, secretMustNotEcho)},
				{Name: "gate pre-commit", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"), After: secretMustNotEcho},
			},
		},
		{
			ID:     "rw04-mutating-pre-commit-hook",
			Title:  "Husky-style hook rewrites a file during commit: the receipt meets bytes it never approved",
			Source: "family A (ecosystem clutter): hooks that move bytes between review and commit",
			// Distinct path: the normalization ordering rule's enforcement
			// arm. The contract says a mutating hook is allowed only when
			// convergent; this one is not, the commit proof shows the bytes
			// moved, and the pre-push gate answers for a HEAD whose tree the
			// receipt never saw.
			Steps: []Step{
				{Name: "fixture: mutating hook proven installed and executable", Fixture: mutatingPreCommitHook},
				{Name: "fixture: record the tree the review will freeze", Fixture: rememberStagedTree("reviewed-tree")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit before the hook fires", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit, hook proven fired and bytes proven moved", Fixture: commitThroughMutatingHook},
				{Name: "gate pre-push after the hook moved bytes", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-push")},
			},
		},
		{
			ID:     "rw05-dirty-submodule-gitlink-bump",
			Title:  "Staged gitlink bump while the submodule's working tree is dirty",
			Source: "family A (ecosystem clutter): j19's real-world sibling — the checkout is never clean",
			// Distinct path: a candidate whose gitlink points at one commit
			// while the directory behind it holds different bytes. j19's
			// clean add never separated "what the index says" from "what the
			// working tree holds" for a 160000 entry.
			Steps: []Step{
				{Name: "fixture: gitlink bump proven staged, submodule proven dirty", Fixture: dirtySubmodule},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"),
					After: rememberLineage, AbortOnBlock: true},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
				{Name: "gate post-apply", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "post-apply")},
			},
		},
		{
			ID:     "rw06-shallow-clone-depth-1",
			Title:  "Shallow clone: publication-boundary derivation against history that is not there",
			Source: "family A (ecosystem clutter): CI checkouts and laptop clones are depth 1",
			// Distinct path: every ancestry question the pre-push gate asks
			// has to answer inside one commit of history. The core's
			// repositories always hold their full (tiny) history.
			Steps: []Step{
				{Name: "fixture: clone proven shallow with 1 of 3 commits", Fixture: shallowClone},
				{Name: "review start in the shallow clone", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit", Fixture: commitStaged("docs: shallow note")},
				{Name: "gate pre-push against missing history", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-push")},
			},
		},
		{
			ID:     "rw07-fork-topology-tracks-upstream",
			Title:  "origin plus upstream, branch tracking upstream: the cross-remote logic's real-world shape",
			Source: "family A (ecosystem clutter): the recently narrowed cross-remote logic, as contributors meet it",
			// Distinct path: publication-boundary derivation with two remotes
			// where the tracking remote is NOT origin. Every core journey has
			// exactly one remote named origin.
			Steps: []Step{
				{Name: "fixture: fork topology proven, branch proven tracking upstream", Fixture: forkTopology},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit", Fixture: commitStaged("docs: fork note")},
				{Name: "gate pre-push across two remotes", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-push")},
			},
		},
		{
			ID:     "rw08-three-stale-reviewing-lineages",
			Title:  "Three lineages left in `reviewing` by prior sessions: a fresh review must not misroute",
			Source: "family A (ecosystem clutter): community production composite — accumulated review state",
			// Distinct path: discovery and gating against a store holding
			// open residue. Every core journey's store is empty or seconds
			// old; the reporter's held days of sessions. The counted status
			// is the operator's arrival inventory; the fresh start must bind
			// the fourth candidate and not resume a stale scope; the gate
			// must resolve the fresh receipt among four lineages.
			Steps: []Step{
				{Name: "fixture: three stale reviewing lineages proven inventoried by the product", Fixture: staleReviewingLineages},
				{Name: "review status on arrival, over the residue", Requires: statusOnlyCapability, Args: productArgs("review", "status")},
				{Name: "review start of the fresh candidate", Requires: startCapability,
					Args: productArgs("review", "start"), After: rememberLineage, AbortOnBlock: true},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit among four lineages", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "rw09-ignored-15mb-binary",
			Title:  "A 15MB gitignored binary inside the tree: ignored content must cost nothing and be cited nowhere",
			Source: "family A (ecosystem clutter): community production composite — ignored build artifacts",
			// Distinct path: the exclusion side of discovery. rw02 proves the
			// cost of content discovery MUST walk; this proves the content it
			// must NOT: the subprocess and byte counts say what the binary
			// cost, and the After assertion fails the journey if any counted
			// command cites its path.
			Steps: []Step{
				{Name: "fixture: binary proven 15MB, ignored, and invisible to status", Fixture: ignoredLargeBinary},
				{Name: "review start beside the ignored binary", Requires: startCapability,
					Args: productArgs("review", "start"), After: afterEach(rememberLineage, ignoredMustNotBeCited)},
				{Name: "review finalize", Requires: finalizeCapability,
					Args: productArgs("review", "finalize"), After: afterEach(rememberLineage, ignoredMustNotBeCited)},
				{Name: "gate pre-commit", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"), After: ignoredMustNotBeCited},
			},
		},

		// ------------------------------------------------------------ family B
		{
			ID:     "rw10-rebase-onto-moved-main",
			Title:  "Approve, commit, rebase onto moved main, then the gate: base advance with a changed tree",
			Source: "family B (life between commands): the most ordinary detour between approval and push",
			// Distinct path: compatible-base-advance handling where BOTH the
			// commit id and the tree moved. rw11 isolates the id-only half;
			// this is the half where the reviewed content now sits on new
			// bytes, and the gate has to say whether that reopens anything.
			Steps: []Step{
				{Name: "fixture: feature branch with remote, candidate staged", Fixture: featureBranchWithRemote},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit the reviewed candidate", Fixture: commitReviewedCandidate("docs: feature")},
				{Name: "fixture: main proven moved, rebase proven to replace commit and tree", Fixture: rebaseOntoMovedMain},
				{Name: "gate pre-push after the rebase", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-push")},
			},
		},
		{
			ID:     "rw11-amend-identical-tree",
			Title:  "Approve, commit, amend the message: same bytes, new commit id, then the gate",
			Source: "family B (life between commands): the tree-not-commit identity principle, as a human trips it",
			// Distinct path: receipt identity under a commit-id change with a
			// byte-identical tree. If identity is bound to the tree, the gate
			// allows; if anything secretly keyed on the commit id, this is
			// where it surfaces.
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage docs", Fixture: stageDocsOnly("amended")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit the reviewed candidate", Fixture: commitReviewedCandidate("docs: amended")},
				{Name: "fixture: amend proven to change the id and keep the tree", Fixture: amendKeepTree},
				{Name: "gate pre-push after the amend", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-push")},
			},
		},
		{
			ID:     "rw12-pull-into-reviewed-branch",
			Title:  "Approve, commit, pull a colleague's commit into the branch, then the gate",
			Source: "family B (life between commands): mid-lifecycle `git pull` bringing new commits into the branch under review",
			// Distinct path: receipt discovery across a merge commit. After
			// the pull, HEAD is neither the reviewed commit (rw11) nor a
			// linear replacement of it (rw10): it is a merge that CONTAINS
			// it, plus foreign bytes the review never saw.
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: remote proven moved ahead by a colleague", Fixture: seedRemoteAhead},
				{Name: "fixture: stage docs", Fixture: stageDocsOnly("local")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit the reviewed candidate", Fixture: commitReviewedCandidate("docs: local")},
				{Name: "fixture: pull proven to wrap the reviewed commit in a merge", Fixture: pullMergeIntoReviewedBranch},
				{Name: "gate pre-push after the pull", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-push")},
			},
		},
	}
}
