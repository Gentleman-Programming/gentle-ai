package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const writeAuthorityFile = ".gentle-ai-opencode-write-authority.json"

type writeAuthority struct {
	Version   int    `json:"version"`
	Directory string `json:"directory"`
	Basename  string `json:"basename"`
}

func authorityPath(dir string) string { return filepath.Join(dir, writeAuthorityFile) }

// GlobalAuthorityState checks the same global selector used by OpenCode config writes.
func GlobalAuthorityState(homeDir string) (string, bool, error) {
	return AuthorityState(effectiveGlobalConfigDir(homeDir))
}

// AuthorityState validates existing authority without creating it and returns
// the sidecar path for transactional snapshots.
func AuthorityState(dir string) (string, bool, error) {
	if dir == "" {
		return "", false, fmt.Errorf("empty OpenCode config directory")
	}
	_, exists, err := readWriteAuthority(dir)
	return authorityPath(dir), exists, err
}

func readWriteAuthority(dir string) (string, bool, error) {
	path := authorityPath(dir)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("write authority %s is not a regular file", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	var record writeAuthority
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&record); err != nil {
		return "", false, fmt.Errorf("invalid write authority %s: %w", path, err)
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return "", false, fmt.Errorf("trailing write authority data %s", path)
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false, err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", false, err
	}
	if record.Version != 1 || record.Directory != canonical || (record.Basename != "opencode.json" && record.Basename != "opencode.jsonc") {
		return "", false, fmt.Errorf("stale write authority %s", path)
	}
	target := filepath.Join(dir, record.Basename)
	targetInfo, err := os.Lstat(target)
	if err != nil {
		return "", false, fmt.Errorf("write authority target %s: %w", target, err)
	}
	if !targetInfo.Mode().IsRegular() {
		return "", false, fmt.Errorf("write authority target %s is not regular", target)
	}
	return target, true, nil
}

// WriteInitialAuthority records an explicitly selected config path. Call only
// from a transaction that can back up and roll back the sidecar independently.
func WriteInitialAuthority(dir, path string) error {
	if !filepath.IsAbs(path) || (filepath.Base(path) != "opencode.json" && filepath.Base(path) != "opencode.jsonc") {
		return fmt.Errorf("invalid write authority target %s", path)
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return err
	}
	selectedDir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	selectedDir, err = filepath.Abs(selectedDir)
	if err != nil {
		return err
	}
	if selectedDir != canonical {
		return fmt.Errorf("write authority target %s is outside %s", path, dir)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("write authority target %s is not regular", path)
	}
	if _, err = os.Lstat(authorityPath(dir)); err == nil {
		return fmt.Errorf("write authority already exists in %s", dir)
	} else if !os.IsNotExist(err) {
		return err
	}
	raw, err := json.Marshal(writeAuthority{Version: 1, Directory: canonical, Basename: filepath.Base(path)})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".gentle-ai-authority-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	// A concurrent writer must not be overwritten.
	if _, err = os.Lstat(authorityPath(dir)); err == nil {
		return fmt.Errorf("write authority already exists in %s", dir)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Link(temp.Name(), authorityPath(dir))
}

func findEffectiveConfigPathChecked(homeDir, projectDir string) (string, error) {
	for _, dir := range candidateConfigDirs(homeDir, projectDir) {
		pinned, exists, err := readWriteAuthority(dir)
		if err != nil {
			return "", err
		}
		jsonPath := filepath.Join(dir, "opencode.json")
		jsoncPath := filepath.Join(dir, "opencode.jsonc")
		if exists {
			return pinned, nil
		}
		hasJSON, hasJSONC := fileExists(jsonPath), fileExists(jsoncPath)
		switch {
		case hasJSON && hasJSONC:
			jsonPriority, jsoncPriority := managedConfigPriority(jsonPath), managedConfigPriority(jsoncPath)
			chosen := jsonPath
			if jsoncPriority > jsonPriority {
				chosen = jsoncPath
			}
			return chosen, nil
		case hasJSON:
			return jsonPath, nil
		case hasJSONC:
			return jsoncPath, nil
		}
	}
	return "", nil
}
