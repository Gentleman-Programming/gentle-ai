# Sincronización y Setup Aislados por Workspace (Self-Hosting Axiom)

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/workspace-isolated-sync.md`.
> Espejo de recuperación en Engram: topic `odd/workspace-isolated-sync/tasks`, proyecto `axiom`.

## Objetivo

Permitir que Axiom configure y sincronice agentes de IA de forma totalmente aislada a nivel de repositorio/workspace mediante `axiom setup` y `axiom sync --scope=workspace`, permitiendo el **self-hosting** completo ("Axiom programa a Axiom") sin tocar ni sobreescribir ficheros globales de usuario (`~/.config/opencode/`, `~/.claude/`), manteniendo intactos proyectos externos como `Ludeka` bajo Gentle-AI.

## Problema

1. `axiom sync` asume implícitamente el directorio `$HOME` (`ScopeGlobal`) para todos sus pasos de inyección, backups y verificaciones de activos gestionados. No expone el flag `--scope`, por lo que ejecutar `sync` altera los agentes de todo el sistema operativo.
2. No existe un comando de primer nivel `axiom setup` enfocado a la preparación rápida de agentes en el workspace actual.
3. El banner de instalación aún conserva texto heredado ("AI Gentle Stack dry-run").
4. Se requiere comprobar in situ la instalación y sincronización en `c:\repos\axiom` y erradicar cualquier residuo a nivel de proyecto.

## Alcance Autorizado

- `internal/cli/sync.go`: Añadir soporte de `--scope global|workspace` en `SyncFlags`, `ParseSyncFlags`, `newSyncRuntime`, `syncRuntime`, `componentSyncStep` y verificaciones post-sync.
- `internal/cli/run.go`: Limpieza de branding en banners ("AI Gentle Stack" → "Axiom Stack") y asegurar que el aprovisionamiento por workspace cree la jerarquía local adecuada.
- `internal/app/app.go` y `internal/app/help.go`: Registrar el comando canónico `axiom setup` (aprovisionamiento rápido de agentes con `--scope=workspace`).
- Creación y verificación de la configuración local en `c:\repos\axiom` (`opencode.json` con `default_agent: "axiom-orchestrator"`).
- Tests unitarios y de integración para validar `sync --scope=workspace`.

## Restricciones

- **Aislamiento Absoluto de Usuario:** Ningún paso de `axiom setup` o `axiom sync --scope=workspace` debe modificar `C:\Users\lujam\.config\opencode\opencode.jsonc` ni `C:\Users\lujam\.claude\`.
- **Idioma Obligatorio:** Todo el código de soporte de usuario, documentación, planes y mensajes en español (castellano peninsular).
- **Tolerancia y Retrocompatibilidad:** El comportamiento por defecto de `axiom sync` sin argumentos sigue siendo compatible con la invocación global previa si no se indica `--scope=workspace`.

---

## Tareas

- [x] **T1 · Añadir `--scope` a `axiom sync`**
  - Añadir `Scope string` a `SyncFlags` en `internal/cli/sync.go`.
  - Registrar flag `--scope global|workspace` en `ParseSyncFlags`.
  - Propagar `scope InstallScope` a `syncRuntime`, `componentSyncStep`, `syncBackupTargets` y `runPostSyncVerification`.
  - Conectar `componentInjectionDirScoped` para inyectar en `workspaceDir` cuando `scope == ScopeWorkspace`.

- [x] **T2 · Implementar el comando `axiom setup`**
  - Registrar `setup` en `cmd/axiom/main.go`, `internal/app/app.go` y su ayuda en `internal/app/help.go`.
  - `axiom setup` ejecuta `RunInstall` con `--scope=workspace` por defecto para los agentes detectados/solicitados, creando la configuración local del proyecto.
  - Actualizar el banner de salida para mostrar "Axiom Stack" en lugar de "AI Gentle Stack".

- [x] **T3 · Saneamiento de rutas locales para OpenCode en workspace**
  - Asegurar que al instalar o sincronizar OpenCode con `--scope=workspace`, se genere o actualice la configuración bajo el workspace local con `"default_agent": "axiom-orchestrator"`.
  - Evitar fugas de routing guidance y telemetry hacia `$HOME` asegurando que `routingGuidanceOptions`, `routingGuidanceDir` y `ComponentGGA` respeten estrictamente `ScopeWorkspace`.
  - Registrar `.claude/`, `.config/`, `.opencode/` y `.mcp.json` en `.gitignore` para no contaminar el árbol Git del proyecto.

- [x] **T4 · Tests unitarios y de regresión para ScopeWorkspace**
  - Añadidas pruebas en `internal/cli/sync_test.go`: `TestParseSyncFlagsScope`, `TestSyncBackupTargetsScopedWorkspace` y `TestRunSyncWithSelectionScopedWorkspaceDoesNotTouchHomeState`.
  - Actualizado `TestRoutingGuidancePathsWorkspaceScopeReportOrchestratorPromptAgentsAtHome` en `internal/cli/run_component_paths_test.go` para validar que bajo `ScopeWorkspace` no se toquen rutas de `$HOME`.
  - Validada la suite unitaria de `internal/app` y `cmd/axiom` (todos en verde).

- [x] **T5 · Ejecución y verificación in situ en `c:\repos\axiom`**
  - Recompilado e instalado `axiom` (`go install ./cmd/axiom`).
  - Ejecutado `axiom setup --dry-run` y `axiom sync --scope=workspace` en `c:\repos\axiom`.
  - Comprobado que `~/.config/opencode/opencode.jsonc` (Ludeka) no ha sido modificado (`default_agent: "gentle-orchestrator"` intacto).
  - Comprobado que `opencode debug config` en `c:\repos\axiom` resuelve `"default_agent": "axiom-orchestrator"`.

---

## Verificación Ejecutable

1. **Aislamiento de la configuración global de usuario (Ludeka / Gentle-AI):**
   - Comando: `Get-Content "C:\Users\lujam\.config\opencode\opencode.jsonc" | Select-String -Pattern "default_agent"`
   - Resultado: `"default_agent": "gentle-orchestrator"` (intacto, sin cambios).
   - Comando: `Get-Item "C:\Users\lujam\.axiom\state.json" | Select-Object LastWriteTime`
   - Resultado: Inalterado tras `axiom sync --scope=workspace`.

2. **Self-Hosting operativo en `c:\repos\axiom`:**
   - Comando: `opencode debug config | Select-String -Pattern "default_agent"`
   - Resultado: `"default_agent": "axiom-orchestrator"`.
   - Comando: `opencode debug agent axiom-orchestrator`
   - Resultado: Agente `axiom-orchestrator` resuelto y activo con herramientas operativas.

3. **Pruebas Automatizadas:**
   - `go test ./internal/cli -run "TestSyncBackupTargetsScopedWorkspace|TestParseSyncFlagsScope|TestRunSyncWithSelectionScopedWorkspaceDoesNotTouchHomeState|TestRoutingGuidancePathsWorkspaceScopeReportOrchestratorPromptAgentsAtHome" -count=1` -> `ok (0.431s)`
   - `go test ./internal/app -count=1` -> `ok (133.205s)`
   - `go test ./cmd/axiom -count=1` -> `ok (0.843s)`
   - `axiom doctor` -> 8 passed, 0 failed.

