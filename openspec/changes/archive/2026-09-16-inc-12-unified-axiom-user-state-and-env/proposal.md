# Propuesta: Unificación del Estado Global bajo ~/.axiom y Soporte de Variables de Entorno AXIOM_* (INC-12)

## Propósito (Intent)

Actualmente, el sistema de persistencia en disco del usuario mantiene una discrepancia evidente:
1. Por un lado, Axiom almacena su catálogo central de proyectos en `~/.axiom/workspaces.json` (implementado en INC-08).
2. Por otro lado, todo el subsistema de instalación, sincronización, snapshots de respaldo, launchers de OpenCode (`bin/`) y caché de modelos sigue residiendo en la carpeta `~/.gentle-ai/`:
   - `~/.gentle-ai/state.json`
   - `~/.gentle-ai/bin/opencode` (y `opencode.cmd`)
   - `~/.gentle-ai/backups/`
   - `~/.gentle-ai/cache/`
3. Las variables de entorno de control de comportamiento siguen exigiendo el prefijo `GENTLE_AI_*` (`GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS`, `GENTLE_AI_TELEMETRY`, `GENTLE_AI_CHANNEL`, `GENTLE_AI_NO_ANIMATION`).

El **Incremento 12 (INC-12: `unified-axiom-user-state-and-env`)** consolida todo el estado de usuario bajo un único directorio canónico **`~/.axiom/`**, introduce un mecanismo de migración automática transparente para usuarios con datos en `~/.gentle-ai/`, y habilita el uso prioritario de variables de entorno con prefijo **`AXIOM_*`**.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Unificación del Directorio Base en `~/.axiom/`:**
   - Redirigir las rutas internas de gestión de estado a `~/.axiom/`:
     - Archivo de estado: `~/.axiom/state.json`
     - Binarios y launchers: `~/.axiom/bin/` (`opencode.cmd`, `opencode.ps1`, `opencode`)
     - Respaldos de configuración: `~/.axiom/backups/`
     - Caché de modelos y variantes: `~/.axiom/cache/`
   - Modificar las referencias en `internal/state/`, `internal/opencode/background.go`, `internal/update/upgrade/executor.go` y utilidades de paths.

2. **Migración Automática y Transparente:**
   - Al iniciar cualquier comando o proceso de Axiom:
     - Si `~/.axiom/state.json` no existe pero `~/.gentle-ai/state.json` sí existe, copiar o migrar de forma segura los archivos a `~/.axiom/`.
     - Preservar `~/.gentle-ai/` como respaldo no destructivo.
     - Si ambos existen, `~/.axiom/` es siempre la fuente de la verdad autoritativa.

3. **Variables de Entorno con Prefijo `AXIOM_*`:**
   - Introducir lectores de variables de entorno con prioridad `AXIOM_*` y fallback a `GENTLE_AI_*`:
     - `AXIOM_OPENCODE_BACKGROUND_SUBAGENTS` (fallback: `GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS`)
     - `AXIOM_CHANNEL` (fallback: `GENTLE_AI_CHANNEL`)
     - `AXIOM_NO_ANIMATION` (fallback: `GENTLE_AI_NO_ANIMATION`)
     - `AXIOM_TELEMETRY` (fallback: `GENTLE_AI_TELEMETRY`)
   - Documentar las nuevas variables en la ayuda y guías de uso.

### Fuera de Alcance (Out of Scope)

- Modificación de la estructura de specs en `openspec/`.
- Modificación de los comandos de la CLI `axiom init` o `axiom project`.

---

## Plan de Pruebas y Validación

- Pruebas unitarias en `internal/state/` verificando la resolución de rutas en `~/.axiom/`.
- Pruebas de migración automática desde un directorio `~/.gentle-ai/` simulado.
- Pruebas de lectura de variables de entorno demostrando que `AXIOM_*` tiene precedencia sobre `GENTLE_AI_*`.
