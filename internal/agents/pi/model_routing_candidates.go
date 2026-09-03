package pi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type ModelRoutingCandidate struct{ Path, Source string }
type CandidateError struct {
	Source, Path, Kind string
	Cause              error
}

func (e *CandidateError) Error() string {
	message := fmt.Sprintf("Pi model-routing candidate error (%s) from %s at %q", e.Kind, e.Source, e.Path)
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}
func (e *CandidateError) Unwrap() error { return e.Cause }

var errRoutingCandidateNonRegular, errRoutingCandidateNonExecutable = errors.New("candidate is not a regular file"), errors.New("candidate is not executable")

func EnumerateModelRoutingCandidates(cwd, agentDir string) ([]ModelRoutingCandidate, error) {
	selection, selectionErr := SelectPackageSource(cwd, agentDir)
	if selectionErr != nil && !errors.Is(selectionErr, ErrPackageNotConfigured) {
		var settings *SettingsError
		if errors.As(selectionErr, &settings) {
			return nil, &CandidateError{Source: string(settings.Scope), Path: settings.Path, Kind: "settings", Cause: selectionErr}
		}
		return nil, &CandidateError{Source: "settings", Path: "selection", Kind: "settings", Cause: selectionErr}
	}
	pathCandidates, pathFailures := enumerateModelRoutingPATH(selection.CWD)
	if selectionErr != nil {
		if len(pathCandidates) > 0 {
			return pathCandidates, nil
		}
		var notConfigured *NotConfiguredError
		path := "settings"
		if errors.As(selectionErr, &notConfigured) {
			path = notConfigured.Path
		}
		return nil, &CandidateError{Source: "settings", Path: path, Kind: "not-configured", Cause: errors.Join(append([]error{selectionErr}, pathFailures...)...)}
	}
	candidates := append([]ModelRoutingCandidate(nil), pathCandidates...)
	seen := make(map[string]struct{}, len(candidates)+1)
	for _, candidate := range candidates {
		key := candidate.Path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		seen[key] = struct{}{}
	}
	root, err := ResolvePackageRoot(selection)
	if err != nil {
		return nil, &CandidateError{Source: selection.Source, Path: selection.SettingsPath, Kind: "package-root", Cause: err}
	}
	bin, err := ResolvePackageBin(root.Path)
	if err != nil {
		return nil, &CandidateError{Source: selection.Source, Path: root.Path, Kind: "package-bin", Cause: err}
	}
	key := bin
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	if _, duplicate := seen[key]; !duplicate {
		candidates = append(candidates, ModelRoutingCandidate{Path: bin, Source: selection.Source})
	}
	return candidates, nil
}
func enumerateModelRoutingPATH(cwd string) ([]ModelRoutingCandidate, []error) {
	entries, names := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)), modelRoutingExecutableNames()
	seen, candidates, failures := map[string]struct{}{}, []ModelRoutingCandidate{}, []error{}
	for _, entry := range entries {
		directory := entry
		if directory == "" {
			directory = cwd
		} else if !filepath.IsAbs(directory) {
			directory = filepath.Join(cwd, directory)
		}
		directory = filepath.Clean(directory)
		var causes []error
		for _, name := range names {
			canonical, err := canonicalRoutingExecutable(filepath.Join(directory, name))
			if err != nil {
				causes = append(causes, err)
				continue
			}
			key := canonical
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			if _, duplicate := seen[key]; !duplicate {
				seen[key] = struct{}{}
				candidates = append(candidates, ModelRoutingCandidate{Path: canonical, Source: "PATH"})
			}
			causes = nil
			break
		}
		if len(causes) > 0 {
			failures = append(failures, &CandidateError{Source: "PATH", Path: filepath.Join(directory, packageBinName), Kind: routingPathErrorKind(causes[0]), Cause: errors.Join(causes...)})
		}
	}
	return candidates, failures
}
func modelRoutingExecutableNames() []string {
	names := []string{packageBinName}
	if runtime.GOOS != "windows" {
		return names
	}
	extensions := os.Getenv("PATHEXT")
	if extensions == "" {
		extensions = ".COM;.EXE;.BAT;.CMD"
	}
	for _, extension := range strings.Split(extensions, string(os.PathListSeparator)) {
		extension = strings.TrimSpace(extension)
		if extension == "" {
			continue
		}
		if extension[0] != '.' {
			extension = "." + extension
		}
		names = append(names, packageBinName+extension)
	}
	return names
}
func canonicalRoutingExecutable(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errRoutingCandidateNonRegular
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", errRoutingCandidateNonExecutable
	}
	canonical, err = filepath.Abs(canonical)
	return filepath.Clean(canonical), err
}
func routingPathErrorKind(err error) string {
	if os.IsNotExist(err) {
		return "missing"
	}
	if errors.Is(err, errRoutingCandidateNonRegular) {
		return "non-regular"
	}
	if errors.Is(err, errRoutingCandidateNonExecutable) {
		return "non-executable"
	}
	return "unusable"
}
