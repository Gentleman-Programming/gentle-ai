# Delta for OpenCode Model Selector

## Purpose

Defines how Gentle AI honors the user's effective OpenCode configuration for model selection, install/inject, sync assignment preservation, and custom provider/model visibility.

## ADDED Requirements

### Requirement: Effective OpenCode Config Discovery

The system MUST discover the effective OpenCode config file from supported OpenCode config names, including `opencode.jsonc`, and SHALL parse JSONC-tolerant content consistently across Configure Models, install/inject, and sync.

#### Scenario: JSONC config is effective

- GIVEN `opencode.jsonc` exists with comments, trailing commas, providers, and agent assignments
- WHEN Gentle AI reads OpenCode configuration
- THEN it uses `opencode.jsonc` as the effective config source
- AND it reads providers and assignments without requiring strict JSON

#### Scenario: No config exists

- GIVEN no supported OpenCode config file exists
- WHEN Gentle AI needs an OpenCode config path
- THEN it selects a supported default config path for new writes
- AND all flows use the same selected path

### Requirement: Configure Models Shows Effective Providers

Configure Models MUST show provider/model choices from the effective config while preserving runtime discovery through `opencode models --verbose`.

#### Scenario: Custom configured provider appears

- GIVEN the effective config defines a custom provider and model
- WHEN the user opens Configure Models for OpenCode
- THEN the custom provider and model are selectable
- AND built-in runtime providers remain selectable

#### Scenario: Runtime discovery remains authoritative

- GIVEN `opencode models --verbose` returns providers or models not declared in the config file
- WHEN Configure Models refreshes its catalog
- THEN those runtime-discovered providers and models remain visible
- AND file-backed providers do not disable runtime discovery

### Requirement: Install and Sync Preserve Assignment Presence

Install/inject and sync MUST write the effective OpenCode config file and SHALL distinguish absent assignments, present assignments, and explicitly cleared assignments so stale Gentle AI state cannot overwrite current OpenCode intent.

#### Scenario: Install writes existing JSONC config

- GIVEN `opencode.jsonc` is the effective config file
- WHEN Gentle AI installs or injects OpenCode agents
- THEN it updates `opencode.jsonc`
- AND it does not blindly create or write the wrong `opencode.json` path

#### Scenario: Current assignment is preserved

- GIVEN the effective config has a current model assignment for an SDD agent
- WHEN sync runs with older Gentle AI state
- THEN sync preserves the current OpenCode assignment
- AND stale persisted state does not replace it

#### Scenario: Cleared assignment stays cleared

- GIVEN a user explicitly cleared an SDD agent model assignment in OpenCode config
- WHEN sync runs with stale Gentle AI state that still contains the old model
- THEN sync keeps the assignment cleared
- AND the old model is not restored

### Requirement: LM Studio URL Resolution

The system MUST preserve LM Studio URL behavior from #1917: a provider's direct `url` SHALL take precedence, and `options.baseURL` SHALL be used only when direct `url` is absent.

#### Scenario: Direct URL wins

- GIVEN LM Studio config has both `url` and `options.baseURL`
- WHEN Gentle AI reads provider URL metadata
- THEN it uses the direct `url`
- AND it ignores `options.baseURL` for that provider URL

#### Scenario: baseURL fallback is used

- GIVEN LM Studio config omits `url` but includes `options.baseURL`
- WHEN Gentle AI reads provider URL metadata
- THEN it uses `options.baseURL` as the provider URL

### Requirement: Scope Exclusions

This change SHALL NOT modify review plugin/recovery files and MUST NOT absorb unrelated issue scopes #934, #2288, or #1015.

#### Scenario: Excluded files untouched

- GIVEN this #771 change is implemented
- WHEN the candidate diff is reviewed
- THEN `internal/assets/opencode/plugins/review-result-artifacts.ts` is unchanged
- AND `internal/assets/review_plugin_recovery_test.go` is unchanged

#### Scenario: Unrelated issues stay out of scope

- GIVEN #934, #2288, and #1015 describe separate behavior
- WHEN this #771 change is implemented
- THEN tool-call warning UX and SQLite/runtime discovery behavior are not changed
