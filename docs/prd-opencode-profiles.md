# PRD: OpenCode SDD Profiles

> **Create interchangeable model profiles for OpenCode — switch between orchestrator configurations with a single Tab.**

**Version**: 0.1.0-draft
**Author**: Gentleman Programming
**Date**: 2026-04-03
**Status**: Draft

---

## 1. Problem Statement

Hoy, OpenCode permite tener UN solo `sdd-orchestrator` con UN solo set de modelos asignados a los sub-agentes SDD. Esto fuerza al usuario a elegir entre:

- **Calidad máxima** (Opus everywhere → caro y lento)
- **Balance** (Opus orquestador + Sonnet sub-agentes → el default actual)
- **Economía** (Sonnet/Haiku everywhere → rápido y barato pero menos potente)

El problema: **no podés cambiar entre estas configuraciones sin editar manualmente el `opencode.json`** cada vez que querés pasar de un modo a otro. Y en la práctica, un developer necesita diferentes perfiles para diferentes momentos:

- **"Voy a hacer algo heavy"** → orquestador Opus, sub-agentes Sonnet
- **"Es algo simple, no quiero quemar tokens"** → todo Haiku
- **"Quiero probar un modelo nuevo de Google"** → orquestador Gemini, sub-agentes mixtos
- **"Estoy revieweando un PR nomas"** → perfil liviano

Hoy eso es un dolor de cabeza manual. Esta feature lo resuelve.

---

## 2. Vision

**El usuario crea N perfiles de modelos desde la TUI. Cada perfil genera su propio `sdd-orchestrator-{nombre}` con sus propios sub-agentes en `opencode.json`. En OpenCode, le da Tab y ve todos los orquestadores disponibles — cambia entre perfiles como quien cambia de marcha.**

```
┌─────────────────────────────────────────────────────────────┐
│  opencode.json                                               │
│                                                              │
│  ┌──────────────────────┐   ┌──────────────────────────────┐ │
│  │  sdd-orchestrator    │   │  sdd-orchestrator-cheap      │ │
│  │  (opus + sonnet)     │   │  (haiku everywhere)          │ │
│  │                      │   │                              │ │
│  │  sdd-init     sonnet │   │  sdd-init-cheap     haiku   │ │
│  │  sdd-explore  sonnet │   │  sdd-explore-cheap  haiku   │ │
│  │  sdd-apply    sonnet │   │  sdd-apply-cheap    haiku   │ │
│  │  ...                 │   │  ...                        │ │
│  └──────────────────────┘   └──────────────────────────────┘ │
│                                                              │
│  Tab en OpenCode → elegí cuál orquestador usar              │
└─────────────────────────────────────────────────────────────┘
```

---

## 3. Target Users

| User | Pain Point | How Profiles Help |
|------|-----------|-------------------|
| **Power user con múltiples providers** | Quiere probar Anthropic vs Google vs OpenAI para SDD sin tocar config | Crea un perfil por provider, cambia con Tab |
| **Developer cost-conscious** | Quiere un modo "barato" para tareas simples | Perfil "cheap" con Haiku/Flash, perfil "premium" con Opus |
| **Team lead** | Quiere estandarizar perfiles para el equipo | Los perfiles viven en `opencode.json`, synceables |
| **Experimentador** | Quiere testear modelos nuevos sin romper su config default | Perfil experimental, el default intacto |

---

## 4. Scope

### In Scope (v1)
- Creación de perfiles desde la TUI (nuevo screen)
- Visualización de perfiles existentes
- **Edición de perfiles existentes desde la TUI** (seleccionar perfil → modificar modelos → sync)
- **Eliminación de perfiles desde la TUI** (seleccionar perfil → confirmar → elimina orchestrator + sub-agentes del JSON → sync)
- Generación de N orchestrators + N×9 sub-agentes en `opencode.json`
- Actualización de perfiles existentes durante Sync / Update+Sync
- Prompts compartidos: un archivo por fase, reutilizado por todos los perfiles
- CLI flag para crear perfiles (`--profile`)

### Out of Scope (permanently)
- **Perfiles para Claude Code** — NO APLICA. Claude Code usa un mecanismo completamente diferente (CLAUDE.md + Task tool). La feature de profiles es exclusiva de OpenCode porque depende del sistema de agents/sub-agents de `opencode.json` y la selección por Tab. Esto NO es "futuro" — es una decisión de arquitectura.

### Out of Scope (v1, future consideration)
- Exportar/importar perfiles entre máquinas

---

## 5. Detailed Requirements

### 5.1 TUI: Profile Creation Screen

**R-PROF-01**: El Welcome screen DEBE incluir una nueva opción **"OpenCode SDD Profiles"** debajo de "Configure Models".

**R-PROF-02**: Si ya existen perfiles creados, la opción DEBE mostrar el conteo: `"OpenCode SDD Profiles (2)"`.

**R-PROF-03**: El screen de perfiles DEBE mostrar los perfiles existentes con acciones disponibles:

```
┌─────────────────────────────────────────────────────────┐
│  OpenCode SDD Profiles                                   │
│                                                          │
│  Existing profiles:                                      │
│    ✦ default ─── anthropic/claude-opus-4                 │
│    • cheap ───── anthropic/claude-haiku-3.5              │
│    • gemini ──── google/gemini-2.5-pro                   │
│                                                          │
│  ▸ Create new profile                                    │
│    Back                                                  │
│                                                          │
│  j/k: navigate • enter: edit • n: new • d: delete       │
│  esc: back                                               │
└─────────────────────────────────────────────────────────┘
```

**R-PROF-04**: Al seleccionar "Create new profile" (o presionar `n`), el usuario DEBE:
1. **Ingresar un nombre** para el perfil (texto libre, validado a slug: lowercase, sin espacios, alfanumérico + hyphens)
2. **Seleccionar el modelo del orchestrator** (reutilizando el ModelPicker existente — provider → model)
3. **Seleccionar modelos para los sub-agentes** (reutilizando el ModelPicker existente con las 9+1 filas: Set all + 9 fases)
4. **Confirmar** → se genera el perfil y se ejecuta sync

**R-PROF-05**: El nombre "default" ESTÁ RESERVADO para el orchestrator base (`sdd-orchestrator`). El usuario NO puede crear un perfil llamado "default".

**R-PROF-06**: Si el usuario ingresa un nombre que ya existe, se DEBE preguntar si quiere sobreescribir.

### 5.1b TUI: Profile Editing

**R-PROF-07**: Al presionar `enter` sobre un perfil existente en la lista, el usuario entra en modo edición. El flujo es IDÉNTICO al de creación pero:
- El nombre NO se puede cambiar (se muestra como header fijo)
- El modelo del orchestrator viene pre-seleccionado con el valor actual
- Los modelos de sub-agentes vienen pre-seleccionados con los valores actuales
- Al confirmar, se sobreescribe el perfil existente y se ejecuta sync

**R-PROF-07b**: El perfil `default` también se PUEDE editar — es el `sdd-orchestrator` base. Editar el default es equivalente a lo que hoy hace "Configure Models → OpenCode" pero integrado en el flujo de perfiles.

### 5.1c TUI: Profile Deletion

**R-PROF-08**: Al presionar `d` sobre un perfil existente en la lista, se DEBE mostrar un screen de confirmación:

```
┌─────────────────────────────────────────────────────────┐
│  Delete Profile                                          │
│                                                          │
│  Are you sure you want to delete profile "cheap"?        │
│                                                          │
│  This will remove from opencode.json:                    │
│    • sdd-orchestrator-cheap                              │
│    • sdd-init-cheap                                      │
│    • sdd-explore-cheap                                   │
│    • ... (10 agents total)                               │
│                                                          │
│  ▸ Delete                                                │
│    Cancel                                                │
│                                                          │
│  enter: select • esc: cancel                             │
└─────────────────────────────────────────────────────────┘
```

**R-PROF-08b**: Al confirmar la eliminación:
1. Se eliminan TODOS los agent keys del perfil del `opencode.json` (`sdd-orchestrator-{name}` + 10 sub-agentes `sdd-{phase}-{name}`)
2. Se ejecuta un write atómico del JSON actualizado
3. Se muestra resultado (éxito o error)
4. Se vuelve a la lista de perfiles (con el perfil eliminado)

**R-PROF-08c**: El perfil `default` NO se puede eliminar. Presionar `d` sobre el default NO hace nada (el keybinding se ignora). El default es el orchestrator base que siempre debe existir.

**R-PROF-08d**: La eliminación de un perfil NO elimina los archivos de prompt compartidos (`~/.config/opencode/prompts/sdd/*.md`) — esos son compartidos por todos los perfiles y se mantienen mientras exista al menos un perfil.

### 5.2 Naming Convention

**R-PROF-10**: El perfil DEFAULT (sin sufijo) genera los agentes con los nombres actuales:
- `sdd-orchestrator`
- `sdd-init`, `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-apply`, `sdd-verify`, `sdd-archive`

**R-PROF-11**: Un perfil con nombre `cheap` genera agentes con sufijo:
- `sdd-orchestrator-cheap`
- `sdd-init-cheap`, `sdd-explore-cheap`, ..., `sdd-archive-cheap`

**R-PROF-12**: El `sdd-orchestrator-{name}` DEBE tener `"mode": "primary"` para que aparezca como seleccionable con Tab en OpenCode. Los sub-agentes `sdd-{phase}-{name}` DEBEN tener `"mode": "subagent"` y `"hidden": true`.

**R-PROF-13**: Las permissions del orchestrator de un perfil DEBEN scoped a sus propios sub-agentes:
```json
{
  "permission": {
    "task": {
      "*": "deny",
      "sdd-*-cheap": "allow"
    }
  }
}
```

### 5.3 Shared Prompt Architecture

**R-PROF-20**: Los prompts de cada fase SDD DEBEN vivir en archivos separados bajo `~/.config/opencode/prompts/sdd/`:
```
~/.config/opencode/prompts/sdd/
├── orchestrator.md
├── sdd-init.md
├── sdd-explore.md
├── sdd-propose.md
├── sdd-spec.md
├── sdd-design.md
├── sdd-tasks.md
├── sdd-apply.md
├── sdd-verify.md
├── sdd-archive.md
└── sdd-onboard.md
```

**R-PROF-21**: El `prompt` de cada agente en opencode.json DEBE referenciar el archivo compartido usando la sintaxis de OpenCode `{file:path}`:
```json
{
  "sdd-apply": {
    "mode": "subagent",
    "hidden": true,
    "model": "anthropic/claude-sonnet-4-20250514",
    "prompt": "{file:~/.config/opencode/prompts/sdd/sdd-apply.md}"
  },
  "sdd-apply-cheap": {
    "mode": "subagent",
    "hidden": true,
    "model": "anthropic/claude-haiku-3.5-20241022",
    "prompt": "{file:~/.config/opencode/prompts/sdd/sdd-apply.md}"
  }
}
```

**R-PROF-22**: El contenido de estos archivos de prompt DEBE ser EXACTAMENTE el mismo que hoy se inline en el overlay JSON. El refactor es extracto sin cambio de comportamiento.

**R-PROF-23**: El prompt del orchestrator (`orchestrator.md`) DEBE incluir un bloque `<!-- gentle-ai:sdd-model-assignments -->` que se inyecta dinámicamente con la tabla de modelos de ESE perfil específico.

**R-PROF-24**: Para el orchestrator de un perfil, el prompt DEBE referenciar los sub-agentes CON SUFIJO. Esto significa que el `orchestrator.md` compartido necesita un placeholder o que cada orchestrator de perfil tenga su propia copia con los nombres correctos. 

**Decisión arquitectónica**: El orchestrator prompt NO se comparte entre perfiles — cada perfil genera su propia versión con:
- La tabla de model assignments de ese perfil
- Las references a `sdd-{phase}-{suffix}` correctas

Los sub-agente prompts SÍ se comparten porque son idénticos entre perfiles (solo cambia el modelo, no el prompt).

### 5.4 Sync & Update Behavior

**R-PROF-30**: Durante `Sync` o `Update+Sync`, el sistema DEBE:
1. Detectar TODOS los perfiles existentes en `opencode.json` (pattern: `sdd-orchestrator-*`)
2. Actualizar los prompts compartidos en `~/.config/opencode/prompts/sdd/`
3. Regenerar los orchestrator prompts de cada perfil (para inyectar model assignments actualizados)
4. NO modificar las asignaciones de modelos de los perfiles — solo los prompts

**R-PROF-31**: Si un perfil tiene un sub-agente que referencia un modelo que ya no existe en el cache de OpenCode, el Sync DEBE:
- **Warning** al usuario (no error)
- Preservar la asignación existente (el usuario puede haberlo configurado manualmente)

**R-PROF-32**: Los archivos de prompt compartidos DEBEN estar cubiertos por el backup pre-sync, igual que `opencode.json`.

**R-PROF-33**: El Sync DEBE ser idempotente: si los prompts ya están actualizados, `filesChanged` NO debe incrementar.

### 5.5 Profile Detection & State

**R-PROF-40**: Los perfiles DEBEN detectarse leyendo el `opencode.json` existente, NO desde un archivo de estado separado. El `opencode.json` ES la fuente de verdad.

**R-PROF-41**: Un perfil se detecta por la presencia de un agent key que matchea `sdd-orchestrator-{name}` con `"mode": "primary"`.

**R-PROF-42**: Al detectar perfiles existentes, el sistema DEBE inferir:
- **Nombre**: el sufijo después de `sdd-orchestrator-`
- **Modelo del orchestrator**: el campo `"model"` del orchestrator
- **Modelos de sub-agentes**: los campos `"model"` de `sdd-{phase}-{name}`

**R-PROF-43**: El perfil default (`sdd-orchestrator` sin sufijo) SIEMPRE existe cuando SDD está configurado. Los perfiles adicionales son opcionales.

### 5.6 CLI Support

**R-PROF-50**: El comando `sync` DEBE aceptar un flag `--profile <name>:<orchestrator-model>` que crea/actualiza un perfil durante el sync:
```bash
gentle-ai sync --profile cheap:anthropic/claude-haiku-3.5-20241022
```

**R-PROF-51**: Se DEBEN poder especificar múltiples `--profile` flags:
```bash
gentle-ai sync \
  --profile cheap:anthropic/claude-haiku-3.5-20241022 \
  --profile premium:anthropic/claude-opus-4-20250514
```

**R-PROF-52**: El formato del flag es `name:provider/model`. Para asignar modelos individuales a sub-agentes vía CLI, se usa la sintaxis extendida:
```bash
gentle-ai sync --profile cheap:anthropic/claude-haiku-3.5-20241022 \
  --profile-phase cheap:sdd-apply:anthropic/claude-sonnet-4-20250514
```

---

## 6. Technical Design

### 6.1 Data Model

```go
// Profile represents a named SDD orchestrator configuration with model assignments.
type Profile struct {
    Name                string                       // e.g. "cheap", "premium"
    OrchestratorModel   model.ModelAssignment         // orchestrator model
    PhaseAssignments    map[string]model.ModelAssignment // per-phase models (optional overrides)
}
```

### 6.2 OpenCode JSON Structure (per profile)

Para un perfil llamado "cheap" con Haiku:

```json
{
  "agent": {
    "sdd-orchestrator-cheap": {
      "mode": "primary",
      "description": "SDD Orchestrator (cheap profile) — haiku everywhere",
      "model": "anthropic/claude-haiku-3.5-20241022",
      "prompt": "... orchestrator prompt with cheap-specific model table and sub-agent references ...",
      "permission": {
        "task": {
          "*": "deny",
          "sdd-*-cheap": "allow"
        }
      },
      "tools": {
        "read": true,
        "write": true,
        "edit": true,
        "bash": true,
        "delegate": true,
        "delegation_read": true,
        "delegation_list": true
      }
    },
    "sdd-init-cheap": {
      "mode": "subagent",
      "hidden": true,
      "model": "anthropic/claude-haiku-3.5-20241022",
      "description": "Bootstrap SDD context (cheap profile)",
      "prompt": "{file:~/.config/opencode/prompts/sdd/sdd-init.md}"
    },
    "sdd-explore-cheap": {
      "mode": "subagent",
      "hidden": true,
      "model": "anthropic/claude-haiku-3.5-20241022",
      "description": "Investigate codebase (cheap profile)",
      "prompt": "{file:~/.config/opencode/prompts/sdd/sdd-explore.md}"
    }
    // ... remaining 7 sub-agents with -cheap suffix
  }
}
```

### 6.3 Prompt File Architecture

```
~/.config/opencode/
├── opencode.json          (agents with model + prompt refs)
├── prompts/
│   └── sdd/
│       ├── sdd-init.md        (shared by all profiles)
│       ├── sdd-explore.md     (shared by all profiles)
│       ├── sdd-propose.md     (shared)
│       ├── sdd-spec.md        (shared)
│       ├── sdd-design.md      (shared)
│       ├── sdd-tasks.md       (shared)
│       ├── sdd-apply.md       (shared)
│       ├── sdd-verify.md      (shared)
│       ├── sdd-archive.md     (shared)
│       └── sdd-onboard.md     (shared)
├── skills/                (existing SDD skills)
├── commands/              (existing slash commands)
└── plugins/               (existing plugins)
```

**Key insight**: Los orchestrator prompts NO se comparten como archivos externos porque cada perfil necesita su propia tabla de model assignments y referencias a sub-agentes con sufijo. Se inline en el JSON de cada orchestrator durante la generación.

Los sub-agent prompts SÍ se comparten como archivos `{file:...}` porque son idénticos entre perfiles — solo el campo `"model"` cambia.

### 6.4 Affected Files (Implementation Map)

| Area | File | Changes |
|------|------|---------|
| **Domain model** | `internal/model/types.go` | Add `Profile` struct |
| **Domain model** | `internal/model/selection.go` | Add `Profiles []Profile` to `Selection` and `SyncOverrides` |
| **TUI: screens** | `internal/tui/screens/profiles.go` | NEW — profile list screen (list + edit + delete actions) |
| **TUI: screens** | `internal/tui/screens/profile_create.go` | NEW — profile creation/edit flow (name → models → confirm) |
| **TUI: screens** | `internal/tui/screens/profile_delete.go` | NEW — profile delete confirmation screen |
| **TUI: model** | `internal/tui/model.go` | Add `ScreenProfiles`, `ScreenProfileCreate`, `ScreenProfileEdit`, `ScreenProfileDelete`, `ScreenProfileResult` |
| **TUI: router** | `internal/tui/router.go` | Add routes for all profile screens |
| **TUI: welcome** | `internal/tui/screens/welcome.go` | Add "OpenCode SDD Profiles" option |
| **SDD inject** | `internal/components/sdd/inject.go` | Extract prompts to files, generate profile agents |
| **SDD inject** | `internal/components/sdd/profiles.go` | NEW — profile CRUD: generate, detect, delete agents from JSON |
| **SDD inject** | `internal/components/sdd/prompts.go` | NEW — shared prompt file management |
| **SDD inject** | `internal/components/sdd/read_assignments.go` | Add profile detection from opencode.json |
| **Sync** | `internal/cli/sync.go` | Update sync to handle profiles, add `--profile` flag |
| **Assets** | `internal/assets/opencode/sdd-overlay-multi.json` | Refactor to use `{file:...}` references |
| **OpenCode models** | `internal/opencode/models.go` | No changes (reuse existing) |

### 6.5 Sync Flow (Updated)

```
Sync Start
  │
  ├─ 1. Read opencode.json → detect existing profiles
  │     (pattern: sdd-orchestrator-*)
  │
  ├─ 2. Write/update shared prompt files
  │     ~/.config/opencode/prompts/sdd/*.md
  │     (from embedded assets, same as today's inline prompts)
  │
  ├─ 3. Update DEFAULT orchestrator + sub-agents
  │     (sdd-orchestrator, sdd-init, ..., sdd-archive)
  │     - Update prompts (inline for orchestrator, {file:} for sub-agents)
  │     - Preserve model assignments
  │
  ├─ 4. For EACH detected profile:
  │     ├─ Update sub-agent prompts (they use {file:}, auto-updated in step 2)
  │     ├─ Regenerate orchestrator prompt (inline, with profile's model table)
  │     └─ Preserve model assignments
  │
  └─ 5. Verify: all profile orchestrators + sub-agents present
```

### 6.6 Migration Path

**Backward compatibility**: Users sin perfiles no ven cambios. El refactor de prompts a archivos es transparente:

1. **First sync after update**: 
   - Crea `~/.config/opencode/prompts/sdd/` directory
   - Escribe los prompt files
   - Migra sub-agentes del overlay de inline prompt a `{file:...}` reference
   - Resultado: comportamiento idéntico, solo cambia dónde vive el prompt

2. **Users con multi-mode existente**:
   - Sus model assignments se preservan
   - Sus sub-agentes se migran a `{file:...}` automáticamente
   - Cero disruption

---

## 7. UX Flow

### 7.1 Welcome Screen (Updated)

```
┌─────────────────────────────────────────────────────────┐
│                                                          │
│  ★  Gentleman AI Ecosystem — v0.x.x                     │
│     Supercharge your AI agents.                          │
│                                                          │
│  ▸ Install Ecosystem                                     │
│    Update                                                │
│    Sync                                                  │
│    Update + Sync                                         │
│    Configure Models                                      │
│    OpenCode SDD Profiles (2)                     ← NEW   │
│    Manage Backups                                        │
│    Quit                                                  │
│                                                          │
│  j/k: navigate • enter: select • q: quit                │
└─────────────────────────────────────────────────────────┘
```

### 7.2 Profile List Screen

```
┌─────────────────────────────────────────────────────────┐
│  OpenCode SDD Profiles                                   │
│                                                          │
│  Your SDD model profiles for OpenCode. Each profile      │
│  creates its own orchestrator (visible with Tab).        │
│                                                          │
│  Existing profiles:                                      │
│    ✦ default ─── anthropic/claude-opus-4                 │
│  ▸   cheap ───── anthropic/claude-haiku-3.5              │
│      gemini ──── google/gemini-2.5-pro                   │
│                                                          │
│    Create new profile                                    │
│    Back                                                  │
│                                                          │
│  j/k: navigate • enter: edit • n: new • d: delete       │
│  esc: back                                               │
└─────────────────────────────────────────────────────────┘
```

Profiles are navigable items. The cursor can be on a profile OR on "Create new profile" / "Back":
- **enter on a profile** → edit mode (modify models, then sync)
- **d on a profile** → delete confirmation (except default)
- **enter on "Create new profile"** → creation flow
- **n anywhere** → shortcut for "Create new profile"

### 7.3 Profile Edit Flow

Identical to creation but with pre-populated values:

```
┌─────────────────────────────────────────────────────────┐
│  Edit Profile "cheap"                                    │
│                                                          │
│  Current orchestrator: anthropic/claude-haiku-3.5        │
│                                                          │
│  ▸ Change orchestrator model                             │
│    Change sub-agent models                               │
│    Save & Sync                                           │
│    Cancel                                                │
│                                                          │
│  j/k: navigate • enter: select • esc: cancel            │
└─────────────────────────────────────────────────────────┘
```

### 7.4 Profile Delete Flow

```
┌─────────────────────────────────────────────────────────┐
│  Delete Profile                                          │
│                                                          │
│  Are you sure you want to delete profile "cheap"?        │
│                                                          │
│  This will remove from opencode.json:                    │
│    • sdd-orchestrator-cheap                              │
│    • sdd-init-cheap ... sdd-archive-cheap                │
│    • (11 agents total)                                   │
│                                                          │
│  ▸ Delete & Sync                                         │
│    Cancel                                                │
│                                                          │
│  enter: select • esc: cancel                             │
└─────────────────────────────────────────────────────────┘
```

### 7.5 Profile Creation Flow

```
Step 1: Name
┌─────────────────────────────────────────────────────────┐
│  Create SDD Profile                                      │
│                                                          │
│  Profile name: cheap_                                    │
│                                                          │
│  (lowercase, hyphens allowed, no spaces)                 │
│  Reserved: "default"                                     │
│                                                          │
│  enter: confirm • esc: cancel                            │
└─────────────────────────────────────────────────────────┘

Step 2: Orchestrator Model
┌─────────────────────────────────────────────────────────┐
│  Profile "cheap" — Select Orchestrator Model             │
│                                                          │
│  ▸ anthropic                                             │
│    google                                                │
│    openai                                                │
│    Back                                                  │
│                                                          │
│  (reuses existing ModelPicker)                           │
└─────────────────────────────────────────────────────────┘

Step 3: Sub-agent Models
┌─────────────────────────────────────────────────────────┐
│  Profile "cheap" — Assign Sub-agent Models               │
│                                                          │
│  ▸ Set all phases ──── (none)                            │
│    sdd-init ────────── (none)                            │
│    sdd-explore ─────── (none)                            │
│    sdd-propose ─────── (none)                            │
│    sdd-spec ─────────── (none)                           │
│    sdd-design ──────── (none)                            │
│    sdd-tasks ────────── (none)                           │
│    sdd-apply ────────── (none)                           │
│    sdd-verify ──────── (none)                            │
│    sdd-archive ─────── (none)                            │
│    Continue                                              │
│    Back                                                  │
│                                                          │
│  (reuses existing ModelPicker with provider/model drill) │
└─────────────────────────────────────────────────────────┘

Step 4: Confirm + Sync
┌─────────────────────────────────────────────────────────┐
│  Profile "cheap" — Ready to Create                       │
│                                                          │
│  Orchestrator: anthropic/claude-haiku-3.5-20241022      │
│  Sub-agents:   anthropic/claude-haiku-3.5-20241022 (all)│
│                                                          │
│  This will:                                              │
│  • Add sdd-orchestrator-cheap to opencode.json           │
│  • Add 10 sub-agents (sdd-init-cheap ... sdd-archive-cheap) │
│  • Run sync to apply changes                             │
│                                                          │
│  ▸ Create & Sync                                         │
│    Cancel                                                │
│                                                          │
│  enter: select • esc: cancel                             │
└─────────────────────────────────────────────────────────┘
```

---

## 8. Edge Cases & Decisions

### 8.1 OpenCode Model Cache Not Available

Si `~/.cache/opencode/models.json` no existe (OpenCode no se ejecutó nunca), el screen de profile creation DEBE:
- Mostrar un mensaje explicativo: "Run OpenCode at least once to populate the model cache"
- Ofrecer solo "Back"
- NO bloquear el resto de la TUI

### 8.2 Profile Name Validation

| Input | Valid? | Reason |
|-------|--------|--------|
| `cheap` | ✓ | Simple slug |
| `premium-v2` | ✓ | Hyphens allowed |
| `my profile` | ✗ | Spaces not allowed |
| `default` | ✗ | Reserved |
| `LOUD` | → `loud` | Auto-lowercased |
| `sdd-orchestrator` | ✗ | Would create `sdd-orchestrator-sdd-orchestrator` — confusing |
| `a` | ✓ | Minimum 1 char |
| (empty) | ✗ | Must have a name |

### 8.3 Model Inheritance for Sub-agents

When a sub-agent doesn't have an explicit model assignment:
1. Use the orchestrator model from the same profile
2. If orchestrator model is not set, use the root `"model"` from opencode.json
3. If nothing is set, OpenCode uses its default

### 8.4 Deleting a Profile

Deletion is fully supported from the TUI (press `d` on a profile → confirm → agents removed from JSON → sync). The operation:
1. Reads `opencode.json`
2. Removes ALL keys matching `sdd-orchestrator-{name}` and `sdd-{phase}-{name}` (11 keys total)
3. Writes the updated JSON atomically
4. Runs sync to ensure consistency
5. The `default` profile CANNOT be deleted — the keybinding is ignored on it

### 8.5 Orchestrator Prompt — Sub-agent References

El orchestrator prompt del default profile referencia sub-agentes como `sdd-apply`. Un perfil "cheap" necesita que su orchestrator reference `sdd-apply-cheap`. 

**Solution**: Al generar el orchestrator prompt de un perfil, se hace string replacement del pattern `sdd-{phase}` → `sdd-{phase}-{suffix}` SOLO dentro de las secciones que referencian sub-agentes (Model Assignments table, delegation rules). Esto se hace en tiempo de generación, no en el archivo compartido.

---

## 9. Success Metrics

| Metric | Target |
|--------|--------|
| Profile creation time (TUI) | < 60 seconds |
| Sync time with 3 profiles | < 5 seconds additional |
| Zero regression on users without profiles | 100% backward compatible |
| Profile count supported | Tested up to 10 |
| Files changed per sync (no actual changes) | 0 (idempotent) |

---

## 10. Implementation Phases

### Phase 1: Shared Prompt Refactor (Foundation)
- Extract sub-agent prompts to `~/.config/opencode/prompts/sdd/*.md`
- Update `sdd-overlay-multi.json` to use `{file:...}` references
- Update `inject.go` to write prompt files
- Update sync to maintain prompt files
- **Zero behavioral change** — same prompts, different location

### Phase 2: Profile Data Model & Generation
- Add `Profile` type to domain model
- Implement profile agent generation (orchestrator + sub-agents with suffix)
- Profile detection from existing opencode.json
- Update `injectModelAssignments` to handle multiple profiles

### Phase 3: TUI Screens — Create & List
- Profile list screen (shows existing profiles with actions)
- Profile creation flow (name → orchestrator model → sub-agent models → confirm)
- Wire into Welcome screen
- Integrate with sync flow (auto-sync after profile creation)

### Phase 4: TUI Screens — Edit & Delete
- Profile edit flow (select profile → modify models → save & sync)
- Profile delete confirmation screen + JSON cleanup
- `d` keybinding on profile list for delete
- `enter` keybinding on profile for edit
- Default profile protection (no delete, yes edit)

### Phase 5: Sync Integration
- Update sync to detect and maintain all profiles
- Add `--profile` CLI flag
- Update backup targets to include prompt files
- Update post-sync verification for profiles

### Phase 6: Polish & Testing
- E2E tests for profile creation, edit, delete + sync
- Edge case handling (missing cache, invalid names, etc.)
- Documentation update

---

## 11. Open Questions

1. **¿El orchestrator prompt de cada perfil se inline en el JSON o se guarda como archivo?**
   → Decisión: INLINE en el JSON. El orchestrator prompt es profile-specific (model table + sub-agent references), no se puede compartir como archivo. Los sub-agent prompts SÍ se comparten como archivos.

2. **¿Qué pasa con `sdd-onboard` en perfiles?**
   → Decisión: `sdd-onboard-{name}` se genera como sub-agente del perfil, igual que los otros 9 sub-agentes.

3. **¿Los slash commands SDD (`/sdd-new`, `/sdd-ff`, etc.) funcionan con perfiles custom?**
   → Sí. Los comandos están bound al orchestrator. Cuando el usuario selecciona `sdd-orchestrator-cheap` con Tab, los comandos se ejecutan contra ese orchestrator que delega a `sdd-*-cheap` sub-agentes.

4. **¿Cómo maneja OpenCode el `{file:...}` en prompts? ¿Soporta `~` expansion?**
   → Validar con OpenCode docs. Si no soporta `~`, usar path absoluto expandido durante la generación.

5. **¿El `gentleman` agent (persona) también necesita variantes por perfil?**
   → No. El `gentleman` agent es la persona general, no parte de SDD. Solo se mirror el modelo del orchestrator default.
