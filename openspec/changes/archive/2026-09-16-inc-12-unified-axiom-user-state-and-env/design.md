# Diseño Arquitectónico: Unificación del Estado Global bajo ~/.axiom y Variables AXIOM_* (INC-12)

## Contexto y Motivación

Axiom nació como una evolución y desacoplamiento integral de Gentle AI. Hasta el momento:
1. El Hub multi-proyecto almacena su configuración global en `~/.axiom/workspaces.json` (INC-08).
2. Sin embargo, el resto de componentes de persistencia de usuario continúan anclados a `~/.gentle-ai/`:
   - `~/.gentle-ai/state.json`: archivo central de estado de instalación, agentes y modelos seleccionados.
   - `~/.gentle-ai/bin/`: lanzadores (`opencode`, `opencode.cmd`, `opencode.ps1`) para subagentes en segundo plano.
   - `~/.gentle-ai/backups/`: instantáneas y manifiestos de respaldo previo a modificaciones.
   - `~/.gentle-ai/cache/`: caché local de variantes de modelos (`model-variants.json`).
3. Las variables de entorno de configuración operativa exigen el prefijo legado `GENTLE_AI_*` (`GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS`, `GENTLE_AI_CHANNEL`, `GENTLE_AI_NO_ANIMATION`, `GENTLE_AI_TELEMETRY`, etc.).

El **Incremento 12 (INC-12: `unified-axiom-user-state-and-env`)** consolida la totalidad del almacenamiento de usuario bajo **`~/.axiom/`**, implementa un mecanismo transparente y seguro de migración defensiva desde `~/.gentle-ai/`, y establece la precedencia autoritativa de variables de entorno con prefijo **`AXIOM_*`**.

---

## Decisiones de Diseño

### Decisión 1: Directorio Base Canónico `~/.axiom` y Migración Transparente en `internal/state`

- **Rutas Canónicas:**
  - `stateDir = ".axiom"`
  - `legacyStateDir = ".gentle-ai"`
  - `stateFile = "state.json"`
  - `state.Path(homeDir)` devuelve `filepath.Join(homeDir, ".axiom", "state.json")`.
  - `state.LegacyPath(homeDir)` devuelve `filepath.Join(homeDir, ".gentle-ai", "state.json")`.
- **Migración Defensiva y Transparente en `state.Read(homeDir)`:**
  1. Si `~/.axiom/state.json` existe, se lee y devuelve como fuente de la verdad autoritativa.
  2. Si `~/.axiom/state.json` no existe pero `~/.gentle-ai/state.json` sí existe:
     - Se lee el contenido de `~/.gentle-ai/state.json`.
     - Se crea el directorio `~/.axiom/` con permisos `0755`.
     - Se copia atómicamente el archivo a `~/.axiom/state.json` con permisos `0644`.
     - Se preserva intacto `~/.gentle-ai/state.json` como respaldo no destructivo.
     - Se devuelve el estado migrado sin requerir intervención del usuario.
  3. Si no existe ninguno de los dos, devuelve el error `os.ErrNotExist` estándar de primer arranque.
- **Escritura en `state.Write(homeDir, s)`:**
  - Escribe invariablemente sobre `~/.axiom/state.json`, garantizando que ninguna nueva mutación toque `~/.gentle-ai/`.

### Decisión 2: Lanzadores de OpenCode en `~/.axiom/bin` y Aislamiento de Loop en `internal/opencode`

- **Ubicación Canónica de Lanzadores:**
  - `BinDir(homeDir)` pasa a devolver `filepath.Join(homeDir, ".axiom", "bin")`.
  - `LegacyBinDir(homeDir)` devuelve `filepath.Join(homeDir, ".gentle-ai", "bin")`.
- **Resolución Segura del Binario Real en `ResolveTarget`:**
  - Al buscar el ejecutable real de `opencode` en el `PATH`, se excluyen tanto `BinDir(homeDir)` como `LegacyBinDir(homeDir)`.
  - Esto previene bucles de auto-invocación infinita en sistemas donde el usuario aún mantenga la ruta antigua en su variable `PATH`.
- **Desactivación y Limpieza en `PrepareDeactivation`:**
  - Prepara la retirada de lanzadores gestionados bajo `~/.axiom/bin/` y también bajo `~/.gentle-ai/bin/` si estuvieran presentes con el marcador de propiedad de Axiom.

### Decisión 3: Respaldos en `~/.axiom/backups` y Caché en `~/.axiom/cache`

- **Respaldos (`internal/backup` y `internal/app`):**
  - `backupRoot()` resuelve `filepath.Join(home, ".axiom", "backups")`.
  - `isRootDirUnderBackupRoot(dir)` valida si la ruta está bajo `~/.axiom/backups` O bien bajo el legado `~/.gentle-ai/backups`.
  - `ListBackups()` en `internal/app/app.go` escanea primordialmente `~/.axiom/backups` e incorpora cualquier respaldo existente en `~/.gentle-ai/backups`.
- **Caché de Variantes de Modelos:**
  - En el plugin embebido `internal/assets/opencode/plugins/model-variants.ts`, la variable `cacheDir` pasa a `path.join(homedir(), ".axiom", "cache")`.
  - En `internal/components/uninstall/service.go`, el limpiador de caché purga `~/.axiom/cache` y también cualquier remanente en `~/.gentle-ai/cache`.

### Decisión 4: Capa de Precedencia de Variables de Entorno (`AXIOM_*` > `GENTLE_AI_*`)

- **Módulo Centralizado en `internal/system/env.go`:**
  - Se implementan funciones auxiliares de lectura de entorno con fallback ordenado:
    - `LookupEnv(axiomKey, fallbackKey string) (string, bool)`
    - `Getenv(axiomKey, fallbackKey string) string`
- **Variables Cubiertas:**
  1. `AXIOM_OPENCODE_BACKGROUND_SUBAGENTS` (fallback: `GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS`) en `internal/cli/opencode_background.go`.
  2. `AXIOM_CHANNEL` (fallback: `GENTLE_AI_CHANNEL`) en `internal/cli/channel.go`.
  3. `AXIOM_NO_ANIMATION` (fallback: `GENTLE_AI_NO_ANIMATION`) en `internal/tui/model.go`.
  4. `AXIOM_TELEMETRY` (fallback: `GENTLE_AI_TELEMETRY`) en `internal/telemetry/killswitch.go` y `internal/assets/opencode/plugins/telemetry-runtime.ts`.
  5. `AXIOM_PI_BACKGROUND_SUBAGENTS` (fallback: `GENTLE_AI_PI_BACKGROUND_SUBAGENTS`) en `internal/cli/install.go`.
  6. `AXIOM_INSTALL_SCOPE` (fallback: `GENTLE_AI_INSTALL_SCOPE`) en `internal/cli/install.go`.
- **Actualización de Textos de Ayuda en CLI:**
  - Los mensajes de `--help` y opciones de flags reflejan la variable primaria `AXIOM_*` manteniendo mención o compatibilidad con `GENTLE_AI_*`.

### Decisión 5: Diagnóstico en `axiom doctor` (`internal/cli/doctor.go`)

- `checkStateJSON`: Valida la existencia o estado de `~/.axiom/state.json`. Si no existe pero hay un estado legado en `~/.gentle-ai/state.json`, la lectura mediante `state.Read` lo migrará limpiamente en tiempo de ejecución.
- `checkDiskSpace`: Mide el espacio libre sobre el directorio `~/.axiom/` (creándolo o recurriendo al directorio home si no existe aún).

---

## Diagrama de Arquitectura y Flujo

```
+--------------------------------------------------------------------------+
|                         Variables de Entorno                             |
|                                                                          |
|       AXIOM_<VAR>  ---------- (Presente) -------> [ Valor AXIOM ]        |
|            |                                                             |
|         (Ausente)                                                        |
|            v                                                             |
|     GENTLE_AI_<VAR> ------- (Presente) -------> [ Valor GENTLE_AI ]      |
|            |                                                             |
|         (Ausente)                                                        |
|            +----------------------------------> [ Valor por Defecto ]    |
+--------------------------------------------------------------------------+

+--------------------------------------------------------------------------+
|                       Resolución de Estado en Disco                      |
+--------------------------------------------------------------------------+
                                    |
                                    v
                       ¿Existe ~/.axiom/state.json?
                                  /   \
                             Sí  /     \  No
                                /       \
                               v         v
                     [ Leer ~/.axiom ]  ¿Existe ~/.gentle-ai/state.json?
                                               /   \
                                          Sí  /     \  No
                                             /       \
                                            v         v
                             [ Copiar a ~/.axiom ]   [ Retornar NotExist ]
                             [ y devolver datos  ]
```

---

## Matriz de Riesgos y Mitigaciones

| Riesgo | Impacto | Mitigación |
| :--- | :--- | :--- |
| Pérdida de configuración al actualizar usuarios antiguos | Alto | Migración defensiva que copia sin eliminar `~/.gentle-ai/state.json`. `~/.gentle-ai` queda intacto como backup. |
| Bucle infinito al buscar `opencode` en PATH si `~/.gentle-ai/bin` sigue en PATH | Alto | `ResolveTarget` filtra explícitamente tanto `~/.axiom/bin` como `~/.gentle-ai/bin`. |
| Scripts externos de usuario que aún exportan `GENTLE_AI_*` | Medio | Doble lectura (`LookupEnv`) con precedencia estricta: `AXIOM_*` primero, `GENTLE_AI_*` como respaldo transparente. |
| Conflictos al migrar concurrentemente `state.json` | Bajo | Uso de creación atómica de archivo (`filemerge.WriteFileAtomic`) y lectura directa. |
