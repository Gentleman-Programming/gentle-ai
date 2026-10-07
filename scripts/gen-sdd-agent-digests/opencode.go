package main

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/legacyassets"
)

// OpenCode received the retired SDD agents as settings entries and shared
// prompt files, rendered from these sources:
//   - the overlays (sdd-overlay-{single,multi}.json): descriptions and
//     inline prompts of sdd-orchestrator and every sdd-<phase>;
//   - profiles.go: profile phase descriptions and the profile orchestrator
//     description around the profile name;
//   - every <family>/sdd-orchestrator.md asset: the heading a rendered
//     orchestrator prompt starts with;
//   - prompts.go: the prompt file each phase got, either an inline Go
//     string or a model section of the phase skill.
const (
	overlayPrefix = "internal/assets/opencode/sdd-overlay-"
	profilesFile  = "internal/components/sdd/profiles.go"
	promptsFile   = "internal/components/sdd/prompts.go"
	settingsGold  = "testdata/golden/sdd-opencode-multi-settings.golden"
)

var (
	phaseDescriptions  = regexp.MustCompile(`(?s)phaseDescriptions := map\[string\]string\{(.*?)\n\t\}`)
	profileDescription = regexp.MustCompile(`"description":\s*"((?:[^"\\]|\\.)*)"\s*\+\s*profile\.Name\s*\+\s*"((?:[^"\\]|\\.)*)"`)
	inlinePrompts      = regexp.MustCompile(`(?s)var subAgentPromptContent = map\[string\]string\{(.*?)\n\}`)
	stringEntry        = regexp.MustCompile(`"(sdd-[a-z-]+)":\s*"((?:[^"\\]|\\.)*)"`)
	orchestratorAsset  = regexp.MustCompile(`^internal/assets/[^/]+/sdd-orchestrator\.md$`)
)

type openCodeHarvest struct {
	descriptions, prompts, headings, promptFiles *registry
	// goldens holds every released settings render, verified once the
	// registry is complete.
	goldens map[string][]byte
}

func newOpenCodeHarvest() *openCodeHarvest {
	return &openCodeHarvest{descriptions: newRegistry(), prompts: newRegistry(), headings: newRegistry(), promptFiles: newRegistry(), goldens: map[string][]byte{}}
}

// collect harvests one release; files maps repository paths to blob ids.
func (h *openCodeHarvest) collect(tag string, files map[string]string) {
	for _, mode := range []string{"single", "multi"} {
		if blob, ok := files[overlayPrefix+mode+".json"]; ok {
			h.overlay(tag, blob)
		}
	}
	if blob, ok := files[profilesFile]; ok {
		h.profiles(tag, cachedBlob(blob))
	}
	paths := make([]string, 0, len(files))
	for file := range files {
		if orchestratorAsset.MatchString(file) {
			paths = append(paths, file)
		}
	}
	slices.Sort(paths)
	for _, file := range paths {
		for _, line := range strings.Split(cachedBlob(files[file]), "\n") {
			if strings.HasPrefix(line, "# ") {
				h.headings.add("sdd-orchestrator", strings.TrimSpace(line), tag)
				break
			}
		}
	}
	if blob, ok := files[promptsFile]; ok {
		h.promptSources(tag, cachedBlob(blob), files)
	}
	if blob, ok := files[settingsGold]; ok {
		h.goldens[tag] = []byte(cachedBlob(blob))
	}
}

func (h *openCodeHarvest) overlay(tag, blob string) {
	var overlay struct {
		Agent map[string]map[string]any `json:"agent"`
	}
	if err := json.Unmarshal([]byte(cachedBlob(blob)), &overlay); err != nil {
		fail("%s: parse SDD overlay: %v", tag, err)
	}
	for name, entry := range overlay.Agent {
		if !strings.HasPrefix(name, "sdd-") {
			continue
		}
		if description, ok := entry["description"].(string); ok {
			h.descriptions.add(name, description, tag)
		}
		prompt, ok := entry["prompt"].(string)
		if !ok || strings.HasPrefix(prompt, "__PROMPT_FILE_") || legacyassets.IsOpenCodeSDDPromptFileRef(prompt, name) {
			continue
		}
		h.prompts.add(name, legacyassets.OpenCodeSDDPromptDigest(prompt), tag)
	}
}

func (h *openCodeHarvest) profiles(tag, source string) {
	if !strings.Contains(source, "phaseDescriptions") {
		return
	}
	block := phaseDescriptions.FindStringSubmatch(source)
	orchestrators := profileDescription.FindAllStringSubmatch(source, -1)
	if block == nil || len(orchestrators) == 0 {
		fail("%s: %s profile descriptions not recognized; update the generator", tag, profilesFile)
	}
	for _, entry := range stringEntry.FindAllStringSubmatch(block[1], -1) {
		h.descriptions.add(entry[1], unquote(tag, entry[2]), tag)
	}
	for _, match := range orchestrators {
		h.descriptions.add("sdd-orchestrator", unquote(tag, match[1])+"{{PROFILE}}"+unquote(tag, match[2]), tag)
	}
}

func (h *openCodeHarvest) promptSources(tag, source string, files map[string]string) {
	if !strings.Contains(source, "func WriteSharedPromptFiles") {
		return
	}
	if block := inlinePrompts.FindStringSubmatch(source); block != nil {
		for _, entry := range stringEntry.FindAllStringSubmatch(block[1], -1) {
			h.promptFiles.add(entry[1]+".md", legacyassets.OpenCodeSDDPromptDigest(unquote(tag, entry[2])), tag)
		}
		return
	}
	if !strings.Contains(source, "readSkillContent(phase)") {
		fail("%s: %s prompt source not recognized; update the generator", tag, promptsFile)
	}
	for _, phase := range legacyassets.SharedPromptPhases() {
		blob, ok := files["internal/assets/skills/"+phase+"/SKILL.md"]
		if !ok {
			continue
		}
		skill := cachedBlob(blob)
		// Each phase got the section for its model capability, or the
		// whole skill when it has none.
		for _, capability := range []string{"capable", "small"} {
			section := filemerge.ExtractHTMLCommentSection(skill, "model-"+capability)
			h.promptFiles.add(phase+".md", legacyassets.OpenCodeSDDPromptDigest(section), tag)
		}
	}
}

// verifyGoldens fails generation when any retired SDD entry of a released
// settings render is not owned by its shape alone, without the marker.
func (h *openCodeHarvest) verifyGoldens() int {
	registry := legacyassets.OpenCodeSDDRegistry{Descriptions: h.descriptions.values(), Prompts: h.prompts.values(), Headings: h.headings.values()}
	verified := 0
	for tag, golden := range h.goldens {
		root, err := filemerge.UnmarshalJSONObject(golden)
		if err != nil {
			fail("%s: parse %s: %v", tag, settingsGold, err)
		}
		agents, _ := root["agent"].(map[string]any)
		for name, raw := range agents {
			if _, _, retired := registry.Retired(name); !retired {
				continue
			}
			entry, _ := raw.(map[string]any)
			unmarked := map[string]any{}
			for key, value := range entry {
				if key != "__managed_by" {
					unmarked[key] = value
				}
			}
			if !registry.Owns(name, unmarked) {
				fail("%s: %s agent.%s is not owned by its released shape", tag, settingsGold, name)
			}
			verified++
		}
	}
	return verified
}

func (h *openCodeHarvest) write(output string, tags []string) {
	var b bytes.Buffer
	b.WriteString("// Code generated by scripts/gen-sdd-agent-digests; DO NOT EDIT.\n\npackage legacyassets\n\n")
	writeTags(&b, tags)
	h.descriptions.write(&b, "releasedOpenCodeSDDDescriptions", "// releasedOpenCodeSDDDescriptions maps a retired SDD agent to every\n// description a release wrote for it in OpenCode settings.\n")
	h.prompts.write(&b, "releasedOpenCodeSDDPrompts", "// releasedOpenCodeSDDPrompts maps a retired SDD agent to the normalized\n// digest of every inline prompt a release wrote for it.\n")
	h.headings.write(&b, "releasedOpenCodeSDDOrchestratorHeadings", "// releasedOpenCodeSDDOrchestratorHeadings lists the heading every released\n// orchestrator prompt asset starts with.\n")
	h.promptFiles.write(&b, "releasedOpenCodeSDDPromptFiles", "// releasedOpenCodeSDDPromptFiles maps prompts/sdd/<file> to the normalized\n// digest of every shared prompt file a release rendered.\n")
	writeSource(output, b.Bytes())
}

func (h *openCodeHarvest) has(digest string) bool {
	return h.prompts.has(digest) || h.promptFiles.has(digest)
}

func unquote(tag, literal string) string {
	value, err := strconv.Unquote(`"` + literal + `"`)
	if err != nil {
		fail("%s: unquote %q: %v", tag, literal, err)
	}
	return value
}

var blobs = map[string]string{}

func cachedBlob(blob string) string {
	if content, ok := blobs[blob]; ok {
		return content
	}
	content := git("cat-file", "blob", blob)
	blobs[blob] = content
	return content
}

// values returns the registry as the map the generated file declares.
func (r *registry) values() map[string][]string {
	values := make(map[string][]string, len(r.order))
	for key, order := range r.order {
		values[key] = append([]string(nil), order...)
	}
	return values
}
