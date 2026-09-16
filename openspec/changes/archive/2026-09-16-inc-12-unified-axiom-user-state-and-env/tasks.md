# Tareas de Implementación: Unificación de Estado en ~/.axiom y Variables AXIOM_* (INC-12)

## Fase 1: Capa de Entorno de Axiom (`internal/system/`)

- [x] T-01 Crear `internal/system/env.go` con `LookupEnv(axiomKey, fallbackKey string) (string, bool)` y `Getenv(axiomKey, fallbackKey string) string`, junto con sus pruebas unitarias en `internal/system/env_test.go`.

## Fase 2: Unificación de Estado y Migración Automática (`internal/state/`)

- [x] T-02 Actualizar `internal/state/state.go` redefiniendo `stateDir = ".axiom"`, agregando `legacyStateDir = ".gentle-ai"`, `LegacyPath(homeDir)`, y la lógica de migración automática transparente en `Read(homeDir)`.
- [x] T-03 Actualizar y ampliar pruebas en `internal/state/state_test.go` verificando la ruta `~/.axiom/state.json`, la migración defensiva desde `~/.gentle-ai/state.json` y la autoridad de `~/.axiom/` cuando ambos coexisten.

## Fase 3: Binarios y Lanzadores de OpenCode (`internal/opencode/`)

- [x] T-04 Actualizar `internal/opencode/background.go` definiendo `BinDir(homeDir)` en `~/.axiom/bin`, `LegacyBinDir(homeDir)` en `~/.gentle-ai/bin`, y excluyendo ambas rutas en `ResolveTarget` para prevenir bucles.
- [x] T-05 Adaptar `PrepareDeactivation` en `internal/opencode/background.go` para remover lanzadores gestionados en `~/.axiom/bin` y `~/.gentle-ai/bin`. Actualizar pruebas en `internal/opencode/background_test.go`.

## Fase 4: Respaldos y Caché (`internal/backup/`, `internal/app/`, `internal/assets/`, `internal/components/uninstall/`)

- [x] T-06 Actualizar `internal/backup/manifest.go` para usar `~/.axiom/backups` como raíz canónica y validar en `isRootDirUnderBackupRoot` tanto la ruta canónica como la ruta legada.
- [x] T-07 Actualizar `ListBackups()` en `internal/app/app.go` para escanear `~/.axiom/backups` y agregar respaldos preexistentes en `~/.gentle-ai/backups`.
- [x] T-08 Actualizar `internal/assets/opencode/plugins/model-variants.ts` para usar `~/.axiom/cache`, y adaptar `internal/components/uninstall/service.go` para limpiar tanto `~/.axiom/cache` como `~/.gentle-ai/cache`.

## Fase 5: Variables de Entorno en CLI y Runtime (`internal/cli/`, `internal/tui/`, `internal/telemetry/`)

- [x] T-09 Actualizar `internal/cli/channel.go` para priorizar `AXIOM_CHANNEL` con fallback a `GENTLE_AI_CHANNEL`.
- [x] T-10 Actualizar `internal/cli/opencode_background.go` para priorizar `AXIOM_OPENCODE_BACKGROUND_SUBAGENTS` con fallback a `GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS`.
- [x] T-11 Actualizar `internal/tui/model.go` para priorizar `AXIOM_NO_ANIMATION` con fallback a `GENTLE_AI_NO_ANIMATION`.
- [x] T-12 Actualizar `internal/telemetry/killswitch.go` y `internal/assets/opencode/plugins/telemetry-runtime.ts` para priorizar `AXIOM_TELEMETRY` con fallback a `GENTLE_AI_TELEMETRY`.
- [x] T-13 Actualizar flags y textos de ayuda en `internal/cli/install.go` reflejando las variables `AXIOM_*` y soportando `AXIOM_PI_BACKGROUND_SUBAGENTS` y `AXIOM_INSTALL_SCOPE`.
- [x] T-14 Actualizar `internal/cli/doctor.go` para diagnosticar `~/.axiom` en `checkStateJSON` y `checkDiskSpace`.

## Fase 6: Pruebas, Verificación y Estado SDD

- [x] T-15 Actualizar pruebas unitarias en `internal/cli/`, `internal/app/`, `internal/tui/`, `internal/telemetry/` y tests de doctor para validar las nuevas rutas y precedencia de variables.
- [x] T-16 Ejecutar pruebas unitarias de los paquetes modificados (`go test`) asegurando paso limpio.
- [x] T-17 Comprobar el estado SDD con `gentle-ai sdd-status inc-12-unified-axiom-user-state-and-env`.
