package skillregistry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// registryBytes reads one destination file for byte comparisons.
func registryBytes(t *testing.T, path string) string {
	t.Helper()
	return readFile(t, path)
}

// TestRegenerateOneScanFeedsThreeDestinations covers REQ-22.11 «Un escaneo
// alimenta los tres destinos»: registry, cache, AGENTS.md and the mirror port
// all see the same skill set from one scan.
func TestRegenerateOneScanFeedsThreeDestinations(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))
	writeSkill(t, filepath.Join(cwd, "skills", "branch-pr", "SKILL.md"), minimalSkill("branch-pr"))
	writeSkill(t, filepath.Join(cwd, "AGENTS.md"), agentsFixture)

	var calls []MirrorRequest
	mirror := func(req MirrorRequest) error {
		calls = append(calls, req)
		return nil
	}

	result, err := Regenerate(cwd, home, RegenerateOptions{Mirror: mirror})
	if err != nil {
		t.Fatalf("Regenerate() error = %v", err)
	}
	if !result.Regenerated || result.SkillCount != 2 {
		t.Fatalf("Result = %+v, want regenerated with 2 skills", result)
	}

	registry := registryBytes(t, filepath.Join(cwd, RegistryRelPath))
	agents := registryBytes(t, filepath.Join(cwd, AgentsRelPath))
	if _, statErr := os.Stat(filepath.Join(cwd, CacheRelPath)); statErr != nil {
		t.Fatalf("cache not written: %v", statErr)
	}
	for _, want := range []string{"go-testing", "branch-pr"} {
		if !strings.Contains(registry, "`"+want+"`") {
			t.Fatalf("registry missing %q:\n%s", want, registry)
		}
		if !strings.Contains(agents, "`"+want+"`") {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, agents)
		}
	}

	if len(calls) != 1 {
		t.Fatalf("MirrorFunc called %d times, want 1", len(calls))
	}
	req := calls[0]
	if req.TopicKey != MirrorTopicKey || req.Type != MirrorType {
		t.Fatalf("MirrorRequest topic/type = %q/%q, want %q/%q", req.TopicKey, req.Type, MirrorTopicKey, MirrorType)
	}
	if req.CapturePrompt {
		t.Fatal("CapturePrompt must be false for an automated artifact")
	}
	if req.Title == "" || req.Content == "" {
		t.Fatalf("MirrorRequest title/content must be populated: %+v", req)
	}
	for _, want := range []string{"go-testing", "branch-pr"} {
		if !strings.Contains(req.Content, "`"+want+"`") {
			t.Fatalf("mirror content missing %q:\n%s", want, req.Content)
		}
	}
	if result.Agents.Status != DestUpdated || result.Mirror.Status != MirrorOK {
		t.Fatalf("outcomes = agents=%q mirror=%q, want updated/mirror ok", result.Agents.Status, result.Mirror.Status)
	}
}

// TestRegenerateCacheHitWritesNothing covers REQ-22.11 «Sin cambios, la huella
// evita trabajo»: zero writes and all three destinations byte-identical.
func TestRegenerateCacheHitWritesNothing(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))
	writeSkill(t, filepath.Join(cwd, "AGENTS.md"), agentsFixture)

	var calls int
	mirror := func(MirrorRequest) error {
		calls++
		return nil
	}
	if _, err := Regenerate(cwd, home, RegenerateOptions{Mirror: mirror}); err != nil {
		t.Fatalf("first Regenerate() error = %v", err)
	}

	registryPath := filepath.Join(cwd, RegistryRelPath)
	cachePath := filepath.Join(cwd, CacheRelPath)
	agentsPath := filepath.Join(cwd, AgentsRelPath)
	beforeRegistry := registryBytes(t, registryPath)
	beforeCache := registryBytes(t, cachePath)
	beforeAgents := registryBytes(t, agentsPath)
	callsBefore := calls

	result, err := Regenerate(cwd, home, RegenerateOptions{Mirror: mirror})
	if err != nil {
		t.Fatalf("second Regenerate() error = %v", err)
	}
	if result.Regenerated || result.Reason != "cache-hit" {
		t.Fatalf("Result = %+v, want cache-hit without regeneration", result)
	}
	if calls != callsBefore {
		t.Fatalf("cache-hit must not attempt the mirror: %d calls", calls-callsBefore)
	}
	if result.Mirror.Status != "" {
		t.Fatalf("cache-hit Mirror.Status = %q, want the zero value (not attempted)", result.Mirror.Status)
	}
	if result.Agents.Status != DestUnchanged {
		t.Fatalf("cache-hit Agents.Status = %q, want %q", result.Agents.Status, DestUnchanged)
	}
	if got := registryBytes(t, registryPath); got != beforeRegistry {
		t.Fatal("registry changed on cache-hit")
	}
	if got := registryBytes(t, cachePath); got != beforeCache {
		t.Fatal("cache changed on cache-hit")
	}
	if got := registryBytes(t, agentsPath); got != beforeAgents {
		t.Fatal("AGENTS.md changed on cache-hit")
	}

	forced, err := Regenerate(cwd, home, RegenerateOptions{Force: true, Mirror: mirror})
	if err != nil {
		t.Fatalf("forced Regenerate() error = %v", err)
	}
	if !forced.Regenerated || forced.Reason != "forced" {
		t.Fatalf("forced Result = %+v, want regeneration with reason forced", forced)
	}
	if calls == callsBefore {
		t.Fatal("--force must regenerate and attempt the mirror")
	}
}

// TestRegenerateExclusionsReachEveryDestination covers REQ-22.11 «Filtro de
// exclusiones conservado»: the filter runs once in the scan and every
// destination sees only the surviving skills.
func TestRegenerateExclusionsReachEveryDestination(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "_shared", "SKILL.md"), minimalSkill("_shared"))
	writeSkill(t, filepath.Join(cwd, "skills", "skill-registry", "SKILL.md"), minimalSkill("skill-registry"))
	writeSkill(t, filepath.Join(cwd, "skills", "sdd-orchestrator", "SKILL.md"), minimalSkill("sdd-orchestrator"))
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))
	writeSkill(t, filepath.Join(cwd, "AGENTS.md"), agentsFixture)

	var mirrorContent string
	mirror := func(req MirrorRequest) error {
		mirrorContent = req.Content
		return nil
	}
	result, err := Regenerate(cwd, home, RegenerateOptions{Mirror: mirror})
	if err != nil {
		t.Fatalf("Regenerate() error = %v", err)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1 (the exclusion filter ran once)", result.SkillCount)
	}

	registry := registryBytes(t, filepath.Join(cwd, RegistryRelPath))
	agents := registryBytes(t, filepath.Join(cwd, AgentsRelPath))
	for name, content := range map[string]string{"registry": registry, "AGENTS.md": agents, "mirror": mirrorContent} {
		for _, excluded := range []string{"_shared", "skill-registry`", "sdd-orchestrator"} {
			if strings.Contains(content, excluded) {
				t.Fatalf("%s must not contain excluded skill %q:\n%s", name, excluded, content)
			}
		}
		if !strings.Contains(content, "`go-testing`") {
			t.Fatalf("%s missing the surviving skill:\n%s", name, content)
		}
	}
}

// TestRegenerateAgentsAbsentIsOmitted covers spec §3.6 no-creation at the
// engine level.
func TestRegenerateAgentsAbsentIsOmitted(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))

	result, err := Regenerate(cwd, home, RegenerateOptions{})
	if err != nil {
		t.Fatalf("Regenerate() error = %v", err)
	}
	if result.Agents.Status != DestOmitted {
		t.Fatalf("Agents.Status = %q, want %q", result.Agents.Status, DestOmitted)
	}
	if _, statErr := os.Stat(filepath.Join(cwd, AgentsRelPath)); !os.IsNotExist(statErr) {
		t.Fatalf("AGENTS.md must not be created (stat err = %v)", statErr)
	}
}

// TestRegenerateMirrorFailureIsDataNotError covers REQ-22.11 «El espejo Engram
// falla sin arrastrar el índice local».
func TestRegenerateMirrorFailureIsDataNotError(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))
	writeSkill(t, filepath.Join(cwd, "AGENTS.md"), agentsFixture)

	result, err := Regenerate(cwd, home, RegenerateOptions{Mirror: func(MirrorRequest) error {
		return errors.New("engram unreachable")
	}})
	if err != nil {
		t.Fatalf("mirror failure must not be fatal: %v", err)
	}
	if result.Mirror.Status != MirrorFailed || result.Mirror.Err == nil {
		t.Fatalf("Mirror = %+v, want mirror failed with an error", result.Mirror)
	}
	if result.Agents.Status != DestUpdated {
		t.Fatalf("Agents.Status = %q, the local index must still be regenerated", result.Agents.Status)
	}
}

// TestRegeneratePrimaryWriteFailureNamesTheDestination covers the "partial
// already emitted" contract of spec §1.1: the error wraps the destination.
func TestRegeneratePrimaryWriteFailureNamesTheDestination(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))
	if err := os.MkdirAll(filepath.Join(cwd, RegistryRelPath), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Regenerate(cwd, home, RegenerateOptions{})
	if err == nil {
		t.Fatal("registry write failure must be fatal")
	}
	if !strings.Contains(err.Error(), "write registry") {
		t.Fatalf("error %q must name the registry destination", err)
	}
}

// TestRegenerateAgentsFailureNamesTheDestination covers the fatal AGENTS.md
// destination: the error names the destination that broke.
func TestRegenerateAgentsFailureNamesTheDestination(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))
	if err := os.MkdirAll(filepath.Join(cwd, AgentsRelPath), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Regenerate(cwd, home, RegenerateOptions{})
	if err == nil {
		t.Fatal("AGENTS.md read failure must be fatal when the file exists")
	}
	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Fatalf("error %q must name AGENTS.md", err)
	}
}

// TestRegeneratePathContainment covers T-1: the --cwd vectors are resolved
// with filepath.Clean once, fail before any write when they do not exist, and
// no file ever appears outside the test sandbox.
func TestRegeneratePathContainment(t *testing.T) {
	sandbox := t.TempDir()
	home := t.TempDir()

	t.Run("relative", func(t *testing.T) {
		previous, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(sandbox); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(previous) })

		if err := os.MkdirAll(filepath.Join(sandbox, "ws"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeSkill(t, filepath.Join(sandbox, "ws", "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))
		result, err := Regenerate("ws", home, RegenerateOptions{})
		if err != nil {
			t.Fatalf("relative cwd must resolve: %v", err)
		}
		assertInsideSandbox(t, sandbox, result)
	})

	t.Run("absolute", func(t *testing.T) {
		target := filepath.Join(sandbox, "abs")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		result, err := Regenerate(target, home, RegenerateOptions{})
		if err != nil {
			t.Fatalf("absolute cwd must resolve: %v", err)
		}
		assertInsideSandbox(t, sandbox, result)
	})

	t.Run("forward slashes", func(t *testing.T) {
		target := filepath.Join(sandbox, "fwd")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		result, err := Regenerate(strings.ReplaceAll(target, "\\", "/"), home, RegenerateOptions{})
		if err != nil {
			t.Fatalf("forward-slash cwd must resolve: %v", err)
		}
		assertInsideSandbox(t, sandbox, result)
	})

	t.Run("back slashes", func(t *testing.T) {
		target := filepath.Join(sandbox, "back")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		result, err := Regenerate(strings.ReplaceAll(target, "/", "\\"), home, RegenerateOptions{})
		if err != nil {
			t.Fatalf("back-slash cwd must resolve: %v", err)
		}
		assertInsideSandbox(t, sandbox, result)
	})

	t.Run("dot dot resolves inside the sandbox", func(t *testing.T) {
		target := filepath.Join(sandbox, "dd")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		raw := target + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "dd"
		result, err := Regenerate(raw, home, RegenerateOptions{})
		if err != nil {
			t.Fatalf("dot-dot cwd must resolve once and run: %v", err)
		}
		assertInsideSandbox(t, sandbox, result)
	})

	t.Run("dot dot escaping the sandbox fails before any write", func(t *testing.T) {
		raw := sandbox + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "escape"
		if _, err := Regenerate(raw, home, RegenerateOptions{}); err == nil {
			t.Fatal("escaping cwd must fail")
		}
		assertNoWrites(t, raw)
	})

	t.Run("nonexistent fails before any write", func(t *testing.T) {
		target := filepath.Join(sandbox, "nope")
		if _, err := Regenerate(target, home, RegenerateOptions{}); err == nil {
			t.Fatal("nonexistent cwd must fail")
		}
		assertNoWrites(t, target)
	})

	t.Run("reserved device names fail before any write", func(t *testing.T) {
		for _, name := range []string{"con", "nul"} {
			if _, err := Regenerate(name, home, RegenerateOptions{}); err == nil {
				t.Fatalf("reserved name %q must fail", name)
			}
			assertNoWrites(t, name)
		}
	})

	t.Run("300 character path fails before any write", func(t *testing.T) {
		long := filepath.Join(sandbox, strings.Repeat("p", 300))
		if _, err := Regenerate(long, home, RegenerateOptions{}); err == nil {
			t.Fatal("300-character cwd must fail")
		}
		assertNoWrites(t, long)
	})
}

// assertInsideSandbox proves every written destination lives inside sandbox.
func assertInsideSandbox(t *testing.T, sandbox string, result Result) {
	t.Helper()
	for _, path := range []string{result.Registry, result.Cache, result.Agents.Path} {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			t.Fatalf("abs %s: %v", path, err)
		}
		rel, err := filepath.Rel(sandbox, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			t.Fatalf("destination %s escaped the sandbox (rel=%q err=%v)", abs, rel, err)
		}
	}
}

// assertNoWrites proves a failed run left no registry behind at the resolved
// cwd (writes happen only after the existence check passes). A Stat that does
// not report a regular file (absent path, reserved device name) counts as
// "nothing written".
func assertNoWrites(t *testing.T, cwd string) {
	t.Helper()
	for _, rel := range []string{RegistryRelPath, CacheRelPath} {
		info, err := os.Stat(filepath.Join(filepath.Clean(cwd), rel))
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() {
			t.Fatalf("failed run wrote %s under %q", rel, cwd)
		}
	}
}
