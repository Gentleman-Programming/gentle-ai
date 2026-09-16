# Diseño Arquitectónico: axiom-orchestrator, Comandos Slash Canónicos y Migración de Marcadores (INC-11)

## Contexto y Motivación

En la evolución de Axiom como plataforma independiente y desacoplada de Gentle AI:
1. En **OpenCode**, el orquestador principal y sus comandos asociados aún se denominan `gentle-orchestrator`, y el rol se identifica a sí mismo como *"You are the gentle-orchestrator"*.
2. En **Claude Code**, los comandos del ciclo SDD utilizan el prefijo `gentle-sdd-*` (`gentle-sdd-apply.md`, `gentle-sdd-verify.md`, etc.), desviándose del estándar canónico `/sdd-*` de OpenSpec.
3. En la persistencia sobre el sistema de archivos, las secciones gestionadas en Markdown utilizan marcadores `<!-- gentle-ai:... -->` y el centinela de cambio SDD se denomina `.gentle-ai-instance`.

El objetivo de este diseño es definir la transición limpia hacia la identidad de Axiom en todos estos puntos clave, garantizando:
- Identidad nativa de `axiom-orchestrator`.
- Comandos canónicos directos `/sdd-*` en Claude Code y OpenCode.
- Marcadores canónicos `<!-- axiom:... -->` y archivo centinela `.axiom-instance`.
- **Retrocompatibilidad absoluta** para leer, adoptar y limpiar configuraciones existentes con los nombres y marcadores legados de Gentle AI.

---

## Decisiones de Diseño

### Decisión 1: Identidad del Agente `axiom-orchestrator` en OpenCode
- **Cambio en Overlays:** En `internal/assets/opencode/sdd-overlay-multi.json` y `sdd-overlay-single.json`, la clave del orquestador pasa a ser `"axiom-orchestrator"`.
  - Descripción: `"Axiom SDD Orchestrator - coordinates sub-agents, never does work inline"`.
  - Modo: `"primary"`.
  - Prompt: `"{file:./AGENTS.md}"`.
- **Mapeo y retrocompatibilidad en `internal/opencode/config.go`:**
  - `configuredAssignments()` mapea `sdd-orchestrator` y `gentle-orchestrator` hacia la clave canónica `axiom-orchestrator`. De esta forma, un archivo `opencode.json` con asignaciones de modelo previas para `gentle-orchestrator` preserva sus modelos para `axiom-orchestrator`.
  - `managedOpenCodeAgentKeys()` incluye `"axiom-orchestrator"`, `"gentle-orchestrator"`, `"sdd-orchestrator"`, etc.
  - `managedConfigPriority()` reconoce tanto `"axiom/sdd"` como `"gentle-ai/sdd"` en el campo `__managed_by`.
- **Actualización de Comandos en OpenCode:**
  - Los 13 comandos en `internal/assets/opencode/commands/` declaran en su frontmatter: `agent: axiom-orchestrator`.
  - En su cuerpo se identifican como: `"You are the axiom-orchestrator"`.

### Decisión 2: Comandos Slash Canónicos `/sdd-*` en Claude Code
- **Assets de Claude:** Los archivos en `internal/assets/claude/commands/` se renombran de `gentle-sdd-*.md` a `sdd-*.md`.
- **Lógica de Nombre en `internal/components/sdd/commands.go`:**
  - `SlashCommandFileName(agent, name)` devuelve `name + ".md"` para todos los agentes, eliminando el prefijo obligatorio para Claude Code.
  - `LegacyClaudeCommandPath()` y `IsLegacyClaudeCommandPath()` se actualizan para detectar como legados los archivos existentes que comiencen por `gentle-sdd-*.md`.
  - Durante las operaciones de instalación, sincronización (`sync`) o desinstalación (`uninstall`), cualquier comando antiguo con prefijo `gentle-sdd-*.md` presente en `~/.claude/commands/` es retirado limpiamente, siendo reemplazado por `sdd-*.md`.

### Decisión 3: Soporte Dual de Marcadores de Sección Markdown
- **Marcador Canónico Axiom:**
  - Apertura: `<!-- axiom:<sectionID> -->`
  - Cierre: `<!-- /axiom:<sectionID> -->`
- **Marcador Legado Gentle AI:**
  - Apertura: `<!-- gentle-ai:<sectionID> -->`
  - Cierre: `<!-- /gentle-ai:<sectionID> -->`
- **Comportamiento en `internal/components/filemerge/section.go`:**
  - Al inyectar una sección nueva o reemplazar una existente, se escribe siempre el formato canónico `<!-- axiom:<sectionID> --> ... <!-- /axiom:<sectionID> -->`.
  - Al escanear una sección existente para reemplazo o eliminación (`content == ""`), el detector busca tanto el marcador `axiom:` como el marcador `gentle-ai:`.
  - Si un archivo contiene bloques duplicados antiguos o huérfanos con cualquiera de los dos prefijos, son consolidados y limpiados transparentemente.
- **Comportamiento en `internal/components/uninstall/cleaners.go`:**
  - La detección de preámbulos y secciones gestionadas verifica tanto prefijos `<!-- axiom:` como `<!-- gentle-ai:`.

### Decisión 4: Archivo Marcador de Instancia de Cambio SDD
- **Archivo Canónico:** `.axiom-instance`.
- **Archivo Legado:** `.gentle-ai-instance`.
- **Comportamiento en `internal/sddstatus/edit_authority_consent.go`:**
  - Lectura (`readChangeInstanceMarker`): Comprueba en primer lugar si existe `.axiom-instance`. Si no existe, comprueba si existe `.gentle-ai-instance`.
  - Escritura/Creación (`ensureChangeInstanceMarker`): Publica de forma atómica el archivo `.axiom-instance`.
  - Los scripts de prueba y verificadores se actualizan para buscar primordialmente `.axiom-instance` con tolerancia a `.gentle-ai-instance`.

---

## Diagrama de Arquitectura y Flujo

```
+--------------------------------------------------------------------------+
|                            Entornos y Agentes                            |
+--------------------------------------------------------------------------+
       |                                                    |
       v (OpenCode)                                         v (Claude Code)
+--------------------------------+       +---------------------------------+
| opencode.json                  |       | ~/.claude/commands/             |
| - agent: axiom-orchestrator    |       | - sdd-apply.md                  |
| - fallback lectura:            |       | - sdd-verify.md                 |
|   gentle-orchestrator          |       | - sdd-archive.md (sin prefijo)  |
+--------------------------------+       +---------------------------------+
       |                                                    |
       +-------------------------+--------------------------+
                                 |
                                 v
+--------------------------------------------------------------------------+
|                  Capa de Persistencia e Integración                      |
|                                                                          |
|  - filemerge/section.go:                                                 |
|      Canónico: <!-- axiom:ID --> ... <!-- /axiom:ID -->                  |
|      Legado:   <!-- gentle-ai:ID --> ... <!-- /gentle-ai:ID -->          |
|                                                                          |
|  - sddstatus/edit_authority_consent.go:                                  |
|      Canónico: .axiom-instance                                           |
|      Legado:   .gentle-ai-instance                                       |
+--------------------------------------------------------------------------+
```

---

## Matriz de Riesgos y Mitigaciones

| Riesgo | Impacto | Mitigación |
| :--- | :--- | :--- |
| **Colisión de skills en Claude Code sin prefijo** | Si Claude Code prioriza un skill sobre un comando con idéntico nombre (`sdd-apply`), el comando no despacharía. | Los skills de SDD residen en subdirectorios de Axiom y se validan en Claude Code; en caso necesario, el despacho de comando delega explícitamente en el orquestador sin ambigüedad. |
| **Pérdida de configuraciones de modelo en OpenCode** | Un desarrollador que configuró `gentle-orchestrator` podría perder su modelo asignado al renombrarse a `axiom-orchestrator`. | `configuredAssignments` en `internal/opencode/config.go` mapea automáticamente los modelos de `gentle-orchestrator` hacia `axiom-orchestrator`. |
| **Inconsistencia en repositorios con marcadores antiguos** | Archivos `CLAUDE.md`, `SOUL.md` o `AGENTS.md` con `<!-- gentle-ai:... -->` podrían duplicarse en lugar de actualizarse. | `InjectMarkdownSection` detecta y reemplaza tanto `<!-- gentle-ai:... -->` como `<!-- axiom:... -->`, asegurando que la migración actualice el bloque in-place sin duplicados. |
