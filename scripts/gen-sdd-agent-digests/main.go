// Command gen-sdd-agent-digests writes the registry of every native SDD
// sub-agent template a Gentle AI release shipped, keyed by family and file
// name, so sync and install can prove a retired agent is Gentle AI's own
// render (issues #5157, #5253). Digests use legacyassets.NormalizeSDDAgent,
// the same function ownership checks run on installed files.
//
// It also records Kimi's gentleman.yaml as shipped before the v4.0.0 ownership
// ledger: that file declared every SDD subagent by path, so the installer must
// recognize a v3-inherited copy to rewrite it before its subagents retire.
//
// It also writes the OpenCode registry (see opencode.go): the settings values
// and shared prompt files releases wrote for the retired SDD agents of the
// OpenCode family, proven against every release's settings golden render.
//
// SDD was retired in v4.0.0, so the release set is closed at that tag and
// later releases never change the registry. Fetch tags first:
//
//	git fetch --tags
//	go generate ./internal/components/legacyassets/
//
// The generator refuses an incomplete tag set: the anchor tags must exist,
// every tag the committed registry lists must exist locally, and every digest
// it records must be regenerated. It proves the normalization against every
// release's golden renders: each testdata/golden/sdd-<family>-agent-<name>.golden
// must normalize to a digest of that same release's template.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/legacyassets"
)

var anchors = []string{"v1.10.0", "v3.7.0", "v4.0.0"}

// closingTag retired SDD and introduced the native agent ownership ledger.
const closingTag = "v4.0.0"

// preLedgerParents are retained native agent files that referenced SDD
// subagents before the ownership ledger existed.
var preLedgerParents = []string{"kimi/gentleman.yaml"}

type row struct{ first, last string }

// registry is one generated map: key -> digests in first-release order.
type registry struct {
	ranges map[string]map[string]*row
	order  map[string][]string
}

func newRegistry() *registry {
	return &registry{ranges: map[string]map[string]*row{}, order: map[string][]string{}}
}

func (r *registry) add(key, digest, tag string) {
	if r.ranges[key] == nil {
		r.ranges[key] = map[string]*row{}
	}
	if span := r.ranges[key][digest]; span != nil {
		span.last = tag
		return
	}
	r.ranges[key][digest] = &row{tag, tag}
	r.order[key] = append(r.order[key], digest)
}

func (r *registry) write(b *bytes.Buffer, name, doc string) {
	keys := make([]string, 0, len(r.order))
	for key := range r.order {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	b.WriteString(doc)
	fmt.Fprintf(b, "var %s = map[string][]string{\n", name)
	for _, key := range keys {
		fmt.Fprintf(b, "%q: {\n", key)
		for _, digest := range r.order[key] {
			span := r.ranges[key][digest]
			label := span.first
			if span.last != span.first {
				label += " to " + span.last
			}
			fmt.Fprintf(b, "%q, // %s\n", digest, label)
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n\n")
}

func (r *registry) has(digest string) bool {
	for _, digests := range r.order {
		if slices.Contains(digests, digest) {
			return true
		}
	}
	return false
}

func main() {
	if len(os.Args) != 3 {
		fail("usage: gen-sdd-agent-digests <native-agents.go> <opencode.go>")
	}
	output, openCodeOutput := os.Args[1], os.Args[2]
	root = strings.TrimSpace(git("rev-parse", "--show-toplevel"))
	tags := strings.Fields(git("-c", "versionsort.suffix=-", "tag", "-l", "v*", "--sort=v:refname"))
	for _, anchor := range anchors {
		if !slices.Contains(tags, anchor) {
			fail("release tag %s missing; the local tag set is incomplete; run git fetch --tags", anchor)
		}
	}
	tags = tags[:slices.Index(tags, closingTag)+1]
	committedTags, committedDigests := readCommitted(output)
	openCodeTags, openCodeDigests := readCommitted(openCodeOutput)
	for file, recorded := range map[string][]string{output: committedTags, openCodeOutput: openCodeTags} {
		for _, tag := range recorded {
			if !slices.Contains(tags, tag) {
				fail("release tag %s recorded in %s is missing locally; the local tag set is incomplete; run git fetch --tags", tag, file)
			}
		}
	}
	families := sddAgentFamilies()
	blobDigest := map[string]string{}
	agents, parents := newRegistry(), newRegistry()
	openCode := newOpenCodeHarvest()
	for _, tag := range tags {
		templates := map[string]string{}
		files := map[string]string{}
		for _, line := range strings.Split(git("ls-tree", "-r", tag, "--", "internal/assets", profilesFile, promptsFile, settingsGold), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 4 || fields[1] != "blob" {
				continue
			}
			blob, file := fields[2], fields[3]
			files[file] = blob
			// internal/assets/<family>/agents/<name>
			parts := strings.Split(file, "/")
			if len(parts) != 5 || !families[parts[2]] || parts[3] != "agents" {
				continue
			}
			key := parts[2] + "/" + parts[4]
			sdd := strings.HasPrefix(parts[4], "sdd-")
			parent := slices.Contains(preLedgerParents, key) && tag != closingTag
			if !sdd && !parent {
				continue
			}
			digest, ok := blobDigest[blob]
			if !ok {
				content := git("cat-file", "blob", blob)
				if parent && strings.Contains(content, "{{") {
					fail("%s: %s has a render placeholder; its installed bytes are not the asset bytes", tag, file)
				}
				digest = legacyassets.SDDAgentDigest(content)
				blobDigest[blob] = digest
			}
			if sdd {
				templates[key] = digest
				agents.add(key, digest, tag)
			} else {
				parents.add(key, digest, tag)
			}
		}
		verifyGoldens(tag, templates)
		openCode.collect(tag, files)
	}
	settingsVerified := openCode.verifyGoldens()
	fmt.Fprintf(os.Stderr, "gen-sdd-agent-digests: %d golden renders and %d OpenCode settings entries verified across %d releases\n", verifiedGoldens, settingsVerified, len(tags))
	if len(agents.order) == 0 {
		fail("no SDD agent templates found in any release")
	}
	for _, key := range preLedgerParents {
		if len(parents.order[key]) == 0 {
			fail("no release before %s shipped %s", closingTag, key)
		}
	}
	for _, digest := range committedDigests {
		if !agents.has(digest) && !parents.has(digest) {
			fail("digest %s in %s was not regenerated; the local tag set is incomplete; run git fetch --tags", digest, output)
		}
	}
	for _, digest := range openCodeDigests {
		if !openCode.has(digest) {
			fail("digest %s in %s was not regenerated; the local tag set is incomplete; run git fetch --tags", digest, openCodeOutput)
		}
	}
	var b bytes.Buffer
	b.WriteString("// Code generated by scripts/gen-sdd-agent-digests; DO NOT EDIT.\n\npackage legacyassets\n\n")
	writeTags(&b, tags)
	agents.write(&b, "releasedSDDAgentDigests", "// releasedSDDAgentDigests maps <family>/<file> to the normalized digest of\n// every native SDD sub-agent template a Gentle AI release shipped.\n")
	parents.write(&b, "releasedPreLedgerNativeAgentDigests", "// releasedPreLedgerNativeAgentDigests maps <family>/<file> to the normalized\n// digest of every retained native agent file a release before "+closingTag+"\n// shipped that declared SDD subagents.\n")
	writeSource(output, b.Bytes())
	openCode.write(openCodeOutput, tags)
}

func writeTags(b *bytes.Buffer, tags []string) {
	b.WriteString("// The registry was generated from these release tags, closed at " + closingTag + ".\n")
	b.WriteString("// The generator refuses a local tag set missing any of them.\n")
	for start := 0; start < len(tags); start += tagsPerLine {
		b.WriteString(tagsLinePrefix + strings.Join(tags[start:min(start+tagsPerLine, len(tags))], " ") + "\n")
	}
	b.WriteString("\n")
}

func writeSource(output string, source []byte) {
	formatted, err := format.Source(source)
	if err != nil {
		fail("format %s: %v", output, err)
	}
	if err := os.WriteFile(output, formatted, 0o644); err != nil {
		fail("write %s: %v", output, err)
	}
}

const (
	tagsLinePrefix = "// tags: "
	tagsPerLine    = 10
)

var committedDigest = regexp.MustCompile(`"([0-9a-f]{64})"`)

// readCommitted returns the release tags and digests the existing registry
// records, so regeneration from a partial tag set cannot drop history.
func readCommitted(path string) ([]string, []string) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		fail("read committed registry: %v", err)
	}
	var tags []string
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, tagsLinePrefix); ok {
			tags = append(tags, strings.Fields(rest)...)
		}
	}
	var digests []string
	for _, match := range committedDigest.FindAllStringSubmatch(string(data), -1) {
		digests = append(digests, match[1])
	}
	return tags, digests
}

var verifiedGoldens int

// verifyGoldens fails generation when a release's own golden render of an
// SDD agent does not normalize to that release's template.
func verifyGoldens(tag string, templates map[string]string) {
	for _, line := range strings.Split(git("ls-tree", "-r", "--name-only", tag, "--", "testdata/golden"), "\n") {
		name := path.Base(line)
		for family := range sddAgentFamilies() {
			prefix := "sdd-" + family + "-agent-"
			if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".golden") {
				continue
			}
			key := family + "/" + strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".golden") + ".md"
			want, ok := templates[key]
			if !ok {
				continue
			}
			if got := legacyassets.SDDAgentDigest(git("show", tag+":"+line)); got != want {
				fail("%s: golden %s does not normalize to its template %s", tag, line, key)
			}
			verifiedGoldens++
		}
	}
}

func sddAgentFamilies() map[string]bool {
	families := map[string]bool{}
	for _, family := range legacyassets.SDDAgentFamilies {
		families[family] = true
	}
	return families
}

// root is the repository top level; pathspecs are relative to it.
var root string

func git(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		fail("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen-sdd-agent-digests: "+format+"\n", args...)
	os.Exit(1)
}
