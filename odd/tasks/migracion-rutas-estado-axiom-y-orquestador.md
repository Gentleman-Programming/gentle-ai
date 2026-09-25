# ODD: Migración de Rutas de Estado a ~/.axiom/, Variables AXIOM_* y Orquestador Canónico

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/migracion-rutas-estado-axiom-y-orquestador.md`.
> Espejo de recuperación en Engram: topic `odd/migracion-rutas-estado-axiom-y-orquestador/tasks`, proyecto `axiom`.

## Objetivo

Desacoplar el almacenamiento de configuración local y el lanzador de agentes de `~/.gentle-ai/` estableciendo `~/.axiom/` como directorio canónico (`~/.axiom/state.json`, `~/.axiom/cache/`, `~/.axiom/bin/`), implementar una estrategia de migración automática no destructiva desde instalaciones previas de Gentle AI, formalizar las variables de entorno con prefijo `AXIOM_*` y consolidar el subagente base de OpenCode como `axiom-orchestrator` con migración in-place de perfiles existentes.

## Problema

1. Axiom aún almacena el estado global de agentes, configuraciones y presets en `~/.gentle-ai/state.json`, así como la caché de modelos en `~/.gentle-ai/cache/` y binarios administrados en `~/.gentle-ai/bin/`.
2. Las variables de entorno de configuración (`OPENCODE_BACKGROUND_SUBAGENTS`, `CHANNEL`, etc.) utilizan principalmente el prefijo `GENTLE_AI_*`, salvo `AXIOM_INSTALL_SCOPE` que ya cuenta con resolución dual.
3. En OpenCode, el conductor base SDD sigue identificándose como `gentle-orchestrator` en la generación por defecto y en la tabla de orquestadores, perpetuando el branding heredado en la interfaz y en `opencode.json`.
4. Cambiar estas rutas y claves a la fuerza sin una capa de compatibilidad causaría una pérdida aparente de configuración para usuarios que ya poseen `~/.gentle-ai/` o configuraciones de OpenCode activas.

## Alcance Autorizado

- `internal/system/`: Definir rutas canónicas de Axiom (`~/.axiom`) para estado, caché y lanzadores binarios.
- `internal/cli/state.go` y ciclo de arranque: Implementar el bootstrap con auto-migración no destructiva (si no existe `~/.axiom/state.json` y existe `~/.gentle-ai/state.json`, copiar y promover a Axiom).
- `internal/system/env.go` / resolución de variables: Incorporar prefijos `AXIOM_*` (`AXIOM_CHANNEL`, `AXIOM_OPENCODE_BACKGROUND_SUBAGENTS`, etc.) con fallback de compatibilidad a `GENTLE_AI_*`.
- `internal/components/sdd/profiles.go` y adaptadores de OpenCode: Generar `axiom-orchestrator` como conductor por defecto y añadir regla de migración in-place de `gentle-orchestrator` → `axiom-orchestrator`.
- `cmd/gentle-ai/main.go`: Convertir el ejecutable en thin wrapper que emita deprecation warning por stderr y derive a Axiom.
- Pruebas unitarias de migración de estado, resolución de variables y actualización de `opencode.json`.

## Restricciones

- **Migración sin pérdida de datos:** Una instalación preexistente con `~/.gentle-ai/state.json` debe ser reconocida inmediatamente y migrada de forma transparente.
- **Inmutabilidad de estado externo:** No borrar `~/.gentle-ai/state.json` original para permitir rollback o coexistencia durante la transición.
- **Idioma Obligatorio:** Español (castellano peninsular con tuteo profesional) en toda la documentación, comentarios de cambio y mensajes de usuario.

---

## Tareas

- [ ] **T1 · Infraestructura de rutas canónicas `~/.axiom/` y bootstrap de migración**
  - Actualizar helpers de rutas de usuario para resolver `~/.axiom/state.json`, `~/.axiom/cache/` y `~/.axiom/bin/`.
  - Implementar rutina de bootstrap: al cargar el estado, si `~/.axiom/state.json` no existe pero `~/.gentle-ai/state.json` sí, copiar el fichero a la nueva ruta y continuar operando sobre `~/.axiom`.
  - Pruebas unitarias que verifiquen el arranque limpio, arranque con migración y precedencia de `~/.axiom`.

- [ ] **T2 · Unificación de variables de entorno con prefijo `AXIOM_*`**
  - Extender el helper `system.Getenv` para resolver `AXIOM_CHANNEL` (fallback `GENTLE_AI_CHANNEL`), `AXIOM_OPENCODE_BACKGROUND_SUBAGENTS` (fallback `GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS`), y `AXIOM_STATE_DIR` (fallback `GENTLE_AI_STATE_DIR`).
  - Actualizar referencias internas y pruebas unitarias.

- [ ] **T3 · Lanzadores administrados de OpenCode en `~/.axiom/bin/`**
  - Migrar la generación de scripts envoltorios (`opencode`, `opencode.cmd`, `opencode.ps1`) para que residan bajo `~/.axiom/bin/`.
  - Asegurar que la limpieza/desinstalación retire tanto lanzadores antiguos en `~/.gentle-ai/bin/` como los nuevos en `~/.axiom/bin/`.
  - Tests unitarios de activación y desactivación de lanzadores.

- [ ] **T4 · Migración de conductor OpenCode a `axiom-orchestrator`**
  - Establecer `axiom-orchestrator` como clave canónica del agente base en `internal/catalog/` y generación de perfiles SDD.
  - Implementar migración in-place en `internal/components/sdd/profiles.go`: si `opencode.json` contiene `gentle-orchestrator`, sustituirlo por `axiom-orchestrator` y reescribir comandos `/sdd-*` asociados.
  - Tests unitarios de preservación y migración de perfiles OpenCode.

- [ ] **T5 · Thin wrapper deprecado en `cmd/gentle-ai`**
  - Adaptar `cmd/gentle-ai/main.go` para que imprima advertencia de obsolescencia en `stderr` ("gentle-ai CLI está deprecado; usa 'axiom'") y delegue la ejecución a la lógica central de Axiom.

- [ ] **T6 · Verificación funcional completa**
  - Ejecutar suites de tests de `internal/cli`, `internal/system` y `internal/components/sdd`.
  - Validar sincronización en workspace real con `go run ./cmd/axiom sync`.
