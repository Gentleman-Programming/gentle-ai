# VS Code SDD Profiles — Investigación y Análisis de Arquitectura

> **Fecha**: 2026-05-11  
> **Contexto**: Análisis previo a la implementación de "VS Code SDD Profiles" para gentle-ai.  
> **Estado**: Exploración completada — listo para fase de propuesta (`sdd-propose`).

---

## 1. Resumen Ejecutivo

El objetivo es replicar el comportamiento **Multi-mode SDD** de OpenCode (perfiles con modelos asignados a cada fase) pero para **VS Code Copilot**. La exploración inicial concluyó erróneamente que VS Code Copilot no soportaba modelos por fase. Sin embargo, la **documentación oficial de VS Code Copilot (actualizada a mayo 2026)** demuestra que VS Code tiene una infraestructura de custom agents y subagents con asignación de modelos **nativa**, comparable (y en algunos aspectos superior) a la de OpenCode.

**Conclusión**: VS Code Copilot ya soporta multi-mode SDD de forma nativa mediante archivos `.agent.md`. No es necesario emular el comportamiento.

---

## 2. Fuente de la Información

Toda la información de este documento proviene de fuentes oficiales:

- [Custom agents in VS Code](https://code.visualstudio.com/docs/copilot/customization/custom-agents) — docs oficiales, publicadas 2026-05-06
- [Subagents in Visual Studio Code](https://code.visualstudio.com/docs/copilot/agents/subagents) — docs oficiales, publicadas 2026-05-06
- [Visual Studio Code 1.119 Release Notes](https://code.visualstudio.com/updates/v1_119) — release oficial, 2026-05-06
- [vscode-copilot-chat source](https://github.com/microsoft/vscode-copilot-chat) — Context7 indexado

---

## 3. Evidencia Documental: VS Code Copilot Soporta Multi-Mode

### 3.1. Custom Agents con Modelo Asignado

VS Code Copilot permite definir custom agents en archivos `.agent.md` con YAML frontmatter. El campo `model` acepta:

- Un modelo único: `model: "Claude Sonnet 4"`
- Un array de fallback: `model: ['Claude Opus 4.5', 'GPT-5.2']`

**Ejemplo oficial**:

```markdown
---
name: test-writer
description: "Writes comprehensive unit tests for TypeScript code"
model: sonnet
allowedTools:
  - Read
  - Grep
  - Glob
  - Edit
  - Write
  - Bash
---

You are a test-writing specialist...
```

### 3.2. Ubicación Nativa para User-Level Agents

Según la documentación oficial:

| Scope | Default file location |
|---|---|
| Workspace | `.github/agents/` folder |
| Workspace (Claude format) | `.claude/agents/` folder |
| **User profile** | **`~/.copilot/agents/`** or your user data |

> Fuente: [Custom agent file locations](https://code.visualstudio.com/docs/copilot/customization/custom-agents#_custom-agent-file-locations)

**Implicación**: `~/.copilot/agents/` es el directorio nativo documentado para custom agents a nivel usuario. Gentle-ai ya usa `~/.copilot/skills/`, por lo que `~/.copilot/agents/` es consistente con la convención existente.

### 3.3. Model Selection para Subagents

La documentación establece una **prioridad de tres niveles** para la selección del modelo de un subagente:

1. **Explicit model parameter**: el main agent especifica un modelo directamente al invocar `runSubagent`
2. **Agent-configured model**: la propiedad `model` en el frontmatter del `.agent.md`
3. **Main model**: el modelo que ejecuta la conversación padre

**Ejemplo documentado**:

```
Run a subagent with Claude Sonnet 4.6 to research authentication patterns in this codebase.
```

> Fuente: [Model selection for subagents](https://code.visualstudio.com/docs/copilot/agents/subagents#_model-selection-for-subagents)

### 3.4. Handoffs Nativos entre Agentes

VS Code Copilot soporta **handoffs** en el frontmatter del agente — transiciones guiadas entre agentes con botón sugerido. Cada handoff puede especificar un modelo distinto:

```yaml
---
description: Generate an implementation plan
tools: ['search', 'web']
handoffs:
  - label: Start Implementation
    agent: implementation
    prompt: Now implement the plan outlined above.
    send: false
    model: GPT-5.2 (copilot)
---
```

> Fuente: [Handoffs](https://code.visualstudio.com/docs/copilot/customization/custom-agents#_handoffs)

**Nota**: OpenCode **NO** tiene handoffs nativos. Esta es una ventaja de VS Code Copilot.

### 3.5. Restricción de Subagentes

El main agent puede restringir qué subagentes puede invocar:

```yaml
---
name: TDD
tools: ['agent']
agents: ['Red', 'Green', 'Refactor']
---
```

> Fuente: [Restrict which subagents can be used](https://code.visualstudio.com/docs/copilot/agents/subagents#_restrict-which-subagents-can-be-used-experimental)

Esto es equivalente al `task` permission de OpenCode (`"sdd-apply": "allow"`).

### 3.6. Agentes Ocultos (Solo Subagentes)

```yaml
---
name: internal-helper
user-invocable: false
---
```

Equivalente a `"hidden": true` en `opencode.json`.

### 3.7. Background Agents con Modelo Ligero

Las release notes 1.119 (2026-05-06) confirman que VS Code ya usa múltiples modelos en paralelo:

> "By offloading todo list management to a lightweight background agent, the main model can focus on the actual task while a smaller model keeps progress tracking in sync."

Esto demuestra que la arquitectura multi-modelo de VS Code ya está en producción.

---

## 4. Comparativa: OpenCode vs VS Code Copilot (Multi-Mode)

| Feature | OpenCode | VS Code Copilot (oficial) |
|---|---|---|
| **Archivo de config de agentes** | `opencode.json` | `.agent.md` files (YAML frontmatter + Markdown body) |
| **Ubicación user-level** | `~/.config/opencode/` | `~/.copilot/agents/` |
| **Modelo por agente** | `"model": "provider/modelID"` | `model: "Claude Sonnet 4"` o `model: ['Claude Opus', 'GPT-5']` |
| **Subagent invocation con modelo** | ❌ No (hereda del orchestrator) | ✅ **SÍ** — explicit model parameter |
| **Handoffs entre agentes** | ❌ No | ✅ **SÍ** — nativo con `handoffs:` en frontmatter |
| **Tool restrictions** | ✅ Sí | ✅ Sí — `tools: ['read', 'search']` |
| **Agents restriction** | ✅ Sí (via `task` permissions) | ✅ **SÍ** — `agents: ['Planner', 'Implementer']` |
| **Agentes ocultos** | `"hidden": true` | `user-invocable: false` |
| **Fallback de modelos** | ❌ No | ✅ **SÍ** — array de prioridad |
| **Formato Claude compatible** | ❌ No | ✅ **SÍ** — detecta `.claude/agents/*.md` |

---

## 5. Formato Nativo Propuesto para gentle-ai

### 5.1. Sub-agente por fase (ejemplo: `sdd-apply`)

Ubicación: `~/.copilot/agents/sdd-apply.agent.md`

```markdown
---
name: sdd-apply
description: "Implement code changes from task definitions"
model: "Claude Sonnet 4.6 (copilot)"
tools: ['read', 'write', 'edit', 'bash']
user-invocable: false
disable-model-invocation: false
agents: []
---

You are the sdd-apply agent. Implement code changes from task definitions...
```

### 5.2. Orchestrator (ejemplo: `gentle-orchestrator`)

Ubicación: `~/.copilot/agents/gentle-orchestrator.agent.md`

```markdown
---
name: gentle-orchestrator
description: "SDD Orchestrator — coordinates sub-agents, never does work inline"
model: "Claude Opus 4.5 (copilot)"
tools: ['agent', 'read', 'write', 'edit', 'bash', 'delegate']
agents: ['sdd-init', 'sdd-explore', 'sdd-propose', 'sdd-spec', 
         'sdd-design', 'sdd-tasks', 'sdd-apply', 'sdd-verify', 
         'sdd-archive', 'sdd-onboard']
user-invocable: true
---

## Model Assignments

Read this table at session start and cache it for the session.

| Phase | Model | Reason |
|-------|-------|--------|
| orchestrator | Claude Opus 4.5 (copilot) | Coordinates, makes decisions |
| sdd-init | Claude Sonnet 4 (copilot) | Bootstrap SDD context |
| sdd-explore | Claude Sonnet 4 (copilot) | Reads code, structural |
| ... | ... | ... |

## Sub-Agent References

When delegating, always invoke the correct sub-agent by name:
- `sdd-init` for bootstrapping
- `sdd-explore` for investigation
- `sdd-apply` for implementation
...
```

### 5.3. Handoffs (opcional, para flujos guiados)

El orchestrator puede definir handoffs para guiar al usuario entre fases:

```yaml
handoffs:
  - label: "Start Exploration"
    agent: sdd-explore
    prompt: "Explore this codebase to understand..."
    send: false
```

---

## 6. Implicaciones para el Diseño de gentle-ai

### 6.1. Cambios en el Adaptador VS Code

El adaptador actual (`internal/agents/vscode/adapter.go`) tiene:

```go
func (a *Adapter) SupportsSubAgents() bool {
    return false  // ❌ DEBE CAMBIAR A true
}

func (a *Adapter) SubAgentsDir(_ string) string {
    return ""     // ❌ DEBE RETORNAR ~/.copilot/agents/
}
```

**Cambios necesarios**:
- `SupportsSubAgents()`: retornar `true`
- `SubAgentsDir(homeDir)`: retornar `filepath.Join(homeDir, ".copilot", "agents")`
- `EmbeddedSubAgentsDir()`: definir path en assets embebidos (ej: `vscode/agents/`)
- Posiblemente agregar `SupportsWorkflows()` o similar si se usan handoffs

### 6.2. Nuevo Componente: Generador de `.agent.md`

Se necesita un componente equivalente a `GenerateProfileOverlay` de OpenCode, pero que genere archivos `.agent.md` en lugar de JSON.

**Responsabilidades**:
- Generar 11 archivos `.agent.md` por perfil (1 orchestrator + 10 fases)
- Inyectar tabla de model assignments en el body del orchestrator
- Asignar `user-invocable: false` a los sub-agentes
- Asignar `model` a cada agente según el perfil
- Manejar handoffs opcionales

### 6.3. Estrategia de Inyección

A diferencia de OpenCode (que hace deep-merge en `opencode.json`), VS Code Copilot requiere:

- Escribir archivos `.agent.md` físicos en `~/.copilot/agents/`
- No hay merge complejo — cada archivo es independiente
- Borrar archivos de perfiles eliminados (cleanup)
- Manejar nombres de archivo con sufijos para perfiles nombrados (ej: `sdd-apply-cheap.agent.md`)

### 6.4. Desacoplamiento (Golden Rule)

Siguiendo la Golden Rule del CODEBASE-GUIDE:

> "agent-specific paths belong in adapters; reusable behavior belongs in components"

- **Adaptador** (`internal/agents/vscode/`): define `SubAgentsDir()`, `EmbeddedSubAgentsDir()`, capabilities
- **Componente** (`internal/components/sdd/`): generador de `.agent.md` reusable (similar a `profiles.go` para OpenCode)
- **Assets** (`internal/assets/vscode/`): templates de `.agent.md` embebidos

---

## 7. Las 5 Fases de Implementación

### Fase 1: Comprensión del contexto y la arquitectura ✅

**Estado**: COMPLETADA.

Se investigó:
- Adaptador actual de VS Code (`internal/agents/vscode/adapter.go`)
- Adaptador de OpenCode y su mecanismo de perfiles
- Documentación oficial de VS Code Copilot (custom agents, subagents, handoffs)
- Infraestructura de assets embebidos y componentes SDD

**Hallazgo clave**: VS Code Copilot soporta multi-mode nativamente via `.agent.md` files.

### Fase 2: Modificación del adaptador de VS Code

**Objetivo**: Habilitar `SupportsSubAgents`, definir paths, agregar tests.

**Archivos a tocar**:
- `internal/agents/vscode/adapter.go`
- `internal/agents/vscode/adapter_test.go`

### Fase 3: Generación de los archivos `.agent.md` por fase

**Objetivo**: Crear generador de `.agent.md` y templates embebidos.

**Archivos a tocar**:
- `internal/components/sdd/vscode_profiles.go` (nuevo — generador)
- `internal/assets/vscode/` (nuevo — templates embebidos)
- `internal/components/sdd/inject.go` (modificar — agregar path VS Code)
- Tests correspondientes

### Fase 4: Orquestación mediante Handoffs

**Objetivo**: Definir handoffs en el orchestrator para guiar flujos SDD.

**Archivos a tocar**:
- Template del orchestrator `.agent.md`
- Configuración de handoffs en el generador de perfiles

### Fase 5: Revisión y manejo de errores (Fallback)

**Objetivo**: Tests de integración, validación post-inyección, rollback.

**Archivos a tocar**:
- `internal/agents/vscode/adapter_test.go`
- `internal/components/sdd/vscode_profiles_test.go` (nuevo)
- `internal/components/sdd/inject_test.go` (modificar)

---

## 8. Decisiones de Arquitectura

### 8.1. ¿Handoffs o no handoffs?

**Recomendación**: Implementar handoffs en una v2. Para la v1, mantener el mismo patrón que OpenCode: el orchestrator delega explícitamente a sub-agentes. Los handoffs agregan complejidad y no están en OpenCode.

### 8.2. ¿Un solo perfil o múltiples perfiles?

**Recomendación**: Replicar la misma semántica que OpenCode:
- Perfil default (sin sufijo): `gentle-orchestrator.agent.md`, `sdd-apply.agent.md`, etc.
- Perfiles nombrados (con sufijo): `sdd-orchestrator-cheap.agent.md`, `sdd-apply-cheap.agent.md`, etc.
- El TUI de gentle-ai ya tiene flujo de creación de perfiles — reutilizarlo.

### 8.3. ¿Dónde vive el system prompt del orchestrator?

**Opción A**: Todo en `gentle-orchestrator.agent.md` (incluyendo model assignments table).  
**Opción B**: System prompt en `gentle-ai.instructions.md` + `.agent.md` files en `~/.copilot/agents/`.

**Recomendación**: Opción A. El archivo `.agent.md` del orchestrator IS the system prompt. No duplicar en `gentle-ai.instructions.md`. Sin embargo, el `gentle-ai.instructions.md` puede seguir existiendo para instrucciones generales de gentle-ai que no son SDD-specific.

### 8.4. ¿Cómo se detecta que VS Code lee los agentes?

**Validación**: Después de la inyección, gentle-ai debería verificar que los archivos `.agent.md` existen y tienen contenido válido (similar al post-check de OpenCode que valida `gentle-orchestrator` en `opencode.json`).

---

## 8.1. Principio de Desacoplamiento Obligatorio (Golden Rule)

> **"Las rutas y configuraciones específicas de un agente pertenecen a los adaptadores."**
> — CODEBASE-GUIDE.md, Golden Rule

Esta feature debe implementarse con **cero impacto** en cualquier otro agente. Los principios son:

### 8.1.1. Aditivo, no Modificativo
- Se AGREGA el adaptador VS Code (`SupportsSubAgents: true`, `SubAgentsDir()`)
- Se CREA un nuevo componente (`vscode_profiles.go`) — no se modifica `profiles.go` de OpenCode
- Se CREA un nuevo directorio de assets (`internal/assets/vscode/`) — no se toca `internal/assets/opencode/`
- Se AGREGA un nuevo path en `inject.go` para el caso `AgentVSCodeCopilot` — el flujo de OpenCode permanece intacto

### 8.1.2. No Tocar Interfaces Existentes
- `agents.Adapter` interface: NO agregar métodos nuevos que obliguen a otros adaptadores a implementar stubs
- `model.Profile`: reutilizar el tipo existente (ya es agnóstico del agente)
- `model.ModelAssignment`: reutilizar el tipo existente

### 8.1.3. Namespace Aislado
- Todos los archivos generados usan prefijo `gentle-` o `sdd-` (ej: `sdd-apply.agent.md`)
- No sobrescribir agentes existentes del usuario en `~/.copilot/agents/`
- Cleanup al desinstalar: solo borrar archivos que gentle-ai creó (identificables por prefijo)

### 8.1.4. Tests Aislados
- Tests del adaptador VS Code: solo prueban el adaptador VS Code
- Tests del generador `.agent.md`: solo prueban el generador
- Tests de integración: verificar que OpenCode, Claude, Cursor, etc. NO se ven afectados

### 8.1.5. Feature Flag Implícito
- Si el usuario NO selecciona VS Code Copilot como agente, el código nuevo nunca se ejecuta
- Si el usuario tiene VS Code instalado pero NO configura perfiles SDD, el comportamiento default es single-mode (igual que hoy)

---

## 9. Riesgos Identificados

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|
| VS Code Insiders vs Stable: custom agents avanzados pueden requerir Insiders | Media | Alta | Documentar requisito de versión mínima |
| `.agent.md` en `~/.copilot/agents/` no es detectado por VS Code Stable | Baja | Alta | Validar con VS Code 1.119+ antes de release |
| El campo `model` en frontmatter no acepta el mismo formato que OpenCode | Media | Media | Mapear formatos en el generador (provider/model → "Model Name (vendor)") |
| Conflicto con agentes existentes del usuario en `~/.copilot/agents/` | Baja | Baja | Namespace con prefijo `gentle-` o `sdd-` |
| Tamaño del prompt del orchestrator con tabla de model assignments | Baja | Media | La tabla es texto plano, no debería exceder límites |

---

## 10. Archivos Preliminares a Modificar

### Adaptador VS Code
- `internal/agents/vscode/adapter.go` — habilitar sub-agents, definir paths
- `internal/agents/vscode/adapter_test.go` — tests de capabilities y paths

### Componente SDD (nuevo o modificado)
- `internal/components/sdd/vscode_profiles.go` — generador de `.agent.md`
- `internal/components/sdd/vscode_profiles_test.go` — tests del generador
- `internal/components/sdd/inject.go` — agregar path de inyección VS Code
- `internal/components/sdd/inject_test.go` — tests de inyección

### Assets embebidos (nuevo)
- `internal/assets/vscode/sdd-orchestrator.md` — template del orchestrator
- `internal/assets/vscode/sdd-init.md` — template de fase
- `internal/assets/vscode/sdd-explore.md` — template de fase
- ... (10 fases)

### Modelo
- `internal/model/types.go` — posiblemente agregar `AgentVSCodeCopilot` (ya existe)
- `internal/model/model_assignment.go` — posiblemente agregar conversión de formato de modelo

### TUI
- `internal/tui/screens/model_config.go` — posiblemente ajustar picker para VS Code
- `internal/tui/screens/profiles.go` — reutilizar flujo existente

---

## 11. Próximo Paso

Lanzar `sdd-propose` con este contexto para formalizar la propuesta técnica, seguido de `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-apply`, `sdd-verify`, y `sdd-archive`.

---

*Documento generado por gentle-ai SDD exploration phase.*
