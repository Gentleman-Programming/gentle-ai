# Tasks: Custom Workflow Selection in TUI Agent Builder

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 350–450 |
| 400-line budget risk | Medium |
| Chained PRs recommended | Yes |
| Suggested split | PR 1: foundation + template; PR 2: TUI + tests |
| Delivery strategy | ask-on-risk |
| Chain strategy | size-exception |

Decision needed before apply: Yes — resolved: size:exception accepted (single PR).

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Foundation: types, template, workflow JSON | PR 1 | `main` base; unit tests for template included |
| 2 | TUI wiring, screen, registry integration | PR 2 | targets PR 1 branch; integration + E2E tests included |

## Phase 1: Foundation

- [x] **1.1 Extend agent builder types** (~4 lines)
  - File: `internal/agentbuilder/types.go`
  - AC: `GeneratedAgent.WorkflowName string` added; `RegistryEntry.WorkflowName string \`json:"workflow_name,omitempty"\`` added
  - Dep: none

- [x] **1.2 RED: Write failing test for template registration** (~20 lines)
  - File: `internal/workflow/template_test.go`
  - AC: Test asserts `Template("academic-article-review")` returns ok=true with 11 phases; tests pass after implementation
  - Dep: none

- [x] **1.3 GREEN: Register academic-article-review built-in template** (~80 lines)
  - File: `internal/workflow/template.go`
  - AC: `BuiltInTemplates["academic-article-review"]` maps to a new `academicArticleReviewTemplate()` factory implementing the 11-phase DAG from the spec
  - Dep: 1.2

- [x] **1.4 Create on-disk workflow JSON** (~170 lines)
  - File: `openspec/workflows/academic-article-review/workflow.json`
  - AC: Defines the 11-phase DAG from the spec with `"name": "academic-article-review"`
  - Dep: none

## Phase 2: TUI Core Implementation

- [x] **2.1 Add workflow screen constant and state fields** (~12 lines)
  - File: `internal/tui/model.go`
  - AC: `ScreenAgentBuilderWorkflow` constant added between Prompt and SDD; `AgentBuilderState` gains `WorkflowName` and `AvailableWorkflows`
  - Dep: 1.1

- [x] **2.2 RED: Write failing test for workflow screen rendering** (~25 lines)
  - File: `internal/tui/screens/agent_builder_workflow_test.go`
  - AC: Test asserts render contains "sdd" first, all workflows listed, cursor highlighted; tests pass after implementation
  - Dep: none

- [x] **2.3 GREEN: Implement workflow screen renderer** (~45 lines)
  - File: `internal/tui/screens/agent_builder_workflow.go`
  - AC: `RenderABWorkflow()` renders list with cursor; `ABWorkflowOptions()` appends "Back"
  - Dep: 2.2

- [x] **2.4 Wire model navigation for workflow screen** (~35 lines)
  - File: `internal/tui/model.go`
  - AC: `optionCount()`, `View()`, `confirmSelection()`, `goBack()` handle workflow screen; Prompt Tab navigates to workflow screen
  - Dep: 2.1, 2.3

- [x] **2.5 Update router for workflow screen** (~6 lines)
  - File: `internal/tui/router.go`
  - AC: Workflow screen routes forward to SDD and backward to Prompt; SDD backward now points to workflow
  - Dep: 2.1

## Phase 3: Testing & Verification

- [x] **3.1 Integration test TUI navigation paths** (~50 lines)
  - File: `internal/tui/agent_builder_nav_test.go`
  - AC: Prompt→Workflow→SDD path works; Prompt→Workflow→non-SDD skips SDD screen
  - Dep: 2.4, 2.5

- [x] **3.2 Integration test registry persistence** (~35 lines)
  - File: `internal/agentbuilder/registry_test.go`
  - AC: Install with non-SDD workflow persists `workflow_name`; missing field deserializes as SDD
  - Dep: 1.1

- [x] **3.3 E2E validate new template** (~15 lines)
  - File: `internal/workflow/template_test.go`
  - AC: On-disk workflow.json parses and passes validation; `gentle-ai workflow validate academic-article-review` equivalent checked
  - Dep: 1.4
