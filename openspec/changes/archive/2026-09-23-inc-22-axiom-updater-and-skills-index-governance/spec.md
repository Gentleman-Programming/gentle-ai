# Especificación de Requerimientos: Actualizador Autónomo de Axiom, Sincronización y Gobernanza del Índice Unificado de Skills (INC-22)

> **Incremento:** `inc-22-axiom-updater-and-skills-index-governance`
> **Fase del Roadmap:** Fase 4 — Gobernanza de Agentes, Flujo Dual y Ecosistema Autónomo
> **Fase SDD:** `sdd-spec` · **Fecha:** 2026-09-22
> **Capacidades:** `axiom-updater-resilience` (ver §0.2) · `axiom-skills-index-governance` (nueva)
> **Fuente de alcance:** `openspec/changes/inc-22-axiom-updater-and-skills-index-governance/proposal.md` (REQ-1..REQ-4)
> **Idioma:** español (castellano), registro neutro y profesional. Identificadores, rutas, comandos y literales técnicos se mantienen en inglés.

---

## 0. Marco de lectura

### 0.1 Palabras clave RFC 2119

Para preservar la fuerza normativa de RFC 2119 dentro del registro en español, este documento usa la siguiente correspondencia. La fuerza es la de RFC 2119, no una sugerencia:

| Término | Fuerza |
|---|---|
| **DEBE** / **NO DEBE** / **DEBEN** / **NO DEBEN** | Requisito absoluto / prohibición absoluta |
| **DEBERÍA** / **NO DEBERÍA** | Recomendado, con excepción justificada |
| **PUEDE** | Opcional |

Los escenarios usan **DADO** / **CUANDO** / **ENTONCES** / **Y** en formato Given/When/Then.

### 0.2 Divergencias conocidas entre la propuesta y el código verificado

Estas divergencias se verificaron contra el árbol de trabajo antes de redactar los requisitos. Donde el código contradice la propuesta, **manda el código** y el requisito se redacta sobre el comportamiento real, no sobre el supuesto. Ninguna de ellas amplía el alcance: condicionan cómo se expresa.

| # | Afirmación de la propuesta | Realidad verificada | Efecto en esta spec |
|---|---|---|---|
| D-1 | "el comando `axiom skill-registry` no responde", motor "sin enrutar en `cmd/axiom/main.go`" | **Falso.** Sí está enrutado: `cmd/axiom/main.go` (`case "skill-registry"` → `app.RunArgs`) e `internal/app/app.go` (`runSkillRegistry`, con `refresh\|list`). | REQ-22.14 lo fija como verbo de compatibilidad que **NO DEBE** retirarse. Lo que falta es `axiom skill index`, no el enrutado. |
| D-2 | `axiom-updater-resilience` es una capacidad **modificada** | No existe `openspec/specs/axiom-updater-resilience/spec.md`. El nombre solo aparece en la propuesta. | Se redacta como especificación completa (nueva) bajo el cambio. Ver §6. |
| D-3 | Bloqueo Windows: `go install github.com/IGutierrezZ/axiom/cmd/axiom@vX.Y.Z` vs `go.mod` = `module github.com/gentleman-programming/gentle-ai/v3` | **Confirmado** (`cmd/axiom/main.go` fija `GoImportPath: github.com/IGutierrezZ/axiom/cmd/axiom`; `go.mod:1` declara el módulo upstream). Pero hay una **segunda** discrepancia no citada: `update.GentleAISourceInstallCommand` compone `go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@…`, apuntando al módulo **y** al binario upstream. | REQ-22.2 cubre ambas discrepancias. |
| D-4 | (no citado) | Las salvaguardas de Windows están ancladas al nombre `"gentle-ai"` (`binaryUpgrade`, `preflightWindowsGentleAIGoInstall`, `isBetaGentleAIUpgrade` en `internal/update/upgrade/strategy.go`). `cmd/axiom/main.go` renombra la herramienta a `"axiom"` en `init()`, por lo que **ninguna de esas salvaguardas se dispara** y el fallback genérico de Windows apunta a `Gentleman-Programming/${Repo}` (propietario equivocado). | REQ-22.3. Es la causa raíz del bloqueo, no solo la ruta del módulo. |
| D-5 | `filemerge` como paquete propio | Vive en `internal/components/filemerge`. `AGENTS.md` **no** tiene marcadores `<!-- axiom:… -->` hoy. | REQ-22.12 define el contrato de marcadores y la adopción inicial, porque `InjectMarkdownSection` sin marcadores **añade al final** y duplicaría la sección. |
| D-6 | `skillregistry.Regenerate()` debe "extenderse" a tres destinos | Hoy solo escribe `.atl/skill-registry.md` y `.atl/.skill-registry.cache.json`. `sectionMarker = "## Skills"` solo se usa como título del propio registro. No toca `AGENTS.md` ni Engram. | REQ-22.11 describe el comportamiento añadido; el existente se conserva. |
| D-7 | "Ajuste de la versión build-time local en `cmd/axiom/main.go`" | `Version` y `GitCommit` son `const` (`"v0.1.0"`, `"dev"`). `.goreleaser.yaml` y `ci.yml` inyectan `-X main.version=…`, símbolo que **no existe** en `cmd/axiom` (sí en `cmd/gentle-ai`). El `ldflags` es hoy un no-op. | REQ-22.8. |
| D-8 | Web UI: `/api/ecosystem/upgrade` pasa a `upgrade` → `sync` | El contrato vivo `tui-ui-parity` describe ese endpoint como equivalente a `axiom upgrade` **solo**. A la vez, `internal/app/upgrade_test.go` fija que `upgrade` **no** invoca install/sync (solo-binario). | REQ-22.4 modifica el endpoint; REQ-22.5 fija la no-regresión del verbo CLI. El encadenamiento es exclusivo de Web UI y TUI. |
| D-9 | Residuos de branding `gentle-ai` en `internal/tui/screens/upgrade_sync.go` | **Confirmado** (textos de confirmación, salto de sync y reinicio). Hay residuos adicionales fuera del alcance declarado: `internal/tui/screens/upgrade.go`, `internal/update/instructions.go`, `internal/update/upgrade/strategy.go`, `internal/app/help.go`, `internal/app/app.go`, `internal/skillregistry/registry.go` (cabecera "gentle-ai skill-registry"). | REQ-22.7 acota la sustitución al alcance de la propuesta; los demás se listan en §7. |
| D-10 | GitHub Releases de `IGutierrezZ/axiom` sin binarios `.zip`/`.exe` | **No verificado en esta sesión** (la búsqueda no resolvió el repositorio). Se asume de la propuesta sin confirmar. | REQ-22.1 se redacta sobre la garantía de comportamiento, no sobre la existencia de assets. |
| D-11 | `axiom skill index list` | `axiom skill list` **ya existe** con otro significado (skills activas y buzón `--inbox` de autoskill). | REQ-22.10 exige distinción explícita en salida y ayuda. |

---

## 1. Contratos CLI

### 1.1 `axiom skill index` (nuevo)

Superficie canónica del índice unificado. Se añade al grupo `axiom skill` existente (`scan`, `list`, `approve`, `reject`) como subcomando `index`.

#### Forma general

```
axiom skill index <refresh|list> [flags]
```

| Subcomando | Flags admitidas | Efecto |
|---|---|---|
| `refresh` | `--force`, `-f` · `--quiet`, `-q` · `--no-gitignore` · `--cwd <dir>` | Regenera el índice y sus destinos (REQ-22.11) |
| `list` | `--json` · `--cwd <dir>` | Consulta de solo lectura del índice; no escribe ningún destino |

- `<dir>` de `--cwd` DEBE resolverse como directorio de trabajo objetivo; vacío o ausente DEBE significar el `cwd` del proceso.
- `--force` / `-f` DEBE omitir la comprobación de huella y regenerar siempre.
- `--quiet` / `-q` DEBE suprimir la salida estándar en éxito. **No** suprime la salida de error.
- `--no-gitignore` DEBE omitir la garantía de la entrada `.atl/` en `.gitignore`.
- Una bandera desconocida, una bandera sin su valor (`--cwd` sin argumento) o un subcomando desconocido DEBEN abortar **sin efectos** (ninguna escritura) y con código de salida `1`.

#### Salida de `list`

- Por defecto, una línea por skill en TSV, sin cabecera: `name<TAB>scope<TAB>path`, con `scope` ∈ {`project`, `user`}. Sin skills: una única línea `No skills found.`
- Con `--json`: un array JSON con un objeto por skill y las claves `name`, `scope`, `description`, `path`. Con cero skills: `[]`.

#### Salida de `refresh`

- Sin `--quiet`:
  - Éxito con regeneración: una línea `Skill registry refreshed (N skills): <registry-path>`.
  - Éxito sin cambios (huella coincidente y sin `--force`): una línea `Skill registry up to date (<reason>): <registry-path>`.
  - Tras cada destino secundario (`AGENTS.md`, Engram) una línea que declare su resultado (`updated` / `unchanged` / `mirror ok` / `mirror failed`).
- Con `--quiet`: sin salida estándar en éxito.

#### Códigos de salida y comportamiento en error

| Situación | stdout | stderr | Código |
|---|---|---|---|
| Éxito (regenerado o al día) | informe | vacío | `0` |
| Éxito bajo `--quiet` | vacío | vacío | `0` |
| Omisión por no ser raíz de proyecto (`RefreshSkip` ≠ none) **con** `--quiet` | vacío | vacío | `0` |
| Omisión por no ser raíz de proyecto **sin** `--quiet` | aviso de una línea con el motivo y la ruta | vacío | `0` |
| `axiom skill index` sin subcomando | vacío | uso: `axiom skill index <refresh\|list> [flags]` | `1` |
| Subcomando desconocido | vacío | mensaje que nombra `refresh` y `list` | `1` |
| Bandera desconocida o incompleta | vacío | mensaje con la bandera exacta | `1` |
| Fallo de escritura de cualquier destino primario (registro, caché, `AGENTS.md`) | parcial ya emitido | error envuelto con el destino | `1` |
| Fallo del espejo Engram tras un registro primario correcto | informe con `mirror failed` | aviso | `0` (no fatal; ver REQ-22.11) |

#### Distinción obligatoria frente a `axiom skill list`

`axiom skill list` (existente, autoskill) y `axiom skill index list` (nuevo, índice) DEBEN distinguirse de forma inequívoca en la ayuda y en la salida: el primero refiere buzón/activas de autoskill, el segundo el índice unificado. La ayuda de `axiom skill` DEBE listar `index` junto a `scan`, `list`, `approve`, `reject` y DEBE aclarar esta colisión en una línea.

### 1.2 Verbos de actualización afectados

| Verbo | Cambio | Restricción |
|---|---|---|
| `axiom update` | La pista de actualización de la herramienta del fork DEBE nombrar el repositorio y el módulo del fork | NO DEBE ofrecer un `go install` cuyo módulo contradiga `go.mod` (REQ-22.2) |
| `axiom upgrade` | Estrategia resiliente en Windows/Unix (REQ-22.1 a REQ-22.3) | **Sigue siendo solo-binario**: NO DEBE invocar `install` ni `sync` (REQ-22.5) |
| `axiom skill index refresh` | Nuevo | — |
| `axiom skill-registry refresh\|list` | Sin cambio de contrato | DEBE seguir aceptando `--force`, `--quiet`, `--no-gitignore`, `--cwd`, `--json` (REQ-22.14) |
| `axiom skill approve` | Gancho de regeneración (REQ-22.13) | La promoción en sí NO DEBE cambiar de semántica |
| `axiom version` | Coherencia build-time (REQ-22.8) | Formato de salida NO DEBE cambiar |

---

## 2. Contrato JSON en Web UI — `POST /api/ecosystem/upgrade`

### 2.1 Cambio de semántica

El endpoint pasa de disparar solo `upgrade` a ejecutar la secuencia `upgrade` → `sync` y devolver un reporte consolidado de ambas operaciones. `POST /api/ecosystem/sync` NO DEBE cambiar de contrato ni de semántica.

### 2.2 Forma de la respuesta

Se conservan las claves actuales de `EcosystemActionResponse` (`success`, `action`, `message`, `output`, `error`) y se añaden dos claves opcionales, pobladas solo por este endpoint encadenado. Cualquier otro consumidor del DTO (por ejemplo `POST /api/ecosystem/backups/restore`) NO DEBE verse obligado a poblarlas.

```json
{
  "success": true,
  "action": "upgrade",
  "message": "Upgrade y sync completados",
  "output": ["..."],
  "error": "",
  "sequence": "upgrade->sync",
  "phases": {
    "upgrade": {
      "success": true,
      "status": "succeeded",
      "restart_required": false,
      "manual_hint": "",
      "output": ["..."],
      "error": ""
    },
    "sync": {
      "success": true,
      "executed": true,
      "skipped_reason": "",
      "files": ["..."],
      "output": ["..."],
      "error": ""
    }
  }
}
```

| Campo | Tipo | Obligatorio | Semántica |
|---|---|---|---|
| `sequence` | string | no | Identificador literal `upgrade->sync` cuando se ejecutó la cadena |
| `phases` | object | no | Resultado por fase, en orden de ejecución |
| `phases.upgrade.status` | string | sí (dentro de `phases.upgrade`) | `succeeded` \| `failed` \| `skipped` |
| `phases.upgrade.restart_required` | bool | sí | `true` cuando la fase reemplazó el binario `axiom` en ejecución |
| `phases.upgrade.manual_hint` | string | sí | Instrucción de actualización manual cuando `status` es `skipped`; vacío en caso contrario |
| `phases.sync.executed` | bool | sí (dentro de `phases.sync`) | `false` cuando `sync` no llegó a ejecutarse |
| `phases.sync.skipped_reason` | string | sí | Vacío si se ejecutó; `restart-required` si se omitió por auto-reemplazo del binario; `upgrade-failed` si no se ejecutó por fallo fatal de `upgrade` |
| `phases.sync.files` | string[] | no | Rutas que `sync` modificó; puede ser `[]` |

### 2.3 Semántica de la secuencia

1. La fase `upgrade` DEBE ejecutarse primero y corresponder al comportamiento de `axiom upgrade` (solo-binario).
2. La fase `sync` DEBE ejecutarse después, **solo si** `phases.upgrade.status` es `succeeded` y `phases.upgrade.restart_required` es `false`.
3. Si `restart_required` es `true`, `sync` NO DEBE ejecutarse y DEBE quedar `executed: false`, `skipped_reason: "restart-required"` (paridad con la regla de la TUI, REQ-22.6).
4. Si `upgrade` falla de forma fatal (`status: "failed"`), `sync` NO DEBE ejecutarse y DEBE quedar `executed: false`, `skipped_reason: "upgrade-failed"`.
5. `success` (nivel superior) DEBE ser `true` cuando toda fase ejecutada tuvo éxito, incluso si `sync` quedó omitido por `restart-required` (no es un fallo). DEBE ser `false` si alguna fase ejecutada falló o si `sync` quedó omitido por `upgrade-failed`.

### 2.4 Códigos HTTP y errores

| Situación | HTTP | Cuerpo |
|---|---|---|
| Petición `POST` procesada, reporte disponible (total o parcial) | `200` | DTO completo |
| Método distinto de `POST` | `405` | error textual | 
| El servicio no logra producir reporte alguno | `500` | DTO con `success: false` y `error` |

`GET`, `PUT`, `DELETE` sobre el endpoint DEBEN devolver `405` sin efectos.

---

## 3. Contrato de reemplazo de la sección `## Skills` de `AGENTS.md`

### 3.1 Objeto

Reemplazo **atómico** de la sección `## Skills` de `AGENTS.md` mediante `filemerge` (`internal/components/filemerge`), reutilizado íntegro y sin modificar, sin alterar ninguna otra sección del documento.

### 3.2 Contrato de marcadores

| Elemento | Valor exacto |
|---|---|
| `sectionID` | `skills-index` |
| Marcador de apertura | `<!-- axiom:skills-index -->` |
| Marcador de cierre | `<!-- /axiom:skills-index -->` |
| Marcado legado aceptado | `<!-- gentle-ai:skills-index -->` / `<!-- /gentle-ai:skills-index -->` (se reconoce y se eleva al canónico) |

La operación DEBE corresponder a `filemerge.InjectMarkdownSection(existing, "skills-index", content)`, que ya garantiza: sustitución del par completo, reconocimiento y elevación del marcado legado, reparación de marcadores huérfanos, colapso de bloques duplicados y no-modificación del contenido fuera de marcadores.

### 3.3 Contenido gestionado

Exactamente, entre el marcador de apertura y el de cierre:

1. La línea de título `## Skills`.
2. Una línea en blanco.
3. La tabla Markdown de skills, con cabecera fija y una fila por skill indexada.
4. Un salto de línea final.

El título `## Skills` DEBE permanecer **dentro** del contenido gestionado, de modo que título y tabla se sustituyan como un bloque único. Ningún otro texto (prosa introductoria, secciones hermanas, notas) DEBE quedar dentro de los marcadores.

### 3.4 Formato de la tabla

Alineado con `.atl/skill-registry.md` para mantener un único formato reconocible:

```markdown
## Skills

| Skill | Trigger / description | Scope | Path |
| --- | --- | --- | --- |
| `nombre-skill` | disparador o descripción | project | `ruta/relativa/SKILL.md` |
| `otra-skill` | disparador o descripción | user | `/ruta/absoluta/SKILL.md` |
```

| Columna | Contenido |
|---|---|
| `Skill` | Nombre en resaltado de código |
| `Trigger / description` | Campo `description` del frontmatter; `—` si está vacío. Los saltos de línea se colapsan en espacio y `\|` se escapa como `\\\|` |
| `Scope` | `project` o `user` |
| `Path` | Ruta en resaltado de código |

Las filas DEBEN ordenarse por `Skill` ascendente. La tabla actual de tres columnas (`Skill | Trigger | Path`) queda superada por esta de cuatro.

### 3.5 Adopción inicial (marcadores ausentes)

`AGENTS.md` no tiene hoy marcadores `<!-- axiom:… -->`. Como `InjectMarkdownSection` **añade al final** cuando no encuentra el par, una llamada ingenua duplicaría la sección. Por tanto:

1. **DADO** un `AGENTS.md` sin el par `<!-- axiom:skills-index -->` / `<!-- /axiom:skills-index -->` (ni su legado),
2. **CUANDO** se regenera el índice por primera vez,
3. **ENTONCES** el sistema DEBE localizar la primera ocurrencia de `## Skills` a inicio de línea y sustituir desde esa cabecera hasta la línea inmediatamente anterior a la siguiente cabecera `## ` a inicio de línea, o hasta EOF si no existe,
4. **Y** el contenido de esa región DEBE quedar envuelto en el par de marcadores canónico más el contenido gestionado de §3.3.

Si no existe ni el par de marcadores ni una cabecera `## Skills`, el sistema DEBE añadir al final del fichero el par de marcadores con el contenido gestionado (semántica de anexado de `InjectMarkdownSection`).

### 3.6 Invariantes

- **Atomicidad:** el fichero completo DEBE escribirse en una única operación atómica (`filemerge.WriteFileAtomic`).
- **Preservación:** todo byte fuera de los marcadores DEBE permanecer idéntico. En particular las secciones `## REGLA SUPREMA…`, `# Gentle AI™ — Agent Skills Index` y `## How to Use`, y la cabecera del documento.
- **Idempotencia:** dos regeneraciones consecutivas con el mismo conjunto de skills DEBEN producir un `AGENTS.md` byte-idéntico.
- **Reparación:** marcadores huérfanos o pares duplicados DEBEN repararse en la misma operación (comportamiento ya presente en `InjectMarkdownSection`).
- **No-creación:** si `AGENTS.md` no existe en el workspace resuelto, el sistema NO DEBE crearlo; DEBE reportar el destino como omitido sin convertir la operación en fallo.

---

## 4. Requerimientos — `axiom-updater-resilience`

> **Naturaleza:** especificación completa. No existe especificación viva de esta capacidad (divergencia D-2).

### REQ-22.1: Ejecución segura de la actualización en Windows

El sistema DEBE aplicar, en entorno Windows, una estrategia de actualización de la herramienta del fork que no falle por discrepancia de ruta de módulo en `go.mod` y que reemplace el binario de forma segura y transparente. Cuando ninguna estrategia automatizada pueda garantizar esas dos condiciones, el sistema DEBE degradar a una instrucción de actualización manual accionable y NO DEBE dejar el binario a medias.

#### Scenario: Actualización disponible en Windows con estrategia automatizada válida

- **DADO** un entorno Windows con `axiom` instalado y una nueva versión publicada en `IGutierrezZ/axiom`
- **CUANDO** el usuario ejecuta `axiom upgrade`
- **ENTONCES** el sistema completa la actualización sin error de discrepancia de módulo
- **Y** el binario activo queda reemplazado de forma atómica

#### Scenario: Sin estrategia automatizada válida, degradación a manual sin mutación

- **DADO** un entorno Windows donde ninguna estrategia automatizada puede garantizar módulo coherente y reemplazo seguro
- **CUANDO** el usuario ejecuta `axiom upgrade`
- **ENTONCES** el estado de la herramienta es `skipped` con `ManualHint` poblado
- **Y** ningún fichero ha sido modificado
- **Y** la instrucción manual nombra el repositorio `IGutierrezZ/axiom` y la vía de instalación correcta

#### Scenario: El reemplazo del binario no deja duplicados en PATH

- **DADO** un entorno Windows donde el binario activo y el destino de escritura de Go resuelven a rutas distintas
- **CUANDO** el actualizador se dispone a escribir
- **ENTONCES** el sistema NO DEBE escribir el segundo binario
- **Y** DEBE degradar a manual explicando ambas rutas y cómo migrar de forma intencionada

### REQ-22.2: Coherencia de la ruta de módulo en toda instrucción `go install`

Ninguna instrucción `go install` emitida por el sistema (pista de actualización, sugerencia de instalación manual, pista de degradación, estrategia ejecutada) DEBE nombrar una ruta de módulo que contradiga la declaración `module` de `go.mod`, ni un nombre de binario que no sea `axiom`. Toda instrucción `go install` referida al fork DEBE apuntar a `github.com/IGutierrezZ/axiom/cmd/axiom` **solo cuando** la ruta de módulo del repositorio publicado sea resoluble por el toolchain; en caso contrario el sistema DEBE omitir el `go install` y ofrecer la vía manual de REQ-22.1.

#### Scenario: La pista de actualización nombra el fork, no upstream

- **DADO** una instalación del fork con una versión más reciente disponible
- **CUANDO** el sistema compone la instrucción de actualización para la herramienta del fork
- **ENTONCES** la instrucción NO DEBE contener `github.com/gentleman-programming/gentle-ai`
- **Y** NO DEBE contener `cmd/gentle-ai`
- **Y** DEBE referenciar la identidad del fork

#### Scenario: Discrepancia de módulo detectada, sin `go install` abortado a mitad

- **DADO** que la ruta de módulo objetivo no coincide con la declaración `module` de `go.mod` del repositorio resuelto
- **CUANDO** el sistema va a emitir o ejecutar un `go install`
- **ENTONCES** el sistema NO DEBE ejecutarlo
- **Y** DEBE degradar a la instrucción manual de REQ-22.1

### REQ-22.3: Salvaguardas de actualización aplicadas a la identidad del fork

Toda salvaguarda de actualización hoy anclada al nombre literal `gentle-ai` DEBE aplicarse también a la identidad `axiom` del fork. La sustitución de identidad en la inicialización NO DEBE desactivar silenciosamente el bloqueo de distribución de Windows, la verificación de procedencia del `go install` en Windows ni la detección de canal beta.

#### Scenario: El bloqueo de distribución de Windows se dispara con la identidad del fork

- **DADO** un entorno Windows y la herramienta registrada bajo la identidad `axiom`
- **CUANDO** se selecciona la estrategia de binario para esa herramienta
- **ENTONCES** el sistema DEBE aplicar el bloqueo de distribución de Windows (o su equivalente del fork)
- **Y** NO DEBE caer en el fallback genérico que nombra un propietario de repositorio ajeno al fork

#### Scenario: La verificación de procedencia en Windows no queda inerte

- **DADO** un entorno Windows con la herramienta registrada como `axiom`
- **CUANDO** se intenta una actualización vía `go install`
- **ENTONCES** el sistema DEBE ejecutar la verificación de procedencia destino/activo
- **Y** DEBE degradar a manual si las rutas no coinciden

### REQ-22.4: Encadenamiento `upgrade` → `sync` en Web UI con reporte consolidado

Al recibir `POST /api/ecosystem/upgrade`, el servidor local DEBE ejecutar la secuencia `upgrade` → `sync` conforme al contrato de §2 y DEBE devolver el reporte consolidado de ambas operaciones. El frontend web DEBE presentar el resultado de ambas fases, incluida la omisión de `sync` con su motivo.

#### Scenario: Ambas fases completan con éxito

- **DADO** el dashboard web local en ejecución
- **CUANDO** el usuario pulsa "Actualizar Herramientas" y se envía `POST /api/ecosystem/upgrade`
- **ENTONCES** se ejecuta `upgrade` y después `sync`
- **Y** la respuesta HTTP es `200` con `sequence: "upgrade->sync"`
- **Y** `phases.upgrade.status` es `succeeded` y `phases.sync.executed` es `true`

#### Scenario: `upgrade` reemplaza el binario en ejecución y `sync` se omite

- **DADO** el dashboard web local y una actualización disponible de la herramienta del fork
- **CUANDO** `upgrade` completa reemplazando el binario en ejecución
- **ENTONCES** `sync` NO DEBE ejecutarse
- **Y** `phases.upgrade.restart_required` es `true`
- **Y** `phases.sync` declara `executed: false` y `skipped_reason: "restart-required"`
- **Y** `success` es `true`

#### Scenario: `upgrade` falla y `sync` no se dispara

- **DADO** el dashboard web local
- **CUANDO** la fase `upgrade` termina con fallo fatal
- **ENTONCES** `sync` NO DEBE ejecutarse
- **Y** `phases.sync` declara `executed: false` y `skipped_reason: "upgrade-failed"`
- **Y** `success` es `false`

#### Scenario: Método HTTP no admitido

- **DADO** el servidor local en ejecución
- **CUANDO** se envía una petición que no sea `POST` a `/api/ecosystem/upgrade`
- **ENTONCES** la respuesta es `405`
- **Y** no se produce ningún efecto

### REQ-22.5: No-regresión del verbo `axiom upgrade` en CLI

El comando `axiom upgrade` DEBE seguir siendo una actualización solo-binaria. NO DEBE invocar `install` ni `sync` en ninguna plataforma.

#### Scenario: `upgrade` en CLI no dispara pipelines de configuración

- **DADO** una instalación con herramientas pendientes de actualizar
- **CUANDO** el usuario ejecuta `axiom upgrade`
- **ENTONCES** la salida no declara que se ejecutó `install` o `sync`
- **Y** ningún fichero de configuración de agentes es modificado por esta orden

### REQ-22.6: Regla de salto de `sync` tras auto-reemplazo del binario

Cuando la actualización sustituya el binario de la herramienta del fork en ejecución, todo encadenamiento posterior a `sync` (Web UI y TUI) DEBE omitir `sync`, DEBE declararlo explícitamente al usuario y DEBE pedir reinicio antes de sincronizar. Esta regla NO DEBE relajarse por el hecho de que `sync` esté ahora encadenado.

#### Scenario: TUI omite `sync` y pide reinicio

- **DADO** la pantalla combinada de actualización en la TUI
- **CUANDO** la fase de actualización sustituye el binario en ejecución con éxito
- **ENTONCES** la pantalla DEBE indicar que `sync` se omitió porque se actualizó la herramienta del fork
- **Y** DEBE indicar que hay que reiniciar antes de sincronizar
- **Y** NO DEBE presentar `sync` como ejecutado

### REQ-22.7: Sustitución de textos heredados en la vista de actualización

Los textos de la vista combinada de actualización (`internal/tui/screens/upgrade_sync.go`) NO DEBE conservar referencias a `gentle-ai` ni a `Gentle AI`. DEBEN nombrar `axiom`. La semántica de cada estado de la pantalla (confirmación, progreso de `upgrade`, progreso de `sync`, resultado combinado, omisión por reinicio) NO DEBE cambiar.

#### Scenario: Pantalla de confirmación sin branding heredado

- **DADO** la vista combinada de actualización en estado de confirmación
- **CUANDO** se renderiza la pantalla
- **ENTONCES** el texto NO DEBE contener `gentle-ai`
- **Y** DEBE nombrar `axiom`
- **Y** DEBE seguir describiendo las dos operaciones en secuencia

#### Scenario: Resultado de omisión por reinicio sin branding heredado

- **DADO** la vista combinada en estado de resultado con `sync` omitido por reinicio
- **CUANDO** se renderiza la sección de resultados de `sync`
- **ENTONCES** el aviso DEBE nombrar `axiom`
- **Y** NO DEBE contener `gentle-ai`
- **Y** DEBE conservar la instrucción de reinicio

### REQ-22.8: Coherencia de la versión build-time

La versión que el binario `axiom` informa de sí mismo DEBE ser la inyectada en tiempo de compilación por el proceso de release. La inyección por `ldflags` NO DEBE apuntar a un símbolo inexistente y NO DEBE quedar sin efecto. En compilaciones sin inyección, el valor por defecto DEBE seguir siendo el marcador de desarrollo actual.

#### Scenario: Release inyecta la versión real

- **DADO** una compilación de release con inyección de versión
- **CUANDO** se ejecuta `axiom version`
- **ENTONCES** la versión mostrada DEBE ser la inyectada
- **Y** NO DEBE ser el valor por defecto de desarrollo

#### Scenario: Compilación local sin inyectar conserva el marcador de desarrollo

- **DADO** una compilación local sin inyección de versión
- **CUANDO** se ejecuta `axiom version`
- **ENTONCES** la versión mostrada DEBE ser el marcador de desarrollo
- **Y** el formato de salida NO DEBE cambiar

### REQ-22.9: Registro durable de la versión base de upstream

El estado del ecosistema DEBE registrar la versión de upstream contra la que el fork está sincronizado, en el fichero `~/.axiom/state.json`, bajo la clave `upstream_version`. El valor inicial DEBE ser `3.4.0`, correspondiente al techo congelado de `docs/upstream-absorption-ledger.md`. Este registro es material de auditoría manual: el sistema NO DEBE usarlo para disparar sincronizaciones automáticas.

#### Scenario: El estado expone `upstream_version`

- **DADO** un estado de ecosistema escrito por esta versión
- **CUANDO** se lee `~/.axiom/state.json`
- **ENTONCES** el documento DEBE contener `upstream_version`
- **Y** su valor inicial DEBE ser `3.4.0`

#### Scenario: El registro no dispara sincronización

- **DADO** un `state.json` con `upstream_version` poblado
- **CUANDO** se ejecuta `axiom update`, `axiom upgrade` o `axiom sync`
- **ENTONCES** el sistema NO DEBE iniciar comparación ni absorción automática contra upstream

---

## 5. Requerimientos — `axiom-skills-index-governance`

> **Naturaleza:** especificación completa (capacidad nueva).

### REQ-22.10: Contrato CLI `axiom skill index`

El sistema DEBE exponer `axiom skill index [refresh|list]` conforme al contrato de §1.1, con las flags, salidas, códigos de salida y comportamiento en error allí definidos.

#### Scenario: `refresh` regenera y reporta

- **DADO** un workspace que es raíz de proyecto y contiene skills
- **CUANDO** se ejecuta `axiom skill index refresh`
- **ENTONCES** el comando termina con código `0`
- **Y** informa del número de skills y de la ruta del registro
- **Y** informa del resultado de cada destino

#### Scenario: `list --json` devuelve el índice sin escribir

- **DADO** un workspace con skills indexables
- **CUANDO** se ejecuta `axiom skill index list --json`
- **ENTONCES** la salida estándar es un array JSON con `name`, `scope`, `description`, `path` por skill
- **Y** no se escribe registro, caché, `AGENTS.md`, `.gitignore` ni memoria

#### Scenario: Subcomando o bandera inválidos abortan sin efectos

- **DADO** cualquier workspace
- **CUANDO** se ejecuta `axiom skill index` sin subcomando, con un subcomando desconocido o con una bandera desconocida
- **ENTONCES** el código de salida es `1`
- **Y** el mensaje de error nombra el valor rechazado y las opciones válidas
- **Y** no se ha escrito ningún fichero

#### Scenario: No-raíz de proyecto se omite sin error bajo `--quiet`

- **DADO** un directorio que no es raíz de proyecto
- **CUANDO** se ejecuta `axiom skill index refresh --quiet`
- **ENTONCES** el código de salida es `0`
- **Y** no hay salida estándar
- **Y** no se crea ningún fichero

### REQ-22.11: Regeneración unificada del índice en tres destinos

La regeneración del índice (mediante `axiom skill index refresh` o por el gancho de REQ-22.13) DEBE producir, a partir de un único escaneo, los tres destinos:

1. `.atl/skill-registry.md` con los metadatos y rutas exactas (comportamiento existente, conservado).
2. El tópico persistente `skill-registry` en Engram (`type: config`), contenido ya existente si lo hay.
3. La sección `## Skills` de `AGENTS.md`, sustituida de forma atómica conforme a §3.

La caché de huella existente (`.atl/.skill-registry.cache.json`) DEBE seguir permitiendo omitir la regeneración cuando el conjunto de skills no cambia, salvo con `--force`. El fallo del espejo Engram NO DEBE convertir una regeneración local correcta en fallo: DEBE reportarse como `mirror failed` y el comando DEBE terminar con `0`.

#### Scenario: Un escaneo alimenta los tres destinos

- **DADO** un workspace raíz con skills de proyecto y de usuario
- **CUANDO** se ejecuta `axiom skill index refresh`
- **ENTONCES** `.atl/skill-registry.md` refleja el conjunto escaneado
- **Y** `AGENTS.md` refleja el mismo conjunto en su sección `## Skills`
- **Y** el tópico Engram `skill-registry` queda actualizado

#### Scenario: Sin cambios, la huella evita trabajo

- **DADO** un índice ya regenerado y un conjunto de skills sin cambios
- **CUANDO** se ejecuta `axiom skill index refresh` sin `--force`
- **ENTONCES** el comando reporta el motivo de omisión (`cache-hit`)
- **Y** los tres destinos permanecen byte-idénticos

#### Scenario: El espejo Engram falla sin arrastrar el índice local

- **DADO** un workspace raíz y un Engram MCP inaccesible
- **CUANDO** se ejecuta `axiom skill index refresh`
- **ENTONCES** el registro y `AGENTS.md` quedan regenerados
- **Y** el resultado declara `mirror failed`
- **Y** el código de salida es `0`

#### Scenario: Filtro de exclusiones conservado

- **DADO** un workspace con skills `_shared`, `skill-registry`, `sdd-*` y una skill válida
- **CUANDO** se regenera el índice
- **ENTONCES** las skills excluidas NO DEBEN aparecer en ningún destino
- **Y** la skill válida DEBE aparecer en los tres

### REQ-22.12: Reemplazo atómico y marcado de `## Skills` en `AGENTS.md`

La sustitución de la sección `## Skills` de `AGENTS.md` DEBE cumplir el contrato de marcadores, contenido, formato de tabla, adopción inicial e invariantes de §3. El motor `filemerge` DEBE reutilizarse sin modificar.

#### Scenario: Adopción inicial sobre un `AGENTS.md` sin marcadores

- **DADO** un `AGENTS.md` con la sección `## Skills` sin marcadores `<!-- axiom:skills-index -->`
- **CUANDO** se regenera el índice por primera vez
- **ENTONCES** la región `## Skills` existente queda sustituida in situ
- **Y** queda envuelta en el par de marcadores canónico
- **Y** no aparece una segunda sección `## Skills`

#### Scenario: Regeneraciones posteriores son idempotentes

- **DADO** un `AGENTS.md` ya adoptado con marcadores y un conjunto de skills sin cambios
- **CUANDO** se regenera el índice dos veces
- **ENTONCES** el contenido de `AGENTS.md` es byte-idéntico antes y después

#### Scenario: Ninguna otra sección se altera

- **DADO** un `AGENTS.md` con cabecera, reglas, índice de skills, `## How to Use` y `## Skills`
- **CUANDO** se regenera el índice
- **ENTONCES** todo el contenido fuera de los marcadores permanece byte-idéntico

#### Scenario: Marcadores legados se elevan al canónico

- **DADO** un `AGENTS.md` con el par legado `<!-- gentle-ai:skills-index -->`
- **CUANDO** se regenera el índice
- **ENTONCES** el par resultante usa `<!-- axiom:skills-index -->`
- **Y** el contenido gestionado queda actualizado

### REQ-22.13: Disparo automático del índice desde autoskill

Cuando `Manager.Approve()` promueve con éxito una skill desde el buzón transitorio `.axiom/skills/inbox/<nombre>/` hacia `skills/<nombre>/`, el sistema DEBE regenerar automáticamente el índice de skills de forma que la skill recién promovida quede disponible de inmediato en `AGENTS.md`, `.atl/skill-registry.md` y Engram. Un fallo de la regeneración NO DEBE revertir una promoción ya completada.

#### Scenario: La promoción deja la skill indexada sin pasos manuales

- **DADO** una skill propuesta en el buzón transitorio
- **CUANDO** el usuario ejecuta `axiom skill approve <nombre>` y la promoción completa
- **ENTONCES** `skills/<nombre>/` contiene la skill
- **Y** la skill aparece en `AGENTS.md`, `.atl/skill-registry.md` y el tópico Engram
- **Y** el buzón ya no contiene la propuesta

#### Scenario: La promoción no se revierte si falla la regeneración

- **DADO** una promoción de skill completada
- **CUANDO** la regeneración del índice falla
- **ENTONCES** la skill permanece promovida en `skills/<nombre>/`
- **Y** el fallo de indexación queda reportado
- **Y** la promoción NO DEBE deshacerse

#### Scenario: `Reject` no dispara regeneración

- **DADO** una propuesta en el buzón transitorio
- **CUANDO** el usuario ejecuta `axiom skill reject <nombre>`
- **ENTONCES** la propuesta se elimina del buzón
- **Y** no se modifica ningún destino del índice

### REQ-22.14: Compatibilidad del verbo `skill-registry`

El verbo `axiom skill-registry <refresh|list>` DEBE seguir existiendo, enrutado y aceptando sus flags actuales (`--force`/`-f`, `--quiet`/`-q`, `--no-gitignore`, `--cwd`, `--json`), porque consumidores automatizados lo invocan por su argv literal (por ejemplo el plugin `skill-registry.ts`, que ejecuta `skill-registry refresh --quiet --no-gitignore --cwd <ruta>`). Su retirada o renombrado NO DEBE producirse en este incremento. DEBE compartir el mismo motor que `axiom skill index`.

#### Scenario: El argv del plugin sigue resolviendo

- **DADO** un binario con este incremento aplicado
- **CUANDO** se invoca `skill-registry refresh --quiet --no-gitignore --cwd <ruta>`
- **ENTONCES** el comando se acepta y termina con `0`
- **Y** regenera el registro con el mismo motor

#### Scenario: `list` de compatibilidad conserva su forma de salida

- **DADO** un workspace con skills
- **CUANDO** se invoca `axiom skill-registry list --json`
- **ENTONCES** la salida es un array JSON con `name`, `scope`, `description`, `path`

---

## 6. Efectos sobre especificaciones vivas

| Especificación viva | Efecto | Detalle |
|---|---|---|
| `tui-ui-parity` | **Modificada** | Su requerimiento de endpoints `/api/ecosystem/sync` y `/api/ecosystem/upgrade` describe `upgrade` como equivalente a `axiom upgrade` solo. REQ-22.4 lo cambia a la secuencia `upgrade` → `sync` con reporte consolidado. `sdd-archive` DEBE reflejar este cambio en la especificación viva. |
| `axiom-user-state-and-env` | Sin cambio de requisito | REQ-22.9 añade una clave al `state.json` ya gobernado por ella; la raíz y la migración legada no cambian. Se documenta como adición compatible. |
| `axiom-skills-index-governance` | **Nueva** | Promovida desde este cambio por `sdd-archive`. |
| `axiom-updater-resilience` | **Nueva** (pese a figurar como "modificada" en la propuesta) | Ver divergencia D-2. |

---

## 7. Fuera de alcance (explícito)

- **Framework automatizado de sincronización con upstream (Gentle-AI):** descartado expresamente por preferencia del usuario. La absorción de novedades de upstream sigue siendo un proceso manual y guiado bajo demanda, evaluado caso a caso. REQ-22.9 registra el suelo de versión; **no** lo consume ninguna automatización.
- **Modificaciones al motor `filemerge`:** se reutiliza íntegro y sin tocar (`internal/components/filemerge`), incluidos `InjectMarkdownSection` y `WriteFileAtomic`.
- **Limpieza de branding más allá de `internal/tui/screens/upgrade_sync.go`:** los residuos enumerados en D-9 (`internal/tui/screens/upgrade.go`, `internal/update/instructions.go`, `internal/update/upgrade/strategy.go`, `internal/app/help.go`, `internal/app/app.go`, `internal/skillregistry/registry.go`) quedan fuera de este incremento y se anotan como deuda conocida.
- **Retirada o renombrado de `axiom skill-registry`:** explícitamente prohibida en este incremento (REQ-22.14).
- **Creación de `AGENTS.md` donde no exista:** prohibida (§3.6).
- **Publicación de binarios `.zip`/`.exe` en GitHub Releases:** no abordada aquí. REQ-22.1 exige un comportamiento seguro ante su ausencia, no su publicación.
