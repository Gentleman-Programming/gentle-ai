package pi

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrInvalidPackageSource     = errors.New("invalid package source")
	ErrUnsupportedPackageSource = errors.New("unsupported package source")
	npmNamePattern              = regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)
	npmVersionPattern           = regexp.MustCompile(`^[~^]?[0-9]+(\.[0-9]+){0,2}(-[0-9a-z-]+(\.[0-9a-z-]+)*)?(\+[0-9a-z-]+(\.[0-9a-z-]+)*)?$`)
)

type PackageRoot struct {
	Path, Source string
	Scope        PackageSourceScope
}
type SourceError struct {
	Source, Kind string
	Cause        error
}

func (e *SourceError) Error() string {
	return fmt.Sprintf("Pi source error (%s) for %q: %v", e.Kind, e.Source, e.Cause)
}
func (e *SourceError) Unwrap() error { return e.Cause }

type packageRootHandler func(PackageSourceSelection) (root string, matched bool, err error)

// Keep remote rejection last so a future handler (for example git) can be
// inserted before it without changing npm or local semantics.
var packageRootHandlers = []packageRootHandler{resolveNPMPackageRoot, resolveLocalPackageRoot, rejectUnsupportedPackageRoot}

// ResolvePackageRoot is lexical and read-only: it does not reread settings or
// touch the filesystem. Ordered handlers make new source grammars additive.
func ResolvePackageRoot(selection PackageSourceSelection) (PackageRoot, error) {
	if selection.Source == "" {
		return PackageRoot{}, sourceError(selection, "empty-source", ErrInvalidPackageSource, nil)
	}
	if selection.Scope != SettingsScopeProject && selection.Scope != SettingsScopeUser {
		return PackageRoot{}, sourceError(selection, "invalid-scope", ErrInvalidPackageSource, nil)
	}
	for _, path := range []string{selection.CWD, selection.AgentDir, selection.SettingsPath} {
		if path == "" || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) || path != filepath.Clean(path) {
			return PackageRoot{}, sourceError(selection, "invalid-selection-path", ErrInvalidPackageSource, nil)
		}
	}
	expected := selection.AgentDir
	if selection.Scope == SettingsScopeProject {
		expected = filepath.Join(selection.CWD, ".pi")
	}
	if selection.SettingsPath == expected || !pathWithin(expected, selection.SettingsPath) {
		return PackageRoot{}, sourceError(selection, "invalid-selection-settings", ErrInvalidPackageSource, nil)
	}
	for _, handler := range packageRootHandlers {
		root, matched, err := handler(selection)
		if !matched {
			continue
		}
		if err != nil {
			return PackageRoot{}, err
		}
		return PackageRoot{Path: filepath.Clean(root), Source: selection.Source, Scope: selection.Scope}, nil
	}
	return PackageRoot{}, sourceError(selection, "invalid-source", ErrInvalidPackageSource, nil)
}

func sourceError(selection PackageSourceSelection, kind string, sentinel, cause error) *SourceError {
	if cause == nil {
		cause = sentinel
	} else {
		cause = fmt.Errorf("%w: %w", sentinel, cause)
	}
	return &SourceError{Source: selection.Source, Kind: kind, Cause: cause}
}
func resolveNPMPackageRoot(selection PackageSourceSelection) (string, bool, error) {
	spec := selection.Source
	if strings.HasPrefix(spec, "npm:") {
		spec = spec[4:]
	} else if !npmCandidate(spec) {
		return "", false, nil
	}
	if spec == "" || strings.ContainsAny(spec, "?#") {
		return "", true, sourceError(selection, "invalid-npm", ErrInvalidPackageSource, nil)
	}
	name, version, at := spec, "", -1
	if strings.HasPrefix(spec, "@") {
		slash := strings.IndexByte(spec, '/')
		if slash < 2 {
			return "", true, sourceError(selection, "unsafe-npm-name", ErrInvalidPackageSource, nil)
		}
		if suffix := strings.IndexByte(spec[slash+1:], '@'); suffix >= 0 {
			at = slash + 1 + suffix
		}
	} else {
		at = strings.IndexByte(spec, '@')
	}
	if at >= 0 {
		name, version = spec[:at], spec[at+1:]
		if version == "" {
			return "", true, sourceError(selection, "empty-npm-version", ErrInvalidPackageSource, nil)
		}
	}
	scoped, parts := strings.HasPrefix(name, "@"), strings.Split(name, "/")
	if (scoped && len(parts) != 2) || (!scoped && len(parts) != 1) {
		return "", true, sourceError(selection, "unsafe-npm-name", ErrInvalidPackageSource, nil)
	}
	for i, part := range parts {
		if scoped && i == 0 {
			part = part[1:]
		}
		if !npmNamePattern.MatchString(part) {
			return "", true, sourceError(selection, "unsafe-npm-name", ErrInvalidPackageSource, nil)
		}
	}
	if version != "" && !npmVersionPattern.MatchString(version) {
		return "", true, sourceError(selection, "unsafe-npm-version", ErrInvalidPackageSource, nil)
	}
	managed := selection.AgentDir
	if selection.Scope == SettingsScopeProject {
		managed = filepath.Join(selection.CWD, ".pi")
	}
	install := filepath.Join(managed, "npm", "node_modules")
	root := filepath.Join(install, filepath.FromSlash(name))
	if !pathWithin(install, root) {
		return "", true, sourceError(selection, "unsafe-package-root", ErrInvalidPackageSource, nil)
	}
	return root, true, nil
}
func npmCandidate(source string) bool {
	return strings.HasPrefix(source, "@") || !strings.ContainsAny(source, "/\\:")
}
func resolveLocalPackageRoot(selection PackageSourceSelection) (string, bool, error) {
	if strings.HasPrefix(strings.ToLower(selection.Source), "file:") {
		return resolveFilePackageRoot(selection)
	}
	if isUnsupportedPackageSource(selection.Source) {
		return "", false, nil
	}
	if strings.ContainsAny(selection.Source, "?#") {
		return "", true, sourceError(selection, "unsafe-local", ErrInvalidPackageSource, nil)
	}
	if filepath.IsAbs(selection.Source) {
		if root, ok := cleanLocalPath(selection.Source, true); ok {
			return root, true, nil
		}
		return "", true, sourceError(selection, "unsafe-local", ErrInvalidPackageSource, nil)
	}
	if !strings.HasPrefix(selection.Source, "./") || len(selection.Source) == 2 {
		return "", true, sourceError(selection, "invalid-local", ErrInvalidPackageSource, nil)
	}
	relative := strings.TrimPrefix(selection.Source, "./")
	if _, ok := cleanLocalPath(relative, false); !ok {
		return "", true, sourceError(selection, "unsafe-local", ErrInvalidPackageSource, nil)
	}
	base := filepath.Dir(selection.SettingsPath)
	root := filepath.Join(base, filepath.FromSlash(relative))
	if !pathWithin(base, root) {
		return "", true, sourceError(selection, "unsafe-local", ErrInvalidPackageSource, nil)
	}
	return root, true, nil
}
func resolveFilePackageRoot(selection PackageSourceSelection) (string, bool, error) {
	value := selection.Source
	if strings.ContainsAny(value, "?#") {
		return "", true, sourceError(selection, "invalid-file-url", ErrInvalidPackageSource, nil)
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", true, sourceError(selection, "invalid-file-url", ErrInvalidPackageSource, err)
	}
	if u.User != nil || (u.Host != "" && !strings.EqualFold(u.Hostname(), "localhost")) || u.Port() != "" {
		return "", true, sourceError(selection, "invalid-file-url", ErrInvalidPackageSource, nil)
	}
	escaped := u.EscapedPath()
	if escaped == "" || encodedLocalHazard(escaped) || strings.ContainsRune(escaped, '\\') {
		return "", true, sourceError(selection, "unsafe-file-url", ErrInvalidPackageSource, nil)
	}
	path, err := url.PathUnescape(escaped)
	if err != nil {
		return "", true, sourceError(selection, "invalid-file-url", ErrInvalidPackageSource, err)
	}
	if filepath.VolumeName(strings.TrimPrefix(path, "/")) != "" {
		path = strings.TrimPrefix(path, "/")
	}
	if root, ok := cleanLocalPath(path, true); ok {
		return root, true, nil
	}
	return "", true, sourceError(selection, "unsafe-file-url", ErrInvalidPackageSource, nil)
}
func cleanLocalPath(value string, absolute bool) (string, bool) {
	if value == "" || strings.ContainsRune(value, 0) || strings.ContainsRune(value, '%') ||
		(filepath.Separator != '\\' && strings.ContainsRune(value, '\\')) || absolute != filepath.IsAbs(value) {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(value), "/")
	for i, part := range parts {
		if part == "" {
			if absolute && (i == 0 || i == len(parts)-1) {
				continue
			}
			return "", false
		}
		if part == "." || part == ".." {
			return "", false
		}
	}
	return filepath.Clean(value), true
}
func encodedLocalHazard(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "%2e") || strings.Contains(value, "%2f") || strings.Contains(value, "%5c") ||
		strings.Contains(value, "%3a") || strings.Contains(value, "%3f") || strings.Contains(value, "%23") || strings.Contains(value, "%00")
}
func rejectUnsupportedPackageRoot(selection PackageSourceSelection) (string, bool, error) {
	if isUnsupportedPackageSource(selection.Source) {
		return "", true, sourceError(selection, "unsupported-source", ErrUnsupportedPackageSource, nil)
	}
	return "", false, nil
}
func isUnsupportedPackageSource(source string) bool {
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "git:") || strings.HasPrefix(lower, "ssh:") || strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") || strings.Contains(source, "://") || strings.HasPrefix(lower, "git+") {
		return true
	}
	at, colon := strings.IndexByte(source, '@'), strings.IndexByte(source, ':')
	if at > 0 && colon > at && !strings.Contains(source[:colon], "/") {
		return true
	}
	if strings.HasPrefix(source, ".") || filepath.IsAbs(source) {
		return false
	}
	slash := strings.IndexByte(source, '/')
	return slash > 0 && (strings.Contains(source[:slash], ".") || strings.EqualFold(source[:slash], "localhost"))
}
