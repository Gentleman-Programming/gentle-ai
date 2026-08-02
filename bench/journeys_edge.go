package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// This file extends the corpus with edge cases the original fourteen journeys
// never reached. Every journey here is tied to one of the five shapes a night of
// real defects clustered into, and the shape is named on the journey:
//
//	shape 1  asymmetric comparison        one operand canonicalized, the other not
//	shape 2  transient read as permanent  a retryable condition surfacing as terminal
//	shape 3  guard behind wrong condition a check gated on something that is not its
//	                                      own precondition
//	shape 4  a message naming something   the continuation named does not work
//	         that does not work
//	shape 5  two sources of truth         a document and the code disagreeing
//
// A journey that stresses none of them is not added: it would only make the
// number look covered.
//
// Every fixture here PROVES the edge case it claims before the journey is
// allowed to trust the result. A fixture that sets its edge case up wrongly and
// then passes is the failure mode these tests exist to avoid, so each one reads
// the state back out of git and returns an error when it is not what it says.

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

// gitOut runs a fixture git command and returns its stdout. Sandbox.git
// discards stdout, and every proof in this file needs to read git's answer
// back rather than trust that a command exited 0.
func gitOut(sandbox *Sandbox, dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	env := sandbox.env()
	for index, entry := range env {
		if strings.HasPrefix(entry, "GIT_TRACE=") {
			env[index] = "GIT_TRACE="
		}
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// gitExpectFailure runs a fixture git command that is SUPPOSED to fail, such as
// a merge that must conflict. It reports an error when the command unexpectedly
// succeeded, because a fixture whose conflict silently resolved is not the
// fixture the journey claims to be testing.
func gitExpectFailure(sandbox *Sandbox, dir string, args ...string) error {
	if _, err := gitOut(sandbox, dir, args...); err == nil {
		return fmt.Errorf("git %s succeeded but the fixture needs it to conflict", strings.Join(args, " "))
	}
	return nil
}

// gitDir is the absolute .git directory of one working tree. For a linked
// worktree this is the per-worktree directory, NOT the common dir.
func gitDir(sandbox *Sandbox, dir string) (string, error) {
	return gitOut(sandbox, dir, "rev-parse", "--absolute-git-dir")
}

// gitCommonDir is the absolute shared common dir. Every worktree of one
// repository reports the same one, which is why the review store is shared.
func gitCommonDir(sandbox *Sandbox, dir string) (string, error) {
	return gitOut(sandbox, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
}

// stageProse writes one passive documentation file and stages it. It is the
// smallest candidate that reviews at tier 0, so journeys about repository shape
// and git state are not also journeys about risk tiering. An empty repo means
// "the journey's current repository"; a named one lets the linked-worktree
// journey stage into the other tree.
func stageProse(repo, name string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error {
		if repo == "" {
			repo = sandbox.Repo
		}
		path := filepath.Join(repo, "docs", name+".md")
		if err := sandbox.write(path, "# "+name+"\n\nplain prose, no executable content.\n"); err != nil {
			return err
		}
		return sandbox.git(repo, "add", "-A")
	}
}

// ---------------------------------------------------------------------------
// Repository shape fixtures
// ---------------------------------------------------------------------------

// linkedWorktree builds the highest-value single fixture in this file: a second
// working tree that shares the git common dir — and therefore the review store
// — while having its own working tree, its own HEAD and its own branch.
//
// It stresses path comparison, repository identity and store contention at
// once, because two different absolute paths are now one repository.
func linkedWorktree(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	main := sandbox.Repo
	worktree := filepath.Join(sandbox.Home, "demo-worktree")
	if err := sandbox.git(main, "worktree", "add", "-q", "-b", "feature", worktree); err != nil {
		return err
	}

	// Proof 1: the worktree's .git is a regular FILE holding a gitdir pointer,
	// not a directory. A plain clone would fail this.
	info, err := os.Stat(filepath.Join(worktree, ".git"))
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("fixture claims a linked worktree but .git is a directory")
	}

	// Proof 2: its own git dir differs from the common dir, and the common dir
	// is the main repository's git dir. That is what "shares the review store"
	// means mechanically.
	worktreeGitDir, err := gitDir(sandbox, worktree)
	if err != nil {
		return err
	}
	worktreeCommon, err := gitCommonDir(sandbox, worktree)
	if err != nil {
		return err
	}
	mainGitDir, err := gitDir(sandbox, main)
	if err != nil {
		return err
	}
	if worktreeGitDir == worktreeCommon {
		return fmt.Errorf("fixture claims a linked worktree but git dir and common dir are both %q", worktreeGitDir)
	}
	if worktreeCommon != mainGitDir {
		return fmt.Errorf("fixture claims a shared store but common dir %q is not the main git dir %q", worktreeCommon, mainGitDir)
	}

	// Proof 3: the two trees really are on different branches, so HEAD is not
	// shared either.
	mainBranch, err := gitOut(sandbox, main, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	worktreeBranch, err := gitOut(sandbox, worktree, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	if mainBranch == worktreeBranch {
		return fmt.Errorf("fixture claims two heads but both worktrees are on %q", mainBranch)
	}

	sandbox.Scratch["main"] = main
	sandbox.Scratch["worktree"] = worktree
	sandbox.Repo = worktree
	return nil
}

// detachedHead leaves HEAD pointing straight at a commit with no branch.
func detachedHead(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "README.md"), "# demo\n\nhello\n\nsecond\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "second"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "--detach", "HEAD"); err != nil {
		return err
	}

	// Proof: HEAD resolves to no symbolic ref, and --abbrev-ref says HEAD.
	if _, err := gitOut(sandbox, sandbox.Repo, "symbolic-ref", "-q", "HEAD"); err == nil {
		return errors.New("fixture claims a detached HEAD but HEAD is still a symbolic ref")
	}
	branch, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	if branch != "HEAD" {
		return fmt.Errorf("fixture claims a detached HEAD but the branch reads %q", branch)
	}
	return stageProse(sandbox.Repo, "detached")(sandbox)
}

// bareRepository points the journey at a repository with no working tree at
// all. There is nothing to stage, which is the point.
func bareRepository(sandbox *Sandbox) error {
	bare := filepath.Join(sandbox.Home, "bare.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Home, "init", "--bare", "-q", bare); err != nil {
		return err
	}

	// Proof: git itself says this is bare.
	isBare, err := gitOut(sandbox, bare, "rev-parse", "--is-bare-repository")
	if err != nil {
		return err
	}
	if isBare != "true" {
		return fmt.Errorf("fixture claims a bare repository but --is-bare-repository says %q", isBare)
	}
	sandbox.Repo = bare
	return nil
}

// awkwardPath puts the repository behind a directory name carrying spaces and
// non-ASCII characters, so every path that crosses a shell, a JSON envelope or
// a comparison has to survive both.
func awkwardPath(sandbox *Sandbox) error {
	repo := filepath.Join(sandbox.Home, "demo repo ñ 日本 dir")
	if err := sandbox.initRepo(repo); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(repo, "README.md"), "# demo\n\nhello\n"); err != nil {
		return err
	}
	if err := sandbox.git(repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(repo, "commit", "-qm", "initial"); err != nil {
		return err
	}

	// Proof: git reports the same awkward toplevel back, so the characters
	// really did survive creation rather than being silently transliterated.
	toplevel, err := gitOut(sandbox, repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if toplevel != repo {
		return fmt.Errorf("fixture claims path %q but git reports %q", repo, toplevel)
	}
	sandbox.Repo = repo

	// A non-ASCII file name inside the non-ASCII directory, so the candidate
	// path is awkward too and not only the repository root.
	path := filepath.Join(repo, "docs", "ünïcödé.md")
	if err := sandbox.write(path, "# unicode\n\nplain prose, no executable content.\n"); err != nil {
		return err
	}
	return sandbox.git(repo, "add", "-A")
}

// submoduleCandidate stages a gitlink: an index entry in mode 160000 that
// points at a commit and has no blob behind it at all.
func submoduleCandidate(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	sub := filepath.Join(sandbox.Home, "sub")
	if err := sandbox.initRepo(sub); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sub, "sub.txt"), "sub\n"); err != nil {
		return err
	}
	if err := sandbox.git(sub, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sub, "commit", "-qm", "sub initial"); err != nil {
		return err
	}
	// Modern git refuses file:// submodules unless the transport is allowed.
	if err := sandbox.git(sandbox.Repo, "-c", "protocol.file.allow=always",
		"submodule", "add", "-q", sub, "vendor/sub"); err != nil {
		return err
	}

	// Proof: the index entry is a gitlink, not a blob.
	entry, err := gitOut(sandbox, sandbox.Repo, "ls-files", "-s", "vendor/sub")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(entry, "160000 ") {
		return fmt.Errorf("fixture claims a gitlink but the index entry is %q", entry)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Candidate content fixtures
// ---------------------------------------------------------------------------

// symlinkCandidate stages a symbolic link: mode 120000, whose blob content is
// the target path rather than file bytes.
func symlinkCandidate(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := os.Symlink("README.md", filepath.Join(sandbox.Repo, "LINK.md")); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}

	// Proof: git recorded a symlink, not a copied regular file.
	entry, err := gitOut(sandbox, sandbox.Repo, "ls-files", "-s", "LINK.md")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(entry, "120000 ") {
		return fmt.Errorf("fixture claims a symlink but the index entry is %q", entry)
	}
	return nil
}

// modeOnlyChange flips the executable bit and changes nothing else. The blob
// hash on both sides is identical, so a comparison that looks only at content
// sees no change at all.
func modeOnlyChange(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	script := filepath.Join(sandbox.Repo, "tool.sh")
	if err := sandbox.write(script, "#!/bin/sh\necho hi\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "add tool"); err != nil {
		return err
	}
	before, err := gitOut(sandbox, sandbox.Repo, "ls-files", "-s", "tool.sh")
	if err != nil {
		return err
	}
	if err := os.Chmod(script, 0o755); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	after, err := gitOut(sandbox, sandbox.Repo, "ls-files", "-s", "tool.sh")
	if err != nil {
		return err
	}

	// Proof 1: the recorded mode really moved from 100644 to 100755. On a
	// filesystem that drops the executable bit this fails instead of passing.
	if !strings.HasPrefix(before, "100644 ") || !strings.HasPrefix(after, "100755 ") {
		return fmt.Errorf("fixture claims a mode-only change but the index went %q -> %q", before, after)
	}
	// Proof 2: not one line of content moved.
	numstat, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--numstat")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(numstat, "0\t0\t") {
		return fmt.Errorf("fixture claims a mode-only change but the diff is %q", numstat)
	}
	return nil
}

// pureRename moves a file and touches no byte of it.
func pureRename(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "guide.md"),
		"# guide\n\nprose line one.\nprose line two.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "add guide"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "mv", "docs/guide.md", "docs/handbook.md"); err != nil {
		return err
	}

	// Proof: git's own rename detection sees a rename with no edit.
	summary, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "-M", "--summary")
	if err != nil {
		return err
	}
	if !strings.Contains(summary, "rename ") {
		return fmt.Errorf("fixture claims a pure rename but the summary is %q", summary)
	}
	return nil
}

// deletionOnly removes a tracked file and adds nothing.
func deletionOnly(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "old.md"),
		"# old\n\nplain prose.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "add old"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "rm", "-q", "docs/old.md"); err != nil {
		return err
	}

	// Proof: the staged diff is a deletion and nothing else.
	status, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-status")
	if err != nil {
		return err
	}
	if status != "D\tdocs/old.md" {
		return fmt.Errorf("fixture claims a deletion-only change but the staged diff is %q", status)
	}
	return nil
}

// emptyFile stages a file with no bytes in it at all.
func emptyFile(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "EMPTY"), ""); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}

	// Proof: zero bytes on disk, and git's canonical empty-blob hash in the
	// index. Both, because either one alone can be true by accident.
	info, err := os.Stat(filepath.Join(sandbox.Repo, "EMPTY"))
	if err != nil {
		return err
	}
	if info.Size() != 0 {
		return fmt.Errorf("fixture claims an empty file but it holds %d bytes", info.Size())
	}
	entry, err := gitOut(sandbox, sandbox.Repo, "ls-files", "-s", "EMPTY")
	if err != nil {
		return err
	}
	if !strings.Contains(entry, "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391") {
		return fmt.Errorf("fixture claims an empty blob but the index entry is %q", entry)
	}
	return nil
}

// noTrailingNewline stages a file whose last line has no terminator, the case
// every line-oriented diff has to special-case.
func noTrailingNewline(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	path := filepath.Join(sandbox.Repo, "docs", "nonewline.md")
	if err := sandbox.write(path, "# nonewline\n\nlast line without a terminator"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}

	// Proof: the last byte on disk is not a newline, and git says so in the
	// staged diff.
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(content) == 0 || content[len(content)-1] == '\n' {
		return errors.New("fixture claims no trailing newline but the file ends with one")
	}
	diff, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached")
	if err != nil {
		return err
	}
	if !strings.Contains(diff, "\\ No newline at end of file") {
		return errors.New("fixture claims no trailing newline but git's diff does not mark one")
	}
	return nil
}

// crlfContent stages CRLF line endings, the other operand of every
// line-ending canonicalization.
func crlfContent(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "crlf.md"),
		"# crlf\r\n\r\nplain prose with carriage returns.\r\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}

	// Proof: the CR survived into the staged BLOB. A repository configured to
	// normalize on check-in would strip it, and this fixture would then be
	// testing nothing.
	blob, err := gitOut(sandbox, sandbox.Repo, "cat-file", "-p", ":docs/crlf.md")
	if err != nil {
		return err
	}
	if !strings.Contains(blob, "\r\n") {
		return errors.New("fixture claims CRLF content but the staged blob carries no carriage return")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Git mid-operation fixtures
// ---------------------------------------------------------------------------

// divergeOnProse builds two branches that edit the same prose lines, so any
// merge, rebase or cherry-pick between them conflicts deterministically.
func divergeOnProse(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	shared := filepath.Join(sandbox.Repo, "docs", "shared.md")
	if err := sandbox.write(shared, "# shared\n\nshared prose.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "shared base"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "-b", "other"); err != nil {
		return err
	}
	if err := sandbox.write(shared, "# shared\n\nother side prose.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "other side"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "main"); err != nil {
		return err
	}
	if err := sandbox.write(shared, "# shared\n\nmain side prose.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "main side")
}

// resolveConflict writes the resolution and stages it, leaving the repository
// mid-operation with a real candidate to review.
func resolveConflict(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "shared.md"),
		"# shared\n\nresolved prose combining both sides.\n"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "docs/shared.md")
}

// requireMidOperationMarker proves the repository really is mid-operation by
// reading the marker git writes for it, under this worktree's own git dir.
func requireMidOperationMarker(sandbox *Sandbox, marker, operation string) error {
	dir, err := gitDir(sandbox, sandbox.Repo)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, marker)); err != nil {
		return fmt.Errorf("fixture claims a %s in progress but %s is absent: %w", operation, marker, err)
	}
	return nil
}

func mergeInProgress(sandbox *Sandbox) error {
	if err := divergeOnProse(sandbox); err != nil {
		return err
	}
	if err := gitExpectFailure(sandbox, sandbox.Repo, "merge", "other"); err != nil {
		return err
	}
	if err := requireMidOperationMarker(sandbox, "MERGE_HEAD", "merge"); err != nil {
		return err
	}
	if err := resolveConflict(sandbox); err != nil {
		return err
	}
	// Proof: resolving the conflict did NOT end the merge. If it had, the
	// journey would be reviewing an ordinary staged change.
	return requireMidOperationMarker(sandbox, "MERGE_HEAD", "merge")
}

func rebaseInProgress(sandbox *Sandbox) error {
	if err := divergeOnProse(sandbox); err != nil {
		return err
	}
	if err := gitExpectFailure(sandbox, sandbox.Repo, "rebase", "other"); err != nil {
		return err
	}
	dir, err := gitDir(sandbox, sandbox.Repo)
	if err != nil {
		return err
	}
	merge, mergeErr := os.Stat(filepath.Join(dir, "rebase-merge"))
	apply, applyErr := os.Stat(filepath.Join(dir, "rebase-apply"))
	if (mergeErr != nil || !merge.IsDir()) && (applyErr != nil || !apply.IsDir()) {
		return errors.New("fixture claims a rebase in progress but neither rebase-merge nor rebase-apply exists")
	}
	return resolveConflict(sandbox)
}

func cherryPickInProgress(sandbox *Sandbox) error {
	if err := divergeOnProse(sandbox); err != nil {
		return err
	}
	if err := gitExpectFailure(sandbox, sandbox.Repo, "cherry-pick", "other"); err != nil {
		return err
	}
	if err := requireMidOperationMarker(sandbox, "CHERRY_PICK_HEAD", "cherry-pick"); err != nil {
		return err
	}
	if err := resolveConflict(sandbox); err != nil {
		return err
	}
	return requireMidOperationMarker(sandbox, "CHERRY_PICK_HEAD", "cherry-pick")
}

// ---------------------------------------------------------------------------
// Lifecycle fixtures
// ---------------------------------------------------------------------------

// globalModeRecord is the user's uncommitted global state file. Only the two
// fields this fixture needs are modelled; the product tolerates the rest.
type globalModeRecord struct {
	InstalledAgents any    `json:"installed_agents"`
	RDDMode         string `json:"rdd_mode"`
	RDDModeRecorded string `json:"rdd_mode_recorded_at"`
}

// nonsenseGlobalMode writes a kill-switch record that is perfectly READABLE —
// valid JSON, every field present, the right shape — and holds a value that is
// not one of the two the product defines. This is deliberately NOT the corrupt
// path: an unreadable record already fails closed and stays covered elsewhere.
func nonsenseGlobalMode(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := stageProse(sandbox.Repo, "nonsense")(sandbox); err != nil {
		return err
	}
	path := filepath.Join(sandbox.Home, ".gentle-ai", "state.json")
	record := globalModeRecord{RDDMode: "sometimes", RDDModeRecorded: "2026-01-01T00:00:00Z"}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := sandbox.write(path, string(append(encoded, '\n'))); err != nil {
		return err
	}

	// Proof: the file parses cleanly and carries exactly the nonsense value.
	// If this ever stops parsing, the journey would silently become the
	// already-covered corrupt-record case instead.
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var readback globalModeRecord
	if err := json.Unmarshal(content, &readback); err != nil {
		return fmt.Errorf("fixture claims a readable record but it does not parse: %w", err)
	}
	if readback.RDDMode != "sometimes" {
		return fmt.Errorf("fixture claims the value %q but read back %q", "sometimes", readback.RDDMode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Argument builders
// ---------------------------------------------------------------------------

// scratchArgs targets a repository the fixture parked in Scratch rather than
// the journey's current one. The linked-worktree journey needs both.
func scratchArgs(key string, parts ...string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		repo, ok := sandbox.Scratch[key]
		if !ok {
			return nil, fmt.Errorf("no scratch repository named %q", key)
		}
		args := make([]string, 0, len(parts)+2)
		args = append(args, parts...)
		return append(args, "--cwd", repo), nil
	}
}

// ---------------------------------------------------------------------------
// Composite sub-flows
// ---------------------------------------------------------------------------

// gateDenial is the subset of a denied lifecycle gate a recovery needs. The
// recovery inputs come from the product's own denial, never from the journey's
// memory of what it did: following the message literally is the whole point.
type gateDenial struct {
	Result  string `json:"result"`
	Context struct {
		ScopeChange struct {
			PredecessorLineageID  string   `json:"predecessor_lineage_id"`
			PredecessorRevision   string   `json:"predecessor_revision"`
			RecoveryOperation     string   `json:"recovery_operation"`
			RecoveryRequiredInput []string `json:"recovery_required_inputs"`
		} `json:"scope_change"`
	} `json:"context"`
}

// recoverScopeChangeRoundTrip runs the gate, reads the scope-changed denial,
// runs exactly the recovery the denial named, finalizes the successor, and runs
// the gate again. If the named continuation does not actually unblock the flow,
// the second gate stays denied and the journey records it.
func recoverScopeChangeRoundTrip(successor string) func(*journeyRun) error {
	return func(r *journeyRun) error {
		observation := r.run(productArgsFor(r, "review", "validate", "--gate", "pre-commit"), false)
		var denial gateDenial
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &denial); err != nil {
			return fmt.Errorf("parse gate denial: %w (stderr: %s)", err, firstLine(observation.Stderr))
		}
		change := denial.Context.ScopeChange
		if change.PredecessorLineageID == "" || change.PredecessorRevision == "" {
			return fmt.Errorf("gate result %q named no recoverable predecessor", denial.Result)
		}
		r.run(productArgsFor(r, "review", "recover",
			"--predecessor-lineage", change.PredecessorLineageID,
			"--expected-predecessor-revision", change.PredecessorRevision,
			"--successor-lineage", successor,
			"--disposition", "scope_changed"), false)
		r.run(productArgsFor(r, "review", "finalize"), false)
		r.run(productArgsFor(r, "review", "validate", "--gate", "pre-commit"), false)
		return nil
	}
}

// productArgsFor is productArgs for a composite step, which already has the
// sandbox in hand.
func productArgsFor(r *journeyRun, parts ...string) []string {
	return append(append([]string{}, parts...), "--cwd", r.sandbox.Repo)
}

// finalizeAsFailedVerification binds a FAILED final verification, which is the
// only deterministic route to an escalated lineage.
func finalizeAsFailedVerification(r *journeyRun) error {
	path, err := writeScratch(r.sandbox, "failed-verification.txt",
		[]byte("go build ./... ok\ngo test ./... FAIL\n2 packages failed\n"))
	if err != nil {
		return err
	}
	r.run(productArgsFor(r, "review", "finalize", "--evidence", path, "--failed=true"), false)
	return nil
}

// authorityHead is the subset of `review status` a recovery of an escalated
// lineage needs.
type authorityHead struct {
	Status  string `json:"status"`
	Entries []struct {
		LineageID        string `json:"lineage_id"`
		State            string `json:"state"`
		Revision         string `json:"revision"`
		SnapshotIdentity string `json:"snapshot_identity"`
	} `json:"entries"`
}

// entry finds one lineage by name. Journeys that hold more than one authority
// must select by identity, never by position.
func (h authorityHead) entry(lineage string) (string, string, bool) {
	for _, candidate := range h.Entries {
		if candidate.LineageID == lineage {
			return candidate.Revision, candidate.SnapshotIdentity, true
		}
	}
	return "", "", false
}

// recoverEscalated tries the recovery twice on purpose. The first attempt is
// what a developer does reflexively — recover straight away, having changed
// nothing — and the product refuses it. The second is after the candidate is
// actually fixed, which is what the refusal was really asking for. Whether that
// refusal says so is exactly what this measures.
func recoverEscalated(r *journeyRun) error {
	observation := r.run(productArgsFor(r, "review", "status"), false)
	var head authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &head); err != nil {
		return fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if len(head.Entries) == 0 {
		return errors.New("review status listed no authority to recover")
	}
	entry := head.Entries[0]
	if entry.State != "escalated" {
		return fmt.Errorf("expected an escalated lineage, got state %q", entry.State)
	}
	recovery := []string{
		"review", "recover",
		"--predecessor-lineage", entry.LineageID,
		"--expected-predecessor-revision", entry.Revision,
		"--successor-lineage", "review-after-escalation",
		"--disposition", "escalated",
	}
	r.run(productArgsFor(r, recovery...), false)

	// Fix the candidate, exactly as a developer does after a failed
	// verification, then follow the same recovery again.
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, "docs", "escalated.md"),
		"# escalated\n\nplain prose, no executable content.\nthe failure the verification named is fixed.\n"); err != nil {
		return err
	}
	if err := r.sandbox.git(r.sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	r.run(productArgsFor(r, recovery...), false)
	r.run(productArgsFor(r, "review", "finalize"), false)
	r.run(productArgsFor(r, "review", "validate", "--gate", "pre-commit"), false)
	return nil
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

var startContractCapability = &Capability{Verb: []string{"review", "start"}, Flags: []string{"--cwd", "--contract"}}
var recoverCapability = &Capability{Verb: []string{"review", "recover"},
	Flags: []string{"--cwd", "--predecessor-lineage", "--expected-predecessor-revision", "--successor-lineage", "--disposition"}}
var finalizeFailedCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{"--cwd", "--evidence", "--failed"}}
var finalizeCorrectionCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{"--cwd", "--correction-lines"}}
var statusOnlyCapability = &Capability{Verb: []string{"review", "status"}, Flags: []string{"--cwd"}}

// ---------------------------------------------------------------------------
// Corpus
// ---------------------------------------------------------------------------

// edgeJourneys is the second half of the corpus: everything reproducible on
// Linux in a temp directory that the original fourteen never reached. Cases
// that cannot be built deterministically here — network mounts, read-only
// filesystems, full disks, case-insensitive volumes, Windows antivirus and
// long paths, and a clock moving backwards — are flows in
// docs/testing/organic-rdd-testing-guide.md instead, because a flaky journey in
// a loop is worse than no journey: it gets blamed on whatever changed last.
func edgeJourneys() []Journey {
	return []Journey{
		// -------------------------------------------------------------- shape
		{
			ID:     "j15-linked-worktree",
			Title:  "Linked worktree shares the review store: both trees review, approve and gate independently",
			Source: "shape 1 (asymmetric comparison) + shape 5 (two sources of truth about repository identity)",
			// Expected: every step succeeds. One repository, two absolute
			// paths, two HEADs, one shared review store — and each worktree's
			// gate must resolve ITS OWN receipt, not the other one's.
			Steps: []Step{
				{Name: "fixture: linked worktree proven linked", Fixture: linkedWorktree},
				{Name: "fixture: stage docs in the linked worktree", Fixture: func(s *Sandbox) error {
					return stageProse(s.Scratch["worktree"], "worktree")(s)
				}},
				{Name: "review start in the linked worktree", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize in the linked worktree", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate post-apply in the linked worktree", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "post-apply")},
				{Name: "fixture: stage docs in the main worktree", Fixture: func(s *Sandbox) error {
					return stageProse(s.Scratch["main"], "main")(s)
				}},
				{Name: "review start in the main worktree", Requires: startCapability, Args: scratchArgs("main", "review", "start")},
				{Name: "review finalize in the main worktree", Requires: finalizeCapability, Args: scratchArgs("main", "review", "finalize")},
				{Name: "gate pre-commit in the main worktree", Requires: validateCapability, Args: scratchArgs("main", "review", "validate", "--gate", "pre-commit")},
				{Name: "gate pre-commit in the linked worktree again", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j16-detached-head",
			Title:  "Detached HEAD: the whole cycle works with no branch at all",
			Source: "shape 3 (a guard gated on HEAD being a branch, which is not its own precondition)",
			// Expected: every step succeeds. Nothing in a content-bound review
			// needs a branch name.
			Steps: []Step{
				{Name: "fixture: detached HEAD with staged docs", Fixture: detachedHead},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j17-bare-repository",
			Title:  "Bare repository: a refusal that must name what to do instead",
			Source: "shape 4 (a refusal naming nothing runnable) + shape 2 (a shape error read as a state error)",
			// Expected: start is refused — a bare repository legitimately has
			// no candidate. What is measured is whether the refusal names a
			// continuation or leaks raw git plumbing.
			//
			// It names one, and it is by design that the name is not a
			// command. The product cannot know where the operator's checkout
			// is, so the only command it could print is
			// `--cwd <path-to-a-checkout>`, an unfillable template — naming a
			// dead end, which is worse than naming nothing. What it prints
			// instead is the action itself, quoted below and verified against
			// the bytes before the exemption applies.
			Steps: []Step{
				{Name: "fixture: bare repository proven bare", Fixture: bareRepository},
				{Name: "review start in a bare repository", Requires: startCapability,
					Args: productArgs("review", "start"), AbortOnBlock: true,
					ByDesign: &ByDesignDeclaration{
						Shape:      ByDesignOperatorKnowledge,
						NextAction: "run the same command again from a checkout",
					}},
			},
		},
		{
			ID:     "j18-space-and-non-ascii-path",
			Title:  "Repository path with spaces and non-ASCII: the whole cycle works",
			Source: "shape 1 (one operand path-canonicalized, the other not)",
			// Expected: every step succeeds, and the awkward path round-trips
			// through the JSON envelopes unchanged.
			Steps: []Step{
				{Name: "fixture: repository behind a space/non-ASCII path", Fixture: awkwardPath},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j19-submodule-gitlink",
			Title:  "Gitlink in the candidate: a tree entry with no blob behind it",
			Source: "shape 1 (comparing a 160000 entry as if it had content)",
			// Expected: start succeeds and classifies above tier 0 (a
			// .gitmodules change is not passive prose), the lens is captured,
			// and the cycle reaches an approved receipt.
			Steps: []Step{
				{Name: "fixture: submodule staged as a gitlink", Fixture: submoduleCandidate},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
				{Name: "gate post-apply", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "post-apply")},
			},
		},

		// ----------------------------------------------------------- content
		{
			ID:     "j20-symlink-candidate",
			Title:  "Symlink in the candidate: mode 120000 whose blob is a path",
			Source: "shape 1 (link target read as file content on one side only)",
			// Expected: start succeeds, the change is not treated as passive
			// prose, and the cycle reaches an approved receipt.
			Steps: []Step{
				{Name: "fixture: symlink proven recorded as a symlink", Fixture: symlinkCandidate},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
				{Name: "gate post-apply", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "post-apply")},
			},
		},
		{
			ID:     "j21-mode-only-change",
			Title:  "Mode-only change 100644 to 100755: identical blob on both sides",
			Source: "shape 1 (a content-only comparison sees no change at all)",
			// Expected: start succeeds and does NOT report a no-op. An
			// executable permission change is the classic silent privilege
			// escalation, so it must be visible to the review.
			Steps: []Step{
				{Name: "fixture: mode bit proven flipped, zero lines changed", Fixture: modeOnlyChange},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
				{Name: "gate post-apply", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "post-apply")},
			},
		},
		{
			ID:     "j22-pure-rename",
			Title:  "Pure rename: every byte identical, only the path moved",
			Source: "shape 1 (a rename read as one deletion plus one creation)",
			// Expected: every step succeeds and the change stays tier 0.
			Steps: []Step{
				{Name: "fixture: rename proven detected by git", Fixture: pureRename},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j23-deletion-only",
			Title:  "Deletion-only candidate: nothing added anywhere",
			Source: "shape 1 (a candidate whose new side is empty)",
			// Expected: every step succeeds. There is no post-image to inspect
			// and the review must not need one.
			Steps: []Step{
				{Name: "fixture: deletion proven to be the whole staged diff", Fixture: deletionOnly},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j24-empty-file",
			Title:  "Empty file: zero bytes, zero changed lines",
			Source: "shape 4 (a message describing content that is not there)",
			// Expected: start succeeds and fails closed on unknown content
			// rather than waving a zero-byte file through. What is measured is
			// whether the stated reason matches a file with no bytes in it.
			Steps: []Step{
				{Name: "fixture: empty blob proven empty", Fixture: emptyFile},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
				{Name: "gate post-apply", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "post-apply")},
			},
		},
		{
			ID:     "j25-no-trailing-newline",
			Title:  "File with no trailing newline: the last line has no terminator",
			Source: "shape 1 (one side line-terminated, the other not)",
			// Expected: every step succeeds and the change stays tier 0.
			Steps: []Step{
				{Name: "fixture: missing terminator proven by git's own marker", Fixture: noTrailingNewline},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j26-crlf-content",
			Title:  "CRLF content: carriage returns survive into the staged blob",
			Source: "shape 1 (line endings normalized on one operand only)",
			// Expected: every step succeeds and the receipt stays bound to the
			// bytes actually staged, carriage returns included.
			Steps: []Step{
				{Name: "fixture: CRLF proven present in the staged blob", Fixture: crlfContent},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},

		// ------------------------------------------------- git mid-operation
		{
			ID:     "j27-merge-in-progress",
			Title:  "Merge in progress: review a conflict resolution before the merge commit exists",
			Source: "shape 3 (a guard gated on a clean git state rather than on its own precondition)",
			// Expected: every step succeeds. MERGE_HEAD is still present the
			// whole way through.
			Steps: []Step{
				{Name: "fixture: MERGE_HEAD proven present after resolving", Fixture: mergeInProgress},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j28-rebase-in-progress",
			Title:  "Rebase in progress: detached HEAD plus a rebase state directory",
			Source: "shape 3 (a guard gated on a clean git state)",
			// Expected: every step succeeds. A rebase detaches HEAD as well, so
			// this is the two conditions at once.
			Steps: []Step{
				{Name: "fixture: rebase state directory proven present", Fixture: rebaseInProgress},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j29-cherry-pick-in-progress",
			Title:  "Cherry-pick in progress: CHERRY_PICK_HEAD present throughout",
			Source: "shape 3 (a guard gated on a clean git state)",
			// Expected: every step succeeds.
			Steps: []Step{
				{Name: "fixture: CHERRY_PICK_HEAD proven present after resolving", Fixture: cherryPickInProgress},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},

		// ---------------------------------------------------------- lifecycle
		{
			ID:     "j30-kill-switch-flipped-mid-review",
			Title:  "Kill switch flipped between START and FINALIZE: the documented answer is that FINALIZE is refused",
			Source: "shape 3 (the mutate guard sits behind the start path) + shape 5 (rdd_mode.go documents a rejection the code never issues)",
			// Expected, per the product's own documentation: disabling freezes
			// authority READ-ONLY, so `review finalize` — which transitions
			// reviewing to approved and writes a receipt — must be refused with
			// the same typed stop `review start` gets, naming the command that
			// turns reviews back on. `review status` and the gate must keep
			// working, because reads always pass.
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage docs", Fixture: stageProse("", "flipped")},
				{Name: "review start with reviews on", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "mode disable mid-review", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "mode status confirms off", Requires: modeCapability, Args: productArgs("review", "mode", "status", "--json")},
				{Name: "review finalize while reviews are off", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "review status while reviews are off", Requires: statusOnlyCapability, Args: productArgs("review", "status")},
				{Name: "gate pre-commit while reviews are off", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j31-nonsense-mode-value",
			Title:  "Kill-switch record readable but holding a value that is not on/off",
			Source: "shape 2 (a correctable input read as terminal) + shape 4 (a refusal naming nothing runnable)",
			// Expected: it fails CLOSED — reviews off — because an unknown
			// value must never be read as permission. And, because a refusal
			// that exits non-zero and names no runnable continuation is the one
			// shape this project says it does not ship, the refusal must name
			// how to put the record back to a value it understands.
			Steps: []Step{
				{Name: "fixture: readable record holding \"sometimes\"", Fixture: nonsenseGlobalMode},
				{Name: "mode status with a nonsense value", Requires: modeCapability, Args: productArgs("review", "mode", "status", "--json")},
				{Name: "review start with a nonsense value", Requires: startCapability,
					Args: productArgs("review", "start"), AbortOnBlock: true},
			},
		},
		{
			ID:     "j32-recovery-of-a-recovery",
			Title:  "Recovery of a recovery: the named continuation has to work twice",
			Source: "shape 4 (guide flow 9 step 7 — the message named a recovery that landed you back at the same denial)",
			// Expected: both recoveries succeed and each ends at `allow`. The
			// inputs are read out of the product's own denial envelope, never
			// remembered from earlier steps, so a message that names inputs it
			// does not actually accept fails here.
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage first docs", Fixture: stageProse("", "one")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "fixture: widen the candidate scope", Fixture: stageProse("", "two")},
				{Name: "first recovery, following exactly what the gate named",
					Requires: recoverCapability, Composite: recoverScopeChangeRoundTrip("review-successor-one")},
				{Name: "fixture: widen the candidate scope again", Fixture: stageProse("", "three")},
				{Name: "second recovery, of the successor the first one created",
					Requires: recoverCapability, Composite: recoverScopeChangeRoundTrip("review-successor-two")},
			},
		},
		{
			ID:     "j33-escalate-then-recover",
			Title:  "Escalate on a failed verification, then recover once the candidate is fixed",
			Source: "shape 2 (an escalation is recoverable, not terminal) + shape 4 (the refusal must say the candidate has to change)",
			// Expected: finalize with failed evidence escalates; the gate
			// denies; recovering with nothing changed is refused and that
			// refusal must say WHY; recovering after the candidate is fixed
			// succeeds and the gate ends at `allow`.
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage docs", Fixture: stageProse("", "escalated")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "finalize binding a FAILED verification", Requires: finalizeFailedCapability, Composite: finalizeAsFailedVerification},
				// The denial names the recover command with both derivable
				// selectors already filled in, and leaves exactly one hole:
				// the successor's name. That is the operator's to choose --
				// the CLI help says so in as many words -- so the product
				// cannot print a command that runs as typed without inventing
				// a name on their behalf. The command is unrunnable by one
				// value, and it is the one value only the operator has.
				{Name: "gate pre-commit on an escalated lineage", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"),
					ByDesign: &ByDesignDeclaration{
						Shape:      ByDesignOperatorKnowledge,
						NextAction: "change the candidate and review the fix as a successor",
					}},
				{Name: "recover before and after fixing the candidate", Requires: recoverCapability, Composite: recoverEscalated},
			},
		},
		{
			ID:     "j34-abandon-then-start-again",
			Title:  "Abandon a lineage, then start a fresh review of the same candidate",
			Source: "shape 2 (an abandoned lineage must not poison the repository permanently)",
			// Expected: after the abandonment the same candidate starts a new
			// review, finalizes and gates to `allow`. A quarantined lineage
			// still sitting in the shared store must not be discovered as
			// authority for the new one.
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage docs", Fixture: stageProse("", "abandoned")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "abandon a pristine lineage", Requires: abandonCapability, Composite: abandonPristineLineage},
				{Name: "review start again after the abandonment", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "j35-correction-budget-exactly-zero",
			Title:  "Correction budget of exactly zero: forecasting a correction against it",
			Source: "shape 3 (the budget check sitting behind a different precondition) + shape 4 (guide flow 17 promises spent/remaining/total)",
			// Expected: a mode-only change has zero changed lines, so
			// min(200, ceil(0/2)) is a budget of 0 — no correction is
			// affordable. Forecasting one line of correction against it must
			// say so, with spent, remaining and total, rather than answering a
			// different question.
			Steps: []Step{
				{Name: "fixture: mode-only change, zero changed lines", Fixture: modeOnlyChange},
				{Name: "review start reporting correction_budget 0", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "forecast one correction line against a budget of zero", Requires: finalizeCorrectionCapability,
					Args: productArgs("review", "finalize", "--correction-lines", "1")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
			},
		},

		// ----------------------------------------------------------- contract
		{
			ID:     "j36-contract-right-name-wrong-version",
			Title:  "Negotiated contract with the right name and a version this build does not have",
			Source: "shape 2 (a correctable request read as terminal) + shape 4 (the continuation must be runnable, not a category)",
			// Expected: a typed `unsupported_contract` refusal that names the
			// version this build DOES support, so the caller can correct the
			// request in one step instead of guessing.
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage docs", Fixture: stageProse("", "contract")},
				{Name: "review start with contract version v2", Requires: startContractCapability,
					Args:         productArgs("review", "start", "--contract", "gentle-ai.review-integration/v2"),
					AbortOnBlock: true},
			},
		},
	}
}
