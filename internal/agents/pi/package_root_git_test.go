package pi

import (
	"errors"
	"net/url"
	"path/filepath"
	"testing"
)

func TestResolvePackageRootGitSources(t *testing.T) {
	base := t.TempDir()
	cases := []struct {
		name, source, repo string
		scope              PackageSourceScope
	}{
		{"git shorthand project", "git:github.com/team/repo@v1", "github.com/team/repo", SettingsScopeProject},
		{"git scp shorthand user", "git:git@GITHUB.com:Team/Repo.git@release/v1", "github.com/Team/Repo", SettingsScopeUser},
		{"generic host preserves casing", "git:EXAMPLE.com/Team/Repo@v1", "EXAMPLE.com/Team/Repo", SettingsScopeProject},
		{"https project", "https://github.com/team/repo.git@v1.2.3", "github.com/team/repo", SettingsScopeProject},
		{"ssh user", "ssh://git@github.com/team/repo@feature/fix-1", "github.com/team/repo", SettingsScopeUser},
		{"http project", "http://github.com/team/repo@v1", "github.com/team/repo", SettingsScopeProject},
		{"git protocol user without ref", "git://github.com/team/repo", "github.com/team/repo", SettingsScopeUser},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := rootSelection(base, tt.source, tt.scope)
			managed := s.AgentDir
			if tt.scope == SettingsScopeProject {
				managed = filepath.Join(s.CWD, ".pi")
			}
			assertRoot(t, s, filepath.Join(managed, "git", filepath.FromSlash(tt.repo)))
		})
	}
}

func TestResolvePackageRootGitRefsShareInstalledRoot(t *testing.T) {
	base := t.TempDir()
	first := rootSelection(base, "git:github.com/team/repo@v1", SettingsScopeProject)
	second := rootSelection(base, "git:github.com/team/repo@v2", SettingsScopeProject)
	want := filepath.Join(first.CWD, ".pi", "git", "github.com", "team", "repo")
	assertRoot(t, first, want)
	assertRoot(t, second, want)
}

func TestResolvePackageRootGitRejectsUnsafeSources(t *testing.T) {
	base := t.TempDir()
	cases := []struct {
		name, source, kind string
	}{
		{"empty git source", "git:", "invalid-git"},
		{"empty authority", "git:/team/repo@v1", "invalid-git"},
		{"bad host label", "git:-github.com/team/repo@v1", "unsafe-git-host"},
		{"empty host label", "git:github..com/team/repo@v1", "unsafe-git-host"},
		{"underscore host label", "git:github_com/team/repo@v1", "unsafe-git-host"},
		{"scp user is not git", "git:deploy@github.com:team/repo@v1", "invalid-git"},
		{"scp authority is missing user", "git:github.com:team/repo@v1", "invalid-git"},
		{"http credentials", "https://user:pass@github.com/team/repo@v1", "invalid-git"},
		{"ssh user is not git", "ssh://root@github.com/team/repo@v1", "invalid-git"},
		{"ssh password", "ssh://git:secret@github.com/team/repo@v1", "invalid-git"},
		{"port", "https://github.com:443/team/repo@v1", "invalid-git"},
		{"encoded authority", "https://github%2ecom/team/repo@v1", "invalid-git"},
		{"encoded ssh user", "ssh://g%69t@github.com/team/repo@v1", "invalid-git"},
		{"empty path component", "git:github.com//team/repo@v1", "unsafe-git-path"},
		{"dot path component", "git:github.com/team/../repo@v1", "unsafe-git-path"},
		{"backslash path", `git:github.com/team\repo@v1`, "unsafe-git-path"},
		{"encoded separator", "git:github.com/team/repo%2Fchild@v1", "unsafe-git-path"},
		{"encoded traversal", "https://github.com/team/repo%2e%2e@v1", "unsafe-git-path"},
		{"empty repository", "git:github.com/team/@v1", "unsafe-git-path"},
		{"query", "git:github.com/team/repo?query@v1", "invalid-git"},
		{"fragment", "https://github.com/team/repo#v1", "invalid-git"},
		{"empty ref", "git:github.com/team/repo@", "unsafe-git-ref"},
		{"ref traversal", "git:github.com/team/repo@../v1", "unsafe-git-ref"},
		{"ref leading slash", "git:github.com/team/repo@/v1", "unsafe-git-ref"},
		{"ref trailing slash", "git:github.com/team/repo@v1/", "unsafe-git-ref"},
		{"ref repeated separator", "git:github.com/team/repo@feature//x", "unsafe-git-ref"},
		{"ref tilde", "git:github.com/team/repo@feature~x", "unsafe-git-ref"},
		{"ref caret", "git:github.com/team/repo@feature^x", "unsafe-git-ref"},
		{"ref colon", "git:github.com/team/repo@feature:x", "unsafe-git-ref"},
		{"ref glob", "git:github.com/team/repo@feature*x", "unsafe-git-ref"},
		{"ref backslash", `git:github.com/team/repo@feature\\x`, "unsafe-git-ref"},
		{"ref at brace", "git:github.com/team/repo@feature@{x}", "unsafe-git-ref"},
		{"ref control", "git:github.com/team/repo@feature\x01", "unsafe-git-ref"},
		{"ref lock suffix", "git:github.com/team/repo@feature.lock", "unsafe-git-ref"},
		{"ref leading dot", "git:github.com/team/repo@.hidden", "unsafe-git-ref"},
		{"ref trailing dot", "git:github.com/team/repo@branch.", "unsafe-git-ref"},
		{"ref whitespace", "git:github.com/team/repo@feature branch", "unsafe-git-ref"},
		{"encoded ref separator", "https://github.com/team/repo@feature%2Fx", "unsafe-git-ref"},
		{"dot git repository", "git:github.com/team/.git@v1", "unsafe-git-path"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := rootSelection(base, tt.source, SettingsScopeProject)
			before := s
			tree := snapshotPiTree(t, filepath.Dir(s.CWD))
			_, err := ResolvePackageRoot(s)
			if s != before || snapshotPiTree(t, filepath.Dir(s.CWD)) != tree {
				t.Fatal("selection or filesystem changed on failure")
			}
			wantSourceError(t, err, tt.source, tt.kind, ErrInvalidPackageSource)
		})
	}
}

func TestResolvePackageRootGitPreservesParseCause(t *testing.T) {
	base := t.TempDir()
	_, err := ResolvePackageRoot(rootSelection(base, "https://github.com/team/repo%zz@v1", SettingsScopeProject))
	wantSourceError(t, err, "https://github.com/team/repo%zz@v1", "invalid-git", ErrInvalidPackageSource)
	var escape url.EscapeError
	if !errors.As(err, &escape) {
		t.Fatalf("error = %v; want URL escape cause", err)
	}
}

func TestResolvePackageRootGitLeavesUndocumentedFormsUnsupported(t *testing.T) {
	base := t.TempDir()
	for _, source := range []string{
		"git@github.com:team/repo@v1",
		"github.com/team/repo@v1",
		"git+ssh://git@github.com/team/repo@v1",
	} {
		t.Run(source, func(t *testing.T) {
			_, err := ResolvePackageRoot(rootSelection(base, source, SettingsScopeProject))
			wantSourceError(t, err, source, "unsupported-source", ErrUnsupportedPackageSource)
		})
	}
}
