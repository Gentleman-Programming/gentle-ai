# Reporte de Archivado: Unificación de Estado en ~/.axiom y Variables AXIOM_* (INC-12)

**Fecha:** 2026-09-16  
**Incremento:** `inc-12-unified-axiom-user-state-and-env`  
**Estado:** ARCHIVED  

---

## Resumen del Incremento

El **Incremento 12 (INC-12: `inc-12-unified-axiom-user-state-and-env`)** consolida la persistencia del usuario y la configuración de runtime de Axiom bajo su propia identidad y sistema de variables:

1. **Estado de Usuario Centralizado en `~/.axiom/`:**
   - Se redefinió `stateDir = ".axiom"` en `internal/state/state.go`.
   - Se implementó migración automática, defensiva y no destructiva en `state.Read()`: si `~/.axiom/state.json` no existe pero sí `~/.gentle-ai/state.json`, se migra automáticamente preservando el archivo original.
   - Autoridad canónica absoluta: una vez existe `~/.axiom/state.json`, todas las lecturas y escrituras operan de forma exclusiva sobre él.

2. **Binarios Gestionados y Aislamiento de Rutas (`internal/opencode`):**
   - Se redefinió `BinDir(homeDir)` apuntando a `~/.axiom/bin`.
   - Se implementó exclusión dual en `ResolveTarget` tanto para `~/.axiom/bin` como para `~/.gentle-ai/bin`, evitando bucles de auto-invocación cuando el usuario retiene rutas legadas en su `PATH`.
   - Limpieza simultánea de lanzadores gestionados en ambas rutas durante `PrepareDeactivation`.

3. **Respaldos y Caché (`internal/backup`, `internal/app`, `internal/assets`):**
   - `backupRoot()` resuelve `~/.axiom/backups/`. `isRootDirUnderBackupRoot` valida tanto la ruta canónica como la legada `~/.gentle-ai/backups`.
   - `ListBackups()` en `internal/app/app.go` escanea primordialmente `~/.axiom/backups` e incorpora respaldos preexistentes en `~/.gentle-ai/backups` con deduplicación por ID.
   - En el plugin `model-variants.ts`, la variable `cacheDir` resuelve a `~/.axiom/cache`. El servicio de desinstalación purga ambas carpetas de caché de forma segura.

4. **Capa Centralizada de Variables de Entorno (`internal/system/env.go`):**
   - Módulo `internal/system` con `LookupEnv` y `Getenv` para evaluar ordenadamente la precedencia `AXIOM_*` con fallback transparente a `GENTLE_AI_*`.
   - Variables migradas:
     - `AXIOM_CHANNEL` (fallback: `GENTLE_AI_CHANNEL`)
     - `AXIOM_OPENCODE_BACKGROUND_SUBAGENTS` (fallback: `GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS`)
     - `AXIOM_PI_BACKGROUND_SUBAGENTS` (fallback: `GENTLE_AI_PI_BACKGROUND_SUBAGENTS`)
     - `AXIOM_INSTALL_SCOPE` (fallback: `GENTLE_AI_INSTALL_SCOPE`)
     - `AXIOM_NO_ANIMATION` (fallback: `GENTLE_AI_NO_ANIMATION`)
     - `AXIOM_TELEMETRY` y `AXIOM_TELEMETRY_ENDPOINT` (fallback: `GENTLE_AI_TELEMETRY` y endpoint asociado)
   - Actualización de `--help` y textos de ayuda en `install.go` y `sync.go`.

5. **Diagnóstico en `axiom doctor` (`internal/cli/doctor.go`):**
   - `checkStateJSON` diagnostica `~/.axiom/state.json`.
   - `checkDiskSpace` mide el espacio libre sobre `~/.axiom/` (con fallback dinámico a `~/.gentle-ai/` o `homeDir` si aún no existe).

6. **Tolerancia y Preservación de Marcadores en SDD (`internal/components/sdd/inject.go`):**
   - `extractManagedSection` ampliado para extraer secciones tanto con prefijo canónico `<!-- axiom:... -->` como legado `<!-- gentle-ai:... -->`, garantizando idempotencia total en syncs repetidos de OpenCode.

---

## Artefactos Consolidados y Modificados

- **Entorno y Estado:**
  - `internal/system/env.go` (nuevo)
  - `internal/system/env_test.go` (nuevo)
  - `internal/state/state.go`
  - `internal/state/state_test.go`
- **OpenCode Background & Binarios:**
  - `internal/opencode/background.go`
  - `internal/opencode/background_test.go`
- **Respaldos, Caché y App:**
  - `internal/backup/manifest.go`
  - `internal/app/app.go`
  - `internal/app/app_test.go`
  - `internal/assets/opencode/plugins/model-variants.ts`
  - `internal/components/uninstall/service.go`
  - `internal/components/uninstall/service_test.go`
- **CLI, Flags y Doctor:**
  - `internal/cli/channel.go`
  - `internal/cli/channel_test.go`
  - `internal/cli/opencode_background.go`
  - `internal/cli/opencode_background_test.go`
  - `internal/cli/pi_background.go`
  - `internal/cli/pi_background_test.go`
  - `internal/cli/scope.go`
  - `internal/cli/scope_test.go`
  - `internal/cli/install.go`
  - `internal/cli/sync.go`
  - `internal/cli/sync_test.go`
  - `internal/cli/doctor.go`
  - `internal/cli/doctor_test.go`
  - `internal/cli/run_component_paths_test.go`
- **TUI & Telemetría:**
  - `internal/tui/model.go`
  - `internal/tui/model_test.go`
  - `internal/telemetry/telemetry.go`
  - `internal/telemetry/killswitch.go`
  - `internal/telemetry/killswitch_test.go`
  - `internal/assets/opencode/plugins/telemetry-runtime.ts`
- **Inyección de Componentes SDD:**
  - `internal/components/sdd/inject.go`
