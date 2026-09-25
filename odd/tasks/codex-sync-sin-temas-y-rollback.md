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
- [x] **T2 · Retirada segura de temas** — Eliminar la instalación de temas visuales de presets, catálogo, install y sync; migrar temas Axiom anteriores solo con bytes exactos y ruta sin symlink, conservar preferencias/temas ajenos. Commit principal: `122769b7`; corrección de los dos goldens del selector: pendiente de commit. `go test ./internal/components/theme ./internal/components/uninstall -count=1`, CLI focalizada y `TestPresetSelectionNextScreenFlowMatrix` pasan con entorno temporal; `gofmt -l` y `git diff --check` limpios. Las pruebas de symlink se omiten por falta de privilegio Windows; junctions no verificados.
- [ ] **T3 · Rollback de Axiom por defecto** — Separar la procedencia de los respaldos en la selección inicial sin impedir la restauración explícita de respaldos Gentle AI; cubrir ordenación y selección con pruebas. Ruta: delegada (backup, aplicación o TUI y pruebas). Verificación: pruebas focalizadas de backup y TUI.
- [ ] **T4 · Documentación operativa de Axiom** — Corregir README y guías vigentes de instalación, componentes, agentes y rollback; conservar información histórica veraz y comprobar enlaces/comandos. Ruta: delegada (varios documentos). Verificación: búsqueda de referencias operativas heredadas y lectura estructural de los documentos modificados.

## Modo de trabajo y entrega

- TDD efectivo: desactivado en `openspec/config.yaml` (`testing.strict_tdd: false`). Runner declarado: `go test ./...`. Cada tarea mantiene sus comprobaciones funcionales; no se exige RED previo.
- RDD efectivo: desactivado por configuración global. No se iniciará revisión RDD.
- Una unidad de trabajo y al menos un commit convencional por tarea, con pruebas o documentación junto al comportamiento correspondiente.
- Previsión inicial: entre 450 y 650 líneas redactadas cambiadas, sin incluir generados. Estrategia de entrega `exception-ok`: el usuario ha elegido expresamente un único PR y acepta superar el umbral orientativo de unas 400 líneas. No se recortarán pruebas, documentación ni claridad por tamaño.

## Estado y evidencias

- Rama de trabajo: `codex/codex-sync-sin-temas-y-rollback`, creada desde `main` en `5410abe2`.
- Exploración: `internal/components/engram/inject.go` condiciona perfiles Codex a la validación del runtime; `internal/cli/run.go` y `internal/cli/sync.go` los verifican sin esa condición. Los temas entran por `full-gentleman`; los respaldos de Axiom y Gentle AI se mezclan por fecha.
- Pendiente: T1–T4, comprobaciones, commits y sincronización del espejo Engram. Engram rechazó una escritura por múltiples sesiones activas sin identidad de sesión disponible; no se inventará una.
- Decisión de entrega: un solo PR, con excepción de tamaño autorizada por el usuario; no se ha autorizado su creación remota ni un push.
- T1: `verificationComponentPaths` omite los perfiles solo cuando falta el ejecutable Codex; un runtime incompatible mantiene el error. `go test ./internal/cli -run 'TestVerificationComponentPathsCodexRuntimeGate|TestRunSyncCodexVerificationMatchesRuntimeProfileOutput|TestComponentSyncStepCodexRuntimeGate|TestRunInstallCodexKeepsOldRuntimeFailure' -count=1` correcto con `GOCACHE` en temporal (comprobación independiente). `go test ./internal/agents/codex ./internal/components/engram ./internal/cli`: dos primeros paquetes correctos; `internal/cli` falló con errores de permisos en Windows, expectativas heredadas `gentle-ai` y límite de 10 minutos. Es inferencia, no prueba, que todos esos fallos existieran en la base. `gofmt -l` y `git diff --check` sin incidencias.
- T2: `axiom sync` deja de instalar temas y retira activos Axiom previos únicamente con bytes exactos, si los ancestros son directorios normales; los incluye en snapshot y `ChangedFiles`. `settings.theme`, temas modificados y nombres Gentleman sin procedencia quedan intactos. `go test ./internal/components/theme ./internal/components/uninstall -count=1`, `go test ./internal/cli -run 'TestRunSyncRetiresOnlyExactManagedVisualTheme|TestRunSyncDoesNotRefreshPersistedVisualThemes|Test.*Theme|Test.*Preset' -count=1`, `gofmt -l` y `git diff --check` pasan. `TestRetireManagedVisualThemesRejectsLinkedThemeAncestor` pasa, pero ambas subpruebas hacen SKIP al no poder crear symlinks en este Windows; junctions carecen de prueba específica. Revisión independiente no encontró otros defectos deterministas; queda carrera TOCTOU entre comprobación y borrado.

## Siguiente paso

Registrar la corrección de los goldens de T2 y continuar con T3.
