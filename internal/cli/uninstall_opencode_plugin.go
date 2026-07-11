package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/opencodeplugin"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// UninstallOpenCodePluginFlags are the parsed flags for the
// `gentle-ai uninstall opencode-plugin <id>` sub-command.
type UninstallOpenCodePluginFlags struct {
	PluginID model.OpenCodeCommunityPluginID
	Yes      bool
}

// validOpenCodePluginIDs enumerates the OpenCode community plugin IDs that
// the uninstall CLI accepts. It mirrors the constants in
// internal/model/types.go — kept inline so this module is the single place
// that surfaces the canonical list to a user typing an invalid value.
var validOpenCodePluginIDs = []model.OpenCodeCommunityPluginID{
	model.OpenCodePluginSubAgentStatusline,
	model.OpenCodePluginSDDEngramManage,
	model.OpenCodePluginGentleLogo,
}

// ParseUninstallOpenCodePluginFlags parses the args after "uninstall opencode-plugin".
// Expects exactly one positional argument (the plugin id). --yes/-y are hoisted
// before flag.Parse because Go's flag package stops scanning flags at the
// first non-flag argument, and the natural usage
// `gentle-ai uninstall opencode-plugin <id> --yes` puts the flag after the id.
func ParseUninstallOpenCodePluginFlags(args []string) (UninstallOpenCodePluginFlags, error) {
	var opts UninstallOpenCodePluginFlags

	yesFound, rest := hoistBoolFlag(args, "--yes")
	shortFound, rest := hoistBoolFlag(rest, "-y")
	opts.Yes = yesFound || shortFound

	fs := flag.NewFlagSet("uninstall opencode-plugin", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})

	if err := fs.Parse(rest); err != nil {
		return UninstallOpenCodePluginFlags{}, err
	}
	if fs.NArg() != 1 {
		return UninstallOpenCodePluginFlags{}, fmt.Errorf("usage: gentle-ai uninstall opencode-plugin <id> [--yes]")
	}

	id := model.OpenCodeCommunityPluginID(strings.TrimSpace(fs.Arg(0)))
	if !isValidOpenCodePluginID(id) {
		return UninstallOpenCodePluginFlags{}, fmt.Errorf(
			"invalid opencode plugin id %q — valid ids: %s",
			id,
			strings.Join(validIDsAsStrings(), ", "),
		)
	}
	opts.PluginID = id

	return opts, nil
}

// hoistBoolFlag pulls name out of args (if present) and returns (true, args
// without it). Both the bare --name and --name=value forms are accepted; the
// value side is ignored because this helper only signals a boolean switch.
func hoistBoolFlag(args []string, name string) (bool, []string) {
	filtered := make([]string, 0, len(args))
	found := false
	for _, arg := range args {
		if arg == name {
			found = true
			continue
		}
		if strings.HasPrefix(arg, name+"=") {
			found = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return found, filtered
}

// RunUninstallOpenCodePlugin is the CLI entry point for the sub-command.
// Prompts for confirmation unless flags.Yes, then calls opencodeplugin.Uninstall.
// Writes a report to stdout on success or surfaces the error.
func RunUninstallOpenCodePlugin(args []string, stdout io.Writer) (opencodeplugin.UninstallResult, error) {
	return runUninstallOpenCodePluginWithInput(args, stdout, os.Stdin)
}

func runUninstallOpenCodePluginWithInput(args []string, stdout io.Writer, stdin io.Reader) (opencodeplugin.UninstallResult, error) {
	flags, err := ParseUninstallOpenCodePluginFlags(args)
	if err != nil {
		return opencodeplugin.UninstallResult{}, err
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return opencodeplugin.UninstallResult{}, fmt.Errorf("uninstall opencode plugin %q: resolve home directory: %w", flags.PluginID, err)
	}

	if !flags.Yes {
		confirmed, err := promptUninstallOpenCodePluginConfirm(flags.PluginID, stdout, stdin)
		if err != nil {
			return opencodeplugin.UninstallResult{}, err
		}
		if !confirmed {
			_, _ = fmt.Fprintln(stdout, "uninstall cancelled")
			return opencodeplugin.UninstallResult{}, nil
		}
	}

	result, err := opencodeplugin.Uninstall(homeDir, flags.PluginID)
	if err != nil {
		return result, fmt.Errorf("uninstall opencode plugin %q: %w", flags.PluginID, err)
	}

	_, _ = fmt.Fprintln(stdout, RenderUninstallOpenCodePluginReport(result))
	return result, nil
}

func promptUninstallOpenCodePluginConfirm(id model.OpenCodeCommunityPluginID, stdout io.Writer, stdin io.Reader) (bool, error) {
	name := pluginDisplayName(id)
	def, hasDef := opencodeplugin.DefinitionFor(id)
	pkg := string(id)
	if hasDef {
		pkg = def.PackageName
	}

	_, _ = fmt.Fprintf(stdout, "This will uninstall community plugin: %s\n", name)
	_, _ = fmt.Fprintf(stdout, "  Layer 1: removes entry from ~/.config/opencode/tui.json\n")
	if hasDef {
		_, _ = fmt.Fprintf(stdout, "  Layer 2: removes %s from package.json dependencies\n", pkg)
		_, _ = fmt.Fprintf(stdout, "  Layer 3: removes ~/.config/opencode/node_modules/%s/\n", pkg)
		_, _ = fmt.Fprintf(stdout, "  Layer 4: removes ~/.cache/opencode/packages/%s@latest\n", pkg)
	} else {
		_, _ = fmt.Fprintf(stdout, "  Plus: removes the local .tsx file ~/.config/opencode/tui-plugins/gentle-logo.tsx\n")
	}
	_, _ = fmt.Fprintln(stdout, "A journal snapshot will be created; on failure the changes are rolled back.")
	_, _ = fmt.Fprint(stdout, "Type 'yes' to confirm: ")

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if scanErr := scanner.Err(); scanErr != nil {
			return false, fmt.Errorf("read uninstall confirmation: %w", scanErr)
		}
		return false, nil
	}
	return strings.EqualFold(strings.TrimSpace(scanner.Text()), "yes"), nil
}

// RenderUninstallOpenCodePluginReport renders a human-readable report of
// what the 4-layer engine touched.
func RenderUninstallOpenCodePluginReport(result opencodeplugin.UninstallResult) string {
	var b strings.Builder

	name := pluginDisplayName(result.PluginID)
	_, _ = fmt.Fprintf(&b, "Uninstalled community plugin: %s\n", name)
	_, _ = fmt.Fprintf(&b, "  Plugin id: %s\n", string(result.PluginID))

	def, hasDef := opencodeplugin.DefinitionFor(result.PluginID)
	pkg := string(result.PluginID)
	if hasDef {
		pkg = def.PackageName
	}

	if result.ChangedTUI {
		_, _ = fmt.Fprintln(&b, "  Layer 1 (tui.json): updated — entry removed")
	} else {
		_, _ = fmt.Fprintln(&b, "  Layer 1 (tui.json): unchanged")
	}

	if result.PluginID == model.OpenCodePluginGentleLogo {
		if result.TSXPath != "" {
			_, _ = fmt.Fprintf(&b, "  Layer TSX: removed %s\n", result.TSXPath)
		} else {
			_, _ = fmt.Fprintln(&b, "  Layer TSX: no file present")
		}
		return strings.TrimRight(b.String(), "\n")
	}

	if result.ChangedPackageJSON {
		_, _ = fmt.Fprintf(&b, "  Layer 2 (package.json): updated — %s removed from dependencies\n", pkg)
	} else {
		_, _ = fmt.Fprintln(&b, "  Layer 2 (package.json): unchanged")
	}

	if result.ChangedNodeModules {
		_, _ = fmt.Fprintf(&b, "  Layer 3 (node_modules): removed %s\n", result.NodeModulesPath)
	} else {
		_, _ = fmt.Fprintln(&b, "  Layer 3 (node_modules): no directory present")
	}

	if result.CacheEntryRemoved != "" {
		_, _ = fmt.Fprintf(&b, "  Layer 4 (cache): removed %s\n", result.CacheEntryRemoved)
	} else {
		_, _ = fmt.Fprintln(&b, "  Layer 4 (cache): no entry present")
	}

	if hasDef {
		_, _ = fmt.Fprintf(&b, "  Repository: %s\n", def.RepoURL)
	}

	return strings.TrimRight(b.String(), "\n")
}

func pluginDisplayName(id model.OpenCodeCommunityPluginID) string {
	if def, ok := opencodeplugin.DefinitionFor(id); ok {
		return def.Name
	}
	switch id {
	case model.OpenCodePluginGentleLogo:
		return "Gentle Logo"
	}
	return string(id)
}

func isValidOpenCodePluginID(id model.OpenCodeCommunityPluginID) bool {
	for _, candidate := range validOpenCodePluginIDs {
		if candidate == id {
			return true
		}
	}
	return false
}

func validIDsAsStrings() []string {
	out := make([]string, 0, len(validOpenCodePluginIDs))
	for _, id := range validOpenCodePluginIDs {
		out = append(out, string(id))
	}
	return out
}