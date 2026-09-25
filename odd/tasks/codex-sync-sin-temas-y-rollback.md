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
- [ ] **T5 · Referencias operativas restantes en documentación** — Presentar como Axiom y actualizar comandos en los documentos vigentes: `docs/architecture.md`, `docs/kiro.md`, `docs/opencode-profiles.md`, `docs/intended-usage.md`, `docs/non-interactive.md`, `docs/pi.md`, `docs/platforms.md`, `docs/release-signing.md`, `docs/trigger-rules.md`, `docs/review-integration.md`, `docs/architecture/organic-rdd.md`, `docs/telemetry.md`, `docs/skill-registry.md`, `docs/skill-style-guide.md` y `docs/codebase/*`. Conservar referencias literales en `docs/archive/ROADMAP.md`, `docs/audits/*`, `docs/upstream-absorption-ledger.md` y `docs/prd-opencode-profiles.md`; no cambiar el wrapper `cmd/gentle-ai` que aún usan las pruebas Docker ni renombrar esquemas `gentle-ai.*`, imports, rutas `.gentle-ai`, variables `GENTLE_AI_*` o nombres externos como `gentle-pi`. Catalogar expresamente las excepciones justificadas para análisis.
- [x] **T6 · Ámbito workspace por defecto en axiom sync y salvaguarda de usuario** — Configurar `ScopeWorkspace` como ámbito predeterminado en `ResolveInstallScope` (`ScopeGlobal` solo mediante flag `--scope global` o variable `AXIOM_INSTALL_SCOPE=global`). Garantizar que componentes que no pueden residir en workspace (`ComponentGGA`, `ComponentPermission`) y adaptadores sin soporte de workspace (`vscode`, `trae`, `windsurf` user settings) se instalen/sincronicen en el usuario (`homeDir`), evitando la creación de directorios espurios como `AppData/` dentro del repositorio del proyecto.
- [ ] **T7 · Reemplazo de comandos gentle-ai por axiom en hooks y agentes** — Actualizar los comandos inyectados en hooks de Codex y Claude (`axiom skill-registry refresh`, `axiom telemetry runtime`, `axiom sdd-preflight-hook`, `axiom review stop-hook`), actualizar el desinstalador para reconocer ambos comandos, cambiar `gentle-ai review` por `axiom review` en la guía de enrutamiento común (`routing.go`) e instrucciones de actualización, y regenerar `.codex/hooks.json` y `.codex/AGENTS.md`.

## Modo de trabajo y entrega

- TDD efectivo: desactivado en `openspec/config.yaml` (`testing.strict_tdd: false`). Runner declarado: `go test ./...`. Cada tarea mantiene sus comprobaciones funcionales; no se exige RED previo.
- RDD efectivo: desactivado por configuración global. No se iniciará revisión RDD.
- Una unidad de trabajo y al menos un commit convencional por tarea, con pruebas o documentación junto al comportamiento correspondiente.
- Previsión inicial: entre 450 y 650 líneas redactadas cambiadas, sin incluir generados; la ampliación de documentación pendiente hará que el total supere esa previsión. Estrategia de entrega `exception-ok`: el usuario ha elegido expresamente un único PR y acepta superar el umbral orientativo de unas 400 líneas. No se recortarán pruebas, documentación ni claridad por tamaño.

## Estado y evidencias

- Rama de trabajo: `codex/codex-sync-sin-temas-y-rollback`, creada desde `main` en `5410abe2`.
- Exploración: `internal/components/engram/inject.go` condiciona perfiles Codex a la validación del runtime; `internal/cli/run.go` y `internal/cli/sync.go` los verifican sin esa condición. Los temas entran por `full-gentleman`; los respaldos de Axiom y Gentle AI se mezclan por fecha.
- Pendiente: T5, T7, cierre de comprobaciones y sincronización del espejo Engram.
- Decisión de entrega: un solo PR, con excepción de tamaño autorizada por el usuario; no se ha autorizado su creación remota ni un push.
- T1: `verificationComponentPaths` omite los perfiles solo cuando falta el ejecutable Codex; un runtime incompatible mantiene el error. `go test ./internal/cli -run 'TestVerificationComponentPathsCodexRuntimeGate|TestRunSyncCodexVerificationMatchesRuntimeProfileOutput|TestComponentSyncStepCodexRuntimeGate|TestRunInstallCodexKeepsOldRuntimeFailure' -count=1` correcto con `GOCACHE` en temporal (comprobación independiente).
- T2: `axiom sync` deja de instalar temas y retira activos Axiom previos únicamente con bytes exactos; `settings.theme`, temas modificados y nombres Gentleman sin procedencia quedan intactos.
- T3: Rollback TUI distingue procedencia por raíz y preselecciona Axiom más reciente.
- T4: README y guías principales presentan Axiom como producto vigente.
- T6: `ResolveInstallScope` devuelve `ScopeWorkspace` por defecto cuando no se pasa `--scope` ni variable de entorno; `--scope global` preservado. `componentInjectionDirScoped` redirige adaptadores de escritorio sin soporte de workspace (`vscode-copilot`, `trae-ide`, `windsurf`) al directorio del usuario (`homeDir`), impidiendo la creación de carpetas como `AppData/Roaming/Code/User` en el workspace. `ComponentGGA` ya no se omite en `ScopeWorkspace`, sincronizándose sobre `homeDir`. Carpeta espuria `AppData/` eliminada del workspace. Pruebas `TestResolveInstallScope`, `TestResolveAgentConfigDir`, `TestParseSyncFlagsScope` y `TestComponentInjectionDirScopedWorkspaceSafeguard` pasando.

## Siguiente paso

Iniciar T7: reemplazar comandos inyectados de `gentle-ai` por `axiom` en hooks de Codex y Claude (`internal/components/sdd/inject.go`), guía de enrutamiento común (`agentguidance/routing.go`), desinstalador y mensajes de actualización.
