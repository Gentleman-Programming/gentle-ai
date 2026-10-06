package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Two retired SDD files were rendered by Go code, with no golden of either:
//   - Codex's sdd-{strong,mid,cheap}.config.toml profiles (v1.36.0 to
//     v3.7.0), upserted by codex.WriteCodexProfiles into whatever the file
//     held, so a render depends on the state the previous release left;
//   - Kimi's sdd-orchestrator.md Jinja module (v1.21.0 to v3.7.0), the
//     embedded asset verbatim until v1.46.0 and rendered by the SDD
//     component afterwards.
//
// The generator replays each release's own writer: it extracts the release's
// internal tree, adds a test that calls the exact expression the release used,
// and runs it offline (GOTOOLCHAIN=local, -mod=readonly; the module cache must
// hold the release's dependencies). Releases whose render inputs are
// byte-identical share one replay. Codex profiles are replayed from absence
// and from every render an earlier release produced, so every upgrade path
// between releases is covered.
const (
	codexProfilesFile = "internal/agents/codex/profiles.go"
	kimiModuleAsset   = "internal/assets/kimi/sdd-orchestrator.md"
	kimiModuleKey     = "modules/sdd-orchestrator.md"
	replayTestFile    = "zz_gentle_ai_replay_test.go"
)

var (
	kimiModuleWrite     = regexp.MustCompile(`(?m)^\s*content := (.+)\n\s*modulePath := filepath\.Join\(configDir, "sdd-orchestrator\.md"\)\n\s*writeResult, err := filemerge\.WriteFileAtomic\(modulePath, \[\]byte\(content\), 0o644\)$`)
	kimiVerbatimAsset   = regexp.MustCompile(`case model\.AgentKimi:\n\s*return "kimi/sdd-orchestrator\.md"`)
	sessionPreflight    = "prompt, lazyWorkflow, workflows, err := prepareSessionPreflight(homeDir, adapter, opts)"
	promptReassignment  = regexp.MustCompile(`(?m)^\s*prompt\s*(?:=|\+=)[^=]`)
	codexWriterArgs     = map[string]string{"func WriteCodexProfiles(codexHomeDir string) (": "", "func WriteCodexProfiles(codexHomeDir string, assignments []ProfileAssignment) (": ", nil"}
	codexProfilePaths   = "func SddProfilePaths(codexHomeDir string) []string"
	modulePath          = regexp.MustCompile(`(?m)^module (\S+)$`)
	maxCodexReplayRound = 5
)

type replayGroup struct {
	tags []string
	pkg  string
	// test is the replay test source with the module path still to fill in.
	test string
	dir  string
	// outputs maps an output file name to every distinct render.
	outputs map[string][]string
}

type replayHarvest struct {
	kimi, codex []*replayGroup
	byKey       map[string]*replayGroup
	// verbatim maps a tag to the Kimi asset it installed unchanged.
	verbatim map[string]string
	tmp      string
}

func newReplayHarvest() *replayHarvest {
	return &replayHarvest{byKey: map[string]*replayGroup{}, verbatim: map[string]string{}}
}

// collect classifies one release's Kimi module and Codex profile writers.
func (h *replayHarvest) collect(tag string, files map[string]string) {
	if blob, ok := files[kimiModuleAsset]; ok {
		h.collectKimi(tag, files, cachedBlob(blob))
	}
	if blob, ok := files[codexProfilesFile]; ok {
		h.collectCodex(tag, cachedBlob(blob))
	}
}

func (h *replayHarvest) collectKimi(tag string, files map[string]string, asset string) {
	inject := cachedBlob(files[sddInjectFile])
	m := kimiModuleWrite.FindStringSubmatch(inject)
	if m == nil {
		fail("%s: the Kimi sdd-orchestrator.md module write is not recognized; update the generator", tag)
	}
	var render string
	switch expr := strings.TrimSpace(m[1]); expr {
	case "assets.MustRead(sddOrchestratorAsset(adapter.Agent()))":
		if !kimiVerbatimAsset.MatchString(inject) {
			fail("%s: sddOrchestratorAsset does not map Kimi to its own asset; update the generator", tag)
		}
		h.verbatim[tag] = asset
		return
	case "renderSDDOrchestratorAsset(adapter.Agent())", "renderSDDOrchestratorAsset(adapter.Agent(), opts.orchestratorPolicyRenderOptions())":
		render = "\treturn " + expr + "\n"
	case "prompt":
		body := inject[strings.Index(inject, "\nfunc Inject("):]
		body = body[:strings.Index(body[1:], "\nfunc ")+1]
		if strings.Count(body, sessionPreflight) != 1 || promptReassignment.MatchString(body) {
			fail("%s: the Kimi module prompt is not the session preflight render; update the generator", tag)
		}
		render = "\tprompt, _, _, err := prepareSessionPreflight(homeDir, adapter, opts)\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\treturn prompt\n"
	default:
		fail("%s: Kimi module render %q is not recognized; update the generator", tag, expr)
	}
	options := "\treturn []InjectOptions{{}}\n"
	if strings.Contains(render, "opts") {
		options = kimiReplayPolicyOptions
	}
	test := fmt.Sprintf(kimiReplayTest, options, render)
	h.group(tag, "./internal/components/sdd", test, "internal/components", "internal/assets", "internal/model", "internal/agents")
}

func (h *replayHarvest) collectCodex(tag, source string) {
	if !strings.Contains(source, "func WriteCodexProfiles(") {
		return // v4.0.0 keeps only the path inventory
	}
	args, found := "", false
	for signature, call := range codexWriterArgs {
		if strings.Contains(source, signature) {
			args, found = call, true
		}
	}
	if !found || !strings.Contains(source, codexProfilePaths) {
		fail("%s: the Codex profile writer is not recognized; update the generator", tag)
	}
	h.group(tag, "./internal/agents/codex", fmt.Sprintf(codexReplayTest, args), "internal/agents/codex", "internal/components/filemerge", "internal/model")
}

// group files tag under the replay of its render inputs: the test and every
// tree the writer reads.
func (h *replayHarvest) group(tag, pkg, test string, trees ...string) {
	args := []string{"rev-parse", tag + ":go.mod"}
	for _, tree := range trees {
		args = append(args, tag+":"+tree)
	}
	key := pkg + "\n" + test + "\n" + git(args...)
	if g := h.byKey[key]; g != nil {
		g.tags = append(g.tags, tag)
		return
	}
	g := &replayGroup{tags: []string{tag}, pkg: pkg, test: test}
	h.byKey[key] = g
	if pkg == "./internal/agents/codex" {
		h.codex = append(h.codex, g)
	} else {
		h.kimi = append(h.kimi, g)
	}
}

// run replays every group: Kimi once, Codex until its renders are closed
// under every upgrade.
func (h *replayHarvest) run() {
	var err error
	if h.tmp, err = os.MkdirTemp("", "gen-sdd-replay-"); err != nil {
		fail("replay workspace: %v", err)
	}
	cleanups = append(cleanups, func() { _ = os.RemoveAll(h.tmp) })
	// A stable directory per group keeps the Go build cache warm across
	// Codex rounds; each run extracts it and removes it again.
	for i, g := range append(append([]*replayGroup(nil), h.kimi...), h.codex...) {
		g.dir = filepath.Join(h.tmp, strconv.Itoa(i))
	}
	parallel(h.kimi, func(g *replayGroup) { g.replay(nil) })
	// Groups are in first-release order. Each writer rewrites absence, every
	// render an earlier release left, and its own renders until they are
	// stable. Downgrades are not modeled: a v1.36.0 writer appends a blank
	// line to a later render on every run, so their renders are unbounded,
	// and such a file is preserved and reported instead.
	var states []string
	seen := map[string]bool{}
	for _, g := range h.codex {
		inputs := append([]string(nil), states...)
		for round := 1; ; round++ {
			if round > maxCodexReplayRound {
				fail("%s: Codex profile renders did not stabilize after %d rounds", g.tags[0], maxCodexReplayRound)
			}
			g.replay(inputs)
			grown := false
			for _, name := range sortedKeys(g.outputs) {
				for _, render := range g.outputs[name] {
					if !slices.Contains(inputs, render) {
						inputs = append(inputs, render)
						grown = true
					}
				}
			}
			if !grown {
				break
			}
		}
		for _, name := range sortedKeys(g.outputs) {
			for _, render := range g.outputs[name] {
				if !seen[render] {
					seen[render] = true
					states = append(states, render)
				}
			}
		}
	}
}

// replay runs the group's test with inputs (the Codex states to rewrite) and
// records every render it writes.
func (g *replayGroup) replay(inputs []string) {
	extract(g.tags[0], g.dir)
	defer os.RemoveAll(g.dir)
	module := modulePath.FindStringSubmatch(readFile(filepath.Join(g.dir, "go.mod")))
	if module == nil {
		fail("%s: go.mod declares no module", g.tags[0])
	}
	test := strings.ReplaceAll(g.test, "{{MODULE}}", module[1])
	if err := os.WriteFile(filepath.Join(g.dir, filepath.FromSlash(strings.TrimPrefix(g.pkg, "./")), replayTestFile), []byte(test), 0o644); err != nil {
		fail("write replay test: %v", err)
	}
	in, out := filepath.Join(g.dir, "replay-in"), filepath.Join(g.dir, "replay-out")
	for _, dir := range []string{in, out} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail("create %s: %v", dir, err)
		}
	}
	for i, input := range inputs {
		if err := os.WriteFile(filepath.Join(in, strconv.Itoa(i+1)), []byte(input), 0o644); err != nil {
			fail("write replay input: %v", err)
		}
	}
	cmd := exec.Command("go", "test", "-count=1", "-vet=off", "-run", "^TestGentleAIReplay$", g.pkg)
	cmd.Dir = g.dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly", "GOWORK=off", "GENTLE_AI_REPLAY_IN="+in, "GENTLE_AI_REPLAY_OUT="+out)
	if output, err := cmd.CombinedOutput(); err != nil {
		fail("%s: replay %s: %v\n%s", g.tags[0], g.pkg, err, output)
	}
	if g.outputs == nil {
		g.outputs = map[string][]string{}
	}
	err := filepath.WalkDir(out, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		render := readFile(path)
		if name := entry.Name(); !slices.Contains(g.outputs[name], render) {
			g.outputs[name] = append(g.outputs[name], render)
		}
		return nil
	})
	if err != nil || len(g.outputs) == 0 {
		fail("%s: replay %s wrote no render: %v", g.tags[0], g.pkg, err)
	}
}

// register adds every render to the asset registry in release order.
func (h *replayHarvest) register(tags []string, assets *assetHarvest) {
	groupOf := map[string]*replayGroup{}
	for _, g := range append(append([]*replayGroup(nil), h.kimi...), h.codex...) {
		for _, tag := range g.tags {
			groupOf[g.pkg+tag] = g
		}
	}
	for _, tag := range tags {
		if asset, ok := h.verbatim[tag]; ok {
			assets.add(kimiModuleKey, asset, tag)
		}
		if g := groupOf["./internal/components/sdd"+tag]; g != nil {
			for _, render := range g.outputs["kimi"] {
				assets.add(kimiModuleKey, render, tag)
			}
		}
		if g := groupOf["./internal/agents/codex"+tag]; g != nil {
			for _, name := range sortedKeys(g.outputs) {
				if !strings.HasPrefix(name, "sdd-") || !strings.HasSuffix(name, ".config.toml") {
					fail("%s: unexpected Codex profile %s", tag, name)
				}
				for _, render := range g.outputs[name] {
					assets.add("profiles/"+name, render, tag)
				}
			}
		}
	}
	fmt.Fprintf(os.Stderr, "gen-sdd-agent-digests: replayed %d Kimi module and %d Codex profile writers\n", len(h.kimi), len(h.codex))
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func parallel(groups []*replayGroup, run func(*replayGroup)) {
	var wg sync.WaitGroup
	next := make(chan int)
	for range min(4, runtime.NumCPU()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				run(groups[i])
			}
		}()
	}
	for i := range groups {
		next <- i
	}
	close(next)
	wg.Wait()
}

// extract writes tag's module files and internal tree, without tests, to dir.
func extract(tag, dir string) {
	archive := gitBytes("archive", "--format=tar", tag, "--", "go.mod", "go.sum", "internal", ":(exclude)*_test.go")
	reader := tar.NewReader(bytes.NewReader(archive))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			fail("%s: read archive: %v", tag, err)
		}
		path := filepath.Join(dir, filepath.FromSlash(header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, 0o755)
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
				var data []byte
				if data, err = io.ReadAll(reader); err == nil {
					err = os.WriteFile(path, data, 0o644)
				}
			}
		}
		if err != nil {
			fail("%s: extract %s: %v", tag, header.Name, err)
		}
	}
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		fail("read %s: %v", path, err)
	}
	return string(data)
}

const kimiReplayTest = `package sdd

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"{{MODULE}}/internal/agents/kimi"
)

var _ = reflect.TypeOf

func TestGentleAIReplay(t *testing.T) {
	out := os.Getenv("GENTLE_AI_REPLAY_OUT")
	for i, opts := range gentleAIReplayOptions(t) {
		dir := filepath.Join(out, strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "kimi"), []byte(gentleAIReplayRender(t, t.TempDir(), kimi.NewAdapter(), opts)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// gentleAIReplayOptions covers every value of the orchestrator policy the
// release's render reads from its options.
func gentleAIReplayOptions(t *testing.T) []InjectOptions {
%s}

func gentleAIReplayRender(t *testing.T, homeDir string, adapter *kimi.Adapter, opts InjectOptions) string {
	_, _ = homeDir, opts
%s}
`

const kimiReplayPolicyOptions = `	policy := reflect.TypeOf(OrchestratorRenderOptions{})
	if policy.NumField() != 1 || policy.Field(0).Type.Kind() != reflect.Bool {
		t.Fatalf("OrchestratorRenderOptions changed shape; update the generator")
	}
	var on InjectOptions
	field := reflect.ValueOf(&on).Elem().FieldByName(policy.Field(0).Name)
	if !field.IsValid() || field.Kind() != reflect.Bool {
		t.Fatalf("InjectOptions has no %s policy option; update the generator", policy.Field(0).Name)
	}
	field.SetBool(true)
	return []InjectOptions{{}, on}
`

const codexReplayTest = `package codex

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestGentleAIReplay writes the profiles from absence (input 0) and over
// every numbered input state.
func TestGentleAIReplay(t *testing.T) {
	in, out := os.Getenv("GENTLE_AI_REPLAY_IN"), os.Getenv("GENTLE_AI_REPLAY_OUT")
	for i := 0; ; i++ {
		dir := t.TempDir()
		if i > 0 {
			data, err := os.ReadFile(filepath.Join(in, strconv.Itoa(i)))
			if os.IsNotExist(err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range SddProfilePaths(dir) {
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, _, err := WriteCodexProfiles(dir%s); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(out, strconv.Itoa(i))
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, path := range SddProfilePaths(dir) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, filepath.Base(path)), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
`
