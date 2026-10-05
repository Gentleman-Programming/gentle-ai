package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// MaxPackageManifestBytes is the maximum accepted package.json size; resolution reads one extra byte to detect overflow.
const MaxPackageManifestBytes, packageBinName = 64 << 10, "gentle-pi-models"

type parserError struct {
	Path, Kind string
	Cause      error
}
type PackageError parserError
type ManifestError parserError
type BinError parserError
type UnsafeBinError struct {
	Path, Reason string
	Cause        error
}

func (e *PackageError) Error() string {
	return fmt.Sprintf("Pi package error (%s) at %q", e.Kind, e.Path)
}
func (e *PackageError) Unwrap() error { return e.Cause }
func (e *ManifestError) Error() string {
	return fmt.Sprintf("Pi manifest error (%s) at %q", e.Kind, e.Path)
}
func (e *ManifestError) Unwrap() error { return e.Cause }
func (e *BinError) Error() string      { return fmt.Sprintf("Pi bin error (%s) at %q", e.Kind, e.Path) }
func (e *BinError) Unwrap() error      { return e.Cause }
func (e *UnsafeBinError) Error() string {
	return fmt.Sprintf("unsafe %s bin %q: %s", packageBinName, e.Path, e.Reason)
}
func (e *UnsafeBinError) Unwrap() error                  { return e.Cause }
func packageError(path, kind string, cause error) error  { return &PackageError{path, kind, cause} }
func manifestError(path, kind string, cause error) error { return &ManifestError{path, kind, cause} }
func binError(path, kind string, cause error) error      { return &BinError{path, kind, cause} }
func missingKind(err error, missing, other string) string {
	if os.IsNotExist(err) {
		return missing
	}
	return other
}

// ResolvePackageBin returns the canonical executable selected by package.json.
// The process-launch slice must revalidate the returned path immediately before
// exec; this read-only boundary cannot eliminate the validation/exec TOCTOU race.
func ResolvePackageBin(packageRoot string) (string, error) {
	if !filepath.IsAbs(packageRoot) {
		return "", packageError(packageRoot, "invalid-package-root", nil)
	}
	packageRoot = filepath.Clean(packageRoot)
	info, err := os.Stat(packageRoot)
	if err != nil {
		return "", packageError(packageRoot, missingKind(err, "missing-package", "package-unavailable"), err)
	}
	if !info.IsDir() {
		return "", packageError(packageRoot, "package-not-directory", nil)
	}
	canonicalRoot, err := filepath.EvalSymlinks(packageRoot)
	if err != nil {
		return "", packageError(packageRoot, missingKind(err, "missing-package", "package-unavailable"), err)
	}
	manifestPath := filepath.Join(packageRoot, "package.json")
	file, err := os.Open(manifestPath)
	if err != nil {
		return "", manifestError(manifestPath, missingKind(err, "missing-manifest", "unreadable-manifest"), err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxPackageManifestBytes+1))
	if err != nil {
		return "", manifestError(manifestPath, "unreadable-manifest", err)
	}
	if len(data) > MaxPackageManifestBytes {
		return "", manifestError(manifestPath, "manifest-too-large", errors.New("package manifest exceeds maximum size"))
	}
	document, err := decodeManifest(data)
	if err != nil {
		var duplicate *duplicateJSONKeyError
		if errors.As(err, &duplicate) && duplicate.Bin {
			return "", binError(manifestPath, "malformed-bin", err)
		}
		return "", manifestError(manifestPath, "malformed-manifest", err)
	}
	relative, err := selectPackageBin(document, manifestPath)
	if err != nil {
		return "", err
	}
	if reason := unsafeBinPath(relative); reason != "" {
		return "", binError(relative, "unsafe-bin", &UnsafeBinError{Path: relative, Reason: reason})
	}
	target := filepath.Join(packageRoot, filepath.FromSlash(relative))
	if !pathWithin(packageRoot, target) {
		return "", binError(target, "unsafe-bin", &UnsafeBinError{Path: target, Reason: "lexical path escapes package root"})
	}
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", binError(target, missingKind(err, "missing-bin-target", "unreadable-bin-target"), err)
	}
	if !pathWithin(canonicalRoot, canonicalTarget) {
		return "", binError(target, "unsafe-bin", &UnsafeBinError{Path: target, Reason: "symlink escapes package root"})
	}
	info, err = os.Stat(canonicalTarget)
	if err != nil {
		return "", binError(target, missingKind(err, "missing-bin-target", "unreadable-bin-target"), err)
	}
	if !info.Mode().IsRegular() {
		return "", binError(target, "non-regular-bin-target", nil)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", binError(target, "non-executable-bin-target", nil)
	}
	return filepath.Clean(canonicalTarget), nil
}
func selectPackageBin(document map[string]json.RawMessage, manifestPath string) (string, error) {
	raw, ok := document["bin"]
	if !ok {
		return "", binError(manifestPath, "absent-bin", nil)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", binError(manifestPath, "malformed-bin", nil)
	}
	var target string
	switch raw[0] {
	case '"':
		if err := json.Unmarshal(raw, &target); err != nil || target == "" {
			return "", binError(manifestPath, "malformed-bin", err)
		}
		name, exists := document["name"]
		if !exists {
			return "", binError(manifestPath, "absent-bin", nil)
		}
		name = bytes.TrimSpace(name)
		if len(name) == 0 || name[0] != '"' {
			return "", manifestError(manifestPath, "malformed-manifest", nil)
		}
		var packageName string
		if err := json.Unmarshal(name, &packageName); err != nil {
			return "", manifestError(manifestPath, "malformed-manifest", err)
		}
		if packageName != packageBinName {
			return "", binError(manifestPath, "absent-bin", nil)
		}
	case '{':
		var entries map[string]string
		if err := json.Unmarshal(raw, &entries); err != nil || entries == nil {
			return "", binError(manifestPath, "malformed-bin", err)
		}
		target, ok = entries[packageBinName]
		if !ok {
			return "", binError(manifestPath, "absent-bin", nil)
		}
		if target == "" {
			return "", binError(manifestPath, "malformed-bin", nil)
		}
	default:
		return "", binError(manifestPath, "malformed-bin", nil)
	}
	return target, nil
}
func unsafeBinPath(value string) string {
	if value == "" {
		return "empty bin path"
	}
	// A leading slash is an absolute claim in the manifest's slash-relative
	// vocabulary even where filepath.IsAbs says otherwise: on Windows a
	// rooted-but-driveless path is not IsAbs, and letting it through would
	// classify the same manifest differently per platform.
	if filepath.IsAbs(value) || strings.HasPrefix(value, "/") || filepath.VolumeName(value) != "" || windowsDrivePath(value) {
		return "absolute bin path"
	}
	if strings.ContainsRune(value, '\\') {
		return "unsafe platform separator"
	}
	if strings.Contains("/"+value+"/", "/../") {
		return "lexical parent traversal"
	}
	return ""
}
func windowsDrivePath(value string) bool {
	return len(value) >= 2 && value[1] == ':' &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z'))
}
func pathWithin(root, value string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(value))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type jsonObjectScope uint8

const otherObject, manifestObject, binObject jsonObjectScope = iota, iota + 1, iota + 2

type duplicateJSONKeyError struct {
	Key string
	Bin bool
}

func (e *duplicateJSONKeyError) Error() string {
	return fmt.Sprintf("duplicate JSON object key %q", e.Key)
}
func decodeManifest(data []byte) (map[string]json.RawMessage, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document == nil {
		return nil, errors.New("manifest must be a JSON object")
	}
	if err := scanJSONValue(json.NewDecoder(bytes.NewReader(data)), manifestObject); err != nil {
		return nil, err
	}
	return document, nil
}
func scanJSONValue(decoder *json.Decoder, scope jsonObjectScope) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == json.Delim('[') {
		for decoder.More() {
			if err := scanJSONValue(decoder, scope); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	if token != json.Delim('{') {
		return nil
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		name := key.(string)
		if seen[name] {
			return &duplicateJSONKeyError{Key: name, Bin: scope == binObject || scope == manifestObject && name == "bin"}
		}
		seen[name] = true
		child := otherObject
		if scope == manifestObject && name == "bin" {
			child = binObject
		}
		if err := scanJSONValue(decoder, child); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
