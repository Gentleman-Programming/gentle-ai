# Apply Progress: copilot-cli-adapter

## Batch 1 — Phase 1 & 2

### Completed Tasks
- [x] 1.1 Add `AgentCopilotCLI AgentID = "copilot-cli"` to `internal/model/types.go`
- [x] 2.1 Create `internal/agents/copilotcli/adapter.go` — implement `Adapter` interface
- [x] 2.2 Create `internal/agents/copilotcli/adapter_test.go`

### TDD Cycle Evidence (Batch 1)
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `internal/model/types_copilotcli_test.go` | Unit | ✅ 0/0 (new) | ✅ Written | ✅ Passed | ➖ Single (constant) | ➖ None needed |
| 2.1+2.2 | `internal/agents/copilotcli/adapter_test.go` | Unit | N/A (new pkg) | ✅ Written | ✅ 7/7 Passed | ✅ 3 detection + 4 paths + 7 caps | ➖ None needed |

## Batch 2 — Phase 3 & 4

### Completed Tasks
- [x] 3.1 Add `AgentCopilotCLI` to `internal/catalog/agents.go`
- [x] 3.2 Add `internal/agents/copilotcli` import to `internal/agents/factory.go`
- [x] 3.3 Add `model.AgentCopilotCLI` to `defaultAgentIDs` slice in `internal/agents/factory.go`
- [x] 3.4 Add `case model.AgentCopilotCLI: return copilotcli.NewAdapter(), nil` to `NewAdapter()` switch
- [x] 4.1 Add `"copilot-cli"` entry to `knownAgentConfigDirs` in `internal/system/config_scan.go`
- [x] 4.2 Create `CopilotCLIMCPConfigJSON()` in `internal/components/mcp/context7.go`
- [x] 4.3 Branch `AgentCopilotCLI` in `injectMCPConfigFile` in `internal/components/mcp/inject.go`

### TDD Cycle Evidence (Batch 2)
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 3.1 | `internal/catalog/agents_test.go` | Unit | ✅ 5/5 | ✅ Written | ✅ 2/2 Passed | ✅ Catalog + IsSupportedAgent | ➖ None needed |
| 3.2-3.4 | `internal/agents/factory_test.go` | Unit | ✅ 7/7 | ✅ Written | ✅ 3/3 Passed | ✅ Factory + Registry + SupportedAgents list | ➖ None needed |
| 4.1 | `internal/system/config_scan_test.go` | Unit | ✅ 5/5 | ✅ Written | ✅ 3/3 Passed | ✅ knownAgents map + count + specific path test | ➖ None needed |
| 4.2 | `internal/components/mcp/context7_test.go` | Unit | ✅ 3/3 | ✅ Written | ✅ 2/2 Passed | ✅ mcpServers key + stdio npx command | ➖ None needed |
| 4.3 | `internal/components/mcp/inject_test.go` | Unit | ✅ 8/8 | ✅ Written | ✅ 2/2 Passed | ✅ Write + idempotency + explicit overlay verification | ➖ None needed |

### Test Summary (Cumulative)
- **Total tests written**: 17 (8 batch 1 + 9 batch 2)
- **Total tests passing**: 17
- **Layers used**: Unit (17)
- **Pure functions created**: 0 (all methods on structs or var-backed functions)

### Files Changed (Batch 2)
| File | Action | What Was Done |
|------|--------|---------------|
| `internal/catalog/agents.go` | Modified | Added `{AgentCopilotCLI, "Copilot CLI", TierFull, "~/.copilot"}` to `allAgents` |
| `internal/catalog/agents_test.go` | Modified | Added `TestAllAgentsIncludesCopilotCLI` and `TestIsSupportedAgentAcceptsCopilotCLI` |
| `internal/agents/factory.go` | Modified | Added `copilotcli` import, `AgentCopilotCLI` to `defaultAgentIDs`, and switch case |
| `internal/agents/factory_test.go` | Modified | Added `TestFactoryResolvesCopilotCLIAdapter`, `TestDefaultRegistryIncludesCopilotCLI`, updated `SupportedAgents` expected list |
| `internal/system/config_scan.go` | Modified | Added `{Agent: "copilot-cli", Path: ~/.copilot}` to `knownAgentConfigDirs` |
| `internal/system/config_scan_test.go` | Modified | Added `copilot-cli` to knownAgents map, updated count 14→15, added `TestScanConfigs_CopilotCLIPathUsesDotCopilot` |
| `internal/components/mcp/context7.go` | Modified | Added `copilotCLIMCPConfigJSON` var and `CopilotCLIMCPConfigJSON()` accessor function |
| `internal/components/mcp/context7_test.go` | Modified | Added `TestCopilotCLIMCPConfigJSONUsesMCPServersKey` and `TestCopilotCLIMCPConfigJSONUsesStdioTransport` |
| `internal/components/mcp/inject.go` | Modified | Added `if adapter.Agent() == model.AgentCopilotCLI { overlay = CopilotCLIMCPConfigJSON() }` branch |
| `internal/components/mcp/inject_test.go` | Modified | Added `copilotcli` import, `copilotcliAdapter()` helper, `TestInjectCopilotCLIWritesContext7ToMCPConfigFile`, `TestInjectCopilotCLIUsesCopilotCLISpecificOverlay`, added `bytes` import |

### Deviations from Design
None — implementation matches design spec exactly.

### Issues Found
None.

### Remaining Tasks
- [ ] 5.1 Run `go test ./internal/agents/copilotcli/...` — all adapter tests pass
- [ ] 5.2 Run `go test ./internal/catalog/...` — verify `IsSupportedAgent("copilot-cli")` returns true
- [ ] 5.3 Run `go test ./internal/agents/...` — factory and registry tests pass (no regression)
- [ ] 5.4 Run `go test ./internal/components/mcp/...` — verify `Inject(home, copilotcli.NewAdapter())` merges `context7` into `~/.copilot/mcp-config.json` and is idempotent
- [ ] 5.5 Run `go test ./internal/system/...` — verify `ScanConfigs` includes `"copilot-cli"` entry
- [ ] 5.6 Run full `go test ./...` — no regressions
- [ ] 6.1 MCP schema discovery (conditional)
