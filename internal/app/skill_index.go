package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v3/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v3/internal/skillregistry"
	"github.com/gentleman-programming/gentle-ai/v3/internal/state"
)

func init() {
	cli.PostSyncSkillRegenerator = func(workspaceDir, homeDir string) (int, error) {
		result, skipped, err := runSharedRefresh(skillIndexRefreshArgs{
			quiet:           true,
			ensureGitignore: true,
			cwd:             workspaceDir,
			home:            homeDir,
		}, io.Discard)
		if err != nil {
			return 0, err
		}
		if skipped {
			return 0, nil
		}
		return result.SkillCount, nil
	}
}

// skillIndexSurface labels parser and usage errors so one parser serves both
// `axiom skill index` (REQ-22.10) and the legacy `axiom skill-registry` verb
// (REQ-22.14) without either surface changing its messages (D-13).
type skillIndexSurface struct {
	// label frames unknown-subcommand and unknown-flag errors.
	label string
	// usage is the exact usage line of the missing-subcommand error.
	usage string
}

var (
	skillIndexSurfaceIndex = skillIndexSurface{
		label: "skill index",
		usage: "usage: axiom skill index <refresh|list> [flags]",
	}
	// skillIndexSurfaceLegacy labels the shared parser errors of the legacy
	// verb. Its own dispatch messages stay literal in app.go (inherited
	// branding, out of scope).
	skillIndexSurfaceLegacy = skillIndexSurface{
		label: "skill-registry",
		usage: "usage: gentle-ai skill-registry <refresh|list> [flags]",
	}
)

// Secondary destination labels of `axiom skill index refresh` (spec §1.1).
const (
	agentsDestinationLabel = "AGENTS.md"
	mirrorDestinationLabel = "Engram"
)

// skillIndexRefreshArgs holds the parsed refresh flags of either surface.
type skillIndexRefreshArgs struct {
	force           bool
	quiet           bool
	ensureGitignore bool
	cwd             string
	// home is the user home used to locate skills. Empty resolves the process
	// user home directory.
	home string
}

// skillIndexListArgs holds the parsed list flags of either surface.
type skillIndexListArgs struct {
	cwd    string
	asJSON bool
}

// RunSkillIndex is the `axiom skill index` entry point for the CLI dispatcher
// (REQ-22.10). It returns the process exit code defined in spec §1.1.
func RunSkillIndex(args []string, stdout, stderr io.Writer) int {
	return runSkillIndex(skillIndexSurfaceIndex, args, stdout, stderr)
}

// runSkillIndex dispatches `refresh`/`list` for one surface and maps every
// outcome to the exit-code table of spec §1.1 (D-13).
func runSkillIndex(surface skillIndexSurface, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "Error: %s\n", surface.usage)
		return 1
	}
	switch args[0] {
	case "refresh":
		return runSkillIndexRefresh(surface, args[1:], stdout, stderr)
	case "list":
		return runSkillIndexList(surface, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "Error: unknown %s command %q (want refresh or list)\n", surface.label, args[0])
		return 1
	}
}

// runSkillIndexRefresh reports the primary line plus one line per secondary
// destination (spec §1.1). A mirror failure is reported as `mirror failed` with
// a warning and exit 0: it never fails a correct primary registration
// (REQ-22.11).
func runSkillIndexRefresh(surface skillIndexSurface, args []string, stdout, stderr io.Writer) int {
	parsed, err := parseSkillIndexRefreshFlags(surface, args)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	result, skipped, err := runSharedRefresh(parsed, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if skipped {
		return 0
	}
	if !parsed.quiet {
		writePrimaryRefreshLine(stdout, result)
		fmt.Fprintf(stdout, "%s: %s\n", agentsDestinationLabel, result.Agents.Status)
		fmt.Fprintf(stdout, "%s: %s\n", mirrorDestinationLabel, mirrorDestinationStatus(result))
	}
	if result.Regenerated && result.Mirror.Status == skillregistry.MirrorFailed {
		fmt.Fprintf(stderr, "Warning: skill index mirror failed: %v\n", result.Mirror.Err)
	}
	return 0
}

// runSkillIndexList prints the read-only index view. No destination, cache,
// .gitignore or memory is written (REQ-22.10).
func runSkillIndexList(surface skillIndexSurface, args []string, stdout, stderr io.Writer) int {
	if err := writeSkillIndexList(surface, args, stdout); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// RegenerateSkillsIndex is the production IndexRegenerator injected into
// autoskill.Manager (REQ-22.13, D-12): it refreshes the unified skills index
// with Force: false and the real Engram mirror. A mirror failure travels the
// returned error because the hook has no richer report channel; callers must
// report it as a warning and never revert the promotion.
func RegenerateSkillsIndex(cwd, home string) error {
	result, skipped, err := runSharedRefresh(skillIndexRefreshArgs{
		quiet:           true,
		ensureGitignore: true,
		cwd:             cwd,
		home:            home,
	}, io.Discard)
	if err != nil {
		return err
	}
	if skipped {
		return fmt.Errorf("skill index regeneration skipped: %s is not a project root", filepath.Clean(cwd))
	}
	if result.Mirror.Status == skillregistry.MirrorFailed {
		return fmt.Errorf("skill index mirror failed: %w", result.Mirror.Err)
	}
	return nil
}

// parseSkillIndexRefreshFlags parses the shared refresh flag set of either
// surface; surface labels the error messages so `skill-registry` keeps its
// bytes (D-13, REQ-22.14).
func parseSkillIndexRefreshFlags(surface skillIndexSurface, args []string) (skillIndexRefreshArgs, error) {
	parsed := skillIndexRefreshArgs{ensureGitignore: true}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--force", "-f":
			parsed.force = true
		case "--quiet", "-q":
			parsed.quiet = true
		case "--no-gitignore":
			parsed.ensureGitignore = false
		case "--cwd":
			if i+1 >= len(args) {
				return parsed, errors.New("--cwd requires a value")
			}
			parsed.cwd = args[i+1]
			i++
		default:
			return parsed, fmt.Errorf("unknown %s refresh argument %q", surface.label, args[i])
		}
	}
	return parsed, nil
}

// parseSkillIndexListFlags parses the shared list flag set of either surface.
func parseSkillIndexListFlags(surface skillIndexSurface, args []string) (skillIndexListArgs, error) {
	var parsed skillIndexListArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			parsed.asJSON = true
		case "--cwd":
			if i+1 >= len(args) {
				return parsed, errors.New("--cwd requires a value")
			}
			parsed.cwd = args[i+1]
			i++
		default:
			return parsed, fmt.Errorf("unknown %s list argument %q", surface.label, args[i])
		}
	}
	return parsed, nil
}

// productionMirrorFn is a package-level seam over productionMirror (the
// execCommandContext/stdioHandshakeFn precedent) so tests can pin the mirror
// outcome without spawning a real engram MCP server.
var productionMirrorFn = productionMirror

// runSharedRefresh resolves the workspace, honours the non-project-root skip
// and drives the unified engine (D-13): one Regenerate call feeds every
// destination. notice receives the one-line skip notice unless quiet.
func runSharedRefresh(parsed skillIndexRefreshArgs, notice io.Writer) (skillregistry.Result, bool, error) {
	cwd, home, err := resolveSkillRegistryDirs(parsed.cwd, parsed.home)
	if err != nil {
		return skillregistry.Result{}, false, err
	}
	// Startup hooks run refresh from whatever directory the host resolved; a
	// brand-new non-project directory can resolve to "/", $HOME, or a
	// markerless folder. Never initialize there: skip silently under --quiet
	// (a startup hook must not scream) and with a one-line notice otherwise.
	if reason := skillregistry.RefreshSkip(cwd, home); reason != skillregistry.SkipNone {
		if !parsed.quiet {
			_, _ = fmt.Fprintf(notice, "Skill registry refresh skipped (%s): %s is not a project root; run it from a project directory (one containing .git or .atl), or create the project first.\n", reason, cwd)
		}
		return skillregistry.Result{}, true, nil
	}
	if parsed.ensureGitignore {
		if err := skillregistry.EnsureATLIgnored(cwd); err != nil {
			return skillregistry.Result{}, false, err
		}
	}
	result, err := skillregistry.Regenerate(cwd, home, skillregistry.RegenerateOptions{
		Force:  parsed.force,
		Mirror: productionMirrorFn(cwd, home),
	})
	return result, false, err
}

// writePrimaryRefreshLine prints the primary line shared by both surfaces.
func writePrimaryRefreshLine(stdout io.Writer, result skillregistry.Result) {
	if result.Regenerated {
		_, _ = fmt.Fprintf(stdout, "Skill registry refreshed (%d skills): %s\n", result.SkillCount, result.Registry)
	} else {
		_, _ = fmt.Fprintf(stdout, "Skill registry up to date (%s): %s\n", result.Reason, result.Registry)
	}
}

// mirrorDestinationStatus maps the engine outcome to the printed literal. A
// cache-hit attempted no write and left the mirror unchanged: T-8 forbids
// claiming `mirror ok` for a write never attempted.
func mirrorDestinationStatus(result skillregistry.Result) string {
	if !result.Regenerated {
		return string(skillregistry.DestUnchanged)
	}
	return string(result.Mirror.Status)
}

// writeSkillIndexList renders the read-only index view shared by both surfaces
// (D-13): TSV by default, JSON with --json.
func writeSkillIndexList(surface skillIndexSurface, args []string, stdout io.Writer) error {
	parsed, err := parseSkillIndexListFlags(surface, args)
	if err != nil {
		return err
	}
	cwd, home, err := resolveSkillRegistryDirs(parsed.cwd, "")
	if err != nil {
		return err
	}
	entries := skillregistry.List(cwd, home)

	if parsed.asJSON {
		type row struct {
			Name        string `json:"name"`
			Scope       string `json:"scope"`
			Description string `json:"description"`
			Path        string `json:"path"`
		}
		rows := make([]row, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, row{
				Name:        e.Name,
				Scope:       skillregistry.ScopeForPath(cwd, e.Path),
				Description: e.Description,
				Path:        e.Path,
			})
		}
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(stdout, string(data))
		return nil
	}

	if len(entries) == 0 {
		_, _ = fmt.Fprintln(stdout, "No skills found.")
		return nil
	}
	for _, e := range entries {
		_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", e.Name, skillregistry.ScopeForPath(cwd, e.Path), e.Path)
	}
	return nil
}

// productionMirror wires the Engram mirror (D-11) through the installed engram
// MCP stdio server. The Engram project name enters at this single point (open
// decision O-5): changing the resolution rule touches this call only, never
// the engine.
func productionMirror(cwd, home string) skillregistry.MirrorFunc {
	project := resolveEngramProject(cwd)
	return func(req skillregistry.MirrorRequest) error {
		command, args, err := resolveEngramStdioCommand(home)
		if err != nil {
			return err
		}
		req.Project = project
		return engram.SaveTopic(context.Background(), command, args, engram.SaveTopicRequest{
			Title:         req.Title,
			Content:       req.Content,
			Type:          req.Type,
			Project:       req.Project,
			TopicKey:      req.TopicKey,
			CapturePrompt: req.CapturePrompt,
		})
	}
}

// resolveEngramProject returns the Engram project scope of a workspace. Open
// decision O-5 (not resolved here): the exact project-name rule for a
// workspace. The workspace directory name is the current resolution and this
// function is the single parameter point (D-11).
func resolveEngramProject(cwd string) string {
	return filepath.Base(cwd)
}

// resolveEngramStdioCommand reads the stdio command installed agents persist
// for Engram (D-11): the doctor's resolution rule, never a guessed PATH
// binary. Every miss is an ordinary error the engine maps to `mirror failed`.
func resolveEngramStdioCommand(home string) (string, []string, error) {
	current, err := state.Read(home)
	if err != nil {
		return "", nil, fmt.Errorf("read install state: %w", err)
	}
	commands, err := engram.ReadPersistedStdioCommands(home, current.InstalledAgents)
	if err != nil {
		return "", nil, fmt.Errorf("read persisted engram MCP configuration: %w", err)
	}
	if len(commands) == 0 {
		return "", nil, errors.New("no persisted engram MCP configuration found for installed agents")
	}
	return commands[0].Command, commands[0].Args, nil
}
