package main

import (
	"bytes"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/legacyassets"
)

// Releases installed retired SDD skills, slash commands, and the Windsurf
// workflow as follows; the asset registry records every render:
//   - skills (internal/assets/skills/sdd-*/** and the shared SDD references):
//     copied verbatim or reduced to one model-capability section, with or
//     without the leading-whitespace trim releases before v1.30.8 lacked;
//   - commands and the workflow: copied verbatim, except that a template
//     carrying a {{GENTLE_AI_*}} placeholder was rendered. Its render is the
//     same release's golden, which must equal the template around every
//     placeholder;
//   - v0.1.1 rendered its OpenCode commands from Go strings (sddCommandsFile).
//
// Every golden render a release kept for these files must be a registered
// digest of that release, which proves the render model.
const (
	goldenDir       = "testdata/golden"
	sddInjectFile   = "internal/components/sdd/inject.go"
	sddCommandsFile = "internal/components/sdd/commands.go"
)

var (
	skillAsset    = regexp.MustCompile(`^internal/assets/skills/((?:sdd-[^/]+/.+)|(?:_shared/(?:sdd-[^/]+|openspec-convention)\.md))$`)
	commandAsset  = regexp.MustCompile(`^internal/assets/(claude|opencode)/commands/((?:gentle-)?sdd-[^/]+\.md)$`)
	workflowAsset = regexp.MustCompile(`^internal/assets/windsurf/workflows/(sdd-[^/]+\.md)$`)
	placeholder   = regexp.MustCompile(`\{\{GENTLE_AI_[A-Z0-9_]+\}\}`)
	skillGolden   = regexp.MustCompile(`^sdd-[a-z0-9]+-skill-(sdd-[a-z0-9-]+)\.golden$`)
	commandGolden = regexp.MustCompile(`^sdd-(claude|opencode)-cmd-((?:gentle-)?sdd-[a-z0-9-]+)\.golden$`)
	flowGolden    = regexp.MustCompile(`^sdd-windsurf-workflow-(sdd-[a-z0-9-]+)\.golden$`)
	goCommand     = regexp.MustCompile(`\{Name: "(sdd-[a-z-]+)", Description: "((?:[^"\\]|\\.)*)", Body: "((?:[^"\\]|\\.)*)"\}`)
)

type assetHarvest struct {
	assets   *registry
	verified int
}

func newAssetHarvest() *assetHarvest { return &assetHarvest{assets: newRegistry()} }

func (h *assetHarvest) add(key, content, tag string) {
	h.assets.add(key, legacyassets.SDDAssetDigest(content), tag)
}

// collect harvests one release; files maps repository paths to blob ids.
func (h *assetHarvest) collect(tag string, files map[string]string) {
	templates := map[string]string{}
	var goldens []string
	names := make([]string, 0, len(files))
	for file := range files {
		names = append(names, file)
	}
	slices.Sort(names)
	for _, file := range names {
		blob := files[file]
		if m := skillAsset.FindStringSubmatch(file); m != nil {
			content, key := cachedBlob(blob), "skills/"+m[1]
			templates[key] = content
			h.add(key, content, tag)
			for _, section := range []string{"model-capable", "model-small"} {
				h.add(key, filemerge.ExtractHTMLCommentSection(content, section), tag)
				h.add(key, untrimmedSection(content, section), tag)
			}
		} else if m := commandAsset.FindStringSubmatch(file); m != nil {
			templates[m[1]+"/"+m[2]] = cachedBlob(blob)
			h.add("commands/"+m[2], templates[m[1]+"/"+m[2]], tag)
		} else if m := workflowAsset.FindStringSubmatch(file); m != nil {
			templates["windsurf/"+m[1]] = cachedBlob(blob)
			h.add("workflows/"+m[1], templates["windsurf/"+m[1]], tag)
		} else if path.Dir(file) == goldenDir {
			goldens = append(goldens, file)
		}
	}
	goCommands := h.goRenderedCommands(tag, files)
	for _, file := range goldens {
		name, golden := path.Base(file), cachedBlob(files[file])
		if m := skillGolden.FindStringSubmatch(name); m != nil {
			key := "skills/" + m[1] + "/SKILL.md"
			if _, ok := templates[key]; !ok || !slices.Contains(h.assets.order[key], legacyassets.SDDAssetDigest(golden)) {
				fail("%s: golden %s is not a registered render of %s", tag, file, key)
			}
		} else if m := commandGolden.FindStringSubmatch(name); m != nil {
			template, ok := templates[m[1]+"/"+m[2]+".md"]
			if !ok && m[1] == "claude" {
				template, ok = templates["opencode/"+m[2]+".md"]
			}
			h.render(tag, file, "commands/"+m[2]+".md", template, golden, ok)
		} else if m := flowGolden.FindStringSubmatch(name); m != nil {
			template, ok := templates["windsurf/"+m[1]+".md"]
			h.render(tag, file, "workflows/"+m[1]+".md", template, golden, ok)
		} else if name == "sdd-command-sdd-init.md" && goCommands {
			if !slices.Contains(h.assets.order["commands/sdd-init.md"], legacyassets.SDDAssetDigest(golden)) {
				fail("%s: golden %s is not the Go-rendered sdd-init command", tag, file)
			}
		} else {
			continue
		}
		h.verified++
	}
}

// render records a release's golden render of template at key. A template
// without placeholders was copied verbatim, so its golden must be that copy;
// otherwise the golden must equal the template around every placeholder.
func (h *assetHarvest) render(tag, file, key, template, golden string, ok bool) {
	if !ok {
		fail("%s: golden %s has no template for %s", tag, file, key)
	}
	golden, template = legacyassets.NormalizeSDDAsset(golden), legacyassets.NormalizeSDDAsset(template)
	parts := placeholder.Split(template, -1)
	if len(parts) == 1 {
		if golden != template {
			fail("%s: golden %s differs from its verbatim template %s", tag, file, key)
		}
		return
	}
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	if !regexp.MustCompile(`^(?s)` + strings.Join(parts, ".+?") + `$`).MatchString(golden) {
		fail("%s: golden %s does not render its template %s", tag, file, key)
	}
	h.add(key, golden, tag)
}

// goRenderedCommands registers the commands a release rendered from Go
// strings instead of embedded assets.
func (h *assetHarvest) goRenderedCommands(tag string, files map[string]string) bool {
	inject, ok := files[sddInjectFile]
	if !ok || !strings.Contains(cachedBlob(inject), `"# " + command.Name + "\n\n" + command.Description + "\n\n" + command.Body + "\n"`) {
		return false
	}
	matches := goCommand.FindAllStringSubmatch(cachedBlob(files[sddCommandsFile]), -1)
	if len(matches) == 0 {
		fail("%s: %s commands not recognized; update the generator", tag, sddCommandsFile)
	}
	for _, m := range matches {
		h.add("commands/"+m[1]+".md", "# "+m[1]+"\n\n"+unquote(tag, m[2])+"\n\n"+unquote(tag, m[3])+"\n", tag)
	}
	return true
}

// untrimmedSection is the capability section as releases before v1.30.8
// extracted it, keeping the whitespace after the opening marker.
func untrimmedSection(content, name string) string {
	open, close := "<!-- section:"+name+" -->", "<!-- /section:"+name+" -->"
	start, end := strings.Index(content, open), strings.Index(content, close)
	if start == -1 || end == -1 || end <= start {
		return content
	}
	return content[start+len(open) : end]
}

func (h *assetHarvest) write(output string, tags []string) {
	var b bytes.Buffer
	b.WriteString("// Code generated by scripts/gen-sdd-agent-digests; DO NOT EDIT.\n\npackage legacyassets\n\n")
	writeTags(&b, tags)
	h.assets.write(&b, "releasedSDDAssetDigests", "// releasedSDDAssetDigests maps <kind>/<installed relative path> to the\n// normalized digest of every retired SDD skill, command, and workflow render\n// a Gentle AI release installed.\n")
	writeSource(output, b.Bytes())
}
