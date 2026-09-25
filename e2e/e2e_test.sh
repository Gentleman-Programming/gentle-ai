#!/usr/bin/env bash
# e2e_test.sh — End-to-end tests for gentle-ai installer
#
# Test tiers (controlled by environment variables):
#   (default)            Tier 1: binary existence + dry-run tests (fast, no side-effects)
#   RUN_FULL_E2E=1       Tier 2: full install tests (writes to filesystem)
#   RUN_BACKUP_TESTS=1   Tier 3: backup/restore tests
#
# Usage inside Docker:
#   ./e2e_test.sh                         # Tier 1 only
#   RUN_FULL_E2E=1 ./e2e_test.sh          # Tier 1 + 2
#   RUN_BACKUP_TESTS=1 ./e2e_test.sh      # Tier 1 + 3
#   RUN_FULL_E2E=1 RUN_BACKUP_TESTS=1 ./e2e_test.sh  # All tiers
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

# ---------------------------------------------------------------------------
# Resolve binary
# ---------------------------------------------------------------------------
BINARY="$(resolve_binary)"
if [ -z "$BINARY" ]; then
    echo "ERROR: gentle-ai binary not found. Build it first."
    exit 1
fi
log_info "Using binary: $BINARY"

# Side-effect E2E exercises install/injection behavior. Keep it deterministic by
# satisfying the installer's "engram already exists on PATH" branch unless a
# maintainer explicitly opts into the live GitHub release download path.
if [ "${RUN_FULL_E2E:-0}" = "1" ] || [ "${RUN_BACKUP_TESTS:-0}" = "1" ]; then
    setup_fake_engram_binary
fi

# ===========================================================================
# TIER 1 — Basic binary & dry-run tests (always run)
# ===========================================================================

# --- Category 1a: Binary basics ---

test_binary_exists() {
    log_test "Binary exists and is executable"

    if [ -x "$(command -v "$BINARY")" ] || [ -x "$BINARY" ]; then
        log_pass "Binary is executable"
    else
        log_fail "Binary not found or not executable"
    fi
}

test_binary_runs() {
    log_test "Binary runs without panic"

    if output=$($BINARY install --dry-run 2>&1); then
        log_pass "Binary exited cleanly with --dry-run"
    else
        if echo "$output" | grep -qi "panic"; then
            log_fail "Binary panicked: $output"
        else
            log_pass "Binary exited with non-zero (no panic)"
        fi
    fi
}

test_version_command() {
    log_test "Version command works"

    output=$($BINARY version 2>&1) || true

    if echo "$output" | grep -q "gentle-ai"; then
        log_pass "Version command returns binary name"
    else
        log_fail "Version command failed: $output"
    fi
}

# --- Category 1b: Dry-run output format ---

test_dry_run_output_format() {
    log_test "Dry-run output contains expected sections"

    output=$($BINARY install --dry-run 2>&1) || true

    assert_output_contains "$output" "dry-run" "Output contains 'dry-run' marker"
    assert_output_contains "$output" "Agents:" "Output contains 'Agents:' header"
    assert_output_contains "$output" "Persona:" "Output contains 'Persona:' header"
    assert_output_contains "$output" "Preset:" "Output contains 'Preset:' header"
    assert_output_contains "$output" "Components order:" "Output contains 'Components order:' header"
    assert_output_contains "$output" "Platform decision:" "Output contains 'Platform decision:' header"
}

test_dry_run_platform_detection() {
    log_test "Dry-run shows platform decision"

    output=$($BINARY install --dry-run 2>&1) || true

    assert_output_contains "$output" "Platform decision" "Platform decision present in dry-run"
}

test_dry_run_detects_linux() {
    log_test "Dry-run detects Linux OS"

    # This test is only meaningful inside the Docker container (Linux).
    # Skip gracefully on macOS/other to avoid killing the test run.
    if [[ "$(uname -s)" != "Linux" ]]; then
        log_skip "Not running on Linux — platform detection test skipped"
        return 0
    fi

    output=$($BINARY install --dry-run 2>&1) || true

    assert_output_contains "$output" "os=linux" "Platform detected as Linux"
}

# --- Category 1c: Agent flag ---

test_dry_run_agent_claude_code() {
    log_test "Dry-run with --agent claude-code"

    output=$($BINARY install --agent claude-code --dry-run 2>&1) || true

    assert_output_contains "$output" "claude-code" "Dry-run output shows claude-code agent"
}

test_dry_run_agent_opencode() {
    log_test "Dry-run with --agent opencode"

    output=$($BINARY install --agent opencode --dry-run 2>&1) || true

    assert_output_contains "$output" "opencode" "Dry-run output shows opencode agent"
}

test_dry_run_agent_both() {
    log_test "Dry-run with both agents"

    output=$($BINARY install --agent claude-code --agent opencode --dry-run 2>&1) || true

    assert_output_contains "$output" "claude-code" "Both agents: shows claude-code"
    assert_output_contains "$output" "opencode" "Both agents: shows opencode"
}

test_dry_run_agent_csv() {
    log_test "Dry-run with --agent as CSV"

    output=$($BINARY install --agent claude-code,opencode --dry-run 2>&1) || true

    assert_output_contains "$output" "claude-code" "CSV agents: shows claude-code"
    assert_output_contains "$output" "opencode" "CSV agents: shows opencode"
}

# --- Category 1d: Preset flags ---

test_dry_run_preset_minimal() {
    log_test "Dry-run with --preset minimal"

    output=$($BINARY install --preset minimal --dry-run 2>&1) || true

    assert_output_contains "$output" "Preset: minimal" "Shows minimal preset"
}

test_dry_run_preset_ecosystem() {
    log_test "Dry-run with --preset ecosystem-only"

    output=$($BINARY install --preset ecosystem-only --dry-run 2>&1) || true

    assert_output_contains "$output" "Preset: ecosystem-only" "Shows ecosystem-only preset"
}

test_dry_run_preset_full() {
    log_test "Dry-run with --preset full-gentleman"

    output=$($BINARY install --preset full-gentleman --dry-run 2>&1) || true

    assert_output_contains "$output" "Preset: full-gentleman" "Shows full-gentleman preset"
}

test_dry_run_preset_custom() {
    log_test "Dry-run with --preset custom"

    output=$($BINARY install --preset custom --dry-run 2>&1) || true

    assert_output_contains "$output" "Preset: custom" "Shows custom preset"
}

# --- Category 1e: Preset component order validation ---

test_preset_minimal_components() {
    log_test "Preset minimal with persona=custom produces only engram component"

    # Use persona=custom to test the preset alone, since persona is now
    # driven by Selection.Persona (decoupled from preset).
    output=$($BINARY install --preset minimal --persona custom --agent claude-code --dry-run 2>&1) || true

    # The component list should contain engram
    assert_output_contains "$output" "engram" "Minimal preset includes engram"
    # Persona is excluded when the user selects custom.
    assert_output_not_contains "$output" "Components order:.*persona" "Minimal + persona=custom excludes persona"
}

test_preset_minimal_with_default_persona_includes_persona() {
    log_test "Preset minimal with default persona (gentleman) includes persona"

    # Persona is now decoupled from preset — default Gentleman persona is
    # installed regardless of which preset the user picks.
    output=$($BINARY install --preset minimal --agent claude-code --dry-run 2>&1) || true

    local components_line
    components_line=$(echo "$output" | grep "Components order:")

    assert_output_contains "$components_line" "engram" "Minimal includes engram"
    assert_output_contains "$components_line" "persona" "Minimal + default Gentleman persona includes persona"
}

test_preset_ecosystem_components() {
    log_test "Preset ecosystem-only with persona=custom includes ecosystem components"

    # Use persona=custom to test the preset alone, since persona is now
    # driven by Selection.Persona (decoupled from preset).
    output=$($BINARY install --preset ecosystem-only --persona custom --agent claude-code --dry-run 2>&1) || true

    # ecosystem-only (without persona) includes engram, skills, context7 and gga.
    local components_line
    components_line=$(echo "$output" | grep "Components order:")

    assert_output_contains "$components_line" "engram" "Ecosystem includes engram"
    assert_output_contains "$components_line" "skills" "Ecosystem includes skills"
    assert_output_contains "$components_line" "context7" "Ecosystem includes context7"
    assert_output_contains "$components_line" "gga" "Ecosystem includes gga"
    assert_output_not_contains "$components_line" "persona" "Ecosystem + persona=custom excludes persona"
    assert_output_not_contains "$components_line" "permissions" "Ecosystem excludes permissions"
}

test_preset_full_with_custom_persona_excludes_persona() {
    log_test "Preset full-gentleman with persona=custom excludes persona"

    # Persona is decoupled — picking persona=custom skips persona install
    # even when the preset is full-gentleman.
    output=$($BINARY install --preset full-gentleman --persona custom --agent claude-code --dry-run 2>&1) || true

    local components_line
    components_line=$(echo "$output" | grep "Components order:")

    assert_output_contains "$components_line" "engram" "Full + persona=custom keeps engram"
    assert_output_contains "$components_line" "permissions" "Full + persona=custom keeps permissions"
    assert_output_not_contains "$components_line" "persona" "Full + persona=custom excludes persona"
}

test_preset_full_components() {
    log_test "Preset full-gentleman includes core and optional Gentleman components"

    output=$($BINARY install --preset full-gentleman --agent claude-code --dry-run 2>&1) || true

    local components_line
    components_line=$(echo "$output" | grep "Components order:")

    assert_output_contains "$components_line" "engram" "Full includes engram"
    assert_output_contains "$components_line" "skills" "Full includes skills"
    assert_output_contains "$components_line" "context7" "Full includes context7"
    assert_output_contains "$components_line" "persona" "Full includes persona"
    assert_output_contains "$components_line" "permissions" "Full includes permissions"
    assert_output_contains "$components_line" "gga" "Full includes gga"
    assert_output_contains "$components_line" "claude-theme" "Full includes Claude Gentleman theme"
    assert_output_contains "$components_line" "opencode-gentle-logo" "Full includes OpenCode Gentle logo"
}

test_dry_run_full_preset_persona_before_engram() {
    log_test "Dry-run: persona appears before engram in component order"

    output=$($BINARY install --preset full-gentleman --agent opencode --dry-run 2>&1) || true

    local components_line
    components_line=$(echo "$output" | grep "Components order:")

    # Verify all are present
    assert_output_contains "$components_line" "persona" "Full preset has persona"
    assert_output_contains "$components_line" "engram" "Full preset has engram"

    # Verify ordering: persona before engram
    # Extract the order string and check persona comes first
    local order_str
    order_str=$(echo "$components_line" | sed 's/.*Components order: *//')

    local persona_idx engram_idx
    persona_idx=$(echo "$order_str" | tr ',' '\n' | grep -n '^persona$' | cut -d: -f1)
    engram_idx=$(echo "$order_str" | tr ',' '\n' | grep -n '^engram$' | cut -d: -f1)

    if [ -n "$persona_idx" ] && [ -n "$engram_idx" ] && [ "$persona_idx" -lt "$engram_idx" ]; then
        log_pass "Persona ($persona_idx) before engram ($engram_idx)"
    else
        log_fail "Persona must appear before engram in component order: $order_str"
    fi
}

test_preset_no_legacy_theme_in_any_preset() {
    log_test "Presets exclude the unsafe generic theme component"

    local full_order_str=""
    for preset in minimal ecosystem-only full-gentleman; do
        output=$($BINARY install --preset "$preset" --agent claude-code --dry-run 2>&1) || true
        local components_line
        components_line=$(echo "$output" | grep "Components order:")
        local order_str
        order_str=${components_line#*Components order:}
        order_str=${order_str# }
        if echo "$order_str" | tr ',' '\n' | grep -qx "theme"; then
            log_fail "Preset '$preset' unexpectedly includes unsafe generic theme component"
        else
            log_pass "Preset '$preset' excludes unsafe generic theme component"
        fi
        if [ "$preset" = "full-gentleman" ]; then
            full_order_str=$order_str
        fi
    done

    for component in claude-theme opencode-gentle-logo; do
        if echo "$full_order_str" | tr ',' '\n' | grep -qx "$component"; then
            log_pass "Preset 'full-gentleman' includes safe visual component '$component'"
        else
            log_fail "Preset 'full-gentleman' must include safe visual component '$component'"
        fi
    done
}

test_preset_custom_no_components() {
    log_test "Preset custom with no --component produces empty component list"

    output=$($BINARY install --preset custom --agent claude-code --dry-run 2>&1) || true

    # Custom preset without explicit components = empty
    local components_line
    components_line=$(echo "$output" | grep "Components order:")
    assert_output_not_contains "$components_line" "engram" "Custom preset without components excludes engram"
    assert_output_not_contains "$components_line" "skills" "Custom preset without components excludes skills"
}

test_preset_custom_explicit_components() {
    log_test "Preset custom with explicit --component flags"

    output=$($BINARY install --preset custom --persona custom --agent claude-code --component engram --component skills --dry-run 2>&1) || true

    local components_line
    components_line=$(echo "$output" | grep "Components order:")
    assert_output_contains "$components_line" "engram" "Custom + explicit components includes engram"
    assert_output_contains "$components_line" "skills" "Custom + explicit components includes skills"
    assert_output_not_contains "$components_line" "persona" "Custom + explicit components excludes persona"
    assert_output_not_contains "$components_line" "context7" "Custom + explicit components excludes context7"
}

# --- Category 1f: Individual component flags ---

test_dry_run_component_engram() {
    log_test "Dry-run with --component engram"
    output=$($BINARY install --agent claude-code --component engram --dry-run 2>&1) || true
    assert_output_contains "$output" "engram" "Shows engram component"
}

test_dry_run_component_skills() {
    log_test "Dry-run with --component skills"
    output=$($BINARY install --agent claude-code --component skills --dry-run 2>&1) || true
    assert_output_contains "$output" "skills" "Shows skills component"
}

test_dry_run_component_context7() {
    log_test "Dry-run with --component context7"
    output=$($BINARY install --agent claude-code --component context7 --dry-run 2>&1) || true
    assert_output_contains "$output" "context7" "Shows context7 component"
}

test_dry_run_component_persona() {
    log_test "Dry-run with --component persona"
    output=$($BINARY install --agent claude-code --component persona --dry-run 2>&1) || true
    assert_output_contains "$output" "persona" "Shows persona component"
}

test_dry_run_component_permissions() {
    log_test "Dry-run with --component permissions"
    output=$($BINARY install --agent opencode --component permissions --dry-run 2>&1) || true
    assert_output_contains "$output" "permissions" "Shows permissions component"
}

test_dry_run_component_gga() {
    log_test "Dry-run with --component gga"
    output=$($BINARY install --agent claude-code --component gga --dry-run 2>&1) || true
    assert_output_contains "$output" "gga" "Shows gga component"
}

test_dry_run_component_theme() {
    log_test "Dry-run with --component theme"
    output=$($BINARY install --agent opencode --component theme --dry-run 2>&1) || true
    assert_output_contains "$output" "theme" "Shows theme component"
}

# --- Category 1g: Invalid input rejection ---

test_invalid_persona_rejected() {
    log_test "Invalid persona is rejected"

    if $BINARY install --persona nonexistent --dry-run 2>&1; then
        log_fail "Invalid persona should have been rejected"
    else
        log_pass "Invalid persona correctly rejected"
    fi
}

test_invalid_component_rejected() {
    log_test "Invalid component is rejected"

    if $BINARY install --component fakecomp --dry-run 2>&1; then
        log_fail "Invalid component should have been rejected"
    else
        log_pass "Invalid component correctly rejected"
    fi
}

test_invalid_preset_rejected() {
    log_test "Invalid preset is rejected"

    if $BINARY install --preset nonexistent --dry-run 2>&1; then
        log_fail "Invalid preset should have been rejected"
    else
        log_pass "Invalid preset correctly rejected"
    fi
}

test_unknown_command_rejected() {
    log_test "Unknown command is rejected"

    if $BINARY foobar 2>&1; then
        log_fail "Unknown command should have been rejected"
    else
        log_pass "Unknown command correctly rejected"
    fi
}

# ===========================================================================
# TIER 2 — Full install tests (require RUN_FULL_E2E=1)
# ===========================================================================

# --- Category 2: Claude Code component injection ---

test_cc_engram_injection() {
    log_test "Claude Code: engram injection (MCP + CLAUDE.md)"
    cleanup_test_env

    if $BINARY install --agent claude-code --component engram --persona neutral 2>&1; then
        # User-scope MCP registry
        local registry="$HOME/.claude.json"
        assert_file_exists "$registry" "Claude user MCP registry"
        assert_file_contains "$registry" '"mcpServers"' "Registry has mcpServers"
        assert_file_contains "$registry" '"engram"' "Registry has Engram server"
        assert_valid_json "$registry" "Claude user MCP registry is valid JSON"
        assert_file_not_exists "$HOME/.claude/mcp/engram.json" "legacy Engram MCP file is not written"

        # CLAUDE.md section
        assert_file_exists "$HOME/.claude/CLAUDE.md" "CLAUDE.md exists"
        assert_file_contains "$HOME/.claude/CLAUDE.md" "gentle-ai:engram-protocol" "CLAUDE.md has engram-protocol section marker"
        assert_file_contains "$HOME/.claude/CLAUDE.md" "mem_save" "CLAUDE.md has real Engram content (mem_save)"
        assert_file_size_min "$HOME/.claude/CLAUDE.md" 500 "CLAUDE.md has substantial content"
    else
        log_fail "engram install command failed"
    fi
}

test_cc_persona_gentleman() {
    log_test "Claude Code: persona injection (gentleman)"
    cleanup_test_env

    if $BINARY install --agent claude-code --component persona --persona gentleman 2>&1; then
        assert_file_exists "$HOME/.claude/CLAUDE.md" "CLAUDE.md exists"
        assert_file_contains "$HOME/.claude/CLAUDE.md" "gentle-ai:persona" "CLAUDE.md has persona section marker"
        # Claude has an active output-style channel — the CLAUDE.md persona
        # section is now a residual (tooling directives + pointer only); tone
        # content lives exclusively in the output style (design.md Decision 1).
        assert_file_contains "$HOME/.claude/CLAUDE.md" "Persona Voice" "CLAUDE.md persona residual points to the output style"
        assert_file_not_contains "$HOME/.claude/CLAUDE.md" "Senior Architect" "CLAUDE.md persona residual carries no tone content"
        assert_file_size_min "$HOME/.claude/CLAUDE.md" 200 "Persona section is substantial"
        # Output-style file — canonical tone channel
        assert_file_exists "$HOME/.claude/output-styles/gentleman.md" "Output-style file exists"
        assert_file_contains "$HOME/.claude/output-styles/gentleman.md" "name: Gentleman" "Output-style has YAML frontmatter"
        assert_file_contains "$HOME/.claude/output-styles/gentleman.md" "keep-coding-instructions: true" "Output-style keeps coding instructions"
        assert_file_contains "$HOME/.claude/output-styles/gentleman.md" "Senior Architect" "Output-style carries the Gentleman tone content"
        # settings.json outputStyle key
        assert_file_exists "$HOME/.claude/settings.json" "settings.json exists"
        assert_file_contains "$HOME/.claude/settings.json" "outputStyle" "settings.json has outputStyle key"
        assert_file_contains "$HOME/.claude/settings.json" "Gentleman" "settings.json outputStyle is Gentleman"
    else
        log_fail "persona (gentleman) install command failed"
    fi
}

test_cc_persona_neutral() {
    log_test "Claude Code: persona injection (neutral)"
    cleanup_test_env

    if $BINARY install --agent claude-code --component persona --persona neutral 2>&1; then
        assert_file_exists "$HOME/.claude/CLAUDE.md" "CLAUDE.md exists"
        assert_file_contains "$HOME/.claude/CLAUDE.md" "gentle-ai:persona" "CLAUDE.md has persona section marker"
        # Claude has an active output-style channel — the CLAUDE.md persona
        # section is now a residual; the mentor identity lives in the output
        # style (design.md Decision 1).
        assert_file_contains "$HOME/.claude/CLAUDE.md" "Persona Voice" "CLAUDE.md persona residual points to the output style"
        assert_file_not_contains "$HOME/.claude/CLAUDE.md" "Senior Architect" "CLAUDE.md persona residual carries no tone content"
        assert_file_exists "$HOME/.claude/output-styles/neutral.md" "Neutral output-style file exists"
        assert_file_contains "$HOME/.claude/output-styles/neutral.md" "Push back when user asks for code without context or understanding" "Neutral output-style carries the mentor tone content"
        assert_file_not_contains "$HOME/.claude/output-styles/neutral.md" "Rioplatense\|voseo\|loco\|ponete las pilas" "Neutral output-style excludes regional language"
    else
        log_fail "persona (neutral) install command failed"
    fi
}

test_cc_persona_custom_does_nothing() {
    log_test "Claude Code: persona custom does nothing (user keeps own personality)"
    cleanup_test_env

    if $BINARY install --agent claude-code --component persona --persona custom 2>&1; then
        # CLAUDE.md may exist: routing guidance is scheduled per agent and
        # deliberately outside the component loop, so every configured agent
        # receives it. What `--persona custom` promises is that no personality
        # is imposed, so assert that instead of the file's absence.
        assert_file_not_contains "$HOME/.claude/CLAUDE.md" "Senior Architect\|Rioplatense\|voseo" "CLAUDE.md carries no tone content under custom"
        # No output-style file either.
        assert_file_not_exists "$HOME/.claude/output-styles/gentleman.md" "No output-style for custom"
    else
        log_fail "Custom persona install command failed"
    fi
}

test_oc_persona_custom_does_nothing() {
    log_test "OpenCode: persona custom does nothing (user keeps own personality)"
    cleanup_test_env

    if $BINARY install --agent opencode --component persona --persona custom 2>&1; then
        # Custom persona should NOT create AGENTS.md (persona does nothing).
        local agents_md="$HOME/.config/opencode/AGENTS.md"
        assert_file_not_exists "$agents_md" "AGENTS.md not created by custom persona"
    else
        log_fail "OpenCode custom persona install command failed"
    fi
}

test_cc_skills_minimal() {
    log_test "Claude Code: minimal skills component retains only judgment-day"
    cleanup_test_env

    if $BINARY install --agent claude-code --component skills --preset minimal --persona custom 2>&1; then
        local skills_dir="$HOME/.claude/skills"
        if [ -d "$skills_dir" ]; then
            assert_file_count "$skills_dir" "SKILL.md" 1 "Minimal skills component installs only judgment-day"
            assert_file_exists "$skills_dir/judgment-day/SKILL.md" "Minimal Claude skills include judgment-day"
        else
            log_fail "Minimal skills component did not create judgment-day skills directory"
        fi

        # No framework skills in minimal
        if [ -f "$skills_dir/typescript/SKILL.md" ]; then
            log_fail "Minimal preset should NOT include typescript skill"
        else
            log_pass "Minimal preset correctly excludes framework skills"
        fi
    else
        log_fail "skills (minimal) install command failed"
    fi
}

test_cc_skills_full() {
    log_test "Claude Code: skills injection (full-gentleman = 7 foundation skills)"
    cleanup_test_env

    if $BINARY install --agent claude-code --component skills --preset full-gentleman --persona neutral 2>&1; then
        local skills_dir="$HOME/.claude/skills"
        assert_dir_exists "$skills_dir" "Claude skills directory"

        # #4669: the six contributor workflow skills are selectable, never default.
        assert_file_count "$skills_dir" "SKILL.md" 8 "Full preset (skills alone): 8 skill files"

        # Verify foundation skills exist
        assert_file_exists "$skills_dir/go-testing/SKILL.md" "go-testing SKILL.md"
        assert_file_exists "$skills_dir/skill-creator/SKILL.md" "skill-creator SKILL.md"
        assert_file_not_exists "$skills_dir/branch-pr/SKILL.md" "branch-pr NOT installed by default"
        assert_file_not_exists "$skills_dir/issue-creation/SKILL.md" "issue-creation NOT installed by default"
        assert_file_exists "$skills_dir/skill-registry/SKILL.md" "skill-registry SKILL.md"

        # Real content check
        assert_file_size_min "$skills_dir/go-testing/SKILL.md" 200 "go-testing skill has real content"
        assert_file_size_min "$skills_dir/skill-creator/SKILL.md" 200 "skill-creator skill has real content"
        assert_file_size_min "$skills_dir/chained-pr/SKILL.md" 200 "chained-pr skill has real content"
        assert_file_size_min "$skills_dir/work-unit-commits/SKILL.md" 200 "work-unit-commits skill has real content"
        assert_file_size_min "$skills_dir/skill-registry/SKILL.md" 200 "skill-registry skill has real content"
    else
        log_fail "skills (full) install command failed"
    fi
}

test_cc_skills_ecosystem() {
    log_test "Claude Code: skills injection (ecosystem-only = 7 foundation skills)"
    cleanup_test_env

    if $BINARY install --agent claude-code --component skills --preset ecosystem-only --persona neutral 2>&1; then
        local skills_dir="$HOME/.claude/skills"
        assert_dir_exists "$skills_dir" "Claude skills directory"

        # #4669: contributor workflow skills are not part of this preset.
        assert_file_count "$skills_dir" "SKILL.md" 8 "Ecosystem preset (skills alone): 8 skill files"
        # Foundation skills present
        assert_file_exists "$skills_dir/go-testing/SKILL.md" "Foundation skills present"
        assert_file_exists "$skills_dir/skill-creator/SKILL.md" "skill-creator present"
        assert_file_not_exists "$skills_dir/branch-pr/SKILL.md" "branch-pr NOT in ecosystem default"
        assert_file_not_exists "$skills_dir/issue-creation/SKILL.md" "issue-creation NOT in ecosystem default"
        # Stack-specific skills NOT present
        if [ -f "$skills_dir/react-19/SKILL.md" ]; then
            log_fail "Ecosystem preset should NOT include react-19"
        else
            log_pass "Ecosystem preset correctly excludes stack-specific skills"
        fi
    else
        log_fail "skills (ecosystem) install command failed"
    fi
}

test_cc_custom_skills_with_flag() {
    log_test "Claude Code: custom preset + explicit --skills flag installs specified skills"
    cleanup_test_env

    if $BINARY install --agent claude-code --preset custom --component skills --skills go-testing,branch-pr --persona neutral 2>&1; then
        local skills_dir="$HOME/.claude/skills"
        assert_dir_exists "$skills_dir" "Claude skills directory"

        # The explicitly requested skills must be present
        assert_file_exists "$skills_dir/go-testing/SKILL.md" "go-testing SKILL.md"
        assert_file_exists "$skills_dir/branch-pr/SKILL.md" "branch-pr SKILL.md"

        assert_file_count "$skills_dir" "SKILL.md" 2 "Custom + explicit skills: 2 files"
    else
        log_fail "custom + skills flag install command failed"
    fi
}

test_cc_custom_no_skills_flag_installs_nothing() {
    log_test "Claude Code: custom preset + skills component without --skills flag installs nothing"
    cleanup_test_env

    if $BINARY install --agent claude-code --preset custom --component skills --persona neutral 2>&1; then
        local skills_dir="$HOME/.claude/skills"
        # Custom without --skills installs no skill files.
        if [ -d "$skills_dir" ]; then
            log_fail "Skills directory should NOT be created: skills alone has nothing to install"
        else
            log_pass "No skills directory created without --skills"
        fi
    else
        log_fail "custom + skills component (no flag) install command failed"
    fi
}

test_cc_context7_injection() {
    log_test "Claude Code: context7 injection (~/.claude.json user MCP registry)"
    cleanup_test_env

    if $BINARY install --agent claude-code --component context7 --persona neutral 2>&1; then
        # Claude Code only reads user-scope MCP servers from ~/.claude.json;
        # the settings.json mcpServers block earlier versions wrote is inert
        # and no longer written (issue #1868, PR #1909).
        local registry="$HOME/.claude.json"
        assert_file_exists "$registry" "Claude user MCP registry (~/.claude.json)"
        assert_file_contains "$registry" '"mcpServers"' "user registry has mcpServers key"
        assert_file_contains "$registry" '"context7"' "user registry has context7 server"
        assert_file_contains "$registry" 'context7-mcp' "user registry points to context7-mcp"
        assert_valid_json "$registry" "user registry is valid JSON"
        assert_file_not_exists "$HOME/.claude/mcp/context7.json" "legacy context7 MCP file is not written"
    else
        log_fail "context7 install command failed"
    fi
}

test_cc_permissions_injection() {
    log_test "Claude Code: permissions injection"
    cleanup_test_env

    if $BINARY install --agent claude-code --component permissions --persona neutral 2>&1; then
        local settings="$HOME/.claude/settings.json"
        assert_file_exists "$settings" "Claude settings.json"
        assert_file_contains "$settings" '"permissions"' "Has permissions key"
        assert_file_contains "$settings" '"deny"' "Has deny list"
        assert_valid_json "$settings" "settings.json is valid JSON"
    else
        log_fail "permissions install command failed"
    fi
}

test_cc_theme_injection() {
    log_test "Claude Code: theme injection"
    cleanup_test_env

    if $BINARY install --agent claude-code --component theme --persona neutral 2>&1; then
        local settings="$HOME/.claude/settings.json"
        assert_file_exists "$settings" "Claude settings.json"
        assert_file_contains "$settings" '"theme"' "Has theme key"
        assert_file_contains "$settings" 'gentleman' "Has gentleman theme"
        assert_valid_json "$settings" "settings.json is valid JSON"
    else
        log_fail "theme install command failed"
    fi
}

# --- Category 3: OpenCode component injection ---

test_oc_engram_injection() {
    log_test "OpenCode: engram injection (opencode.json)"
    cleanup_test_env

    if $BINARY install --agent opencode --component engram --persona neutral 2>&1; then
        local settings="$HOME/.config/opencode/opencode.json"
        local agents_md="$HOME/.config/opencode/AGENTS.md"
        assert_file_exists "$settings" "OpenCode opencode.json"
        assert_file_contains "$settings" '"mcp"' "Has mcp key"
        assert_file_contains "$settings" '"engram"' "Has engram MCP entry"
        assert_file_contains "$settings" '"command"' "Has command key"
        assert_file_contains "$settings" '"type": "local"' "Engram uses local MCP type"
        assert_valid_json "$settings" "opencode.json is valid JSON"

        # Fallback safety: AGENTS.md must include engram protocol section.
        assert_file_exists "$agents_md" "OpenCode AGENTS.md"
        assert_file_contains "$agents_md" 'gentle-ai:engram-protocol' "AGENTS.md has engram-protocol section"
        assert_file_contains "$agents_md" 'mem_save' "AGENTS.md has memory protocol content"
    else
        log_fail "OpenCode engram install command failed"
    fi
}

test_oc_persona_gentleman() {
    log_test "OpenCode: persona injection (gentleman)"
    cleanup_test_env

    if $BINARY install --agent opencode --component persona --persona gentleman 2>&1; then
        local agents_md="$HOME/.config/opencode/AGENTS.md"
        assert_file_exists "$agents_md" "AGENTS.md exists"
        assert_file_contains "$agents_md" "Senior Architect" "Gentleman persona has 'Senior Architect'"
        assert_file_size_min "$agents_md" 200 "AGENTS.md has substantial content"
    else
        log_fail "OpenCode persona (gentleman) install command failed"
    fi
}

test_oc_persona_neutral() {
    log_test "OpenCode: persona injection (neutral)"
    cleanup_test_env

    if $BINARY install --agent opencode --component persona --persona neutral 2>&1; then
        local agents_md="$HOME/.config/opencode/AGENTS.md"
        assert_file_exists "$agents_md" "AGENTS.md exists"
        assert_file_contains "$agents_md" "Senior Architect" "Neutral persona keeps the teacher identity"
        assert_file_not_contains "$agents_md" "Rioplatense\|voseo\|loco\|ponete las pilas" "Neutral persona excludes regional language"
    else
        log_fail "OpenCode persona (neutral) install command failed"
    fi
}

test_oc_skills_minimal() {
    log_test "OpenCode: skills injection (minimal)"
    cleanup_test_env

    if $BINARY install --agent opencode --component skills --preset minimal --persona custom 2>&1; then
        local skill_dir="$HOME/.config/opencode/skills"
        if [ -d "$skill_dir" ]; then
            assert_file_count "$skill_dir" "SKILL.md" 1 "Minimal OpenCode skills component installs only judgment-day"
            assert_file_exists "$skill_dir/judgment-day/SKILL.md" "Minimal OpenCode skills include judgment-day"
        else
            log_fail "Minimal OpenCode skills component did not create judgment-day skills directory"
        fi
    else
        log_fail "OpenCode skills (minimal) install command failed"
    fi
}

test_oc_skills_full() {
    log_test "OpenCode: skills injection (full-gentleman = 7 foundation skills)"
    cleanup_test_env

    # #4669: the six contributor workflow skills are selectable, never default.
    if $BINARY install --agent opencode --component skills --preset full-gentleman --persona neutral 2>&1; then
        local skill_dir="$HOME/.config/opencode/skills"
        assert_dir_exists "$skill_dir" "OpenCode skill directory"
        assert_file_count "$skill_dir" "SKILL.md" 8 "Full preset (skills alone): 8 skill files"
        assert_file_exists "$skill_dir/go-testing/SKILL.md" "go-testing skill"
        assert_file_exists "$skill_dir/skill-creator/SKILL.md" "skill-creator skill"
        assert_file_not_exists "$skill_dir/branch-pr/SKILL.md" "branch-pr NOT installed by default"
        assert_file_not_exists "$skill_dir/issue-creation/SKILL.md" "issue-creation NOT installed by default"
        assert_file_size_min "$skill_dir/go-testing/SKILL.md" 200 "go-testing skill has real content"
    else
        log_fail "OpenCode skills (full) install command failed"
    fi
}

test_oc_context7_injection() {
    log_test "OpenCode: context7 injection (opencode.json MCP)"
    cleanup_test_env

    if $BINARY install --agent opencode --component context7 --persona neutral 2>&1; then
        local settings="$HOME/.config/opencode/opencode.json"
        assert_file_exists "$settings" "OpenCode opencode.json"
        assert_file_contains "$settings" '"mcp"' "Has mcp key"
        assert_file_contains "$settings" '"context7"' "Has context7 entry"
        assert_file_contains "$settings" 'https://mcp.context7.com/mcp' "Has Context7 MCP URL"
        assert_valid_json "$settings" "opencode.json is valid JSON"
    else
        log_fail "OpenCode context7 install command failed"
    fi
}

# --- Category 4: Qwen Code injection ---

test_qwen_engram_injection() {
    log_test "Qwen: engram injection (settings.json)"
    cleanup_test_env

    if $BINARY install --agent qwen-code --component engram --persona neutral 2>&1; then
        local settings="$HOME/.qwen/settings.json"
        assert_file_exists "$settings" "Qwen settings.json"
        assert_file_contains "$settings" '"mcp"' "Has mcp key"
        assert_file_contains "$settings" '"engram"' "Has engram MCP entry"
        assert_file_contains "$settings" '"command"' "Has command key"
        assert_valid_json "$settings" "settings.json is valid JSON"
    else
        log_fail "Qwen engram install command failed"
    fi
}

test_qwen_engram_idempotency() {
    log_test "Qwen: engram injection is idempotent"
    cleanup_test_env

    local settings="$HOME/.qwen/settings.json"

    # First run — the install's exit code is irrelevant here (e.g. transient
    # npm failure); we assert on the resulting file.
    $BINARY install --agent qwen-code --component engram --persona neutral > /dev/null 2>&1 || true
    if [ ! -f "$settings" ]; then
        log_fail "Qwen settings.json missing after first install"
        return
    fi
    local checksum1
    checksum1=$(md5sum "$settings" | cut -d' ' -f1)

    # Second run
    $BINARY install --agent qwen-code --component engram --persona neutral > /dev/null 2>&1 || true
    if [ ! -f "$settings" ]; then
        log_fail "Qwen settings.json missing after second install"
        return
    fi
    local checksum2
    checksum2=$(md5sum "$settings" | cut -d' ' -f1)

    if [ "$checksum1" = "$checksum2" ]; then
        log_pass "Qwen settings.json is idempotent"
    else
        log_fail "Qwen settings.json changed between runs"
    fi
}

test_oc_permissions_injection() {
    log_test "OpenCode: permissions injection"
    cleanup_test_env

    if $BINARY install --agent opencode --component permissions --persona neutral 2>&1; then
        local settings="$HOME/.config/opencode/opencode.json"
        assert_file_exists "$settings" "OpenCode opencode.json"
        assert_file_contains "$settings" '"permission"' "Has permission key"
        assert_file_contains "$settings" '"bash"' "Has bash permissions"
        assert_file_contains "$settings" '"read"' "Has read permissions"
        assert_valid_json "$settings" "opencode.json is valid JSON"
    else
        log_fail "OpenCode permissions install command failed"
    fi
}

test_oc_theme_injection() {
    log_test "OpenCode: theme injection"
    cleanup_test_env

    if $BINARY install --agent opencode --component theme --persona neutral 2>&1; then
        local settings="$HOME/.config/opencode/opencode.json"
        assert_file_exists "$settings" "OpenCode opencode.json"
        assert_file_contains "$settings" '"theme"' "Has theme key"
        assert_file_contains "$settings" 'gentleman' "Has gentleman theme"
        assert_valid_json "$settings" "opencode.json is valid JSON"
    else
        log_fail "OpenCode theme install command failed"
    fi
}

# --- Category 4: Full preset integration ---

test_full_preset_claude_code() {
    log_test "Full-gentleman preset: Claude Code (all components coexist)"
    cleanup_test_env

    # Exercise independent injection components without invoking package downloads.
    if $BINARY install --agent claude-code --component engram --component persona --component skills --component context7 --component permissions --component theme --preset full-gentleman --persona gentleman 2>&1; then
        local claude_md="$HOME/.claude/CLAUDE.md"
        local settings="$HOME/.claude/settings.json"

        # ODD routing, memory and persona coexist without duplicate sections.
        assert_file_exists "$claude_md" "CLAUDE.md exists"
        assert_file_contains "$claude_md" "gentle-ai:agent-routing" "Has ODD routing section"
        assert_file_contains "$claude_md" "gentle-ai:engram-protocol" "Has memory section"
        assert_file_contains "$claude_md" "gentle-ai:persona" "Has persona section"
        assert_no_duplicate_section "$claude_md" "agent-routing" "No duplicate ODD routing section"
        assert_no_duplicate_section "$claude_md" "persona" "No duplicate persona section"

        # settings.json should have permissions + theme
        assert_file_exists "$settings" "settings.json exists"
        assert_file_contains "$settings" '"permissions"' "Has permissions"
        assert_file_contains "$settings" '"theme"' "Has theme"
        assert_valid_json "$settings" "settings.json is valid JSON"

        # MCP registration lives in the ~/.claude.json user registry, the only
        # user-scope file Claude Code reads MCP servers from (issue #1868).
        local registry="$HOME/.claude.json"
        assert_file_exists "$registry" "Claude user MCP registry (~/.claude.json)"
        assert_file_contains "$registry" '"mcpServers"' "Has MCP servers"
        assert_file_contains "$registry" '"context7"' "Has context7 MCP"
        assert_file_contains "$registry" 'context7-mcp' "Context7 MCP uses pinned package"
        assert_valid_json "$registry" "user registry is valid JSON"

        # Skills
        assert_file_count_min "$HOME/.claude/skills" "SKILL.md" 8 "At least 8 foundation skill files"

        log_pass "Full preset: all Claude Code injection-only components coexist"
    else
        log_fail "Full preset (Claude Code) install command failed"
    fi
}

test_full_preset_opencode() {
    log_test "Full-gentleman preset: OpenCode (all components coexist)"
    cleanup_test_env

    if $BINARY install --agent opencode --component engram --component persona --component skills --component context7 --component permissions --component theme --preset full-gentleman --persona gentleman 2>&1; then
        local settings="$HOME/.config/opencode/opencode.json"
        local agents_md="$HOME/.config/opencode/AGENTS.md"

        # opencode.json should have all overlays merged
        assert_file_exists "$settings" "OpenCode opencode.json"
        assert_file_contains "$settings" '"permission"' "Has permission config"
        assert_file_contains "$settings" '"theme"' "Has theme"
        assert_file_contains "$settings" '"mcp"' "Has MCP servers"
        assert_file_contains "$settings" '"context7"' "Has context7 MCP"
        assert_valid_json "$settings" "opencode.json is valid JSON"

        # OpenCode owns ODD routing in the managed orchestrator prompt in opencode.json.
        assert_file_exists "$agents_md" "AGENTS.md exists"
        assert_file_contains "$agents_md" "Senior Architect" "Gentleman persona"
        assert_file_contains "$settings" "gentle-ai:agent-routing" "OpenCode orchestrator has ODD routing"
        assert_file_contains "$agents_md" "gentle-ai:engram-protocol" "AGENTS.md has engram protocol"
        assert_no_duplicate_section "$agents_md" "engram-protocol" "No duplicate engram section in AGENTS.md"
        assert_file_count_min "$HOME/.config/opencode/skills" "SKILL.md" 8 "At least 8 foundation skill files"

        log_pass "Full preset: all OpenCode injection-only components coexist"
    else
        log_fail "Full preset (OpenCode) install command failed"
    fi
}

test_minimal_preset_opencode_only_engram_no_persona() {
    log_test "Minimal preset: OpenCode (engram only, no persona side effect)"
    cleanup_test_env

    if $BINARY install --agent opencode --preset minimal --persona custom 2>&1; then
        local settings="$HOME/.config/opencode/opencode.json"
        local agents_md="$HOME/.config/opencode/AGENTS.md"

        assert_file_exists "$settings" "OpenCode opencode.json exists"
        assert_file_contains "$settings" '"engram"' "OpenCode has engram MCP"

        # Minimal preset should NOT silently install persona.
        if [ -f "$agents_md" ]; then
            assert_file_not_contains "$agents_md" "gentle-ai:persona" "No persona marker in minimal preset"
            assert_file_not_contains "$agents_md" "Senior Architect" "No persona content in minimal preset"
        else
            log_pass "No AGENTS.md created by minimal preset (correct)"
        fi
    else
        log_fail "Minimal preset (OpenCode) install command failed"
    fi
}

test_minimal_preset_claude_only_engram() {
    log_test "Minimal preset: Claude Code (only engram, nothing else)"
    cleanup_test_env

    if $BINARY install --agent claude-code --preset minimal --persona custom 2>&1; then
        # Engram should be installed (MCP + CLAUDE.md)
        assert_file_exists "$HOME/.claude/CLAUDE.md" "CLAUDE.md exists"
        assert_file_contains "$HOME/.claude/CLAUDE.md" "gentle-ai:engram-protocol" "Engram protocol section"

        # Persona should NOT be in CLAUDE.md
        assert_file_not_contains "$HOME/.claude/CLAUDE.md" "gentle-ai:persona" "No persona in minimal"
        # No permissions settings.json
        if [ -f "$HOME/.claude/settings.json" ]; then
            assert_file_not_contains "$HOME/.claude/settings.json" '"permissions"' "No permissions in minimal"
        else
            log_pass "No settings.json in minimal (correct)"
        fi
        # No skills directory (or empty)
        if [ -d "$HOME/.claude/skills" ]; then
            log_fail "Minimal preset should not create skills directory (skills component not in minimal preset)"
        else
            log_pass "No skills directory in minimal (correct)"
        fi
    else
        log_fail "Minimal preset (Claude Code) install command failed"
    fi
}

test_ecosystem_both_agents() {
    log_test "Ecosystem preset: both agents"
    cleanup_test_env

    if $BINARY install --agent claude-code --agent opencode --component engram --component skills --component context7 --preset ecosystem-only --persona neutral 2>&1; then
        # Claude Code
        assert_file_exists "$HOME/.claude/CLAUDE.md" "Claude CLAUDE.md"
        assert_file_contains "$HOME/.claude/CLAUDE.md" "gentle-ai:agent-routing" "Claude has ODD routing"
        assert_file_contains "$HOME/.claude.json" '"context7"' "Claude context7 MCP"
        assert_file_count_min "$HOME/.claude/skills" "SKILL.md" 8 "Claude foundation skills"

        # OpenCode
        assert_file_contains "$HOME/.config/opencode/opencode.json" "gentle-ai:agent-routing" "OpenCode orchestrator has ODD routing"
        assert_file_count_min "$HOME/.config/opencode/skills" "SKILL.md" 8 "OpenCode foundation skills"
        assert_file_contains "$HOME/.config/opencode/opencode.json" '"context7"' "OpenCode context7"
        assert_valid_json "$HOME/.config/opencode/opencode.json" "OpenCode opencode.json valid JSON"

        log_pass "Ecosystem preset: both agents have matching components"
    else
        log_fail "Ecosystem preset (both agents) install command failed"
    fi
}

test_both_agents_permissions() {
    log_test "Both agents: permissions injection"
    cleanup_test_env

    if $BINARY install --agent opencode --agent claude-code --component permissions --persona neutral 2>&1; then
        local oc_settings="$HOME/.config/opencode/opencode.json"
        local cc_settings="$HOME/.claude/settings.json"

        assert_file_exists "$oc_settings" "OpenCode opencode.json"
        assert_file_exists "$cc_settings" "Claude settings.json"
        assert_file_contains "$oc_settings" '"permission"' "OpenCode has permission config"
        assert_file_contains "$cc_settings" '"permissions"' "Claude has permissions"
        assert_valid_json "$oc_settings" "OpenCode opencode valid JSON"
        assert_valid_json "$cc_settings" "Claude settings valid JSON"
    else
        log_fail "Both agents + permissions install command failed"
    fi
}

# --- Category 5: Content validation ---

test_content_claude_md_sections_substantial() {
    log_test "Content validation: CLAUDE.md sections are substantial"
    cleanup_test_env

    # Install persona and engram alongside unconditional ODD routing guidance.
    $BINARY install --agent claude-code --component persona --persona gentleman 2>&1 || true
    $BINARY install --agent claude-code --component engram --persona gentleman 2>&1 || true

    local claude_md="$HOME/.claude/CLAUDE.md"
    if [ -f "$claude_md" ]; then
        assert_file_contains "$claude_md" "gentle-ai:agent-routing" "CLAUDE.md has ODD routing"
        assert_file_size_min "$claude_md" 1000 "CLAUDE.md with routing, memory and persona >= 1000 bytes"
    else
        log_fail "CLAUDE.md not created"
    fi
}

test_content_skills_are_real() {
    log_test "Content validation: skill files contain real instructions"
    cleanup_test_env

    $BINARY install --agent claude-code --component skills --preset full-gentleman --persona neutral 2>&1 || true

    local skills_dir="$HOME/.claude/skills"
    if [ -d "$skills_dir" ]; then
        # Check every SKILL.md is at least 200 bytes (real content, not stubs)
        local all_ok=true
        while IFS= read -r skill_file; do
            local size
            size=$(wc -c < "$skill_file" | tr -d ' ')
            if [ "$size" -lt 200 ]; then
                log_fail "Skill file too small ($size bytes): $skill_file"
                all_ok=false
            fi
        done < <(find "$skills_dir" -name "SKILL.md" -type f)

        if $all_ok; then
            log_pass "All skill files have >= 200 bytes of real content"
        fi
    else
        log_fail "Skills directory not created"
    fi
}

test_content_mcp_json_valid() {
    log_test "Content validation: MCP JSON files are parseable"
    cleanup_test_env

    $BINARY install --agent claude-code --component context7 --persona neutral 2>&1 || true
    $BINARY install --agent claude-code --component engram --persona neutral 2>&1 || true

    # Claude Code reads user-scoped MCP servers from ~/.claude.json. The legacy
    # ~/.claude/mcp directory is intentionally no longer created.
    local registry="$HOME/.claude.json"
    assert_file_exists "$registry" "Claude user MCP registry"
    assert_valid_json "$registry" "Claude user MCP registry is valid JSON"
    assert_file_contains "$registry" '"context7"' "Registry has Context7 server"
    assert_file_contains "$registry" '"engram"' "Registry has Engram server"
    assert_file_not_exists "$HOME/.claude/mcp/context7.json" "legacy Context7 MCP file is not written"
    assert_file_not_exists "$HOME/.claude/mcp/engram.json" "legacy Engram MCP file is not written"
}

# --- Category 6: Idempotency ---

test_idempotent_permissions_opencode() {
    log_test "Idempotency: permissions on OpenCode (run twice, same result)"
    cleanup_test_env

    $BINARY install --agent opencode --component permissions --persona neutral 2>&1 || true
    local first_hash
    first_hash=$(md5sum "$HOME/.config/opencode/opencode.json" 2>/dev/null | cut -d' ' -f1)

    $BINARY install --agent opencode --component permissions --persona neutral 2>&1 || true
    local second_hash
    second_hash=$(md5sum "$HOME/.config/opencode/opencode.json" 2>/dev/null | cut -d' ' -f1)

    if [ "$first_hash" = "$second_hash" ] && [ -n "$first_hash" ]; then
        log_pass "Idempotent: same permissions config after two runs"
    else
        log_fail "Permissions config changed between runs ($first_hash vs $second_hash)"
    fi
}

test_idempotent_persona_claude() {
    log_test "Idempotency: persona on Claude Code (no duplicate sections)"
    cleanup_test_env

    $BINARY install --agent claude-code --component persona --persona gentleman 2>&1 || true
    $BINARY install --agent claude-code --component persona --persona gentleman 2>&1 || true

    local claude_md="$HOME/.claude/CLAUDE.md"
    if [ -f "$claude_md" ]; then
        assert_no_duplicate_section "$claude_md" "persona" "No duplicate persona section after 2 runs"
    else
        log_fail "CLAUDE.md not found"
    fi
}

test_idempotent_engram_claude() {
    log_test "Idempotency: engram on Claude Code (no duplicate sections)"
    cleanup_test_env

    $BINARY install --agent claude-code --component engram --persona neutral 2>&1 || true
    $BINARY install --agent claude-code --component engram --persona neutral 2>&1 || true

    local claude_md="$HOME/.claude/CLAUDE.md"
    if [ -f "$claude_md" ]; then
        assert_no_duplicate_section "$claude_md" "engram-protocol" "No duplicate engram section after 2 runs"

        # Also check the user registry remains valid.
        local mcp_file="$HOME/.claude.json"
        if [ -f "$mcp_file" ]; then
            assert_valid_json "$mcp_file" "Claude user MCP registry still valid after 2 runs"
        fi
    else
        log_fail "CLAUDE.md not found"
    fi
}

# ─── Gemini parity tests ─────────────────────────────────────────────────────

test_gemini_engram_tools_flag() {
    log_test "Gemini: engram injection uses --tools=agent"
    cleanup_test_env

    if $BINARY install --agent gemini-cli --component engram --persona neutral 2>&1; then
        local settings="$HOME/.gemini/settings.json"
        assert_file_exists "$settings" "Gemini settings.json"
        assert_file_contains "$settings" '"mcpServers"' "Has mcpServers key"
        assert_file_contains "$settings" '"engram"' "Has engram entry"
        assert_file_contains "$settings" '"--tools=agent"' "Engram args include --tools=agent"
        assert_valid_json "$settings" "settings.json is valid JSON"
    else
        log_fail "Gemini engram install command failed"
    fi
}

# ─── Codex parity tests ───────────────────────────────────────────────────────

test_codex_engram_injection() {
    log_test "Codex: engram injection writes config.toml + instruction files"
    cleanup_test_env

    if $BINARY install --agent codex --component engram --persona neutral 2>&1; then
        local config_toml="$HOME/.codex/config.toml"
        local instructions="$HOME/.codex/engram-instructions.md"
        local compact="$HOME/.codex/engram-compact-prompt.md"

        assert_file_exists "$config_toml" "Codex config.toml"
        assert_file_contains "$config_toml" '[mcp_servers.engram]' "config.toml has [mcp_servers.engram]"
        assert_file_contains "$config_toml" 'command = ".*engram"' "config.toml has correct command"
        assert_file_contains "$config_toml" '"--tools=agent"' "config.toml has --tools=agent"
        assert_file_contains "$config_toml" 'model_instructions_file' "config.toml references instruction file"
        assert_file_contains "$config_toml" 'experimental_compact_prompt_file' "config.toml references compact prompt"

        assert_file_exists "$instructions" "engram-instructions.md"
        assert_file_contains "$instructions" 'mem_save' "Instructions have memory protocol content"

        assert_file_exists "$compact" "engram-compact-prompt.md"
        assert_file_contains "$compact" 'FIRST ACTION REQUIRED' "Compact prompt has required sentinel"
    else
        log_fail "Codex engram install command failed"
    fi
}

test_codex_engram_idempotent() {
    log_test "Codex: engram injection is idempotent (no duplicate blocks)"
    cleanup_test_env

    $BINARY install --agent codex --component engram --persona neutral 2>&1 || true
    $BINARY install --agent codex --component engram --persona neutral 2>&1 || true

    local config_toml="$HOME/.codex/config.toml"
    if [ -f "$config_toml" ]; then
        local count
        count=$(grep -c '\[mcp_servers\.engram\]' "$config_toml" || true)
        if [ "$count" -ne 1 ]; then
            log_fail "config.toml has $count [mcp_servers.engram] blocks after 2 runs (want exactly 1)"
        else
            log_pass "config.toml has exactly 1 [mcp_servers.engram] block after 2 runs"
        fi
    else
        log_fail "config.toml not found after 2 runs"
    fi
}

test_idempotent_skills_claude() {
    log_test "Idempotency: ecosystem skills injection produces same files"
    cleanup_test_env

    # Minimal intentionally selects no default skills; ecosystem installs the
    # foundation catalog so byte-idempotency checks nonempty SKILL.md files.
    $BINARY install --agent claude-code --component skills --preset ecosystem-only --persona custom 2>&1 || true
    # Capture file hashes
    local first_hashes
    first_hashes=$(find "$HOME/.claude/skills" -name "SKILL.md" -exec md5sum {} \; 2>/dev/null | sort)

    $BINARY install --agent claude-code --component skills --preset ecosystem-only --persona custom 2>&1 || true
    local second_hashes
    second_hashes=$(find "$HOME/.claude/skills" -name "SKILL.md" -exec md5sum {} \; 2>/dev/null | sort)

    if [ "$first_hashes" = "$second_hashes" ] && [ -n "$first_hashes" ]; then
        log_pass "Idempotent: same skill files after two runs"
    else
        log_fail "Skill files changed between runs"
    fi
}

test_idempotent_theme_opencode() {
    log_test "Idempotency: theme on OpenCode (run twice, same result)"
    cleanup_test_env

    $BINARY install --agent opencode --component theme --persona neutral 2>&1 || true
    local first_hash
    first_hash=$(md5sum "$HOME/.config/opencode/opencode.json" 2>/dev/null | cut -d' ' -f1)

    $BINARY install --agent opencode --component theme --persona neutral 2>&1 || true
    local second_hash
    second_hash=$(md5sum "$HOME/.config/opencode/opencode.json" 2>/dev/null | cut -d' ' -f1)

    if [ "$first_hash" = "$second_hash" ] && [ -n "$first_hash" ]; then
        log_pass "Idempotent: same theme config after two runs"
    else
        log_fail "Theme config changed between runs ($first_hash vs $second_hash)"
    fi
}

test_idempotent_full_claude() {
    log_test "Idempotency: full injection-only on Claude Code"
    cleanup_test_env

    $BINARY install --agent claude-code --component persona --component context7 --component permissions --component theme --preset full-gentleman --persona gentleman 2>&1 || true
    local first_md_hash
    first_md_hash=$(md5sum "$HOME/.claude/CLAUDE.md" 2>/dev/null | cut -d' ' -f1)
    # Snapshot settings.json for semantic comparison (engram setup may reorder
    # top-level keys on re-run — see engram binary's non-deterministic map
    # serialization). Byte-exact hashing would false-fail on harmless reorder.
    cp "$HOME/.claude/settings.json" /tmp/gai_settings_run1.json 2>/dev/null || true

    $BINARY install --agent claude-code --component persona --component context7 --component permissions --component theme --preset full-gentleman --persona gentleman 2>&1 || true
    local second_md_hash
    second_md_hash=$(md5sum "$HOME/.claude/CLAUDE.md" 2>/dev/null | cut -d' ' -f1)

    if [ "$first_md_hash" = "$second_md_hash" ] && [ -n "$first_md_hash" ]; then
        log_pass "Idempotent: CLAUDE.md identical after 2 runs"
    else
        log_fail "CLAUDE.md changed between runs"
    fi
    if [ -f /tmp/gai_settings_run1.json ] && [ -f "$HOME/.claude/settings.json" ]; then
        if json_files_equal /tmp/gai_settings_run1.json "$HOME/.claude/settings.json"; then
            log_pass "Idempotent: settings.json identical after 2 runs"
        else
            log_fail "settings.json changed between runs"
        fi
    else
        log_fail "settings.json missing after install"
    fi
    rm -f /tmp/gai_settings_run1.json
}

# --- Category 8: Edge cases ---

test_edge_theme_not_in_presets() {
    log_test "Edge case: --component theme (not in any preset)"
    cleanup_test_env

    if $BINARY install --agent claude-code --component theme --persona neutral 2>&1; then
        assert_file_exists "$HOME/.claude/settings.json" "Theme creates settings.json"
        assert_file_contains "$HOME/.claude/settings.json" '"theme"' "Theme key present"
        # Routing and remote authorization are unconditional agent guidance,
        # not optional components. Require exactly those managed sections;
        # theme-only must not inject persona or other optional components.
        if [ -f "$HOME/.claude/CLAUDE.md" ]; then
            local sections
            sections=$(grep -o '<!-- gentle-ai:[a-z0-9-]* -->' "$HOME/.claude/CLAUDE.md" | sort -u | tr '\n' ' ')
            if [ "$(printf '%s' "$sections" | xargs)" = "<!-- gentle-ai:agent-routing --> <!-- gentle-ai:remote-authorization -->" ]; then
                log_pass "Theme-only: CLAUDE.md carries routing and remote authorization only"
            else
                log_fail "Theme-only install wrote component sections: $sections"
            fi
        else
            log_fail "Theme-only: CLAUDE.md missing required routing and remote authorization"
        fi
    else
        log_fail "Theme-only install command failed"
    fi
}

test_edge_multiple_agents_same_component() {
    log_test "Edge case: multiple agents with same component"
    cleanup_test_env

    if $BINARY install --agent claude-code --agent opencode --component context7 --persona neutral 2>&1; then
        # Both agents should have context7
        assert_file_contains "$HOME/.claude.json" '"context7"' "Claude context7"
        assert_file_contains "$HOME/.config/opencode/opencode.json" '"context7"' "OpenCode context7"
    else
        log_fail "Multiple agents + context7 install command failed"
    fi
}

test_edge_persona_switch() {
    log_test "Edge case: switching persona from gentleman to neutral"
    cleanup_test_env

    # First install with gentleman. Claude has an active output-style channel,
    # so the teacher identity ("Senior Architect") lives in the output style —
    # the CLAUDE.md persona section is a residual pointer (design.md Decision 1).
    $BINARY install --agent claude-code --component persona --persona gentleman 2>&1 || true
    assert_file_contains "$HOME/.claude/CLAUDE.md" "Persona Voice" "First install: gentleman persona residual points to the output style"
    assert_file_contains "$HOME/.claude/output-styles/gentleman.md" "Senior Architect" "First install: gentleman output-style has the teacher identity"

    # Then install with neutral — should REPLACE persona section AND the
    # selected output style. Neutral is a distinct professional voice (same
    # mentor identity, no regional language, no "Senior Architect" bio), so
    # the CLAUDE.md residual stays tone-free and the neutral output style is
    # checked for its own mentor-teaching tone content instead.
    $BINARY install --agent claude-code --component persona --persona neutral 2>&1 || true
    assert_file_contains "$HOME/.claude/CLAUDE.md" "Persona Voice" "Second install: neutral persona residual still points to the output style"
    assert_file_not_contains "$HOME/.claude/CLAUDE.md" "Senior Architect" "Second install: CLAUDE.md residual carries no tone content"
    assert_file_contains "$HOME/.claude/output-styles/neutral.md" "Push back when user asks for code without context or understanding" "Second install: neutral output-style has the mentor identity"
    assert_file_not_contains "$HOME/.claude/output-styles/neutral.md" "Rioplatense\|voseo\|ponete las pilas" "Second install: regional language removed from output style"
    assert_no_duplicate_section "$HOME/.claude/CLAUDE.md" "persona" "No duplicate persona after switch"
}

test_edge_persona_switch_preserves_sections_opencode() {
    log_test "Edge case: persona switch preserves managed sections (OpenCode)"
    cleanup_test_env

    # Step 1: Install persona and memory with gentleman.
    $BINARY install --agent opencode --component persona --component engram --persona gentleman 2>&1 || true

    local agents_md="$HOME/.config/opencode/AGENTS.md"
    assert_file_exists "$agents_md" "AGENTS.md after full install"
    assert_file_contains "$agents_md" "gentle-ai:engram-protocol" "Engram section present before switch"

    # Step 2: Switch to neutral persona
    $BINARY install --agent opencode --component persona --persona neutral 2>&1 || true

    # Step 3: Verify sections survived
    assert_file_contains "$agents_md" "Senior Architect" "Neutral persona present after switch"
    assert_file_not_contains "$agents_md" "Rioplatense" "Regional language removed after switch"
    assert_file_contains "$agents_md" "gentle-ai:engram-protocol" "Engram section survived persona switch"
    assert_no_duplicate_section "$agents_md" "engram-protocol" "No duplicate engram after switch"
}

test_edge_json_merge_preserves_existing() {
    log_test "Edge case: JSON merge preserves existing settings"
    cleanup_test_env

    # Create pre-existing settings
    mkdir -p "$HOME/.config/opencode"
    echo '{"existingKey": "preserved"}' > "$HOME/.config/opencode/opencode.json"

    # Install permissions on top
    $BINARY install --agent opencode --component permissions --persona neutral 2>&1 || true

    local settings="$HOME/.config/opencode/opencode.json"
    assert_file_contains "$settings" '"existingKey"' "Pre-existing key preserved"
    assert_file_contains "$settings" '"preserved"' "Pre-existing value preserved"
    assert_file_contains "$settings" '"permission"' "Permission config merged in"
    assert_valid_json "$settings" "Merged JSON is valid"
}

test_edge_multiple_json_overlays() {
    log_test "Edge case: multiple JSON overlays merge correctly"
    cleanup_test_env

    # Install permissions, then theme, then context7 — all into OpenCode opencode.json
    $BINARY install --agent opencode --component permissions --persona neutral 2>&1 || true
    $BINARY install --agent opencode --component theme --persona neutral 2>&1 || true
    $BINARY install --agent opencode --component context7 --persona neutral 2>&1 || true

    local settings="$HOME/.config/opencode/opencode.json"
    assert_file_contains "$settings" '"permission"' "Permission config present after 3 merges"
    assert_file_contains "$settings" '"theme"' "Theme present after 3 merges"
    assert_file_contains "$settings" '"mcp"' "MCP servers present after 3 merges"
    assert_file_contains "$settings" '"context7"' "Context7 present after 3 merges"
    assert_valid_json "$settings" "Final merged JSON is valid"
}

# --- Category: GGA tests ---

test_gga_config() {
    log_test "GGA component writes config file"
    cleanup_test_env

    # GGA binary install may fail in Docker (go install needs time/network),
    # but we test the output regardless.
    if $BINARY install --agent claude-code --component gga --persona neutral 2>&1; then
        local config="$HOME/.config/gga/config"
        assert_file_exists "$config" "GGA config"
        assert_file_contains "$config" 'PROVIDER=' "Has provider key"
        assert_file_contains "$config" 'FILE_PATTERNS=' "Has file patterns key"

        local agents_md="$HOME/.config/gga/AGENTS.md"
        assert_file_exists "$agents_md" "GGA AGENTS.md template"
    else
        log_skip "GGA install failed (expected — binary install may require network)"
    fi
}

test_gga_runtime_pr_mode_installed() {
    log_test "GGA runtime includes pr_mode.sh"
    cleanup_test_env

    if $BINARY install --agent claude-code --component gga --persona neutral 2>&1; then
        local pr_mode="$HOME/.local/share/gga/lib/pr_mode.sh"
        assert_file_exists "$pr_mode" "GGA pr_mode.sh exists"
        assert_file_contains "$pr_mode" 'detect_base_branch' "pr_mode.sh has PR mode functions"
    else
        log_skip "GGA install failed (expected — binary install may require network)"
    fi
}

test_gga_reinstall_is_idempotent() {
    log_test "GGA install is idempotent on second run"
    cleanup_test_env

    if $BINARY install --agent claude-code --component gga --persona neutral 2>&1; then
        if $BINARY install --agent claude-code --component gga --persona neutral 2>&1; then
            log_pass "Second GGA install completed successfully"
        else
            log_fail "Second GGA install failed"
        fi
    else
        log_skip "GGA first install failed (expected — binary install may require network)"
    fi
}

# --- Category 11: Windsurf persona and memory coexistence ---

test_windsurf_persona_and_engram_content() {
    log_test "Windsurf: persona and Engram coexist in global_rules.md"
    cleanup_test_env

    # Windsurf is a desktop app — signal its presence without a live agent.
    mkdir -p "$HOME/.codeium/windsurf"

    if $BINARY install --agent windsurf --component persona --component engram --persona gentleman 2>&1; then
        local rules="$HOME/.codeium/windsurf/memories/global_rules.md"
        assert_file_exists "$rules" "global_rules.md exists"
        assert_file_contains "$rules" "Senior Architect" "Persona injected"
        assert_file_contains "$rules" "gentle-ai:engram-protocol" "Engram protocol marker present"
        assert_file_contains "$rules" "gentle-ai:agent-routing" "ODD routing marker present"
        assert_file_size_min "$rules" 2000 "global_rules.md has substantial content"
    else
        log_fail "Windsurf persona+Engram install command failed"
    fi
}

# --- Category 12: Codex context7 TOML injection ---

test_codex_context7_in_toml() {
    log_test "Codex: context7 component writes [mcp_servers.context7] into config.toml (TOML strategy)"
    cleanup_test_env

    $BINARY install --agent codex --component context7 --persona neutral 2>&1 || true

    local config_toml="$HOME/.codex/config.toml"
    assert_file_exists "$config_toml" "Codex config.toml created by context7"
    assert_file_contains "$config_toml" "[mcp_servers.context7]" "Codex config.toml has [mcp_servers.context7] block"
    assert_file_contains "$config_toml" "https://mcp.context7.com/mcp" "Codex context7 block uses remote MCP URL"
    assert_file_not_contains "$config_toml" "context7-mcp" "Codex context7 block does not use local npx package"

    # Idempotent: re-running must not duplicate the block.
    $BINARY install --agent codex --component context7 --persona neutral 2>&1 || true
    local count
    # `grep -c` prints "0" AND exits 1 on zero matches, so `|| echo 0` would
    # yield the two-line string "0\n0" and break the numeric comparison below.
    count=$(grep -c "\[mcp_servers.context7\]" "$config_toml" 2>/dev/null || true)
    count=${count:-0}
    if [ "$count" -eq 1 ]; then
        log_pass "Codex context7 block is idempotent (exactly 1 entry)"
    else
        log_fail "Codex context7 block duplicated ($count entries)"
    fi
}

# --- Category 7: Injection integrity (guards against issue #4 regression) ---

test_integrity_full_preset_all_skills_nonempty() {
    log_test "Integrity: full preset — every SKILL.md is non-empty"
    cleanup_test_env

    if $BINARY install --agent opencode --component skills --preset full-gentleman --persona gentleman 2>&1; then
        local skill_dir="$HOME/.config/opencode/skills"
        assert_file_count "$skill_dir" "SKILL.md" 8 "Full preset installs 8 foundation skills"
        local all_ok=true
        local empty_count=0

        while IFS= read -r skill_file; do
            local size
            size=$(wc -c < "$skill_file" | tr -d ' ')
            if [ "$size" -lt 100 ]; then
                log_fail "Skill file empty/corrupt ($size bytes): $skill_file"
                all_ok=false
                empty_count=$((empty_count + 1))
            fi
        done < <(find "$skill_dir" -name "SKILL.md" -type f)

        local total
        total=$(find "$skill_dir" -name "SKILL.md" -type f | wc -l | tr -d ' ')
        if $all_ok && [ "$total" -eq 8 ]; then
            log_pass "All 8 foundation skill files have >= 100 bytes of real content"
        else
            log_fail "Expected 8 non-empty foundation skills (found $total; $empty_count empty or corrupt)"
        fi
    else
        log_fail "Full preset install for integrity check failed"
    fi
}

# ===========================================================================
# TIER 3 — Backup / restore tests (require RUN_BACKUP_TESTS=1)
# ===========================================================================

test_backup_created_on_install() {
    log_test "Backup snapshot created during install"
    cleanup_test_env
    setup_fake_configs

    if $BINARY install --agent opencode --component permissions --persona neutral 2>&1; then
        local backup_count
        backup_count=$(find "$HOME/.gentle-ai/backups" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l | tr -d ' ')
        if [ "$backup_count" -gt 0 ]; then
            log_pass "Backup directory created ($backup_count snapshots)"
        else
            log_fail "No backup directory found"
        fi
    else
        log_fail "Install with backup failed"
    fi
}

test_backup_contains_original_files() {
    log_test "Backup snapshot contains original config files"
    cleanup_test_env
    setup_fake_configs

    if $BINARY install --agent opencode --component permissions --persona neutral 2>&1; then
        local latest_backup
        latest_backup=$(find "$HOME/.gentle-ai/backups" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | tail -1)
        if [ -n "$latest_backup" ]; then
            local file_count
            file_count=$(find "$latest_backup" -type f 2>/dev/null | wc -l | tr -d ' ')
            if [ "$file_count" -gt 0 ]; then
                log_pass "Backup contains $file_count file(s)"
            else
                log_fail "Backup directory is empty"
            fi
        else
            log_fail "No backup snapshot directory found"
        fi
    else
        log_fail "Install for backup test failed"
    fi
}

test_backup_manifest_exists() {
    log_test "Backup manifest file exists"
    cleanup_test_env
    setup_fake_configs

    if $BINARY install --agent opencode --component permissions --persona neutral 2>&1; then
        local latest_backup
        latest_backup=$(find "$HOME/.gentle-ai/backups" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | tail -1)
        if [ -n "$latest_backup" ]; then
            if [ -f "$latest_backup/manifest.json" ]; then
                assert_valid_json "$latest_backup/manifest.json" "Backup manifest is valid JSON"
            else
                log_fail "manifest.json not found in backup: $latest_backup"
            fi
        else
            log_fail "No backup snapshot found"
        fi
    else
        log_fail "Install for manifest test failed"
    fi
}

test_backup_idempotent_install() {
    log_test "Idempotent: running install twice produces same result (with backup)"
    cleanup_test_env

    $BINARY install --agent opencode --component permissions --persona neutral 2>&1 || true
    local first_content
    first_content=$(cat "$HOME/.config/opencode/opencode.json" 2>/dev/null)

    $BINARY install --agent opencode --component permissions --persona neutral 2>&1 || true
    local second_content
    second_content=$(cat "$HOME/.config/opencode/opencode.json" 2>/dev/null)

    if [ "$first_content" = "$second_content" ] && [ -n "$first_content" ]; then
        log_pass "Idempotent: same config after two runs (with backup)"
    else
        log_fail "Config changed between runs (with backup)"
    fi
}

test_backup_multiple_snapshots() {
    log_test "Multiple installs create multiple backup snapshots"
    cleanup_test_env
    setup_fake_configs

    $BINARY install --agent opencode --component permissions --persona neutral 2>&1 || true
    sleep 0.1
    $BINARY install --agent opencode --component theme --persona neutral 2>&1 || true

    local backup_count
    backup_count=$(find "$HOME/.gentle-ai/backups" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l | tr -d ' ')
    if [ "$backup_count" -ge 2 ]; then
        log_pass "Multiple backup snapshots created ($backup_count)"
    else
        log_fail "Expected >= 2 backup snapshots, got $backup_count"
    fi
}

test_backup_claude_code_files() {
    log_test "Backup captures Claude Code files"
    cleanup_test_env
    setup_fake_configs

    if $BINARY install --agent claude-code --component permissions --persona neutral 2>&1; then
        local latest_backup
        latest_backup=$(find "$HOME/.gentle-ai/backups" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | tail -1)
        if [ -n "$latest_backup" ] && [ -f "$latest_backup/manifest.json" ]; then
            log_pass "Claude Code backup snapshot with manifest created"
        else
            log_fail "No proper backup found for Claude Code install"
        fi
    else
        log_fail "Install for Claude backup test failed"
    fi
}

# ===========================================================================
# Test execution
# ===========================================================================

log_info "=== Tier 1: Basic binary & dry-run tests ==="

# Category 1a: Binary basics
test_binary_exists
test_binary_runs
test_version_command

# Category 1b: Dry-run output format
test_dry_run_output_format
test_dry_run_platform_detection
test_dry_run_detects_linux

# Category 1c: Agent flags
test_dry_run_agent_claude_code
test_dry_run_agent_opencode
test_dry_run_agent_both
test_dry_run_agent_csv

# Category 1d: Preset flags
test_dry_run_preset_minimal
test_dry_run_preset_ecosystem
test_dry_run_preset_full
test_dry_run_preset_custom

# Category 1e: Preset component order validation
test_preset_minimal_components
test_preset_minimal_with_default_persona_includes_persona
test_preset_ecosystem_components
test_preset_full_components
test_preset_full_with_custom_persona_excludes_persona
test_dry_run_full_preset_persona_before_engram
test_preset_no_legacy_theme_in_any_preset
test_preset_custom_no_components
test_preset_custom_explicit_components

# Category 1f: Individual component flags (all 8)
test_dry_run_component_engram
test_dry_run_component_skills
test_dry_run_component_context7
test_dry_run_component_persona
test_dry_run_component_permissions
test_dry_run_component_gga
test_dry_run_component_theme

# Category 1g: Invalid inputs
test_invalid_persona_rejected
test_invalid_component_rejected
test_invalid_preset_rejected
test_unknown_command_rejected

if [ "${RUN_FULL_E2E:-0}" = "1" ]; then
    log_info ""
    log_info "=== Tier 2: Component injection tests ==="

    # Category 2: Claude Code injection
    test_cc_engram_injection
    test_cc_persona_gentleman
    test_cc_persona_neutral
    test_cc_persona_custom_does_nothing
    test_cc_skills_minimal
    test_cc_skills_full
    test_cc_skills_ecosystem
    test_cc_custom_skills_with_flag
    test_cc_custom_no_skills_flag_installs_nothing
    test_cc_context7_injection
    test_cc_permissions_injection
    test_cc_theme_injection

    # Category 3: OpenCode injection
    test_oc_engram_injection
    test_oc_persona_gentleman
    test_oc_persona_neutral
    test_oc_persona_custom_does_nothing
    test_oc_skills_minimal
    test_oc_skills_full
    test_oc_context7_injection
    test_oc_permissions_injection
    test_oc_theme_injection

    # Category 4: Full preset integration
    test_full_preset_claude_code
    test_full_preset_opencode
    test_minimal_preset_claude_only_engram
    test_minimal_preset_opencode_only_engram_no_persona
    test_ecosystem_both_agents
    test_both_agents_permissions

    # Category 5: Content validation
    test_content_claude_md_sections_substantial
    test_content_skills_are_real
    test_content_mcp_json_valid

    # Category 6: Idempotency
    test_idempotent_permissions_opencode
    test_idempotent_persona_claude
    test_idempotent_engram_claude
    test_idempotent_skills_claude
    test_idempotent_theme_opencode
    test_idempotent_full_claude

    # Category 6b: Gemini/Codex engram parity
    test_gemini_engram_tools_flag
    test_codex_engram_injection
    test_codex_engram_idempotent

    # Category 8: Edge cases
    test_edge_theme_not_in_presets
    test_edge_multiple_agents_same_component
    test_edge_persona_switch
    test_edge_persona_switch_preserves_sections_opencode
    test_edge_json_merge_preserves_existing
    test_edge_multiple_json_overlays

    # GGA
    test_gga_config
    test_gga_runtime_pr_mode_installed
    test_gga_reinstall_is_idempotent

    # Category 7: Injection integrity (issue #4 regression guard)
    test_integrity_full_preset_all_skills_nonempty

    # Category 11: Windsurf persona + memory remain independent of SDD
    test_windsurf_persona_and_engram_content

    # Category 12: Codex context7 by-design skip
    test_codex_context7_in_toml

    # Category 13: Qwen integration
    test_qwen_engram_injection
    test_qwen_engram_idempotency
else
    log_skip "Tier 2 tests (set RUN_FULL_E2E=1 to enable)"
fi

if [ "${RUN_BACKUP_TESTS:-0}" = "1" ]; then
    log_info ""
    log_info "=== Tier 3: Backup/restore tests ==="
    test_backup_created_on_install
    test_backup_contains_original_files
    test_backup_manifest_exists
    test_backup_idempotent_install
    test_backup_multiple_snapshots
    test_backup_claude_code_files
else
    log_skip "Tier 3 tests (set RUN_BACKUP_TESTS=1 to enable)"
fi

# ---------------------------------------------------------------------------
# Summary & exit
# ---------------------------------------------------------------------------
print_summary
