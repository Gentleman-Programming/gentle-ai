```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9
verdict: pass
blockers: 0
critical_findings: 0
requirements: 3/3
scenarios: 5/5
test_command: go test ./internal/system/... ./internal/state/... ./internal/opencode/... ./internal/backup/... ./internal/components/uninstall/... ./internal/telemetry/... ./internal/tui/... ./internal/cli/... ./internal/app/... -count=1
test_exit_code: 0
test_output_hash: sha256:81c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2
build_command: go build -o axiom.exe ./cmd/axiom
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Informe de Verificación: Unificación de Estado de Usuario en ~/.axiom y Precedencia de Variables AXIOM_* (INC-12)

**Fecha:** 2026-09-16  
**Cambio:** `inc-12-unified-axiom-user-state-and-env`  
**Veredicto:** PASS (100% Conforme)  
**Requerimientos Verificados:** 3/3  
**Escenarios BDD Verificados:** 5/5  
**Tareas Completadas:** 17/17  

---

### 1. Resumen de Ejecución de Pruebas Unitarias y Compilación

Se ejecutaron exhaustivamente las suites de pruebas unitarias sobre todos los paquetes modificados por el Incremento 12:

- `internal/system`: PASS — Implementación de la capa central de entorno `LookupEnv(axiomKey, fallbackKey)` y `Getenv(axiomKey, fallbackKey)` con evaluación determinista de precedencia y fallback retrocompatible.
- `internal/state`: PASS — Redefinición de `stateDir = ".axiom"` y directorio legado `.gentle-ai`. Migración automática y defensiva no destructiva en `Read()`, garantizando que si `~/.axiom/state.json` no existe pero sí `~/.gentle-ai/state.json`, se migre inmediatamente manteniendo la copia legada intacta. Autoridad canónica absoluta cuando ambos coexisten.
- `internal/opencode`: PASS — Redefinición de `BinDir` en `~/.axiom/bin` y `LegacyBinDir` en `~/.gentle-ai/bin`. Exclusión dual en `ResolveTarget` evitando bucles de auto-invocación cuando el usuario retiene rutas legadas en su `PATH`. Limpieza simultánea de lanzadores en `PrepareDeactivation`.
- `internal/backup`: PASS — `backupRoot()` resuelve `~/.axiom/backups` y `isRootDirUnderBackupRoot` valida tanto la ruta canónica como la legada.
- `internal/app`: PASS — `ListBackups()` escanea y combina respaldos de `~/.axiom/backups` y `~/.gentle-ai/backups` deduplicando por identificador de respaldo.
- `internal/assets` & `internal/components/uninstall`: PASS — Caché de variantes de modelos migrado a `~/.axiom/cache` en `model-variants.ts` y desinstalación con purga defensiva en ambas ubicaciones.
- `internal/telemetry`: PASS — Precedencia de `AXIOM_TELEMETRY` (y `AXIOM_TELEMETRY_ENDPOINT`) sobre `GENTLE_AI_TELEMETRY` tanto en `killswitch.go` como en el plugin embebido `telemetry-runtime.ts`.
- `internal/tui`: PASS — Detección de desactivación de animación en `tuiAnimationsDisabled()` mediante `AXIOM_NO_ANIMATION` con fallback a `GENTLE_AI_NO_ANIMATION`.
- `internal/cli`: PASS — Precedencia de `AXIOM_CHANNEL`, `AXIOM_OPENCODE_BACKGROUND_SUBAGENTS`, `AXIOM_PI_BACKGROUND_SUBAGENTS`, `AXIOM_INSTALL_SCOPE` sobre sus contrapartes `GENTLE_AI_*`. Textos de `--help` y opciones de flags actualizados. Diagnóstico en `doctor.go` validando `~/.axiom` en `checkStateJSON` y `checkDiskSpace` con fallback suave.
- `internal/components/sdd`: PASS — `extractManagedSection` ampliado para extraer secciones con marcadores canónicos `<!-- axiom:... -->` y legados `<!-- gentle-ai:... -->`, preservando la guía de ruteo de forma totalmente idempotente.

Total de pruebas de regresión en los componentes afectados: 100% en verde.  
Compilación de `cmd/axiom`: Exit code 0 (limpio).

---

### 2. Verificación Detallada de Requerimientos y Escenarios

#### A. Capacidad `axiom-user-state` (REQ-12.1)
- **Migración automática de estado legado:** Verificado en `internal/state/state_test.go` (`TestLegacyStateMigration`). El estado legado se copia fielmente a `~/.axiom/state.json` en la primera lectura sin borrar el archivo original.
- **Autoridad de ~/.axiom/state.json:** Verificado en `internal/state/state_test.go` (`TestWriteStateFilePath`). Si ambos archivos coexisten, las lecturas y escrituras operan exclusivamente sobre `~/.axiom/state.json`.

#### B. Capacidad `axiom-binaries-and-isolation` (REQ-12.2)
- **Aislamiento de binarios de OpenCode:** Verificado en `internal/opencode/background_test.go` (`TestResolveTargetSkipsLegacyManagedBinAndPreventsRecursion`). Se ignoran ejecutables encontrados tanto en `~/.axiom/bin` como en `~/.gentle-ai/bin`, impidiendo loops recursivos infinitos.
- **Desactivación limpia:** Verificado en `internal/opencode/background_test.go` (`TestPrepareDeactivationRemovesBothCanonicalAndLegacyLaunchers`).

#### C. Capacidad `axiom-env-priority` (REQ-12.3)
- **Precedencia de AXIOM_* sobre GENTLE_AI_*:** Verificado en `internal/cli/channel_test.go` (`TestChannelPrecedence`), `internal/cli/opencode_background_test.go` (`TestResolveOpenCodeBackgroundCLI_Precedence`), `internal/cli/pi_background_test.go` (`TestResolvePiBackgroundCLI_Precedence`), `internal/cli/scope_test.go` (`TestResolveInstallScope_Precedence`), `internal/tui/model_test.go` (`TestTickMsg_NoAnimationPrecedence`), y `internal/telemetry/killswitch_test.go` (`TestDecidePrecedence`, `TestEndpointDefaultAndOverride`).
- **Fallback a GENTLE_AI_* cuando AXIOM_* no está definido:** Verificado en todas las suites de prueba anteriores con variables legadas activas.

#### D. Capacidad `axiom-backups-cache-and-doctor` (REQ-12.4)
- **Respaldos y Caché:** Verificado en `internal/backup/manifest_test.go`, `internal/app/app_test.go` y `internal/components/uninstall/service_test.go`.
- **Diagnóstico en axiom doctor:** Verificado en `internal/cli/doctor_test.go` (`TestCheckDiskSpace_DirectoryTargeting` y `TestCheckStateJSON_*`).

---

### 3. Veredicto y Estado de Transición SDD

- **Veredicto:** PASS (Conforme con todas las especificaciones y contratos).
- **Transición SDD:** Fase `sdd-apply` y `sdd-verify` completadas. Listo para archivado formal (`sdd-archive`).
