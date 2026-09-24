# Patrón Canónico de Workspace y Auto-Sincronización del Índice de Skills

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/canonical-workspace-and-skills-sync.md`.  
> Espejo de recuperación en Engram: topic `odd/canonical-workspace-and-skills-sync/tasks`, proyecto `axiom`.

## Objetivo

Implementar de forma completa el **Patrón Canónico de Espacio de Trabajo** y la **Auto-Sincronización del Índice de Skills** en Axiom, logrando que:
1. En topologías multirrepo y monorrepos desacoplados, la fuente de la verdad de la configuración de Axiom resida versionada en Git dentro del repositorio de especificaciones (`specs_repository/axiom.yaml`), mientras que la carpeta maestra local resuelve su configuración a través de un archivo puntero ligero (`.axiom-workspace`).
2. El indexador de skills (`skillregistry`) reconozca `.axiom-workspace` y `axiom.yaml` como marcadores de proyecto válidos y descubra automáticamente las skills distribuidas en `specs_repository/skills/` y en los repositorios de código declarados en `roles`.
3. El comando `axiom sync` (así como la sincronización desde `axiom ui` y TUI) invoque automáticamente al finalizar la regeneración del índice (`skillregistry.Regenerate`), garantizando que `AGENTS.md`, `.atl/skill-registry.md` y Engram MCP queden 100% actualizados en una sola operación.

## Problema y Diagnóstico

1. **Aislamiento en Git de `axiom.yaml`:** En multirrepos y monorrepos desacoplados, la carpeta maestra suele ser un contenedor no-versionado. Dejar `axiom.yaml` en la raíz sin un mecanismo que apunte a `specs_repository/axiom.yaml` provoca que la configuración de roles y políticas no esté versionada en Git.
2. **Desconexión entre `sync` e indexación de skills:** Actualmente `axiom sync` actualiza los agentes pero no ejecuta `skillregistry.Regenerate`. El usuario tiene que recordar ejecutar `axiom skill index refresh` manualmente; de lo contrario, `AGENTS.md` queda desactualizado.
3. **Punto ciego en descubrimiento de skills multirrepo:** `ProjectSkillDirs` solo inspecciona `cwd/skills` y las carpetas de agentes de la raíz. Si las skills compartidas residen en `repo-specs/skills/` o en repositorios de roles (`repo-frontend/skills/`), el índice no las incluye.
4. **Falso negativo en el guardián de proyecto:** Si la carpeta maestra no contiene `.git` propio (porque los `.git` están dentro de cada subrepositorio), `hasProjectMarker` no reconoce la carpeta maestra como proyecto, bloqueando la regeneración del índice.

## Alcance Autorizado

- `internal/workspace/loader.go` y `loader_test.go`:
  - Soporte de puntero `.axiom-workspace` (`config: <path>` o `specs: <dir>`).
  - Función `ResolveConfigFile(root string) (string, error)` con resolución en cascada (.axiom-workspace -> axiom.yaml local -> auto-descubrimiento en subdirectorios de specs).
  - Integración en `LoadConfig` para resolver automáticamente la ruta efectiva.
- `internal/skillregistry/guard.go` y `guard_test.go`:
  - Añadir `.axiom-workspace` y `axiom.yaml` a `hasProjectMarker`.
- `internal/skillregistry/registry.go` y `registry_test.go`:
  - Extender `ProjectSkillDirs(cwd)` o función asociada para que lea la configuración resuelta de Axiom y sume `specs_repository/skills` y las rutas `repositories` de cada rol si existen.
- `internal/cli/sync.go` y `internal/app/app.go`:
  - Invocar `skillregistry.Regenerate` de forma segura e idempotente al finalizar la sincronización en ámbito workspace (`ScopeWorkspace`), reportando las skills indexadas en el resumen.
- Pruebas unitarias y de integración end-to-end.

## Restricciones

- **Rutas Relativas:** Todas las rutas dentro de `axiom.yaml` y `.axiom-workspace` deben ser relativas a la carpeta maestra común.
- **Tolerancia y Retrocompatibilidad:** Si no existe `.axiom-workspace`, Axiom debe seguir funcionando con normalidad si existe un `axiom.yaml` en la raíz (monorrepos embebidos).
- **No-Bloqueo ante fallos secundarios:** Un fallo en la indexación de skills no debe revertir una sincronización exitosa de agentes; se reporta como aviso (*warning*).
- **Idioma Obligatorio:** Código, comentarios, mensajes de error y documentación estrictamente en español (castellano peninsular).

---

## Tareas

- [x] **T1 · Resolución de Configuración por Puntero (`internal/workspace/loader.go`)**
  - Implementar soporte para `.axiom-workspace` (YAML con campos `config` o `specs`).
  - Implementar `ResolveConfigFile(baseDir string) (string, error)` con fallback a `axiom.yaml` y búsqueda en `specs/`, `repo-specs/`, `openspec/`.
  - Actualizar `LoadConfig` para admitir directorios o rutas directas.
  - Crear pruebas unitarias completas en `internal/workspace/loader_test.go`.

- [x] **T2 · Reconocimiento de Marcador de Proyecto en Guard (`internal/skillregistry/guard.go`)**
  - Actualizar `hasProjectMarker(cwd string) bool` para reconocer `.axiom-workspace` y `axiom.yaml`.
  - Añadir pruebas unitarias en `internal/skillregistry/guard_test.go`.

- [x] **T3 · Descubrimiento Multirrepo en el Motor de Skills (`internal/skillregistry/registry.go`)**
  - Hacer que la recopilación de directorios de skills de proyecto consulte `axiom.yaml` (vía `ResolveConfigFile`) para incluir `specs_repository/skills` y `<role.repo>/skills`.
  - Normalizar rutas relativas a la carpeta maestra en las entradas del índice.
  - Validar con pruebas unitarias en `internal/skillregistry/registry_test.go`.

- [x] **T4 · Auto-Regeneración del Índice al Finalizar `axiom sync` (`internal/cli/sync.go`)**
  - Integrar la llamada a `skillregistry.Regenerate` al final de `RunSyncWithSelectionScoped` cuando `scope == ScopeWorkspace` (o cuando se detecte un workspace).
  - Incluir en `SyncResult` y en `RenderSyncReport` el resultado de las skills indexadas.
  - Asegurar comportamiento idempotente y no destructivo ante errores de indexación.

- [x] **T5 · Pruebas de Integración y Verificación End-to-End**
  - Crear test de integración que simule un workspace multirrepo con `.axiom-workspace`, `repo-specs/axiom.yaml`, `repo-specs/skills/` y `repo-backend/skills/`.
  - Verificar que `axiom sync` genere `.claude/`, actualice `AGENTS.md` con las skills de ambos repos y sincronice Engram.
  - Ejecutar suite completa `go test ./...` y `axiom doctor`.

---

## Verificación Ejecutable

1. **Pruebas de Loader:**
   - `go test ./internal/workspace -run "TestResolveConfigFile|TestLoadConfigPointer" -v -count=1` -> PASS.
2. **Pruebas de Guard y Registry:**
   - `go test ./internal/skillregistry -run "TestGuardAxiomMarkers|TestProjectSkillDirsMultirepo" -v -count=1` -> PASS.
3. **Prueba de Integración Sync:**
   - `go test ./internal/cli -run "TestRunSyncAutoRegeneratesSkillIndex" -v -count=1` -> PASS.
4. **Salud del Sistema:**
   - `go test ./internal/workspace ./internal/skillregistry ./internal/cli ./cmd/axiom -count=1` -> PASS.
