package legacyassets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Registry keys of retired SDD files outside agent definitions are
// "<kind>/<path relative to the kind's directory>", so one key covers every
// runtime that received the file.
const (
	sddSkillKind    = "skills/"
	sddCommandKind  = "commands/"
	sddWorkflowKind = "workflows/"
	sddSharedDir    = "_shared/"
)

// claudeSDDWorkflowKey is the lazy SDD workflow v1.43.3 to v3.7.0 rendered
// into Claude Code's shared skill references. Releases composed it per run,
// most of them with the user's model assignments, and kept no golden of it,
// so no installed copy can be proven Gentle AI's: retirement reports it and
// never removes it.
const claudeSDDWorkflowKey = sddSkillKind + sddSharedDir + "sdd-orchestrator-workflow.md"

// NormalizeSDDAsset maps every installed copy of a released skill, command,
// or workflow file to the same bytes: CRLF line endings become LF and
// trailing whitespace at the end of the document is ignored. Releases wrote
// these files verbatim (or as a release-rendered whole the registry records),
// so nothing else is erased.
func NormalizeSDDAsset(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.TrimRight(content, " \t\n") + "\n"
}

// SDDAssetDigest is the registry key of content: SHA-256 of its normalization.
func SDDAssetDigest(content string) string {
	sum := sha256.Sum256([]byte(NormalizeSDDAsset(content)))
	return hex.EncodeToString(sum[:])
}

// SDDAssetDirs are the directories where one runtime received retired SDD
// files. An empty field means the runtime has no such directory.
type SDDAssetDirs struct {
	Skills, Commands, Workflows string
}

// RetiredSDDAssetPaths lists every retired SDD skill, slash command, and
// workflow path under dirs, sorted. Install, sync, and upgrade snapshot these
// paths and retirement inspects only them.
func RetiredSDDAssetPaths(agent model.AgentID, dirs SDDAssetDirs) []string {
	var paths []string
	for _, item := range retiredSDDAssetItems(agent, dirs) {
		paths = append(paths, item.path)
	}
	slices.Sort(paths)
	return paths
}

type sddAssetItem struct {
	// root is the runtime directory the item lives under; every directory
	// from root down to the item must be a real directory.
	root, path, key string
	// group is retired as a unit: every file of one skill directory, or a
	// single command, workflow, or shared reference.
	group string
}

func retiredSDDAssetItems(agent model.AgentID, dirs SDDAssetDirs) []sddAssetItem {
	var items []sddAssetItem
	add := func(dir, key, group string) {
		items = append(items, sddAssetItem{root: dir, path: filepath.Join(dir, filepath.FromSlash(strings.SplitN(key, "/", 2)[1])), key: key, group: group})
	}
	keys := make([]string, 0, len(releasedSDDAssetDigests))
	for key := range releasedSDDAssetDigests {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		switch rel, _ := strings.CutPrefix(key, sddSkillKind); {
		case dirs.Skills != "" && rel != key && strings.HasPrefix(rel, sddSharedDir):
			add(dirs.Skills, key, key)
		case dirs.Skills != "" && rel != key:
			add(dirs.Skills, key, sddSkillKind+strings.SplitN(rel, "/", 2)[0])
		case dirs.Workflows != "" && strings.HasPrefix(key, sddWorkflowKind):
			add(dirs.Workflows, key, key)
		}
	}
	if agent == model.AgentClaudeCode && dirs.Skills != "" {
		add(dirs.Skills, claudeSDDWorkflowKey, claudeSDDWorkflowKey)
	}
	if dirs.Commands != "" {
		for _, path := range SlashCommandPaths(agent, dirs.Commands) {
			key := sddCommandKind + filepath.Base(path)
			items = append(items, sddAssetItem{root: dirs.Commands, path: path, key: key, group: key})
		}
	}
	return items
}

// OwnsRetiredSDDAsset reports whether content is a render some release
// installed at the registry key.
func OwnsRetiredSDDAsset(key string, content []byte) bool {
	return len(content) > 0 && slices.Contains(releasedSDDAssetDigests[key], SDDAssetDigest(string(content)))
}

// AssetRetireResult lists the retired SDD files a retirement removed, the
// ones it preserved because no release wrote their bytes, and the
// directories holding retired SDD files it did not enter because they are
// not real directories (a dotfiles symlink, for example).
type AssetRetireResult struct {
	Removed, Preserved, UnsupportedDirs []string
}

// RetireSDDAssets removes the retired SDD skills, slash commands, and
// workflows under dirs whose bytes a release installed. A skill directory is
// one unit: it is removed only when every inventory file present in it is
// owned, so an edited skill keeps the references it ships with. Shared SDD
// references stay while any edited SDD skill remains, since those skills read
// them. Symlinks, non-regular files, and bytes no release wrote are preserved
// and reported; files outside the inventory are never inspected. Like the
// OpenCode prompt retirement, it never enters a directory that is not a real
// directory, from the runtime root down: the files behind it are the user's,
// so its group is kept and the directory is reported.
func RetireSDDAssets(agent model.AgentID, dirs SDDAssetDirs) (AssetRetireResult, error) {
	var result AssetRetireResult
	items := retiredSDDAssetItems(agent, dirs)
	present := map[string][]string{}
	var groups []string
	kept := map[string]bool{}
	for _, item := range items {
		unsupported, err := unsupportedAncestor(item.root, item.path)
		if err != nil {
			return result, err
		}
		if unsupported != "" {
			// Reading through the link is safe; report it only when it
			// actually holds a retired SDD file.
			if _, err := os.Stat(item.path); err == nil {
				kept[item.group] = true
				if !slices.Contains(result.UnsupportedDirs, unsupported) {
					result.UnsupportedDirs = append(result.UnsupportedDirs, unsupported)
				}
			}
			continue
		}
		info, err := os.Lstat(item.path)
		if os.IsNotExist(err) || err == nil && info.IsDir() {
			continue
		}
		if err != nil {
			return result, fmt.Errorf("inspect retired SDD file %s: %w", item.path, err)
		}
		if len(present[item.group]) == 0 {
			groups = append(groups, item.group)
		}
		present[item.group] = append(present[item.group], item.path)
		owned := false
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(item.path)
			if err != nil {
				return result, fmt.Errorf("read retired SDD file %s: %w", item.path, err)
			}
			owned = OwnsRetiredSDDAsset(item.key, data)
		}
		if !owned {
			kept[item.group] = true
			result.Preserved = append(result.Preserved, item.path)
		}
	}
	editedSkill := false
	for group := range kept {
		if strings.HasPrefix(group, sddSkillKind) && !strings.HasPrefix(group, sddSkillKind+sddSharedDir) {
			editedSkill = true
		}
	}
	for _, group := range groups {
		if kept[group] || editedSkill && strings.HasPrefix(group, sddSkillKind+sddSharedDir) {
			continue
		}
		for _, path := range present[group] {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return result, fmt.Errorf("remove retired SDD file %s: %w", path, err)
			}
			result.Removed = append(result.Removed, path)
		}
		if rel, ok := strings.CutPrefix(group, sddSkillKind); ok && !strings.HasPrefix(rel, sddSharedDir) {
			removeEmptyDirs(filepath.Join(dirs.Skills, rel), present[group])
		}
	}
	slices.Sort(result.Removed)
	slices.Sort(result.Preserved)
	slices.Sort(result.UnsupportedDirs)
	return result, nil
}

// unsupportedAncestor returns the first directory from root down to path's
// parent that exists but is not a real directory, or "" when every existing
// one is. A missing directory ends the walk: nothing below it exists.
func unsupportedAncestor(root, path string) (string, error) {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("retired SDD file %s is outside %s", path, root)
	}
	dir := root
	parts := []string{"."}
	if rel != "." {
		parts = append(parts, strings.Split(rel, string(filepath.Separator))...)
	}
	for _, part := range parts {
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("inspect %s: %w", dir, err)
		}
		if !info.IsDir() {
			return dir, nil
		}
	}
	return "", nil
}

// removeEmptyDirs removes the directories between each removed file and root,
// deepest first, when retirement left them empty. Symlinked directories and
// directories holding anything else stay.
func removeEmptyDirs(root string, removed []string) {
	var dirs []string
	for _, path := range removed {
		for dir := filepath.Dir(path); strings.HasPrefix(dir, root); dir = filepath.Dir(dir) {
			if !slices.Contains(dirs, dir) {
				dirs = append(dirs, dir)
			}
			if dir == root {
				break
			}
		}
	}
	slices.SortFunc(dirs, func(a, b string) int { return len(b) - len(a) })
	for _, dir := range dirs {
		if info, err := os.Lstat(dir); err == nil && info.IsDir() {
			_ = os.Remove(dir) // fails, and keeps the directory, unless empty
		}
	}
}

// ManualActions tells the user what to do with every file retirement kept.
func (r AssetRetireResult) ManualActions() []string {
	actions := make([]string, 0, len(r.UnsupportedDirs)+len(r.Preserved))
	for _, dir := range r.UnsupportedDirs {
		actions = append(actions, fmt.Sprintf("%s is not a real directory (for example a symlink), so Gentle AI did not inspect or remove the retired SDD files behind it. SDD was retired in v4.0.0 and those files are no longer maintained; if you no longer need them, move or delete them yourself.", dir))
	}
	for _, path := range r.Preserved {
		if strings.HasSuffix(filepath.ToSlash(path), "/"+strings.TrimPrefix(claudeSDDWorkflowKey, sddSkillKind)) {
			actions = append(actions, fmt.Sprintf("Retired SDD workflow %s was preserved: Gentle AI rendered it per installation before v4.0.0 and cannot prove it is unchanged, so it may contain your changes. SDD was retired in v4.0.0 and Claude Code no longer reads this file; if you no longer need it, move or delete it.", path))
			continue
		}
		actions = append(actions, fmt.Sprintf("Retired SDD file %s was preserved: Gentle AI cannot prove it wrote this file (its content differs from every released version, or it is not a regular file), so it may contain your changes. SDD was retired in v4.0.0 and this file is no longer maintained; if you no longer need it, move or delete it, then rerun `gentle-ai sync` to retire the SDD files kept beside it.", path))
	}
	return actions
}
