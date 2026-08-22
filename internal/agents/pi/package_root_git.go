package pi

import (
	"net/url"
	"path/filepath"
	"strings"
	"unicode"
)

type gitSourceParts struct {
	host, repo, ref string
	hasRef          bool
}

func resolveGitPackageRoot(selection PackageSourceSelection) (string, bool, error) {
	if !gitSourceCandidate(selection.Source) {
		return "", false, nil
	}
	parts, kind, cause := parseGitSource(selection.Source)
	if kind != "" {
		return "", true, sourceError(selection, kind, ErrInvalidPackageSource, cause)
	}
	if !validGitHost(parts.host) {
		return "", true, sourceError(selection, "unsafe-git-host", ErrInvalidPackageSource, nil)
	}
	repo, ok := cleanGitRepo(parts.repo)
	if !ok {
		return "", true, sourceError(selection, "unsafe-git-path", ErrInvalidPackageSource, nil)
	}
	parts.repo = repo
	if parts.hasRef && !validGitRef(parts.ref) {
		return "", true, sourceError(selection, "unsafe-git-ref", ErrInvalidPackageSource, nil)
	}
	managed := selection.AgentDir
	if selection.Scope == SettingsScopeProject {
		managed = filepath.Join(selection.CWD, ".pi")
	}
	install := filepath.Join(managed, "git")
	// Pi canonicalizes known hosted-git-info domains, preserves generic host and
	// repository casing, and ignores refs in the installed root.
	root := filepath.Join(install, installedGitHost(parts.host), filepath.FromSlash(parts.repo))
	if !filepath.IsAbs(root) || root != filepath.Clean(root) || !pathWithin(install, root) {
		return "", true, sourceError(selection, "unsafe-package-root", ErrInvalidPackageSource, nil)
	}
	return root, true, nil
}

func gitSourceCandidate(source string) bool {
	lower := strings.ToLower(source)
	return strings.HasPrefix(lower, "git:") || strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "ssh://") ||
		strings.HasPrefix(lower, "git://")
}

func parseGitSource(source string) (gitSourceParts, string, error) {
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "git:") && !strings.HasPrefix(lower, "git://") {
		return parseGitShorthand(source[4:])
	}
	return parseGitURL(source)
}

func parseGitShorthand(value string) (gitSourceParts, string, error) {
	fail := func(kind string) (gitSourceParts, string, error) { return gitSourceParts{}, kind, nil }
	if value == "" || strings.ContainsRune(value, 0) || strings.ContainsAny(value, "?#") || strings.Contains(value, "://") {
		return fail("invalid-git")
	}
	var host, repoRef string
	at, colon := strings.IndexByte(value, '@'), strings.IndexByte(value, ':')
	slash := strings.IndexByte(value, '/')
	if at > 0 && colon > at && (slash < 0 || colon < slash) {
		if value[:at] != "git" {
			return fail("invalid-git")
		}
		host, repoRef = value[at+1:colon], value[colon+1:]
	} else {
		if slash <= 0 {
			return fail("invalid-git")
		}
		host, repoRef = value[:slash], value[slash+1:]
		if strings.ContainsRune(host, ':') {
			return fail("invalid-git")
		}
	}
	repo, ref, hasRef := splitGitRef(repoRef)
	return gitSourceParts{host: host, repo: repo, ref: ref, hasRef: hasRef}, "", nil
}

func parseGitURL(value string) (gitSourceParts, string, error) {
	fail := func(kind string) (gitSourceParts, string, error) { return gitSourceParts{}, kind, nil }
	authority := value[strings.Index(value, "://")+3:]
	if end := strings.IndexAny(authority, "/?#"); end >= 0 {
		authority = authority[:end]
	}
	if strings.ContainsRune(authority, '%') {
		return fail("invalid-git")
	}
	u, err := url.Parse(value)
	if err != nil {
		return gitSourceParts{}, "invalid-git", err
	}
	scheme := strings.ToLower(u.Scheme)
	if u.Opaque != "" || u.Host == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fail("invalid-git")
	}
	if strings.ContainsAny(u.Host, "%/\\?#:") || (u.User != nil && strings.Contains(u.User.String(), "%")) {
		return fail("invalid-git")
	}
	switch scheme {
	case "http", "https":
		if u.User != nil {
			return fail("invalid-git")
		}
	case "ssh":
		if u.User == nil || u.User.Username() != "git" {
			return fail("invalid-git")
		}
		if _, set := u.User.Password(); set {
			return fail("invalid-git")
		}
	case "git":
		if u.User != nil {
			return fail("invalid-git")
		}
	default:
		return fail("invalid-git")
	}
	path := u.EscapedPath()
	if path == "" || strings.HasPrefix(path, "//") || strings.ContainsRune(path, '\\') {
		return fail("unsafe-git-path")
	}
	rawRepo, rawRef, hasRef := splitGitRef(strings.TrimPrefix(path, "/"))
	if gitEncodedHazard(rawRepo) {
		return fail("unsafe-git-path")
	}
	if hasRef && gitEncodedHazard(rawRef) {
		return fail("unsafe-git-ref")
	}
	path, err = url.PathUnescape(path)
	if err != nil {
		return gitSourceParts{}, "invalid-git", err
	}
	return parseGitURLPath(u.Hostname(), strings.TrimPrefix(path, "/"))
}

func parseGitURLPath(host, repoRef string) (gitSourceParts, string, error) {
	if host == "" || repoRef == "" {
		return gitSourceParts{}, "invalid-git", nil
	}
	repo, ref, hasRef := splitGitRef(repoRef)
	return gitSourceParts{host: host, repo: repo, ref: ref, hasRef: hasRef}, "", nil
}

func splitGitRef(value string) (string, string, bool) {
	at := strings.IndexByte(value, '@')
	if at < 0 {
		return value, "", false
	}
	return value[:at], value[at+1:], true
}

func installedGitHost(host string) string {
	switch lower := strings.ToLower(host); lower {
	case "github.com", "gist.github.com", "bitbucket.org", "gitlab.com", "git.sr.ht":
		return lower
	default:
		return host
	}
}

func validGitHost(host string) bool {
	if host == "" || len(host) > 253 || strings.HasSuffix(host, ".") || gitEncodedHazard(host) {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !isGitAlphaNum(r) && r != '-' {
				return false
			}
		}
	}
	return true
}

func cleanGitRepo(repo string) (string, bool) {
	if strings.HasSuffix(repo, ".git") {
		repo = strings.TrimSuffix(repo, ".git")
	}
	parts := strings.Split(repo, "/")
	if len(parts) < 2 || repo == "" || strings.HasPrefix(repo, "/") || strings.HasSuffix(repo, "/") {
		return "", false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || !validGitComponent(part) {
			return "", false
		}
	}
	return strings.Join(parts, "/"), true
}

func validGitRef(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") ||
		strings.HasPrefix(ref, ".") || strings.HasSuffix(ref, ".") || strings.Contains(ref, "..") ||
		strings.Contains(ref, "@{") || strings.Contains(ref, "//") || gitEncodedHazard(ref) {
		return false
	}
	for _, part := range strings.Split(ref, "/") {
		if part == "" || strings.HasPrefix(part, "-") || strings.HasPrefix(part, ".") ||
			strings.HasSuffix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
		for _, r := range part {
			if unicode.IsSpace(r) || r < 0x20 || r == 0x7f || !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._+-", r) {
				return false
			}
		}
	}
	return true
}

func validGitComponent(value string) bool {
	for _, r := range value {
		if !isGitAlphaNum(r) && !strings.ContainsRune("._+-~", r) {
			return false
		}
	}
	return true
}

func isGitAlphaNum(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func gitEncodedHazard(value string) bool {
	value = strings.ToLower(value)
	for _, code := range []string{"%00", "%2e", "%2f", "%5c", "%3a", "%3f", "%23", "%40"} {
		if strings.Contains(value, code) {
			return true
		}
	}
	return false
}
