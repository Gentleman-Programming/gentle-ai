# Tareas: Actualizador Autónomo de Axiom, Sincronización y Gobernanza del Índice Unificado de Skills (inc-22-axiom-updater-and-skills-index-governance)

> **Fuentes de alcance:** `openspec/changes/inc-22-axiom-updater-and-skills-index-governance/spec.md` (REQ-22.1–REQ-22.14, 36 escenarios), `openspec/changes/inc-22-axiom-updater-and-skills-index-governance/design.md` (D-01 a D-14, hallazgos H-1 a H-6, rebanadas S1–S9, §6 estrategia de pruebas, §7 amenazas T-1 a T-9, §11 trazabilidad), `openspec/changes/inc-22-axiom-updater-and-skills-index-governance/proposal.md` como contexto de intención.
> **Precedente de forma:** `openspec/changes/archive/2026-09-22-inc-21-upfront-flow-governance/tasks.md` (read-only).
> **Idioma del artefacto:** español (castellano), registro neutro y profesional, tuteo. Identificadores Go, rutas, comandos CLI, banderas, claves JSON y nombres de test permanecen en inglés.
> **Decisión O-1 cerrada por el orquestador (no reabrir):** el símbolo de versión es `var version = "v0.1.0"` en `cmd/axiom`, con `var Version = version` como nombre exportado que `cli.AppVersion`, `app.Version` y `main_test.go` ya esperan. Las tareas de la Fase 8 se redactan con ese valor ya fijado.
> **Kickoff sellado (REQ-21.4, ya ejecutado):** `execution_style: continuous`, `handoff_policy: none`, rol `fullstack:blocking` con `tasksFile: tasks.md` y `verifyFile: verify-report.md`. Este documento es por tanto `tasks.md` (singular).
> **Frontera normativa (design.md §1.3, no se repite en cada tarea):** ninguna tarea de este documento modifica `internal/components/filemerge/**`. Toda la semántica del dominio de skills vive en `internal/skillregistry`. Ninguna tarea retira ni renombra `axiom skill-registry`.

---

## Resumen ejecutivo del pronóstico (detalle completo al final del documento)

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
Estimated changed lines: ~4.800–7.200
```

Estimación agregada: **~4.800–7.200 líneas cambiadas de autoría (adiciones + eliminaciones, excluidos generados)** repartidas en **22 PRs apiladas contra `main`** (una por fase). Ninguna fase se planifica por encima de ~450 líneas; las marcadas `Medium-High` llevan una **válvula de alivio** explícita por si el diff real crece más de lo previsto. La única razón por la que `Decision needed before apply` es `Yes` (pese a la estrategia de entrega `auto-chain`) es la decisión de producto **O-2**, que condiciona el contenido de la columna `Path` de la tabla `## Skills` y por tanto las Fases 10 y 12.

---

## Nota de partición de rebanadas (ajuste sobre el diseño)

El diseño mide **9 rebanadas** (S1–S9) y deja en §4 la planificación de PRs y el ajuste a la política de 400 líneas como incumbencia de `sdd-tasks`. Contrastadas con el mapa de ficheros de §4, cinco rebanadas agrupan responsabilidades independientes que juntas superan el presupuesto:

| Rebanada del diseño | Por qué no cabe en una PR | Partición final en este documento |
|---|---|---|
| **S1** (identidad, módulo declarado, instrucciones derivadas, `init()` campo a campo) | Cinco ficheros de producción con tres responsabilidades verificables por separado (predicado de identidad, composición de mensajes, mutación de registro) más cuatro tablas de test | **Fase 1** (identidad + módulo) → **Fase 2** (instrucciones derivadas) → **Fase 3** (`init()` campo a campo) |
| **S2** (vía resiliente Windows + salvaguardas) | El preflight compartido, la compilación desde clon y el rescate de salvaguardas en `strategy.go` son tres historias con sus propios vectores adversariales (T-5, T-2/T-7) | **Fase 4** (preflight + enum) → **Fase 5** (`sourceBuildUpgrade` + ruteo) → **Fase 6** (salvaguardas de `strategy.go`) |
| **S5** (motor de índice, tres destinos, adopción de marcadores) | Tipos/puerto/renderizador, adopción de marcadores (con los cinco casos de corrupción de T-4) y cableado de destinos son tres unidades independientes | **Fase 10** (tipos + puerto + tabla) → **Fase 11** (adopción) → **Fase 12** (tres destinos) |
| **S6** (cliente MCP `SaveTopic`) | El camino feliz y los cuatro modos de fallo de T-3 exigen presupuestos y terminación de hijo distintos | **Fase 13** (handshake + éxito) → **Fase 14** (modos de fallo) |
| **S7** (CLI `skill index` + compatibilidad) | El motor/parsers compartidos y el enrutado/ayuda con su compuerta de compatibilidad son verificables por separado | **Fase 16** (`runSkillIndex` + delegación) → **Fase 17** (enrutado + ayuda) |
| **S9** (cadena Web UI + `upstream_version`) | Reporte estructurado con su compuerta de control, DTO/cadena/servidor, presentación web y el campo durable son cuatro fronteras de reversión distintas | **Fase 19** (reporte + compuerta solo-binario) → **Fase 20** (DTO + cadena) → **Fase 21** (presentación web) → **Fase 22** (`upstream_version`) |

**S3** (Fase 7), **S4** (Fase 8) y **S8** (Fase 18) caben cada una en una PR. La **Fase 9** es un hito de decisión (O-2), sin código. La **Fase 15** es una guarda estructural de alcance (solo test), colocada tras S6 para poder afirmar que `internal/skillregistry` no importa `internal/components/engram`.

Resultado: **22 fases** en vez de 9 rebanadas. El orden de §8.2 del diseño se conserva literalmente: S1 → S2 → S3 → S4 → S5 → S6 → S7 → S8 → S9, con la Fase 9 como compuerta humana antes de S5 y la Fase 15 insertada tras S6.

---

## DECISIÓN PENDIENTE (O-2) — Formato de la columna `Path` de la tabla `## Skills`

**Esta es una decisión de producto humana. No la resuelve este documento ni `sdd-apply`.** Debe registrarse la elección antes de ejecutar la Fase 10 (primera tarea de S5 que renderiza la columna `Path`). No bloquea las Fases 1–8 ni 13–14 (S6), pero sí todo S5 y, por derivación, S7–S9.

El conflicto real, verificado en el diseño (H-5 y O-2): `spec.md` §3.4 muestra `ruta/relativa/SKILL.md` para scope `project`, y a la vez exige «Alineado con `.atl/skill-registry.md`», cuyo renderizador emite hoy la **ruta descubierta** (absoluta salvo que `--cwd` sea relativa), que es además la «ruta exacta» con la que habla la propuesta.

| Alternativa | Qué implica | Consecuencias |
|---|---|---|
| **A — Ruta descubierta** (absoluta, alineada con `.atl/skill-registry.md` (read-only) y con la «ruta exacta» de la propuesta) | `renderSkillsTable` publica **el mismo** valor que `skillregistry` descubre, en los tres destinos (`AGENTS.md` (read-only), `.atl/skill-registry.md` (read-only), `axiom skill index list`/`--json`) | + Un único valor por skill en todos los destinos (REQ-22.11 «rutas exactas»). + Cero lógica de relativización. − La tabla de `AGENTS.md` pierde la navegabilidad como enlace relativo en GitHub (riesgo R-3 del diseño). − Los lectores humanos ven rutas largas de máquina. |
| **B — Ruta relativa navegable en GitHub** | La columna `Path` de `AGENTS.md` (read-only) usa rutas relativas navegables para scope `project` | + Conserva la navegabilidad en GitHub. − Hay que relativizar **también** `.atl/skill-registry.md` (read-only) y `list`/`--json`, o declarar una excepción de formato explícita; si no, los destinos divergen y se rompe «un único formato reconocible» (§3.4). − La «ruta exacta» deja de ser idéntica entre destinos salvo regla de resolución documentada. − Añade superficie de resolución de rutas (y con ella vectores de T-1). |

**Tareas del hito:**

- [ ] 9.1 [DECISIÓN PENDIENTE (O-2)] Registrar aquí, con marca de fecha y actor, la elección humana entre la **Alternativa A** y la **Alternativa B** de la tabla superior. Ninguna tarea de las Fases 10–12 se ejecuta hasta que este registro exista.
- [ ] 9.2 [DECISIÓN PENDIENTE (O-2)] Si se elige la Alternativa B, dejar constancia de la regla de relativización (base de la ruta, tratamiento del scope `user`) y de si se acepta una excepción de formato entre destinos; esta regla es entrada obligatoria de `renderSkillsTable` en la Fase 10.
- [ ] 9.3 [DECISIÓN PENDIENTE (O-2)] Propagar la elección a los criterios de aceptación de las Fases 10 y 12 (tests de igualdad de rutas entre destinos) antes de escribir sus tests RED.

---

## Convención de ramas (`stacked-to-main`, según design.md §8.2)

- La estrategia de entrega de la sesión es `auto-chain` y el diseño fija que las rebanadas **se apilan contra `main`** (§8.2: «Las nueve rebana se apilan contra `main`»). No se abre otra pregunta de estrategia: `stacked-to-main` es la cadena cacheada.
- Ramas: `inc-22/<NN>-slug`, con `NN` = número de fase con cero a la izquierda (p. ej. `inc-22/01-update-identity-module`).
- Cada PR se fusiona en `main` en orden estricto (Fase 1 → Fase 22). Antes de abrir/revisar cada PR siguiente, rebase sobre `main` actualizada para que su diff muestre **solo** su rebanada. Si un diff muestra la rebanada anterior, reapuntar o rebasar hasta que quede limpio (defecto de base, no del cambio).
- Las 22 ramas, en orden: `inc-22/01-update-identity-module` · `inc-22/02-update-derived-instructions` · `inc-22/03-update-init-mutation` · `inc-22/04-update-write-preflight` · `inc-22/05-update-source-build` · `inc-22/06-update-strategy-safeguards` · `inc-22/07-tui-identity-branding` · `inc-22/08-version-symbol` · *(sin rama: hito de decisión O-2)* · `inc-22/10-skillregistry-types-table` · `inc-22/11-skillregistry-marker-adoption` · `inc-22/12-skillregistry-three-destinations` · `inc-22/13-engram-save-topic` · `inc-22/14-engram-save-failure-modes` · `inc-22/15-scope-structural-guard` · `inc-22/16-app-skill-index-engine` · `inc-22/17-cli-skill-index-routing` · `inc-22/18-autoskill-approve-hook` · `inc-22/19-app-upgrade-report` · `inc-22/20-dashboard-upgrade-sequence` · `inc-22/21-dashboard-web-phases` · `inc-22/22-state-upstream-version`.

---

## Protocolo de compuertas de control (aplica a las rebanadas marcadas)

El diseño fija cinco compuertas de control que deben observarse en verde **antes y después** del trabajo de su rebanada. No son tests RED convencionales: capturan un contrato vivo que este incremento no debe romper.

1. **Compuerta de la Fase 6 (S2):** ninguna salvaguarda de actualización queda anclada al literal `"gentle-ai"` en `internal/update/upgrade/strategy.go`. Se comprueba con búsqueda sobre el fichero tras el refactor.
2. **Compuerta de la Fase 7 (S3):** `internal/tui/screens/upgrade_sync_test.go` conserva la subcadena `"sync skipped"` y la instrucción de reinicio, en verde antes y después del rebranding (T-9).
3. **Compuerta de la Fase 12 (S5):** idempotencia byte a byte de `AGENTS.md` (read-only) en dos regeneraciones y preservación de todo byte fuera de marcadores.
4. **Compuerta de la Fase 17 (S7):** el argv literal del plugin (`skill-registry refresh --quiet --no-gitignore --cwd <ruta>`) termina `0` y la salida sin `--quiet` de `skill-registry refresh` es idéntica a la de hoy (solo la línea primaria).
5. **Compuerta de la Fase 19 (S9):** `internal/app/upgrade_test.go` (read-only) — `TestRunArgs_UpgradeDryRun` y `TestRunArgs_UpgradeOutput_BinariesOnly` — en verde **sin modificar sus aserciones**, antes y después de S9. Si alguna exigiese cambios para compilar, se eleva como defecto de derivación y no se ajusta en silencio.

Ninguna fase se considera cerrada si su compuerta correspondiente no está en verde.

---

## Reglas de Comprobación y Alcance (aplican a TODAS las fases)

1. **Rutas prohibidas (criterio de aborto, design.md §4.1):** ningún *diff* de ninguna fase puede contener `internal/components/filemerge/**`, `openspec/config.yaml`, `openspec/changes/inc-22-*/proposal.md`, `openspec/changes/inc-22-*/spec.md`, `docs/upstream-absorption-ledger.md`, `internal/multirole/detector.go`, ni el branding heredado fuera de `internal/tui/screens/upgrade_sync.go` (concretamente: el texto de `internal/tui/screens/upgrade.go` en su línea de branding heredado, el de `internal/skillregistry/registry.go` en la cabecera «gentle-ai skill-registry», `internal/app/help.go` y el literal heredado de `internal/app/app.go`). Un *diff* que toque cualquiera de estas rutas se rechaza y se revierte hasta el último estado verde. Nota: `internal/tui/screens/upgrade.go`, `internal/skillregistry/registry.go` y `internal/app/app.go` **sí** se editan en fases concretas; lo prohibido es tocar en ellos los textos de branding heredado enumerados.
2. **Marcado `(read-only)`:** cuando una tarea cita como referencia un fichero que esa fase no crea ni modifica, la ruta lleva `(read-only)` inmediatamente después. Toda ruta sin ese marcador en una línea de casilla cuenta como objetivo de edición de esa fase. Los artefactos que el producto escribe en tiempo de ejecución (`AGENTS.md`, `.atl/skill-registry.md`, `.atl/.skill-registry.cache.json`, `.gitignore`, `~/.axiom/state.json`) **no** son objetivos de edición del incremento: los tests usan fixtures en `t.TempDir()`.
3. **TDD estricto** (`strict_tdd: true` en `openspec/config.yaml`, (read-only)): toda tarea `[RED]` se escribe y se observa en fallo **antes** de la producción que la satisface; después `[GREEN]` y `[REFACTOR]`. Ninguna tarea `[GREEN]` se ejecuta sin que su `[RED]` (o su compuerta de control / caracterización) se haya observado primero. Las tareas de caracterización se marcan `[Caracterización]` y deben pasar en verde contra el árbol sin modificar.
4. **Protocolo de verificación por fase (V-A a V-F), referenciado por número en cada «Verificación de cierre»:**
   - **V-A** — `go build ./...` y `go vet ./...` en verde.
   - **V-B** — `go test ./<paquetes tocados por la fase>/... -timeout 300s` en verde. La verificación es **por paquete afectado**; jamás un único `go test ./...` global como criterio de tarea intermedia.
   - **V-C** — `gofmt -l` sobre los ficheros tocados por la fase, sin salida.
   - **V-D** — El diff no toca ninguna ruta prohibida (regla 1) ni altera el branding heredado acotado.
   - **V-E** — Cuando la fase tiene compuerta de control (6, 7, 12, 17, 19), esa compuerta sigue en verde.
   - **V-F** — Cierre de PR (al cerrar cada fase) y cierre de cadena (Fase 22): `go test ./... -timeout 900s`, con `go test ./internal/update/... -timeout 600s` **aparte** y `go test ./internal/sddstatus/... -timeout 600s` **aparte**.
5. **Fallos ambientales conocidos de este entorno Windows (no se corrigen aquí; se reflejan en la verificación):**
   - `internal/sddstatus` cuelga en `RuntimeStore.Finish` → `SnapshotBuilder.Build` → `runGitCapturedRangeWithTimeout` (`internal/reviewtransaction/snapshot.go`, (read-only)): subproceso git sin timeout efectivo. Este incremento **no toca** ese paquete; solo se invoca en V-F.
   - `internal/update`: `TestNoUpdatesPath` depende de estado de máquina (axiom instalado) y `TestDetectHomebrewOwnershipWith` exige `SeCreateSymbolicLinkPrivilege` sin `t.Skip`. Al ejecutar `go test ./internal/update/...`, estos dos fallos se registran **como fallos ambientales conocidos**; se reejecuta con `-run` acotado al resto y se documenta el resultado real de cada uno. **Nunca** se marca como aprobado por omisión ni se inventa un pase.
   - Cualquier agotamiento de presupuesto (`-timeout`) se registra como **no ejecutado**, nunca como fallo inventado ni como aprobado.
6. **Herméticidad de tests nuevos:** todo test que dependa de git, de un proceso hijo o del sistema de ficheros usa `t.TempDir()`, `git init` local y servidores falsos por stdio, sin red. Los tests de `sourceBuildUpgrade` usan doble de `execCommand`.
7. **Granularidad RED/GREEN:** las tareas RED agrupan la tabla de casos de un mismo fichero o preocupación, en vez de una tarea por función individual (mismo criterio que INC-21).
8. **Etiquetas de PR:** `type:feat`, `type:test` o `type:refactor` según corresponda, más `area:ecosystem` (actualizador y estado) o `area:skills` (índice y autoskill) o `area:dashboard` (cadena Web UI).

---

## Fase 1: S1a — Identidad del self-tool y módulo declarado (REQ-22.2, REQ-22.3; D-01)

Aditivo puro en `internal/update`. Los literales upstream de los mensajes se rescandan en la Fase 2; las salvaguardas de `strategy.go` en la Fase 6.

- [x] 1.1 [RED] Escribir `internal/update/identity_test.go`: tabla de `IsSelfToolName` — acepta `axiom`, `gentle-ai`, `AXIOM`, ` Gentle-AI ` (insensible a caja y a espacios de borde) y rechaza `engram`, `gga`, `""`, `Ax1om`; `IsSelfTool` sobre `ToolInfo{Name: "axiom"}` y `ToolInfo{Name: "gentle-ai"}` ⇒ `true`, sobre `ToolInfo{Name: "engram"}` ⇒ `false`. Los símbolos no existen todavía: RED por fallo de compilación.
- [x] 1.2 [RED] En `internal/update/identity_test.go` (o `internal/update/module_test.go`): tabla de `GoInstallResolvable()` — `true` cuando `GoImportPath` está bajo `GoModulePath` (par upstream intacto: `github.com/gentleman-programming/gentle-ai/v3` + `.../v3/cmd/gentle-ai`); `true` con igualdad exacta; `false` para el par fork (`GoModulePath` de upstream + `GoImportPath` `github.com/IGutierrezZ/axiom/cmd/axiom`); `false` con cualquiera de los dos vacío; `false` cuando `GoModulePath` es solo un prefijo parcial (`github.com/gentleman-programming/gentle-ai` sin `/v3`) porque `.../gentle-ai` no es prefijo de `.../gentle-ai/v3/...` con la regla de `D-01`.
- [x] 1.3 [GREEN] Modificar `internal/update/types.go`: `selfToolNames` (conjunto cerrado `{"axiom", "gentle-ai"}` con la invariante documentada en comentario GoDoc en inglés), `IsSelfToolName`, `IsSelfTool`, campo `ToolInfo.GoModulePath` y método `ToolInfo.GoInstallResolvable()` tal como fija `D-01`. Criterio de aceptación: `GoInstallResolvable` **nunca** deriva el módulo de `Owner`/`Repo`; solo lee el módulo declarado.
- [x] 1.4 [GREEN] Modificar `internal/update/registry.go`: la entrada self-tool declara `GoModulePath: "github.com/gentleman-programming/gentle-ai/v3"` (literal exacto de `go.mod` (read-only)), **sin** renombrar la entrada ni reordenar campos existentes.
- [x] 1.5 [REFACTOR] Confirmar que ningún literal `"gentle-ai"` nuevo queda fuera de `selfToolNames` y que `types.go` no duplica la lógica de identidad en predicados sueltos.
- [x] 1.6 [Verificación de cierre] V-A; V-B (`go test ./internal/update/... -run 'TestIsSelfTool|TestGoInstallResolvable' -timeout 300s`, registrando por separado los fallos ambientales conocidos de la regla 5); V-C; V-D.

## Fase 2: S1b — Instrucciones de instalación derivadas de `ToolInfo` (REQ-22.2; D-04)

Elimina la tercera discrepancia de H-1 (frente del mensaje manual). Depende de la Fase 1.

- [x] 2.1 [RED] Escribir `internal/update/source_install_test.go`: tabla de `SourceInstallCommand(tool, version)` — con `GoInstallResolvable()==true` emite `go install <GoImportPath>@<target>`; con `false` emite una instrucción de clon + build que nombra `https://github.com/<Owner>/<Repo>` y `./cmd/<Name>` y contiene **cero** `go install`; targets `v<X>`, `main` (canal beta) y vacío (último release); para la herramienta registrada como fork ninguna instrucción contiene `gentleman-programming/gentle-ai` ni `cmd/gentle-ai` (escenarios de REQ-22.2 «La pista de actualización nombra el fork, no upstream» y «Discrepancia de módulo detectada, sin `go install` abortado a mitad»).
- [x] 2.2 [RED] Escribir `internal/update/instructions_test.go`: escaneo de la salida real de `updateHint` y `gentleAIHint` parametrizados con la entrada del fork ⇒ ninguna cadena contiene `Gentleman-Programming/gentle-ai` ni `cmd/gentle-ai`; con la entrada upstream intacta, el mensaje sigue siendo coherente con su `ToolInfo`.
- [x] 2.3 [GREEN] Modificar `internal/update/instructions.go`: sustituir `GentleAISourceInstallCommand` por `SourceInstallCommand(tool ToolInfo, version string) string` (D-04, dos ramas: `go install` solo si `GoInstallResolvable()`, clon + build en caso contrario); parametrizar `updateHint`/`gentleAIHint` por `tool`; derivar `brew upgrade <Name>`, `homebrewPackageInstalled(tool.Name)` y `raw.githubusercontent.com/<Owner>/<Repo>/main/scripts/install.sh` de `ToolInfo`. **Eliminar** `GentleAISourceInstallCommand` sin envoltorio deprecado (D-04).
- [x] 2.4 [GREEN] Modificar `internal/update/check.go`: `isGentleAIRepo` delega en `IsSelfToolName`/`IsSelfTool`; `applyBetaMainHeadStatus` compone la instrucción vía `SourceInstallCommand`.
- [x] 2.5 [GREEN] Modificar `internal/update/check_test.go`: el caso que hoy cita `GentleAISourceInstallCommand` pasa a la API nueva; ninguna aserción sobre la identidad del fork se debilita.
- [x] 2.6 [REFACTOR] Confirmar que ninguna función de `internal/update` recibe `owner, repo, name` sueltos para componer mensajes: todo deriva de `ToolInfo`.
- [x] 2.7 [Verificación de cierre] V-A; V-B (`go test ./internal/update/... -run 'TestSourceInstall|TestInstructions|TestUpdateHint|TestCheck|TestGentleAI' -timeout 300s`, con la regla 5 para los dos tests ambientales); V-C; V-D.

## Fase 3: S1c — Mutación campo a campo de `init()` en `cmd/axiom` (REQ-22.3; D-01)

Depende de la Fase 1 (el campo `GoModulePath` debe existir para poder perderlo). Cierra el defecto del reemplazo de struct completo que hoy hace `init()`.

- [ ] 3.1 [RED] Escribir `cmd/axiom/init_registry_test.go`: tras la inicialización del paquete, la entrada self-tool de `update.Tools` tiene `Name: "axiom"`, `Owner: "IGutierrezZ"`, `Repo: "axiom"`, `GoImportPath: "github.com/IGutierrezZ/axiom/cmd/axiom"`, `DetectCmd == nil`, `VersionPrefix: "v"` y —clave de este test— **`GoModulePath` conservado** en `github.com/gentleman-programming/gentle-ai/v3` (hoy el reemplazo de struct completo lo vaciaría: RED). Además, `IsSelfToolName` sigue reconociendo `axiom` y `gentle-ai` tras la mutación.
- [ ] 3.2 [GREEN] Modificar `cmd/axiom/main.go`: `init()` pasa a **mutación campo a campo** sobre cada `update.Tools[i]` cuyo nombre cumpla `update.IsSelfToolName`, preservando `GoModulePath` y cualquier campo futuro (D-01).
- [ ] 3.3 [REFACTOR] Confirmar que `init()` no contiene literales de ruta de módulo más allá de `GoImportPath` del fork y que la invariante de «mutación campo a campo» está comentada en inglés.
- [ ] 3.4 [Verificación de cierre] V-A; V-B (`go test ./cmd/axiom/... ./internal/update/... -run 'TestInit|TestIsSelfTool|TestAppVersion' -timeout 300s`); V-C; V-D.

## Fase 4: S2a — Preflight compartido de escritura del binario y enum `InstallSourceBuild` (REQ-22.1, REQ-22.3; D-03; T-5)

Independiente de la Fase 2 y 3 salvo por `ToolInfo`. La garantía es **una** («no crear un segundo binario visible en PATH»), compartida por toda vía que escriba el binario.

- [ ] 4.1 [RED] Escribir `internal/update/upgrade/write_preflight_test.go`: tabla de `preflightWindowsSelfBinaryWrite` — destino (`GOBIN`/`GOPATH`/`go env`) y activo (`lookPathFn(tool.Name)`) ambos resolubles y distintos ⇒ `ManualFallbackError` que **nombra las dos rutas** y cómo migrar de forma intencionada, con **cero** escrituras (escenario REQ-22.1 «El reemplazo del binario no deja duplicados en PATH»); activo irresoluble ⇒ `ManualFallbackError`; destino irresoluble ⇒ `ManualFallbackError`; caso feliz (misma ruta) ⇒ sin error. Se afirma que la **misma** función se invoca desde las dos vías de escritura [T-5].
- [ ] 4.2 [RED] Escribir `internal/update/upgrade/install_method_test.go`: `InstallSourceBuild` forma parte del enum `InstallMethod` y es un valor distinto de `InstallGoInstall`/`InstallBinary`.
- [ ] 4.3 [GREEN] Modificar `internal/update/types.go`: añadir `InstallSourceBuild` al enum `InstallMethod`.
- [ ] 4.4 [GREEN] Modificar `internal/update/upgrade/go_install_destination.go`: generalizar `preflightWindowsGentleAIGoInstallWithDestination` a `preflightWindowsSelfBinaryWrite(tool, profile)`, activa para el self-tool (por `IsSelfTool`) y para toda vía que escriba el binario [D-03]. Se conservan sin cambiar de semántica `sameBinaryPathForOS`, `goInstallDestinationDir`, `goInstallBinaryName` y `absoluteBinaryPath`.
- [ ] 4.5 [REFACTOR] Confirmar que no queda ninguna segunda implementación de la misma garantía de procedencia y que los nombres de error citan siempre ambas rutas.
- [ ] 4.6 [Verificación de cierre] V-A; V-B (`go test ./internal/update/upgrade/... -run 'TestPreflight|TestWritePreflight|TestInstallMethod' -timeout 300s`); V-C; V-D.

## Fase 5: S2b — `sourceBuildUpgrade` y ruteo Windows (REQ-22.1; D-02; T-2, T-7)

Depende de la Fase 4 (preflight compartido) y de la Fase 1 (`GoInstallResolvable`). Es la vía resiliente real de Windows: compilación controlada desde clon (H-2 descarta los binarios de Releases).

- [ ] 5.1 [RED] Escribir `internal/update/upgrade/method_routing_test.go`: tabla de `effectiveMethod` — self-tool + Windows + `!GoInstallResolvable()` ⇒ `InstallSourceBuild`; self-tool + Windows + `GoInstallResolvable()` ⇒ `InstallGoInstall`; self-tool + linux ⇒ `InstallBinary`; herramienta ajena ⇒ sin cambios respecto a hoy.
- [ ] 5.2 [RED] Escribir `internal/update/upgrade/source_build_test.go` con doble de `execCommand` y `t.TempDir()` (sin red): tag exacto elegido (`refs/tags/v<LatestVersion>`); canal beta usa `refs/heads/main` (una sola implementación para release y beta, D-02); `git` ausente del `PATH` ⇒ manual sin tocar nada; `go` ausente ⇒ manual; `go build` con salida distinta de 0 ⇒ manual **sin binario a medias**; contexto cancelado ⇒ manual; el reemplazo atómico ocurre **solo** tras preflight OK; clon en directorio temporal propio e impredecible, nunca en el árbol del usuario; tag `v9.9.9` inexistente ⇒ fallo de `git fetch` ⇒ manual sin residuo [T-2, T-7].
- [ ] 5.3 [GREEN] Crear `internal/update/upgrade/source_build.go`: `sourceBuildUpgrade(ctx, r, profile, targetRef)` — (1) preflight de procedencia; (2) `git clone --depth 1` del tag exacto en temporal impredecible (patrón de `ggaScriptUpgradeForOS` de `internal/update/upgrade/strategy.go`, (read-only) como precedente de forma); (3) `go build -trimpath -o <tmp>/<binaryName> ./cmd/<Name>` dentro del clon; (4) reemplazo atómico del binario activo (reutilizando el mecanismo de reemplazo atómico existente); (5) cualquier fallo de 1–4 antes del paso 4 ⇒ `ManualFallbackError` accionable que nombra `IGutierrezZ/axiom` y la vía manual correcta, cero ficheros tocados (escenarios REQ-22.1 «Actualización disponible en Windows con estrategia automatizada válida» y «Sin estrategia automatizada válida, degradación a manual sin mutación»).
- [ ] 5.4 [GREEN] Modificar `internal/update/upgrade/executor.go`: `effectiveMethod` implementa la tabla de 5.1. Modificar `internal/update/upgrade/strategy.go` solo en el `case update.InstallSourceBuild` que invoca `sourceBuildUpgrade` (el rescate de salvaguardas del fichero es la Fase 6).
- [ ] 5.5 [REFACTOR] Confirmar que `sourceBuildUpgrade` usa argv como *slice* (nunca cadena de *shell* ni interpolación de entrada del usuario), presupuesto de contexto duro y salida capturada con límite (T-2).
- [ ] 5.6 [Verificación de cierre] V-A; V-B (`go test ./internal/update/upgrade/... -run 'TestSourceBuild|TestEffectiveMethod|TestRouting' -timeout 300s`); V-C; V-D.

## Fase 6: S2c — Salvaguardas de actualización ancladas a la identidad del fork (REQ-22.2, REQ-22.3; D-01, D-04)

Depende de las Fases 1 y 5. Rescata las salvaguardas hoy inertes por el renombrado de `init()` (H-1).

> **Válvula de alivio:** si el diff real supera holgadamente 400 líneas, separar los tests de escaneo de mensajes de `strategy.go` en una PR inmediatamente posterior (`inc-22/06b-update-strategy-message-scan`), reapuntando la Fase 7 a esa PR.

- [ ] 6.1 [RED] Escribir `internal/update/upgrade/strategy_identity_test.go`: escaneo de la salida real de `scriptUpgrade`, `binaryUpgrade`, el fallback por defecto de `runStrategy`, `gentleAIWindowsSourceInstallHint` y `goInstallMainUpgrade` parametrizados con la herramienta registrada como fork ⇒ ninguna cadena contiene `Gentleman-Programming/gentle-ai` ni `cmd/gentle-ai`; en Windows con self-tool, `binaryUpgrade` **no** cae en el fallback genérico que compone `https://github.com/Gentleman-Programming/%s/releases` (H-1) y toda URL derivada nombra `IGutierrezZ/axiom`; `goInstallMainUpgrade` solo emite `go install` si `GoInstallResolvable()` (REQ-22.2: el `go install` no resoluble **no se ejecuta**).
- [ ] 6.2 [RED] En el mismo fichero: tabla de la detección de canal beta — `isBetaGentleAIUpgrade` pasa a predicado de identidad (p. ej. `IsSelfBetaUpgrade`) que detecta el canal para `axiom` **y** para `gentle-ai`, y el bloqueo de distribución de Windows (o su equivalente del fork) se dispara con la identidad `axiom` (escenarios REQ-22.3).
- [ ] 6.3 [GREEN] Modificar `internal/update/upgrade/strategy.go`: todo anclaje al literal `"gentle-ai"` (`binaryUpgrade`, `preflightWindowsGentleAIGoInstall*` —ya renombradas en la Fase 4—, detección de beta) pasa a `IsSelfTool`/`IsSelfToolName`; las URLs se derivan de `tool.Owner`/`tool.Repo`; `gentleAIModulePath` devuelve `tool.GoModulePath` **declarado** en vez de calcular `<Owner>/<Repo>/v3`; `goInstallMainUpgrade` compone `tool.GoImportPath + "@main"` solo si `GoInstallResolvable()`; el fallback genérico de Windows nombra `<Owner>/<Repo>`.
- [ ] 6.4 [REFACTOR] Confirmar que la verificación de procedencia destino/activo es la de la Fase 4 (una sola) y que ningún mensaje vuelve a nombrar un propietario ajeno al fork.
- [ ] 6.5 [Verificación de cierre] V-A; V-B (`go test ./internal/update/upgrade/... -timeout 300s`, con foco en `-run 'TestStrategy|TestBeta|TestBinaryUpgrade|TestGoInstall'`); V-C; V-D; V-E — **compuerta 1**: búsqueda sobre `internal/update/upgrade/strategy.go` sin ningún anclaje de salvaguarda al literal `"gentle-ai"`.

## Fase 7: S3 — Identidad TUI y branding de la vista combinada (REQ-22.6, REQ-22.7; D-08; H-4; T-9)

Independiente de las Fases 4–6 salvo por el predicado de la Fase 1. Cierra el frente TUI de H-4, imprescindible para que el escenario «TUI omite `sync` y pide reinicio» sea alcanzable.

- [ ] 7.1 [RED] Modificar `internal/tui/screens/upgrade_sync_test.go`: la subcadena `"sync skipped"` y la instrucción de reinicio siguen presentes (T-9: la regla de salto no se relaja con el rebranding); los textos de la vista combinada no contienen `gentle-ai` ni `Gentle AI` y nombran `axiom`; la semántica de cada estado (confirmación, progreso de `upgrade`, progreso de `sync`, resultado combinado, omisión por reinicio) queda cubierta por aserciones **antes y después** del cambio de textos.
- [ ] 7.2 [RED] Escribir `internal/tui/self_tool_identity_test.go`: los tres sitios de H-4 —`internal/tui/model.go` (detección de reemplazo del binario en ejecución y `GentleAIUpgradeVersion`), `internal/tui/screens/upgrade.go`— detectan el self-tool bajo el nombre `axiom` **y** bajo `gentle-ai` (tabla sobre el predicado ya existente de la Fase 1).
- [ ] 7.3 [GREEN] Modificar `internal/tui/model.go` e `internal/tui/screens/upgrade.go`: `result.ToolName == "gentle-ai"` pasa a `update.IsSelfToolName(result.ToolName)` en los tres sitios [D-08]. El nombre del método `GentleAIUpgradeVersion` se conserva (identificador interno; D-08). El texto de branding heredado de `internal/tui/screens/upgrade.go` **no** se toca (fuera de alcance, §4.1).
- [ ] 7.4 [GREEN] Modificar `internal/tui/screens/upgrade_sync.go`: los textos de confirmación, de salto de `sync` y de reinicio nombran `axiom` y conservan su semántica y la subcadena `sync skipped` (REQ-22.7 y su escenario «Resultado de omisión por reinicio sin branding heredado»).
- [ ] 7.5 [REFACTOR] Confirmar que la regla de cuándo se omite `sync` no se duplica aquí: la fuente única llega como `ResolveSyncSkip` en la Fase 19; hasta entonces, la señal de reemplazo del self-tool es la única condición tocada.
- [ ] 7.6 [Verificación de cierre] V-A; V-B (`go test ./internal/tui/... ./internal/tui/screens/... -run 'TestUpgradeSync|TestSelfToolIdentity|TestUpgrade' -timeout 300s`); V-C; V-D; V-E — **compuerta 2** (`upgrade_sync_test.go` en verde antes y después).

## Fase 8: S4 — Símbolo de versión build-time (REQ-22.8; D-05; O-1 cerrada)

Aditivo y reversible de forma aislada. El valor por defecto **ya está fijado por O-1**: `v0.1.0`.

- [ ] 8.1 [RED] Escribir `cmd/axiom/version_test.go`: una compilación sin inyección reporta `v0.1.0` (el marcador de desarrollo preservado por O-1, escenario REQ-22.8 «Compilación local sin inyectar conserva el marcador de desarrollo»); el formato de `printVersion` es exactamente `%s version %s (%s/%s) commit:%s\n`; el formato de salida no cambia.
- [ ] 8.2 [GREEN] Modificar `cmd/axiom/main.go`: sustituir el `const Version = "v0.1.0"` por `var version = "v0.1.0"` (punto de inyección `-X main.version=`, mismo símbolo que `cmd/gentle-ai/main.go` (read-only)) y `var Version = version`; `Platform` y `GitCommit = "dev"` quedan como `const` sin cambios [D-05]. `printVersion`, la asignación de `cli.AppVersion`/`app.Version` y el resto de `main()` no cambian.
- [ ] 8.3 [REFACTOR] Confirmar que `cmd/axiom/main_test.go` (read-only) y `TestAppVersionInitialization` siguen en **verde sin modificar**, y que `.goreleaser.yaml` (read-only) y `ci.yml` (read-only) **no** se tocan: sus `-X main.version=` dejan de ser un no-op por el mero hecho de existir el símbolo.
- [ ] 8.4 [Verificación de cierre] V-A; V-B (`go test ./cmd/axiom/... -run 'TestAppVersionInitialization|TestVersion|TestPrintVersion' -timeout 300s`); V-C; V-D. Ejecución manual opcional: `go run ./cmd/axiom version` muestra `v0.1.0` sin inyección.

## Fase 9: DECISIÓN PENDIENTE (O-2) — Formato de la columna `Path`

Hito de decisión de producto **sin código**, descrito íntegramente en la sección «DECISIÓN PENDIENTE (O-2)» de este documento. Bloquea las Fases 10–12.

- [ ] 9.1 [DECISIÓN PENDIENTE (O-2)] Registrar la elección humana entre Alternativa A (ruta descubierta) y Alternativa B (ruta relativa navegable), con fecha y actor.
- [ ] 9.2 [DECISIÓN PENDIENTE (O-2)] Si procede, fijar la regla de relativización de la Alternativa B (base de la ruta, scope `user`, excepción de formato entre destinos).
- [ ] 9.3 [DECISIÓN PENDIENTE (O-2)] Propagar la elección a los criterios de aceptación de las Fases 10 y 12.

## Fase 10: S5a — Tipos del motor, puerto de espejo y renderizador único de tabla (REQ-22.11, REQ-22.12; D-09, D-11)

Depende de la Fase 9 (O-2 decide el contenido de la columna `Path`). `internal/skillregistry` sigue siendo hoja: el espejo entra por puerto, nunca por import.

- [ ] 10.1 [RED] Escribir `internal/skillregistry/table_test.go`: `renderSkillsTable` produce exactamente el mismo bloque para `RenderRegistry` y para el contenido gestionado de `AGENTS.md` (read-only) («un único formato reconocible», §3.4); `markdownCell` colapsa saltos de línea en espacio, escapa `|` como `\|` y sustituye vacíos por `—`; filas ordenadas por `Skill` ascendente; cabecera fija `| Skill | Trigger / description | Scope | Path |`; la columna `Path` cumple la alternativa elegida en O-2 (con la elección ya registrada en la Fase 9).
- [ ] 10.2 [RED] Escribir `internal/skillregistry/types_test.go`: literales de estado exactos — `updated`, `unchanged`, `omitted`, `mirror ok`, `mirror failed` — y constructura de `Result` con `Regenerated`, `SkillCount`, `Reason`, `Registry`, `Cache`, `Agents DestinationOutcome`, `Mirror MirrorOutcome`. `MirrorFunc` nulo se documenta como `mirror failed` (T-8: el comando **nunca** declara `mirror ok` por una escritura que no intentó).
- [ ] 10.3 [GREEN] Crear `internal/skillregistry/types.go`: `RegenerateOptions{Force, Mirror}`, `DestinationStatus`, `DestinationOutcome`, `MirrorStatus`, `MirrorOutcome`, `Result` [D-09]. Crear `internal/skillregistry/mirror.go`: `MirrorRequest` (con `Project`, `TopicKey: "skill-registry"`, `Type: "config"`, `Title`, `Content`, `CapturePrompt: false`) y `MirrorFunc` [D-11]. Crear `internal/skillregistry/table.go`: `renderSkillsTable` y `markdownCell` (movido desde el renderizado en línea).
- [ ] 10.4 [GREEN] Modificar `internal/skillregistry/registry.go`: `RenderRegistry` invoca `renderSkillsTable` en lugar de emitir su tabla en línea. El resto de `Regenerate` **no** cambia todavía (llega en la Fase 12).
- [ ] 10.5 [REFACTOR] Confirmar que `internal/skillregistry` no importa nada de `internal/components/engram` (el espejo es un campo `MirrorFunc`) y que no hay dos renderizadores de tabla.
- [ ] 10.6 [Verificación de cierre] V-A; V-B (`go test ./internal/skillregistry/... -run 'TestRenderSkillsTable|TestMarkdownCell|TestRegenerateTypes' -timeout 300s`); V-C; V-D.

## Fase 11: S5b — Adopción de marcadores de `## Skills` (REQ-22.12; D-10; T-4)

Depende de la Fase 10 (usa `renderSkillsTable` en el paso siguiente de escritura). El motor `filemerge` se reutiliza **sin modificar** (§1.3, §7 de la spec).

- [ ] 11.1 [RED] Escribir `internal/skillregistry/agents_test.go` sobre `AdoptSkillsIndexMarkers` compuesto con `filemerge.InjectMarkdownSection` (`internal/components/filemerge/section.go`, (read-only)) real: sin par de marcadores + `## Skills` a inicio de línea ⇒ la región (desde la cabecera hasta la línea previa a la siguiente cabecera `## ` a inicio de línea, o EOF) se envuelve **in situ** en el par canónico y **no** aparece una segunda sección `## Skills` (escenario REQ-22.12 «Adopción inicial sobre un `AGENTS.md` sin marcadores»); con par canónico `<!-- axiom:skills-index -->` / `<!-- /axiom:skills-index -->` ⇒ idéntico; con par legado `<!-- gentle-ai:skills-index -->` ⇒ elevación al canónico por `InjectMarkdownSection`; sin par ni cabecera ⇒ idéntico (el anexado al final lo hace `InjectMarkdownSection`); `## Skills Index` **no** cuenta como `## Skills`; `## Skills` a mitad de línea **no** cuenta; con dos cabeceras `## Skills`, solo la primera se envuelve [T-4].
- [ ] 11.2 [RED] En `internal/skillregistry/agents_test.go`: invariantes de §3.6 — idempotencia byte a byte en dos regeneraciones consecutivas con el mismo conjunto de skills; preservación byte a byte de todo lo que está fuera de marcadores (cabecera del documento, `## REGLA SUPREMA…`, `# Gentle AI…`, `## How to Use` y secciones hermanas, escenario REQ-22.12 «Ninguna otra sección se altera»); reparación de marcadores huérfanos y de pares duplicados en la misma operación; `AGENTS.md` (read-only) inexistente en el workspace resuelto ⇒ destino `omitted` y **no** se crea (§3.6). Fixture: `t.TempDir()` con un `AGENTS.md` real de ~46 líneas.
- [ ] 11.3 [GREEN] Crear `internal/skillregistry/agents.go`: `AdoptSkillsIndexMarkers(existing string) string` (paso previo puro, fuera de `filemerge`, D-10) y el escritor de `AGENTS.md` que compone el contenido gestionado de §3.3 (`## Skills` + línea en blanco + tabla de `renderSkillsTable` + salto final, con el título **dentro**), invoca `InjectMarkdownSection(adopted, "skills-index", body)` y escribe con `WriteFileAtomic` (`internal/components/filemerge/writer.go`, (read-only)).
- [ ] 11.4 [REFACTOR] Confirmar que `filemerge` no se ha modificado en ninguna línea y que `AdoptSkillsIndexMarkers` es una normalización de forma, no una fusión (no reimplementa reparación ni elevación de legado).
- [ ] 11.5 [Verificación de cierre] V-A; V-B (`go test ./internal/skillregistry/... -run 'TestAdoptSkillsIndexMarkers|TestAgentsInvariants|TestAgents' -timeout 300s`); V-C; V-D.

## Fase 12: S5c — Regeneración unificada: tres destinos desde un escaneo (REQ-22.11, REQ-22.12; D-09; T-1, T-8)

Depende de las Fases 10 y 11. Un escaneo, tres escrituras, con política de fallos por destino.

> **Válvula de alivio:** si el diff real supera holgadamente 400 líneas, separar la tabla de vectores de `--cwd` de T-1 en una PR inmediatamente posterior (`inc-22/12b-skillregistry-path-containment`).

- [ ] 12.1 [RED] Escribir `internal/skillregistry/regenerate_test.go`: un escaneo alimenta `.atl/skill-registry.md`, `.atl/.skill-registry.cache.json`, `AGENTS.md` (read-only) y el puerto de espejo con el **mismo** conjunto de skills (doble de `MirrorFunc` que cuenta invocaciones y captura el `MirrorRequest`); `Reason == "cache-hit"` ⇒ cero escrituras y los tres destinos byte-idénticos (escenario «Sin cambios, la huella evita trabajo»); `--force` regenera; el filtro de exclusiones (`_shared`, `skill-registry`, prefijo `sdd-`) se aplica una sola vez y se respeta en los tres destinos (escenario «Filtro de exclusiones conservado»); `Mirror == nil` ⇒ `mirror failed` con motivo `mirror not configured` y el resultado no es fatal [T-8]; solo un `MirrorFunc` que devuelve `nil` produce `mirror ok`; `AGENTS.md` ausente ⇒ `omitted` sin error; fallo de escritura de destino primario ⇒ error envuelto con el destino («parcial ya emitido»).
- [ ] 12.2 [RED] En `internal/skillregistry/regenerate_test.go`: contención de rutas [T-1] — los ocho vectores de `--cwd` (relativo, absoluto, inexistente, con `..`, `/`, `\`, ruta absoluta, nombres reservados `con`/`nul`, 300 caracteres) se resuelven con `filepath.Clean` una sola vez, fallan **antes** de cualquier escritura si no existen, y ningún fichero aparece fuera de `t.TempDir()`.
- [ ] 12.3 [GREEN] Modificar `internal/skillregistry/registry.go`: `Regenerate(cwd, home string, opts RegenerateOptions) (Result, error)` escribe en orden — (1) `.atl/skill-registry.md` (fatal), (2) `.atl/.skill-registry.cache.json` (fatal), (3) sección `## Skills` de `AGENTS.md` (read-only) (fatal si el fichero existe; `omitted` si no), (4) tópico Engram `skill-registry` vía `opts.Mirror` (**no fatal**: `mirror failed` con exit `0` para el llamador) — y devuelve `Reason` ∈ {`cache-hit`, `fingerprint-changed`, `forced`}. El contenido del espejo es el cuerpo gestionado de §3.3 precedido de una línea de procedencia [D-11].
- [ ] 12.4 [REFACTOR] Confirmar un único escaneo (`findAllSkillFiles` + `LoadSkill` + dedupe) aguas arriba de los tres destinos y la política de fallos por destino en un solo sitio.
- [ ] 12.5 [Verificación de cierre] V-A; V-B (`go test ./internal/skillregistry/... -timeout 300s`); V-C; V-D; V-E — **compuerta 3** (idempotencia y preservación de `AGENTS.md` (read-only) en verde).

## Fase 13: S6a — Cliente MCP stdio acotado `SaveTopic` (REQ-22.11; D-11; T-3)

Independiente de las Fases 10–12 salvo por el contrato de `MirrorRequest`. Vive en `internal/components/engram`, **no** en `internal/skillregistry`.

- [ ] 13.1 [RED] Escribir `internal/components/engram/save_test.go` con servidor stdio falso (precedente: `internal/components/engram/healthprobe_test.go`, (read-only)): la secuencia acotada `initialize` → `notifications/initialized` → `tools/call {"name":"mem_save", ...}` → lectura de respuesta ⇒ `SaveTopic` devuelve `nil`; los argumentos del `tools/call` incluyen `title`, `content`, `type`, `project`, `topic_key` y `capture_prompt: false`; el hijo **siempre** termina en el camino de éxito.
- [ ] 13.2 [GREEN] Crear `internal/components/engram/save.go`: `SaveTopic(ctx context.Context, command string, args []string, req SaveTopicRequest) error` — *spawn* del servidor MCP `engram` por stdio, JSON-RPC 2.0 delimitado por saltos de línea, `protocolVersion: "2024-11-05"`, presupuesto `StdioProbeDeadline` (constante de `internal/components/engram/healthprobe.go`, (read-only)) y terminación garantizada del hijo [D-11, T-3]. El comando del servidor se resuelve con `ReadPersistedStdioCommands` (`internal/components/engram/healthprobe.go`, (read-only)).
- [ ] 13.3 [REFACTOR] Confirmar que `save.go` reutiliza la disciplina de transporte de `stdioHandshake` sin duplicarla en exceso y que no introduce un cliente MCP completo (sin listado de *tools*, sin sesiones, sin reintentos).
- [ ] 13.4 [Verificación de cierre] V-A; V-B (`go test ./internal/components/engram/... -run 'TestSaveTopic' -timeout 300s`); V-C; V-D.

## Fase 14: S6b — Modos de fallo del cliente MCP y terminación del hijo (REQ-22.11; D-11; T-3)

Depende de la Fase 13. Cada fallo es un error ordinario que el llamador mapea a `mirror failed` y **nunca** trata como fatal.

- [ ] 14.1 [RED] Ampliar `internal/components/engram/save_test.go` con los cuatro modos de T-3 — (a) `tools/call` que responde con `error` ⇒ error; (b) servidor que no responde hasta el deadline ⇒ error; (c) salida no-JSON/basura ⇒ error; (d) hijo que se autotermina antes de responder ⇒ error — y la aserción transversal: en los cuatro, el **hijo siempre termina** (no queda huérfano).
- [ ] 14.2 [GREEN] Completar `internal/components/engram/save.go`: manejo de los cuatro modos con error ordinario, límite de captura de salida y `defer` de terminación del hijo en todo camino de retorno.
- [ ] 14.3 [REFACTOR] Confirmar que ningún camino devuelve `nil` sin haber leído una respuesta satisfactoria de `tools/call` (anti-fabricación de `mirror ok`, T-8 en el transporte).
- [ ] 14.4 [Verificación de cierre] V-A; V-B (`go test ./internal/components/engram/... -run 'TestSaveTopic' -timeout 300s`); V-C; V-D.

## Fase 15: Guarda estructural de alcance (S1, S5; §6, fila «Estructural»)

Solo test. Se coloca tras S6 para poder afirmar ya la frontera `skillregistry` → `engram`. No produce código de producción.

- [ ] 15.1 [RED] Escribir `internal/skillregistry/import_boundary_test.go` (escáner `go/ast` al estilo de `internal/sddstatus/review_offer_absence_guard_test.go`, (read-only), como precedente de guarda estructural en este repositorio): (a) ningún fichero de producción de este incremento importa `internal/components/filemerge` salvo `internal/skillregistry`; (b) `internal/skillregistry` **no** importa `internal/components/engram`; (c) ningún fichero de producción compone `github.com/gentleman-programming/gentle-ai` como literal de instrucción de instalación del fork (REQ-22.2 a nivel estructural).
- [ ] 15.2 [GREEN] Ejecutar el escáner sobre el estado del árbol y corregir cualquier violación real que encuentre (no se relajan los predicados del escáner para hacerlo pasar).
- [ ] 15.3 [Verificación de cierre] V-A; V-B (`go test ./internal/skillregistry/... -run 'TestImportBoundary' -timeout 300s`); V-C; V-D.

## Fase 16: S7a — `runSkillIndex` y parsers compartidos por verbo (REQ-22.10, REQ-22.14; D-13; T-1, T-6)

Depende de las Fases 12–15 (motor, espejo y frontera ya existentes). Aquí se cablea el cliente real de espejo en producción.

> **Válvula de alivio:** si el diff real supera holgadamente 400 líneas, separar la delegación de `runSkillRegistry*` (16.3) en una PR inmediatamente posterior (`inc-22/16b-app-skill-registry-delegation`).

- [ ] 16.1 [RED] Escribir `internal/app/skill_index_test.go` con `bytes.Buffer` como `stdout`: la tabla de códigos de salida de §5.1 (éxito `0`; `--quiet` suprime stdout y **no** stderr; no-raíz de proyecto con `--quiet` ⇒ `0` sin salida y sin ficheros creados; no-raíz sin `--quiet` ⇒ `0` con aviso de una línea con motivo y ruta; sin subcomando ⇒ `1` con `usage: axiom skill index <refresh|list> [flags]`; subcomando desconocido ⇒ `1` nombrando `refresh` y `list`; bandera desconocida o `--cwd` sin valor ⇒ `1` con la bandera exacta; fallo de destino primario ⇒ `1`; fallo de espejo ⇒ `0` con `mirror failed`); `list --json` no escribe registro, caché, `AGENTS.md` (read-only), `.gitignore` (read-only) ni memoria; las seis situaciones de enrutamiento de T-6 y los ocho vectores de `--cwd` de T-1 abortan sin efectos.
- [ ] 16.2 [RED] En el mismo fichero: la línea por destino de `refresh` declara `updated` / `unchanged` / `omitted` / `mirror ok` / `mirror failed` tras la línea primaria (§1.1), y con `--quiet` no hay salida estándar en éxito.
- [ ] 16.3 [GREEN] Crear `internal/app/skill_index.go`: `runSkillIndex`, `runSkillIndexRefresh`, `runSkillIndexList` y los *parsers* compartidos con etiqueta de verbo para el mensaje de error [D-13]. El cableado de producción inyecta `Mirror` con `engram.SaveTopic` (`internal/components/engram/save.go`) resolviendo `MirrorRequest.Project` en la capa CLI [D-11]. **Nota O-5 (no resuelta aquí):** la resolución exacta del nombre de proyecto Engram del workspace permanece como la deja el diseño; el valor se recibe como parámetro en un único punto, de modo que fijar O-5 después no toca el motor.
- [ ] 16.4 [GREEN] Modificar `internal/app/app.go`: `runSkillRegistryRefresh`/`runSkillRegistryList` delegan en el motor compartido conservando **byte a byte** sus mensajes y su salida (solo la línea primaria en `refresh`) [D-13, REQ-22.14]. El literal de branding heredado de `internal/app/app.go` **no** se toca (§4.1).
- [ ] 16.5 [REFACTOR] Confirmar que los *parsers* comparten la validación de `--cwd`/contención sin duplicar `filepath.Rel` y que el motor es literalmente la misma función para ambos verbos.
- [ ] 16.6 [Verificación de cierre] V-A; V-B (`go test ./internal/app/... -run 'TestSkillIndex|TestSkillRegistry' -timeout 300s`); V-C; V-D.

## Fase 17: S7b — Enrutado `axiom skill index`, ayuda y compatibilidad (REQ-22.10, REQ-22.14; D-13)

Depende de la Fase 16.

- [ ] 17.1 [RED] Escribir `cmd/axiom/skill_index_route_test.go`: `axiom skill index` sin subcomando ⇒ código `1` con `usage: axiom skill index <refresh|list> [flags]`; subcomando desconocido ⇒ `1` nombrando `refresh` y `list`; la ayuda de `axiom skill` lista `index`, `scan`, `list`, `approve`, `reject` y contiene **exactamente una** línea que aclara la colisión `skill list` (autoskill) vs `skill index list` (índice unificado) (REQ-22.10, distinción obligatoria de la spec D-11); compatibilidad: `skill-registry refresh --quiet --no-gitignore --cwd <ruta>` — el argv literal que ejecuta `internal/assets/opencode/plugins/skill-registry.ts` (read-only) — termina `0`; `axiom skill-registry list --json` conserva `name`, `scope`, `description`, `path`; los mensajes de error de `skill-registry` son idénticos a los de hoy.
- [ ] 17.2 [GREEN] Modificar `cmd/axiom/main.go`: `case "skill"` gana el subverbo `index`; `printHelp` lista `skill index` y la aclaración de colisión en una línea [REQ-22.10]. Confirmar antes de escribir que no existe ya ningún `case "index"`.
- [ ] 17.3 [GREEN] Modificar `internal/app/skill_registry_guard_test.go`: actualizar solo donde la salida cambie; las guardas de `RefreshSkip` se conservan sin debilitarse.
- [ ] 17.4 [REFACTOR] Confirmar que `axiom skill-registry` no se ha retirado ni renombrado y que ambos verbos comparten motor sin compartir presentación [D-13].
- [ ] 17.5 [Verificación de cierre] V-A; V-B (`go test ./cmd/axiom/... ./internal/app/... -run 'TestSkillIndex|TestSkillRegistry|TestHelp|TestRunSkill' -timeout 300s`); V-C; V-D; V-E — **compuerta 4** (argv literal del plugin en `0` y salida de `skill-registry refresh` sin cambios).

## Fase 18: S8 — Gancho de regeneración en `Manager.Approve()` (REQ-22.13; D-12)

Depende de las Fases 12 y 16 (el regenerador real existe). La promoción no cambia de semántica; el índice es una vista derivada y su fallo no revierte.

- [ ] 18.1 [RED] Escribir `internal/autoskill/approve_hook_test.go`: `Approve` con regenerador OK ⇒ skill promovida a `skills/<nombre>/`, buzón vacío e índice al día; regenerador que devuelve error ⇒ `ApproveOutcome.RegenerateError` poblado, `skills/<nombre>/` **existe**, buzón vacío y el `error` de retorno es `nil` (la promoción **no** se deshace — escenario REQ-22.13 «La promoción no se revierte si falla la regeneración»); promoción fallida (skill inexistente o fallo de copia) ⇒ `error` distinto de `nil` y **no** se invoca el regenerador; `Reject` elimina la propuesta del buzón, **no** invoca el regenerador y no modifica ningún destino del índice (escenario «`Reject` no dispara regeneración»).
- [ ] 18.2 [GREEN] Modificar `internal/autoskill/manager.go`: campo inyectado `RegenerateIndex IndexRegenerator`, tipo `ApproveOutcome{Promoted, RegenerateError}` y `Approve(skillName string) (ApproveOutcome, error)` no transaccional, invocado **tras** la copia correcta y el `os.RemoveAll` del buzón [D-12]. `Reject` sin gancho.
- [ ] 18.3 [GREEN] Modificar `cmd/axiom/main.go`: `runSkillApprove` inyecta el regenerador real (envoltorio de `skillregistry.Regenerate` con `Force: false`) e imprime el fallo de indexación como **aviso** con exit `0` [D-12].
- [ ] 18.4 [REFACTOR] Confirmar que `internal/autoskill` no importa internals de `skillregistry` más allá del tipo funcional inyectado y que ningún camino de `Approve` transaccionaliza la copia.
- [ ] 18.5 [Verificación de cierre] V-A; V-B (`go test ./internal/autoskill/... ./cmd/axiom/... -run 'TestApprove|TestReject|TestRunSkillApprove' -timeout 300s`); V-C; V-D.

## Fase 19: S9a — Compuerta de control solo-binario y reporte estructurado de upgrade (REQ-22.5, REQ-22.6; D-06, D-08)

Depende de las Fases 1–6 (identidad y vías) y de la Fase 7 (señal TUI coherente). Empieza por su compuerta de control.

- [ ] 19.1 [Caracterización] Ejecutar **antes de tocar nada** `go test ./internal/app/... -run 'TestRunArgs_UpgradeDryRun|TestRunArgs_UpgradeOutput_BinariesOnly' -timeout 300s` sobre `internal/app/upgrade_test.go` (read-only) y confirmar verde **sin modificar aserciones** [D-08, REQ-22.5]. Registrar la salida exacta como evidencia de la compuerta.
- [ ] 19.2 [RED] Escribir `internal/app/upgrade_report_test.go`: `RunUpgradeReport` clasifica `succeeded` / `failed` / `skipped` por herramienta; `RestartRequired == true` **solo** cuando la herramienta self-tool terminó en `UpgradeSucceeded` (derivado del predicado de identidad, §2.2 de la spec); `ManualHint` poblado cuando `Status == "skipped"` (de `ManualFallbackError.Hint`); `ResolveSyncSkip` — `RestartRequired` ⇒ `(true, "restart-required")`; `Status == "failed"` ⇒ `(true, "upgrade-failed")`; caso feliz ⇒ `(false, "")` (regla de salto de REQ-22.6 como única fuente de verdad).
- [ ] 19.3 [GREEN] Crear `internal/app/upgrade_report.go`: `UpgradeToolOutcome`, `UpgradeRunReport`, `RunUpgradeReport(ctx, parsed, result, stdout) (UpgradeRunReport, error)` y `ResolveSyncSkip` [D-06, D-08]. `RunUpgradeReport` ejecuta **exactamente** lo que `runUpgrade` ejecuta hoy (solo-binario: ni `install` ni `sync`).
- [ ] 19.4 [GREEN] Modificar `internal/app/app.go`: `runUpgrade` se convierte en un envoltorio que invoca `RunUpgradeReport` e imprime, conservando la salida **byte a byte** [D-06]. Criterio de aborto: los dos tests de 19.1 en rojo ⇒ revertir.
- [ ] 19.5 [REFACTOR] Confirmar que ningún parseo de prosa se usa para deducir `status`/`restart_required`/`manual_hint` (los datos viajan en estructura, D-06).
- [ ] 19.6 [Verificación de cierre] V-A; V-B (`go test ./internal/app/... -run 'TestUpgrade|TestRunArgs_Upgrade|TestResolveSyncSkip' -timeout 300s`); V-C; V-D; V-E — **compuerta 5** (repetir 19.1 en verde).

## Fase 20: S9b — DTO por fases y cadena `RunUpgradeSequence` (REQ-22.4; D-06, D-07)

Depende de la Fase 19. El encadenamiento vive **solo** en la capa de servicio Web UI; el verbo CLI sigue siendo solo-binario.

- [ ] 20.1 [RED] Escribir `internal/dashboard/service_sequence_test.go` con `httptest` y dobles: las cinco reglas de semántica de §2.3 — ambas fases OK ⇒ `sequence: "upgrade->sync"`, `phases.upgrade.status: "succeeded"` y `phases.sync.executed: true` (escenario «Ambas fases completan con éxito»); `restart_required` ⇒ `executed: false` + `skipped_reason: "restart-required"` y **`sync` no invocado** (doble que falla si se le llama; escenario «`upgrade` reemplaza el binario en ejecución y `sync` se omite»); `status: "failed"` ⇒ `skipped_reason: "upgrade-failed"` y `success: false` (escenario «`upgrade` falla y `sync` no se dispara»); `success` superior es `true` con `restart-required`; `GET`/`PUT`/`DELETE` sobre `/api/ecosystem/upgrade` ⇒ `405` sin efectos (escenario «Método HTTP no admitido»); `POST` ⇒ `200` con reporte; `500` solo cuando el servicio no logra producir reporte alguno. **Nota O-3 (no se reabre):** se implementa la forma del **ejemplo** JSON de §2.2 (la especificación completa del objeto), tal como deja el diseño; la tabla de campos se trata como subconjunto normativo.
- [ ] 20.2 [GREEN] Modificar `internal/dashboard/types.go`: campos aditivos `Sequence string` y `Phases *EcosystemPhases` en `EcosystemActionResponse`, más `EcosystemPhases`, `UpgradePhaseReport` y `SyncPhaseReport` con los campos obligatorios de `phases` **sin** `omitempty` [D-07]. `POST /api/ecosystem/sync` y `RestoreBackup` no se ven obligados a poblar nada nuevo.
- [ ] 20.3 [GREEN] Modificar `internal/dashboard/service.go`: `RunUpgrade` pasa a `RunUpgradeSequence` y compone `app.RunUpgradeReport` → `cli.RunSync` (mismo primitivo que `RunSync`) **solo si** `ResolveSyncSkip` lo permite [D-06, D-08].
- [ ] 20.4 [GREEN] Modificar `internal/dashboard/server.go`: `handleEcosystemUpgrade` invoca `RunUpgradeSequence` (el `405` ya existe y no cambia) [D-07].
- [ ] 20.5 [REFACTOR] Confirmar que la cadena no se ha colado en `app.runUpgrade` y que `POST /api/ecosystem/sync` conserva su contrato intacto.
- [ ] 20.6 [Verificación de cierre] V-A; V-B (`go test ./internal/dashboard/... -run 'TestRunUpgradeSequence|TestEcosystemUpgrade|TestEcosystemAction' -timeout 300s`); V-C; V-D; V-E (compuerta 5 en verde).

## Fase 21: S9c — Presentación web de ambas fases (REQ-22.4)

Depende de la Fase 20.

- [ ] 21.1 [RED] Ampliar `internal/dashboard/dashboard_test.go`: el resultado de «Actualizar Herramientas» presenta el estado de `phases.upgrade` y `phases.sync`, incluido el motivo de omisión (`restart-required` / `upgrade-failed`) y la instrucción de reinicio cuando aplica (REQ-22.4: «El frontend web DEBE presentar el resultado de ambas fases, incluida la omisión de `sync` con su motivo»).
- [ ] 21.2 [GREEN] Modificar `internal/dashboard/assets/app.js`: presentar ambas fases y el motivo de omisión [REQ-22.4].
- [ ] 21.3 [REFACTOR] Confirmar que la presentación no inventa estados que el DTO no declara.
- [ ] 21.4 [Verificación de cierre] V-A; V-B (`go test ./internal/dashboard/... -run 'TestDashboard|TestEcosystem|TestAssets' -timeout 300s`); V-C; V-D. **Harness en tiempo de ejecución: N/A con motivo** — invocar el endpoint real contra el binario vivo ejecutaría una sustitución real del binario; la verificación es `httptest` + dobles.

## Fase 22: S9d — Registro durable `upstream_version` y cierre de cadena (REQ-22.9; D-14)

Independiente de las Fases 20–21. Última rebanada: cierra la cadena completa.

- [ ] 22.1 [RED] Modificar `internal/state/state_test.go`: un `Write`/`WriteReconciled` de un estado sin el campo deja `upstream_version == "3.4.0"` (sin prefijo `v`, escenario REQ-22.9 «El estado expone `upstream_version`»); un valor ya presente **no** se sobrescribe; `MergeAgents` lo preserva; `fullyPopulatedInstallState` gana el campo y `TestInstallStatePreservesEveryField` lo cubre. Aserción de no-consumo (escenario «El registro no dispara sincronización»): ningún fichero de `internal/update`, `internal/update/upgrade` ni del verbo `sync` referencia `UpstreamVersion` (escaneo estructural).
- [ ] 22.2 [GREEN] Modificar `internal/state/state.go`: constante `DefaultUpstreamVersion = "3.4.0"` (comentario GoDoc en inglés que cita el techo congelado `v3.4.0` / `docs/upstream-absorption-ledger.md` (read-only), decisión D4 de INC-20), campo `UpstreamVersion string` con etiqueta `json:"upstream_version,omitempty"` y respaldo al escribir en `Write`/`WriteReconciled` (nunca sobrescribe uno ya presente) [D-14].
- [ ] 22.3 [REFACTOR] Confirmar que la raíz `~/.axiom/state.json` y la migración legada no cambian (adición compatible, §6 de la spec) y que ningún automatismo lee el campo.
- [ ] 22.4 [Verificación de cierre] V-A; V-B (`go test ./internal/state/... -run 'TestInstallState|TestUpstreamVersion|TestWrite|TestMergeAgents' -timeout 300s`); V-C; V-D.
- [ ] 22.5 [Cierre de cadena — V-F] Ejecutar `go test ./... -timeout 900s`, más `go test ./internal/update/... -timeout 600s` y `go test ./internal/sddstatus/... -timeout 600s` **aparte**. Registrar el resultado real de cada uno: los fallos ambientales conocidos de la regla 5 (`TestNoUpdatesPath`, `TestDetectHomebrewOwnershipWith`, colgado de `internal/sddstatus`) se documentan como tales; cualquier agotamiento de presupuesto se registra como **no ejecutado**. Repetir las cinco compuertas de control (Fases 6, 7, 12, 17, 19) y confirmar que ningún criterio de aborto de `design.md` §8.2 se cumple.

---

## Trazabilidad rápida (fase → rebanada → requerimientos / decisiones)

| Fase | Rebanada | Requerimientos / decisiones cubiertos | Amenazas (§7) |
|---|---|---|---|
| 1 | S1a | REQ-22.2 (módulo declarado), REQ-22.3 (base de identidad); D-01 | — |
| 2 | S1b | REQ-22.2; D-04 | mensajes sin literal upstream |
| 3 | S1c | REQ-22.3; D-01 | — |
| 4 | S2a | REQ-22.1 (parcial), REQ-22.3; D-03 | T-5 |
| 5 | S2b | REQ-22.1; D-02 | T-2, T-7 |
| 6 | S2c | REQ-22.2, REQ-22.3; D-01, D-04 — **compuerta 1** | mensajes sin literal upstream |
| 7 | S3 | REQ-22.6, REQ-22.7; D-08; H-4 — **compuerta 2** | T-9 |
| 8 | S4 | REQ-22.8; D-05; **O-1 (cerrada)** | — |
| 9 | *(hito O-2)* | REQ-22.11 («rutas exactas»), REQ-22.12 §3.4 (columna `Path`); D-09, H-5 | — |
| 10 | S5a | REQ-22.11, REQ-22.12 (formato §3.4); D-09, D-11 | T-8 (puerto) |
| 11 | S5b | REQ-22.12; D-10 | T-4 |
| 12 | S5c | REQ-22.11, REQ-22.12 (invariantes); D-09 — **compuerta 3** | T-1, T-8 |
| 13 | S6a | REQ-22.11 (espejo); D-11 | T-3 |
| 14 | S6b | REQ-22.11 (espejo no fatal); D-11 | T-3 |
| 15 | *(guarda)* | REQ-22.2 (estructural); D-01, D-09, D-11 (fronteras de import) | — |
| 16 | S7a | REQ-22.10, REQ-22.14 (motor compartido); D-13; O-5 (nota) | T-1, T-6 |
| 17 | S7b | REQ-22.10, REQ-22.14; D-13 — **compuerta 4** | T-6 |
| 18 | S8 | REQ-22.13; D-12 | T-9 (regla de fallo) |
| 19 | S9a | REQ-22.5 (**compuerta 5**), REQ-22.6; D-06, D-08 | — |
| 20 | S9b | REQ-22.4; D-06, D-07; O-3 (no reabierta) | T-1 (endpoint) |
| 21 | S9c | REQ-22.4 (frontend); D-06 | — |
| 22 | S9d | REQ-22.9; D-14 | no-consumo del registro |

**Cobertura completa de REQ-22.1–REQ-22.14:** 22.1 (F4, F5) · 22.2 (F1, F2, F6, F15) · 22.3 (F1, F3, F4, F6) · 22.4 (F20, F21) · 22.5 (F19) · 22.6 (F7, F19) · 22.7 (F7) · 22.8 (F8) · 22.9 (F22) · 22.10 (F16, F17) · 22.11 (F9, F10, F12, F13, F14) · 22.12 (F9, F10, F11, F12) · 22.13 (F18) · 22.14 (F16, F17).

---

## Review Workload Forecast

| Campo | Valor |
|---|---|
| Chained PRs recommended | **Yes** |
| 400-line budget risk | **High** (agregado del incremento; por PR individual ninguna fase se planifica por encima de ~450 y la mayoría son Low/Medium) |
| Estimated changed lines | **~4.800–7.200** (adiciones + eliminaciones de autoría; excluidos generados) |
| Decision needed before apply | **Yes** (por O-2; con `auto-chain` habría sido `No`) |
| Chain strategy | `stacked-to-main` (derivada de design.md §8.2; cacheada, sin reabrir) |
| Delivery strategy | `auto-chain` |
| Suggested split | PR 1 → PR 2 → … → PR 22 (una por fase; la Fase 9 es hito sin PR) |

Líneas de guarda exactas (contrato de herramienta, no traducir):

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
Estimated changed lines: ~4.800–7.200
```

### Estimación y riesgo por PR

| # | PR (rama) | Fase | Contenido | Líneas estimadas | Riesgo |
|---|---|---|---|---|---|
| 1 | `inc-22/01-update-identity-module` | 1 | Identidad + `GoModulePath`/`GoInstallResolvable` | 280–400 | Medium |
| 2 | `inc-22/02-update-derived-instructions` | 2 | `SourceInstallCommand` + hints derivados | 320–450 | Medium-High (válvula: separar escaneo de mensajes) |
| 3 | `inc-22/03-update-init-mutation` | 3 | `init()` campo a campo | 120–200 | Low |
| 4 | `inc-22/04-update-write-preflight` | 4 | Preflight compartido + enum | 260–380 | Medium |
| 5 | `inc-22/05-update-source-build` | 5 | `sourceBuildUpgrade` + ruteo | 350–450 | Medium-High (válvula: separar modos de fallo) |
| 6 | `inc-22/06-update-strategy-safeguards` | 6 | Salvaguardas de `strategy.go` | 350–450 | Medium-High (válvula: separar escaneo de mensajes) |
| 7 | `inc-22/07-tui-identity-branding` | 7 | Identidad TUI + rebranding | 150–250 | Low |
| 8 | `inc-22/08-version-symbol` | 8 | `var version` | 80–150 | Low |
| — | *(sin PR)* | 9 | Hito de decisión O-2 | 0 | — |
| 10 | `inc-22/10-skillregistry-types-table` | 10 | Tipos + puerto + `renderSkillsTable` | 280–380 | Medium |
| 11 | `inc-22/11-skillregistry-marker-adoption` | 11 | `AdoptSkillsIndexMarkers` + invariantes | 300–400 | Medium |
| 12 | `inc-22/12-skillregistry-three-destinations` | 12 | `Regenerate` a tres destinos | 350–450 | Medium-High (válvula: separar vectores T-1) |
| 13 | `inc-22/13-engram-save-topic` | 13 | `SaveTopic` + handshake | 250–350 | Medium |
| 14 | `inc-22/14-engram-save-failure-modes` | 14 | Modos de fallo T-3 | 200–300 | Medium |
| 15 | `inc-22/15-scope-structural-guard` | 15 | Guarda estructural (solo test) | 100–180 | Low |
| 16 | `inc-22/16-app-skill-index-engine` | 16 | `runSkillIndex` + delegación | 350–450 | Medium-High (válvula: separar delegación) |
| 17 | `inc-22/17-cli-skill-index-routing` | 17 | Enrutado + ayuda + compat | 250–350 | Medium |
| 18 | `inc-22/18-autoskill-approve-hook` | 18 | Gancho de `Approve` | 250–350 | Medium |
| 19 | `inc-22/19-app-upgrade-report` | 19 | `RunUpgradeReport` + `ResolveSyncSkip` | 350–450 | Medium-High (compuerta de control) |
| 20 | `inc-22/20-dashboard-upgrade-sequence` | 20 | DTO + `RunUpgradeSequence` + server | 350–450 | Medium-High (válvula: separar tests HTTP) |
| 21 | `inc-22/21-dashboard-web-phases` | 21 | `assets/app.js` | 150–250 | Low |
| 22 | `inc-22/22-state-upstream-version` | 22 | `upstream_version` | 150–250 | Low |

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Predicado de identidad y módulo declarado | PR 1 | `go test ./internal/update/... -run 'TestIsSelfTool\|TestGoInstallResolvable' -timeout 300s` | N/A — biblioteca pura sin superficie de usuario | Revertir `types.go`/`registry.go` de `internal/update`; aditivo sin llamadores nuevos |
| 2 | Instrucciones de instalación derivadas de `ToolInfo` | PR 2 | `go test ./internal/update/... -run 'TestSourceInstall\|TestInstructions' -timeout 300s` | N/A — mensajes verificados por escaneo de cadena sobre la salida real | Revertir `instructions.go`/`check.go`; restaurar `GentleAISourceInstallCommand` |
| 3 | `init()` muta campo a campo sin perder `GoModulePath` | PR 3 | `go test ./cmd/axiom/... -run 'TestInit' -timeout 300s` | N/A — se verifica contra el registro ya mutado | Revertir `cmd/axiom/main.go` |
| 4 | Preflight único de escritura del binario | PR 4 | `go test ./internal/update/upgrade/... -run 'TestPreflight\|TestWritePreflight' -timeout 300s` | N/A — `lookPathFn` inyectado; no se escribe ningún binario real | Revertir `go_install_destination.go` y el enum de `types.go` |
| 5 | Compilación controlada desde clon + ruteo Windows | PR 5 | `go test ./internal/update/upgrade/... -run 'TestSourceBuild\|TestEffectiveMethod' -timeout 300s` | N/A — una sustitución real del binario no es un harness seguro; dobles de `execCommand` | Eliminar `source_build.go` y su `case`; `effectiveMethod` vuelve a su tabla previa |
| 6 | Salvaguardas de `strategy.go` ancladas al fork | PR 6 | `go test ./internal/update/upgrade/... -run 'TestStrategy\|TestBeta\|TestBinaryUpgrade' -timeout 300s` | N/A — igual que el PR 5 | Revertir `strategy.go`; la compuerta 1 vuelve a fallar y se documenta |
| 7 | Identidad TUI + rebranding de `upgrade_sync.go` | PR 7 | `go test ./internal/tui/... -run 'TestUpgradeSync\|TestSelfToolIdentity' -timeout 300s` | N/A — TUI interactiva; verificación de render por test | Revertir los tres ficheros TUI y su test |
| 8 | Símbolo de versión build-time | PR 8 | `go test ./cmd/axiom/... -run 'TestVersion\|TestAppVersionInitialization' -timeout 300s` | `go run ./cmd/axiom version` (muestra `v0.1.0` sin inyección) | Revertir `cmd/axiom/main.go`; `.goreleaser.yaml`/`ci.yml` nunca se tocaron |
| 9 | Hito de decisión O-2 (sin código) | — | N/A | N/A | N/A — registro de decisión, reversible por nueva decisión |
| 10 | Tipos del motor, puerto de espejo y tabla única | PR 10 | `go test ./internal/skillregistry/... -run 'TestRenderSkillsTable\|TestMarkdownCell' -timeout 300s` | `axiom skill-registry list --json` sobre un workspace temporal (salida sin cambios) | Eliminar `types.go`/`mirror.go`/`table.go`; `RenderRegistry` vuelve a su tabla en línea |
| 11 | Adopción de marcadores de `## Skills` | PR 11 | `go test ./internal/skillregistry/... -run 'TestAdoptSkillsIndexMarkers\|TestAgentsInvariants' -timeout 300s` | N/A — la escritura real se dispara en el PR 12 | Eliminar `agents.go` |
| 12 | Tres destinos desde un escaneo | PR 12 | `go test ./internal/skillregistry/... -timeout 300s` | `axiom skill index refresh --cwd <tmp>` sobre un workspace de prueba con `AGENTS.md` de fixture | Revertir `registry.go`; `AGENTS.md` ya adoptado queda como dato inerte |
| 13 | Cliente MCP stdio `SaveTopic` (camino feliz) | PR 13 | `go test ./internal/components/engram/... -run 'TestSaveTopic' -timeout 300s` | N/A — servidor stdio falso; no se invoca un Engram real | Eliminar `save.go`; `Mirror==nil` sigue dando `mirror failed` |
| 14 | Modos de fallo del cliente MCP | PR 14 | `go test ./internal/components/engram/... -run 'TestSaveTopic' -timeout 300s` | N/A — igual que el PR 13 | Revertir a la versión del PR 13 |
| 15 | Guarda estructural de alcance | PR 15 | `go test ./internal/skillregistry/... -run 'TestImportBoundary' -timeout 300s` | N/A — guarda de compilación/AST | Eliminar el test de guarda |
| 16 | `runSkillIndex` y parsers compartidos | PR 16 | `go test ./internal/app/... -run 'TestSkillIndex\|TestSkillRegistry' -timeout 300s` | `axiom skill index list --json` y `axiom skill index refresh --quiet --cwd <tmp>` | Eliminar `skill_index.go`; `skill-registry` vuelve a su implementación previa |
| 17 | Enrutado `axiom skill index` y ayuda | PR 17 | `go test ./cmd/axiom/... ./internal/app/... -run 'TestSkillIndex\|TestHelp' -timeout 300s` | `skill-registry refresh --quiet --no-gitignore --cwd <ruta>` (argv literal del plugin) ⇒ `0` | Revertir el `case "index"` y `printHelp` |
| 18 | Gancho de autoskill en `Approve` | PR 18 | `go test ./internal/autoskill/... -run 'TestApprove\|TestReject' -timeout 300s` | `axiom skill approve <nombre>` sobre un buzón de prueba en `t.TempDir()` | Quitar la inyección de `runSkillApprove` y el campo `RegenerateIndex` |
| 19 | Reporte estructurado de upgrade (compuerta 5) | PR 19 | `go test ./internal/app/... -run 'TestUpgrade\|TestRunArgs_Upgrade\|TestResolveSyncSkip' -timeout 300s` | N/A — `axiom upgrade` real sustituiría el binario; dry-run cubierto por `TestRunArgs_UpgradeDryRun` | Revertir `upgrade_report.go` y el envoltorio de `runUpgrade` |
| 20 | DTO por fases y cadena Web UI | PR 20 | `go test ./internal/dashboard/... -run 'TestRunUpgradeSequence\|TestEcosystemUpgrade' -timeout 300s` | N/A — el endpoint real ejecutaría una actualización binaria; `httptest` + dobles | Revertir `types.go`/`service.go`/`server.go`; `POST /api/ecosystem/sync` intacto |
| 21 | Presentación web de ambas fases | PR 21 | `go test ./internal/dashboard/... -run 'TestDashboard\|TestAssets' -timeout 300s` | N/A — igual que el PR 20 | Revertir `assets/app.js` |
| 22 | `upstream_version` durable + cierre de cadena | PR 22 | `go test ./internal/state/... -run 'TestInstallState\|TestUpstreamVersion' -timeout 300s` | Escritura de `state.json` en un `HOME` temporal dirigido por test (sin instalación real) | Revertir `state.go`/`state_test.go`; la clave desaparece del JSON por `omitempty` |
