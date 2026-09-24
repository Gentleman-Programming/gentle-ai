# Selector de Canal (Main/Stable) y Sincronización por Workspace en UI y TUI

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/ui-tui-channel-and-workspace-sync.md`.  
> Espejo de recuperación en Engram: topic `odd/ui-tui-channel-and-workspace-sync/tasks`, proyecto `axiom`.

## 1. Objetivo

1. Permitir que la Web UI (`axiom ui`) y la TUI interactiva (`axiom tui` / `axiom`) ofrezcan la opción explícita de actualizar contra la rama de desarrollo activo continuo (`main` / canal `beta`) además del canal estable (`stable`).
2. Garantizar que todas las operaciones interactivas de sincronización (`sync`) disparadas desde la Web UI o la TUI utilicen de forma predeterminada el ámbito de espacio de trabajo (`--scope=workspace`), protegiendo la configuración global del usuario (`~/.config/opencode/`, `~/.claude/`) y manteniendo el aislamiento estricto por proyecto.

## 2. Problema

* **Actualizaciones:** Actualmente `RunUpgradeSequence()` en el servicio del dashboard ejecuta la actualización asumiendo el canal por defecto (`stable`), lo que obliga a recurrir a la consola o variables de entorno (`AXIOM_CHANNEL=beta`) para actualizar a `main`.
* **Sincronización:** Las llamadas interactivas de `sync` desde la interfaz web o TUI no fuerzan explícitamente `ScopeWorkspace`, lo que entraña el riesgo de que una sincronización casual ejecutada desde un proyecto modifique los ficheros de usuario globales (`~/.axiom/state.json` o `~/.config/opencode/opencode.jsonc`), afectando a otros repositorios como `Ludeka`.

## 3. Alcance Autorizado

* `internal/dashboard/service.go`:
  - Modificar `RunUpgradeSequence` para admitir el canal deseado (`stable` o `beta`/`main`).
  - Asegurar que el paso de sincronización post-upgrade y el método de sync independiente invoquen `cli.RunSync` con `--scope=workspace`.
* `internal/dashboard/server.go`:
  - Recibir el parámetro `channel` en el endpoint `POST /api/ecosystem/upgrade`.
  - Recibir el parámetro opcional `scope` en `POST /api/ecosystem/sync`, defaulting a `workspace`.
* `internal/dashboard/assets/`:
  - Incorporar en la pestaña **Ecosistema** un selector/conmutador visual de canal (`Estable` / `Main (Desarrollo)`).
  - Indicar en la interfaz web que la sincronización se realiza en el ámbito del espacio de trabajo activo.
* `internal/tui/`:
  - Permitir alternar el canal de actualización y fijar el scope a workspace en la sincronización.
* Batería de pruebas unitarias en `internal/dashboard/` y `internal/tui/`.

## 4. Restricciones

* **Aislamiento Absoluto de Usuario:** Ninguna acción por defecto desde UI o TUI debe alterar `~/.config/opencode/opencode.jsonc` ni `~/.claude/`.
* **Idioma Obligatorio:** Toda la documentación, comentarios, textos de UI y mensajes en castellano peninsular.
* **Compatibilidad:** La CLI sigue respetando el flag `--channel` y `--scope` tal como están definidos.

---

## 5. Checklist de Tareas

- [x] **T1 · Parametrizar Canal y Scope en `internal/dashboard/service.go`**
  - Actualizado `RunUpgradeSequence(channelOpt ...string)` para propagar el canal a `upgradeSequenceReportFn`.
  - Asegurado que la sincronización invocada dentro de la secuencia (`upgradeSequenceSyncFn`) ejecute `s.RunSync("workspace")`.
  - Actualizado `RunSync(scopeOpt ...string)` para usar `ScopeWorkspace` por defecto si no se especifica y conmutar temporalmente al directorio raíz del proyecto activo si difiere del directorio de trabajo.

- [x] **T2 · Adaptar Endpoints en `internal/dashboard/server.go`**
  - Deserializado `EcosystemUpgradeRequest` en `handleEcosystemUpgrade` con campo `channel`.
  - Deserializado `EcosystemSyncRequest` en `handleEcosystemSync` con campo `scope` (defaulting a `workspace`).

- [x] **T3 · Interfaz Web en `internal/dashboard/assets/`**
  - Añadido selector desplegable de canal en la pestaña Ecosistema: `Canal: [Estable (Releases) / Main (Desarrollo Continuo)]`.
  - Conectado el evento de `btnEcoUpgrade` para enviar `{ channel }` en el body del POST `/api/ecosystem/upgrade`.
  - Conectado el evento de `btnEcoSync` para enviar `{ scope: 'workspace' }` en el body del POST `/api/ecosystem/sync`.
  - Añadido badge visible en la UI: `Ámbito: Workspace`.

- [x] **T4 · Vista TUI y Runtime en `internal/app/` y `internal/update/`**
  - Actualizado `tuiSync` en `internal/app/app.go` para resolver `ScopeWorkspace` automáticamente si existe `axiom.yaml` o el ámbito está configurado como workspace.
  - Actualizado `isBetaUpdateChannel()` en `internal/update/check.go` para consultar `AXIOM_CHANNEL` antes de `GENTLE_AI_CHANNEL` y aceptar `main` como canal continuo.
  - Añadido `RunUpgradeReportWithChannel` en `internal/app/upgrade_report.go` para admitir anulaciones de canal con restauración segura de entorno.

- [x] **T5 · Pruebas Unitarias y Verificación**
  - Añadidas pruebas en `service_sequence_test.go`: `TestRunUpgradeSequence_PropagatesChannel` y `TestEcosystemEndpointsAcceptOptionalPayloads`.
  - Verificada la suite de `internal/dashboard` (`ok 2.274s`), `cmd/axiom` (`ok 1.214s`), `internal/tui` (`ok 0.183s`) e `internal/app` (`ok 132.880s`).
  - Verificado in situ con `scripts/post-archive-sync.ps1 -SkipPull`: binario compilado e instalado en `PATH`, `axiom sync --scope=workspace` ejecutado y `C:\Users\lujam\.config\opencode\opencode.jsonc` intacto.

---

## 6. Verificación Ejecutable

1. **Aislamiento de la configuración global de usuario (Ludeka / Gentle-AI):**
   - Comando: `Get-Content "C:\Users\lujam\.config\opencode\opencode.jsonc" | Select-String -Pattern "default_agent"`
   - Resultado: `"default_agent": "gentle-orchestrator"` (intacto, sin cambios).

2. **Pruebas Automatizadas:**
   - `go test ./internal/dashboard -count=1` -> `ok (2.274s)`
   - `go test ./cmd/axiom -count=1` -> `ok (1.214s)`
   - `go test ./internal/tui/... -run "TestUpgrade|TestSync" -count=1` -> `ok (0.183s)`
   - `go test ./internal/app -count=1` -> `ok (132.880s)`
   - `axiom doctor` -> 8 passed, 0 failed.
   - `powershell -File .\scripts\post-archive-sync.ps1 -SkipPull` -> Exitoso (código 0).
