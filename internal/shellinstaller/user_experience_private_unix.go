//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Generated producer preferences may already contain unrelated state. Merge only
// the reviewed keys in the unpublished copy, never an environment-selected home.
func userStagePrivateChoices(ctx context.Context, root, mode string, profile *UserExperience) error {
	if profile == nil || !userQualifiedPrivateChoices(profile) && !userQualifiedLegacyExperience(profile) {
		return nil
	}
	if profile.GlobalRDD != "" {
		err := userStageJSON(ctx, root, "home/.gentle-ai", "state.json", func(fields map[string]json.RawMessage) error {
			if raw, ok := fields["rdd_mode"]; ok {
				var previous string
				if json.Unmarshal(raw, &previous) != nil || (previous != "" && previous != "on" && previous != "off") {
					return errors.New("generated RDD mode is not canonical")
				}
			}
			if raw, ok := fields["rdd_mode_recorded_at"]; ok && string(raw) != "null" {
				var previous time.Time
				if json.Unmarshal(raw, &previous) != nil || previous.IsZero() {
					return errors.New("generated RDD cutoff is not canonical")
				}
			}
			fields["rdd_mode"], _ = json.Marshal(profile.GlobalRDD)
			fields["rdd_mode_recorded_at"], _ = json.Marshal(time.Now().UTC())
			return nil
		})
		if err != nil {
			return err
		}
	}
	if profile.Context7 == "" || mode == "shared" || mode == "agent" {
		// Shared Context7 is merged into the existing agent after its recovery
		// preimages exist; the private stage root agent is not used there.
		return nil
	}
	return userStageJSON(ctx, root, "agent", "mcp.json", userContext7Update(profile.Context7))
}

// Shared Context7 mutates the selected Pi agent only, through the same bounded
// writer and preimage checks used for the private stage.
func userStageContext7Shared(ctx context.Context, agent, choice string) error {
	if choice == "" {
		return nil
	}
	if !privateHierarchyPath(agent) {
		return privateError("preimage", nil)
	}
	return userStageJSON(ctx, agent, ".", "mcp.json", userContext7Update(choice))
}

func userContext7Update(choice string) func(map[string]json.RawMessage) error {
	return func(fields map[string]json.RawMessage) error {
		servers := map[string]json.RawMessage{}
		if raw, ok := fields["mcpServers"]; ok {
			var err error
			servers, err = userStageJSONObject(raw)
			if err != nil {
				return err
			}
		}
		server := map[string]json.RawMessage{}
		if raw, ok := servers["context7"]; ok {
			var err error
			server, err = userStageJSONObject(raw)
			var url string
			if err != nil || json.Unmarshal(server["url"], &url) != nil || url != "https://mcp.context7.com/mcp" {
				return errors.New("generated Context7 preimage differs")
			}
			if raw, ok := server["enabled"]; ok {
				var previous *bool
				if json.Unmarshal(raw, &previous) != nil || previous == nil {
					return errors.New("generated Context7 enabled preimage differs")
				}
			}
			if raw, ok := server["exposure"]; ok {
				var previous *string
				if json.Unmarshal(raw, &previous) != nil || previous == nil {
					return errors.New("generated Context7 exposure preimage differs")
				}
			}
		}
		server["url"], _ = json.Marshal("https://mcp.context7.com/mcp")
		server["enabled"], _ = json.Marshal(choice == "on")
		server["exposure"], _ = json.Marshal("codemode")
		servers["context7"], _ = json.Marshal(server)
		fields["mcpServers"], _ = json.Marshal(servers)
		return nil
	}
}

func userStageJSONObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, errors.New("generated preference must be a JSON object")
	}
	fields := map[string]json.RawMessage{}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key := token.(string)
		folded := strings.ToLower(key)
		if seen[folded] {
			return nil, errors.New("generated preference has ambiguous keys")
		}
		seen[folded] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("generated preference has trailing JSON")
	}
	return fields, nil
}

// relative/name are fixed internal paths. Rooted I/O, exclusive temporary writes,
// preimage identity and synchronized readback keep the producer and caller intact.
func userStageJSON(ctx context.Context, root, relative, name string, update func(map[string]json.RawMessage) error) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	if !privateHierarchyPath(root) {
		return privateError("preimage", nil)
	}
	if _, err := privateDirectory(root); err != nil {
		return err
	}
	parent := filepath.Dir(filepath.Join(root, relative))
	if _, err := privateDirectory(parent); err != nil {
		return err
	}
	folder := filepath.Join(root, relative)
	if err := os.Mkdir(folder, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	identity, err := privateDirectory(folder)
	if err != nil {
		return err
	}
	directory, err := os.OpenRoot(folder)
	if err != nil {
		return err
	}
	defer directory.Close()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(identity, opened) {
		return privateError("preimage", err)
	}
	path := filepath.Join(folder, name)
	before, err := directory.Lstat(name)
	data := []byte("{}")
	if err == nil {
		if !before.Mode().IsRegular() || before.Mode().Perm()&0022 != 0 || before.Sys().(*syscall.Stat_t).Nlink != 1 || before.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || before.Size() > 1<<20 {
			return privateError("preimage", nil)
		}
		file, err := directory.Open(name)
		if err != nil {
			return err
		}
		actual, statErr := file.Stat()
		data, err = io.ReadAll(io.LimitReader(file, (1<<20)+1))
		if err := errors.Join(statErr, err, file.Close()); err != nil || !os.SameFile(before, actual) || len(data) > 1<<20 {
			return privateError("preimage", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	fields, err := userStageJSONObject(data)
	if err != nil {
		return privateError("preimage", err)
	}
	if err := update(fields); err != nil {
		return privateError("preimage", err)
	}
	data, err = json.MarshalIndent(fields, "", "  ")
	if err != nil || ctx.Err() != nil {
		return errors.Join(err, ctx.Err())
	}
	data = append(data, '\n')
	if _, err := userStageJSONObject(data); err != nil {
		return privateError("preimage", err)
	}
	if fresh, err := directory.Lstat(name); before == nil && !os.IsNotExist(err) || before != nil && (err != nil || privateStamp(before) != privateStamp(fresh)) {
		return privateError("preimage", err)
	}
	if err := userWriteWithSync(name+".new", data, 0600, func(path string, flags int, mode os.FileMode) (userSyncedFile, error) {
		return directory.OpenFile(path, flags, mode)
	}); err != nil {
		return err
	}
	if err := directory.Rename(name+".new", name); err != nil {
		return err
	}
	if err := userDirectorySync(folder); err != nil {
		return err
	}
	if fresh, err := privateDirectory(folder); err != nil || !os.SameFile(identity, fresh) {
		return privateError("preimage", err)
	}
	pin := fmt.Sprintf("%x", sha256.Sum256(data))
	_, err = privateNativeFile(ctx, path, 0600, int64(len(data)), pin)
	return err
}
