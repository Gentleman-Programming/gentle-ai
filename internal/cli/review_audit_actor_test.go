package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewAuditActorRepoWithoutIdentity creates a repository with no local Git
// identity and isolates it from the host machine's own global/system Git
// config, so the fallback chain below user.name/user.email can be exercised
// deterministically regardless of the developer's or CI runner's own
// ~/.gitconfig.
func reviewAuditActorRepoWithoutIdentity(t *testing.T) string {
	t.Helper()
	isolatedHome := t.TempDir()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(isolatedHome, "unused-gitconfig"))
	repo := t.TempDir()
	runReviewCLIGit(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	reviewAuditActorCommitWithEnvIdentity(t, repo)
	return repo
}

// reviewAuditActorCommitWithEnvIdentity commits using an env-supplied
// identity so the commit succeeds without ever writing local git config,
// keeping user.name/user.email genuinely unset for the repository.
func reviewAuditActorCommitWithEnvIdentity(t *testing.T, repo string) {
	t.Helper()
	command := exec.Command("git", "-C", repo, "commit", "-qm", "base")
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Isolated Test", "GIT_AUTHOR_EMAIL=isolated@example.com",
		"GIT_COMMITTER_NAME=Isolated Test", "GIT_COMMITTER_EMAIL=isolated@example.com",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
	}
}

func appendReviewAuditActorRawGitConfig(t *testing.T, repo, content string) {
	t.Helper()
	path := filepath.Join(repo, ".git", "config")
	existing, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(existing, []byte(content)...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReviewAuditActorFallbackChain(t *testing.T) {
	ctx := context.Background()

	t.Run("uses name and email when both configured", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		runReviewCLIGit(t, repo, "config", "user.name", "Ada Lovelace")
		runReviewCLIGit(t, repo, "config", "user.email", "ada@example.com")
		if got, want := reviewAuditActor(ctx, repo), "Ada Lovelace <ada@example.com>"; got != want {
			t.Fatalf("reviewAuditActor() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to email alone when name is unset", func(t *testing.T) {
		repo := reviewAuditActorRepoWithoutIdentity(t)
		runReviewCLIGit(t, repo, "config", "user.email", "ada@example.com")
		t.Setenv("GIT_AUTHOR_EMAIL", "")
		t.Setenv("GIT_COMMITTER_EMAIL", "")
		if got, want := reviewAuditActor(ctx, repo), "ada@example.com"; got != want {
			t.Fatalf("reviewAuditActor() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to GIT_AUTHOR_EMAIL when git identity is entirely unset", func(t *testing.T) {
		repo := reviewAuditActorRepoWithoutIdentity(t)
		t.Setenv("GIT_AUTHOR_EMAIL", "author@example.com")
		t.Setenv("GIT_COMMITTER_EMAIL", "")
		if got, want := reviewAuditActor(ctx, repo), "author@example.com"; got != want {
			t.Fatalf("reviewAuditActor() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to GIT_COMMITTER_EMAIL when GIT_AUTHOR_EMAIL is also unset", func(t *testing.T) {
		repo := reviewAuditActorRepoWithoutIdentity(t)
		t.Setenv("GIT_AUTHOR_EMAIL", "")
		t.Setenv("GIT_COMMITTER_EMAIL", "committer@example.com")
		if got, want := reviewAuditActor(ctx, repo), "committer@example.com"; got != want {
			t.Fatalf("reviewAuditActor() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to the fixed literal actor when nothing is configured", func(t *testing.T) {
		repo := reviewAuditActorRepoWithoutIdentity(t)
		t.Setenv("GIT_AUTHOR_EMAIL", "")
		t.Setenv("GIT_COMMITTER_EMAIL", "")
		if got, want := reviewAuditActor(ctx, repo), reviewSelfRecoveryFallbackActor; got != want {
			t.Fatalf("reviewAuditActor() = %q, want %q", got, want)
		}
	})

	t.Run("resolved root is identical across absolute and nested cwd forms", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		runReviewCLIGit(t, repo, "config", "user.name", "Ada Lovelace")
		runReviewCLIGit(t, repo, "config", "user.email", "ada@example.com")
		nested := filepath.Join(repo, "nested")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		want := "Ada Lovelace <ada@example.com>"
		for name, cwd := range map[string]string{"absolute": repo, "nested": nested} {
			t.Run(name, func(t *testing.T) {
				root, err := (reviewtransaction.SnapshotBuilder{Repo: cwd}).ResolveRepositoryRoot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if got := reviewAuditActor(ctx, root); got != want {
					t.Fatalf("reviewAuditActor() = %q, want %q", got, want)
				}
			})
		}
		t.Run("relative", func(t *testing.T) {
			t.Chdir(repo)
			root, err := (reviewtransaction.SnapshotBuilder{Repo: "."}).ResolveRepositoryRoot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got := reviewAuditActor(ctx, root); got != want {
				t.Fatalf("reviewAuditActor() = %q, want %q", got, want)
			}
		})
	})
}

func TestReviewAuditActorRejectsNewlineInjectedIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("poisoned email falls through every remaining candidate to the literal fallback", func(t *testing.T) {
		repo := reviewAuditActorRepoWithoutIdentity(t)
		appendReviewAuditActorRawGitConfig(t, repo, "[user]\n\tname = Ada Lovelace\n\temail = \"evil@example.com\\nforged=binding-line\"\n")
		// Positive control: prove the raw config value genuinely contains an
		// embedded newline, so this test exercises rejection, not merely a
		// missing/absent value.
		if raw := reviewGitConfigValue(ctx, repo, "user.email"); !reviewAuditActorHasNewline(raw) {
			t.Fatalf("test setup did not produce a newline-poisoned git config value: %q", raw)
		}
		t.Setenv("GIT_AUTHOR_EMAIL", "")
		t.Setenv("GIT_COMMITTER_EMAIL", "")
		if got, want := reviewAuditActor(ctx, repo), reviewSelfRecoveryFallbackActor; got != want {
			t.Fatalf("reviewAuditActor() = %q, want %q (must reject the newline-poisoned identity and fall through)", got, want)
		}
	})

	t.Run("poisoned name falls through to a safe email alone", func(t *testing.T) {
		repo := reviewAuditActorRepoWithoutIdentity(t)
		appendReviewAuditActorRawGitConfig(t, repo, "[user]\n\tname = \"Ada\\nX-Forged: header\"\n\temail = ada@example.com\n")
		if got, want := reviewAuditActor(ctx, repo), "ada@example.com"; got != want {
			t.Fatalf("reviewAuditActor() = %q, want %q (poisoned name must fall through to email alone)", got, want)
		}
	})

	t.Run("poisoned GIT_AUTHOR_EMAIL falls through to GIT_COMMITTER_EMAIL", func(t *testing.T) {
		repo := reviewAuditActorRepoWithoutIdentity(t)
		t.Setenv("GIT_AUTHOR_EMAIL", "evil@example.com\nforged=binding-line")
		t.Setenv("GIT_COMMITTER_EMAIL", "committer@example.com")
		if got, want := reviewAuditActor(ctx, repo), "committer@example.com"; got != want {
			t.Fatalf("reviewAuditActor() = %q, want %q", got, want)
		}
	})
}

func reviewAuditActorHasNewline(value string) bool {
	for _, r := range value {
		if r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}
