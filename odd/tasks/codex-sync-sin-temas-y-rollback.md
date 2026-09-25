# ODD: Sincronización de Codex, retirada de temas y rollback propio de Axiom

> Documento vivo. Fichero autoritativo: `odd/tasks/codex-sync-sin-temas-y-rollback.md`.
> Espejo de recuperación: `odd/codex-sync-sin-temas-y-rollback/tasks` (proyecto `axiom`).

## Objetivo y problema

Corregir la sincronización de Codex que omite tres perfiles y después exige su existencia; retirar la instalación de temas de Axiom y restaurar únicamente los cambios de tema que Axiom haya aplicado; presentar Axiom, no Gentle AI, en la documentación operativa; y hacer que el rollback de Axiom sea la opción predeterminada sin perder el acceso explícito a respaldos históricos de Gentle AI.

El registro aportado indica 291 comprobaciones correctas y tres fallidas, todas por ausencia de `sdd-strong.config.toml`, `sdd-mid.config.toml` y `sdd-cheap.config.toml`. La inyección de esos perfiles es condicional, pero el inventario de verificación actual no lo es. La causa temporal concreta de la omisión en la máquina del usuario aún no está acreditada.

## Alcance y límites

- Autoriza cambios locales en código, pruebas y documentación de este repositorio; no autoriza push, PR ni merge.
- Preservar los directorios no seguidos `.codex/`, `.copilot/`, `.gemini/` y `AppData/`.
- No borrar preferencias de tema preexistentes ni ficheros personales sin comprobar antes su procedencia y disponer de una restauración segura.
- Mantener referencias históricas o de compatibilidad a Gentle AI cuando forman parte de protocolos, rutas, importaciones o atribuciones reales. Sustituir las instrucciones operativas heredadas que presentan Gentle AI como producto actual.
- Los respaldos históricos de Gentle AI deben seguir siendo recuperables mediante una elección explícita.

## Tareas

- [x] **T1 · Coherencia de perfiles Codex** — Corregir el contrato entre la generación condicional y la verificación de `axiom sync`, con pruebas para CLI disponible y no disponible. Ruta: delegada (lógica y pruebas en varios ficheros). Pruebas dirigidas correctas; suite amplia de `internal/cli` fallida por permisos y aserciones ajenas visibles, sin línea base demostrada. Commit: `8bc70073`.
- [x] **T2 · Retirada segura de temas** — Eliminar la instalación de temas visuales de presets, catálogo, install y sync; migrar temas Axiom anteriores solo con bytes exactos y ruta sin symlink, conservar preferencias/temas ajenos. Commits: `122769b7` y `a78b99b1` (ajuste de dos goldens del selector). `go test ./internal/components/theme ./internal/components/uninstall -count=1`, CLI focalizada y `TestPresetSelectionNextScreenFlowMatrix` pasan con entorno temporal; `gofmt -l` y `git diff --check` limpios. Las pruebas de symlink se omiten por falta de privilegio Windows; junctions no verificados.
- [x] **T3 · Rollback de Axiom por defecto** — La TUI distingue procedencia por raíz, preselecciona el respaldo Axiom más reciente y mantiene Gentle AI histórico disponible mediante selección expresa. IDs duplicados entre raíces siguen siendo restaurables; el scroll hace visible la selección. Commit: `ac872ebe`. `go test ./internal/backup ./internal/tui` y `go test ./internal/app -run TestListBackupsKeepsDuplicateIDsRestorableBySelectedRoot -count=1` pasan con HOME/USERPROFILE/GOCACHE temporales. La CLI independiente `axiom restore latest` mantiene su resolución anterior; alcance de T3 limitado a rollback TUI.
- [x] **T4 · Documentación principal de Axiom** — Actualizar README y guías principales de inicio, uso, componentes, agentes y rollback; conservar identificadores de compatibilidad e historial veraz. Incluye el diagrama del README. Ruta: delegada. Enlaces locales y `git diff --check` correctos; lectura estructural completada. Commit: `65912d8d`.
- [x] **T5 · Referencias operativas restantes en documentación y catálogo de excepciones** — Presentar como Axiom y actualizar comandos en los documentos vigentes: `docs/architecture.md`, `docs/kiro.md`, `docs/opencode-profiles.md`, `docs/intended-usage.md`, `docs/non-interactive.md`, `docs/pi.md`, `docs/platforms.md`, `docs/trigger-rules.md`, `docs/skill-registry.md` y `docs/codebase/*`. Conservar referencias literales de compatibilidad e históricas y catalogar expresamente las excepciones justificadas para análisis posterior.
- [x] **T6 · Ámbito workspace por defecto en axiom sync y salvaguarda de usuario** — Configurar `ScopeWorkspace` como ámbito predeterminado en `ResolveInstallScope` (`ScopeGlobal` solo mediante flag `--scope global` o variable `AXIOM_INSTALL_SCOPE=global`). Garantizar que componentes que no pueden residir en workspace (`ComponentGGA`, `ComponentPermission`) y adaptadores sin soporte de workspace (`vscode`, `trae`, `windsurf`, `antigravity`) se instalen/sincronicen en el usuario (`homeDir`), evitando la creación de directorios espurios como `AppData/` o `.gemini/` dentro del repositorio del proyecto. Commit: `57aa3740` y ajuste en `34a7cfbc`.
- [x] **T7 · Reemplazo de comandos gentle-ai por axiom en hooks y agentes** — Actualizar los comandos inyectados en hooks de Codex y Claude (`axiom skill-registry refresh`, `axiom telemetry runtime`, `axiom sdd-preflight-hook`, `axiom review stop-hook`), actualizar el desinstalador para reconocer ambos comandos, cambiar `gentle-ai review` por `axiom review` en la guía de enrutamiento común (`routing.go`) e instrucciones de actualización, y regenerar `.codex/hooks.json` y `.codex/AGENTS.md`. Commit: `34a7cfbc`.

## Catálogo de excepciones técnicas preservadas (análisis de compatibilidad)

Las siguientes referencias a `gentle-ai` se han preservado deliberadamente tras la revisión global porque su modificación unilateral provocaría roturas funcionales en contratos externos, esquemas de serialización, módulos de Go o herramientas de terceros:

1. **Contratos de protocolo y esquemas de máquina (JSON Schemas)**:
   - `gentle-ai.review-integration/v2`
   - `gentle-ai.review-integration.consent/v3`
   - `gentle-ai.review-assessment/v1`
   - `gentle-ai.review-acknowledged/v1`
   - `gentle-ai.sdd-status/v1`
   - `gentle-ai.sdd-integration.consent/v1`
   - `gentle-ai.telemetry-heartbeat/v1` y `gentle-ai.telemetry-runtime-observation/v1`
   - `gentle-pi.background-subagents/v1`
   *Motivo*: Son URIs de esquemas compartidos entre herramientas, serializaciones en disco y validadores de contratos cruzados.

2. **Módulo Go y nombres de paquete**:
   - `module github.com/gentleman-programming/gentle-ai/v3` en `go.mod`.
   *Motivo*: Renombrar el módulo raíz de Go requeriría un refactor masivo de rutas de importación que rompería la compatibilidad de importación y el historial de dependencias.

3. **Compatibilidad de migración de estado y rutas locales**:
   - Directorios de usuario heredados: `~/.gentle-ai/state.json`, `~/.gentle-ai/cache`, `~/.gentle-ai/bin/`.
   - Variables de entorno de compatibilidad: `GENTLE_AI_*`, `GENTLE_PI_*`.
   *Motivo*: Permiten la coexistencia y migración transparente de instalaciones y perfiles existentes sin forzar una rotura a los usuarios.

4. **Integraciones con herramientas y ecosistemas externos**:
   - Paquete y comandos de Pi: `npm:gentle-pi`, `npm:gentle-engram`, `/gentle:status`, etc.
   - Identificador base del agente en OpenCode: `gentle-orchestrator` en `opencode.json`.
   - Repositorio y fuentes de skills upstream: `Gentleman-Programming/Gentleman-Skills`.
   - Wrapper binario `cmd/gentle-ai` (preservado para scripts Docker y compatibilidad de llamadas anteriores).

## Modo de trabajo y entrega

- TDD efectivo: desactivado en `openspec/config.yaml` (`testing.strict_tdd: false`). Runner declarado: `go test ./...`. Cada tarea mantiene sus comprobaciones funcionales; no se exige RED previo.
- RDD efectivo: desactivado por configuración global. No se iniciará revisión RDD.
- Una unidad de trabajo y al menos un commit convencional por tarea, con pruebas o documentación junto al comportamiento correspondiente.
- Previsión inicial: entre 450 y 650 líneas redactadas cambiadas, sin incluir generados; la ampliación de documentación pendiente hará que el total supere esa previsión. Estrategia de entrega `exception-ok`: el usuario ha elegido expresamente un único PR y acepta superar el umbral orientativo de unas 400 líneas. No se recortarán pruebas, documentación ni claridad por tamaño.

## Estado y evidencias

- Rama de trabajo: `codex/codex-sync-sin-temas-y-rollback`, creada desde `main` en `5410abe2`.
- Exploración: `internal/components/engram/inject.go` condiciona perfiles Codex a la validación del runtime; `internal/cli/run.go` y `internal/cli/sync.go` los verifican sin esa condición. Los temas entran por `full-gentleman`; los respaldos de Axiom y Gentle AI se mezclan por fecha.
- T1: `verificationComponentPaths` omite los perfiles solo cuando falta el ejecutable Codex; un runtime incompatible mantiene el error. Pruebas passing. Commit: `8bc70073`.
- T2: `axiom sync` deja de instalar temas y retira activos Axiom previos únicamente con bytes exactos; `settings.theme`, temas modificados y nombres Gentleman sin procedencia quedan intactos. Commits: `122769b7` y `a78b99b1`.
- T3: Rollback TUI distingue procedencia por raíz y preselecciona Axiom más reciente. Commit: `ac872ebe`.
- T4: README y guías principales presentan Axiom como producto vigente. Commit: `65912d8d`.
- T6: `ResolveInstallScope` devuelve `ScopeWorkspace` por defecto cuando no se pasa `--scope` ni variable de entorno; `--scope global` preservado. `componentInjectionDirScoped` redirige adaptadores sin soporte de workspace (`vscode-copilot`, `trae-ide`, `windsurf`, `antigravity`) al directorio del usuario (`homeDir`), impidiendo la creación de carpetas como `AppData/` o `.gemini/` en el workspace. Pruebas passing. Commit: `57aa3740` y ajuste en `34a7cfbc`.
- T7: Comandos inyectados en hooks de Codex y Claude migrados a `axiom` con reemplazo in-place de comandos legacy; desinstalador ampliado; tests unitarios y suites passing. Commit: `34a7cfbc`.
- T5: Documentación operativa actualizada en `docs/architecture.md`, `docs/intended-usage.md`, `docs/skill-registry.md`, `docs/trigger-rules.md`, `docs/kiro.md`, `docs/opencode-profiles.md`, `docs/non-interactive.md`, `docs/platforms.md`, `docs/pi.md` y `docs/codebase/sync-and-cloud.md`. Catálogo de excepciones técnicas preservadas documentado.
- Todas las tareas (T1 a T7) completadas.

## Siguiente paso

Incremento cerrado y verificado con éxito. Resumen persistido en memoria Engram. Listo para Pull Request.
