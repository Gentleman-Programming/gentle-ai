# Documento Vivo ODD — INC-22: Actualizador de Axiom e Índice Unificado de Skills

> **Feature:** `inc-22-updater-and-skills`
> **Fichero:** `odd/tasks/inc-22-updater-and-skills.md` (fuente de verdad)
> **Espejo Engram:** `odd/inc-22-updater-and-skills/tasks`
> **Cambio SDD de origen:** `openspec/changes/inc-22-axiom-updater-and-skills-index-governance/`
> **Creado:** 2026-09-22 · **Ruta:** rama `feature/inc-22-updater-and-skills` → **un único PR** (`exception-ok`)

---

## 1. Objetivo

Terminar INC-22 —actualizador autónomo de Axiom, encadenamiento `upgrade`→`sync` y gobernanza del índice unificado de skills— fuera del carril SDD, guiado por pruebas funcionales directas y con empaquetado de trabajo en vez de una delegación por fase.

## 2. Problema y por qué

Las fases 1-2 se implementaron ya en el carril SDD (`901c8f88`, `ce5e7e85`, integrados en `main`). El envoltorio SDD costaba ~3 delegaciones por fase (fase + compuerta + validador fresco), lo que disparaba tiempo y cuota sobre un incremento que ya estaba especificado de forma completa. Con `spec.md`, `design.md` y `tasks.md` cerrados, la especificación no necesita más ceremonia: necesita ejecución.

## 3. Alcance autorizado

**Dentro:** las fases 3-22 de `openspec/changes/inc-22-axiom-updater-and-skills-index-governance/tasks.md`, tal como están descritas en `spec.md` (REQ-22.1–REQ-22.14) y `design.md` (D-01–D-14), agrupadas en las unidades ODD de abajo.

**Fuera:** rehacer las fases 1-2 (cerradas y validadas) · abrir PRs o hacer push (decisión humana) · arreglar los fallos ambientales preexistentes de `internal/sddstatus` e `internal/update` · publicar binarios Windows en Releases · framework de sincronización upstream · modificar el motor `filemerge` · retirar o renombrar `axiom skill-registry`.

## 4. Restricciones activas (preferencias del usuario, 2026-09-22)

| Restricción | Valor | Fuente |
|---|---|---|
| TDD | **DESACTIVADO por defecto** | elección explícita del usuario. Solo se activa si lo pide. Runner si se activa: `go test ./...` |
| Entrega | **un único PR** | `exception-ok`: el mantenedor acepta `size:exception` de antemano; sin troceado obligatorio, sin umbral de 400 líneas, sin pregunta de split |
| Verificación | **pruebas funcionales directas** | tests que ejercen el comportamiento real; sin ciclo RED/GREEN/REFACTOR obligatorio |
| Idioma de artefactos | español (castellano) neutro; identificadores y literales técnicos en inglés | REGLA SUPREMA del proyecto |

## 5. Checklist de tareas (IDs estables)

Referencia cruzada: `F<n>` = fase `n` de `tasks.md` del cambio SDD.

- [ ] **ODD-0 — Decisión O-2 (F9).** Formato de la columna `Path` en la tabla `## Skills` regenerada de `AGENTS.md`. **Bloquea ODD-3, ODD-5 y ODD-6.** Es decisión de producto humana; ver §8.
- [ ] **ODD-1 — Pila de actualización (F3, F4, F5, F6).** `init()` campo a campo en `cmd/axiom`; preflight compartido de escritura del binario + enum `InstallSourceBuild`; `sourceBuildUpgrade` y ruteo Windows; salvaguardas de actualización ancladas a la identidad del fork (`IsSelfToolName`/`IsSelfTool`).
- [ ] **ODD-2 — Identidad TUI y versión (F7, F8).** Branding de la vista combinada `upgrade_sync.go` nombrando `axiom`; `var version = "v0.1.0"` en `cmd/axiom` (mismo símbolo que ya inyecta el linker).
- [ ] **ODD-3 — Motor del índice de skills (F10, F11, F12).** Requiere ODD-0. Tipos del motor, puerto de espejo y renderizador único de tabla; adopción de marcadores `<!-- axiom:skills-index -->` en `AGENTS.md`; regeneración unificada de los tres destinos desde un único escaneo.
- [ ] **ODD-4 — Cliente MCP stdio (F13, F14).** Cliente acotado `SaveTopic` para el espejo Engram, con modos de fallo y terminación garantizada del hijo.
- [ ] **ODD-5 — CLI `axiom skill index` (F16, F17).** Requiere ODD-3. `runSkillIndex` y parsers compartidos por verbo; enrutado, ayuda y compatibilidad byte a byte de `axiom skill-registry`.
- [ ] **ODD-6 — Gancho en autoskill (F18).** Requiere ODD-3. Regeneración automática desde `Manager.Approve()` tras promover una skill del buzón.
- [ ] **ODD-7 — Cadena Web `upgrade`→`sync` y upstream (F19, F20, F21, F22).** Compuerta de control solo-binario y reporte estructurado; DTO por fases y `RunUpgradeSequence`; presentación web de ambas fases; registro durable `upstream_version: "3.4.0"`.
- [ ] **ODD-8 — Guarda estructural de alcance (F15).** Comprobación de que no se ha tocado superficie prohibida (S1, S5). Se ejecuta como verificación, no como implementación.

## 6. Criterios de aceptación (globales)

1. Los 14 requerimientos `REQ-22.1`–`REQ-22.14` quedan cubiertos por comportamiento observable, con la trazabilidad tarea→REQ→decisión de `tasks.md` respetada.
2. `axiom upgrade` sigue siendo **solo-binario**: NO invoca `install` ni `sync`. La compuerta de control es `internal/app/upgrade_test.go` (`TestRunArgs_UpgradeDryRun`, `TestRunArgs_UpgradeOutput_BinariesOnly`).
3. `axiom skill-registry` conserva su contrato byte a byte (no se retira ni se renombra; el plugin `skill-registry.ts` lo invoca por argv literal).
4. Ninguna instrucción `go install` emitida contradice el `module` de `go.mod` ni nombra un binario que no sea `axiom`.
5. La primera regeneración de `## Skills` no duplica la sección (adopción de marcadores previa y pura).
6. Cero regresiones: el set de tests fallidos tras cada unidad coincide con la línea base ambiental.

## 7. Comprobaciones aplicables

- `go test ./internal/update/...` · `go test ./cmd/...` · `go test ./internal/app/...` · `go test ./internal/dashboard/...` · `go test ./internal/skillregistry/...` · `go test ./internal/autoskill/...`
- `go vet ./...` · `go build ./...` · `gofmt -l <ficheros tocados>`
- **Línea base ambiental (preexistente, NO es regresión, NO se arregla):** `internal/sddstatus` cuelga en `runGitCapturedRangeWithTimeout`; `internal/update` falla en `TestNoUpdatesPath`, `TestDetectHomebrewOwnershipWith`, `TestCheckSingleTool_EngramUsesBinaryReleaseChannel`, `TestInstallScriptBetaGoInstallBypassesPublicGoProxy`, `TestReleaseSecurityScriptsAreSyntacticallyValidAndFailClosed`, `TestRequireCISuccessSelectsNewestExactCommitRun`, `TestCanonicalReleasePublicKeysControlRealLinkerBuild`, `TestReleaseDistributionPolicyAssertionFailsClosed`, `TestReleaseDistributionPolicyAcceptsSemanticYAMLFormatting`, `TestModifiedReleaseVerifierCannotGainWriteAuthority`, `TestRunStrategyUsesCaskOwnershipAndMigrationGuidance` (casi todos: `bash`/`printf` ausentes en Windows). `gofmt -l .` marca 18 ficheros no canónicos preexistentes.

## 8. Decisiones abiertas

- **O-2 (bloqueante para ODD-3/5/6):** formato de la columna `Path` de la tabla `## Skills` regenerada de `AGENTS.md`. Alternativa A: ruta descubierta (absoluta), alineada con `.atl/skill-registry.md` y con la «ruta exacta» de la propuesta. Alternativa B: ruta relativa navegable en GitHub. **Pendiente de decisión humana.** Consecuencia ligada: R-3 — la primera regeneración reescribe la tabla de 3 columnas con enlaces a 4 con código.
- Resuelta y NO reabrir: el símbolo de versión es `var version = "v0.1.0"` en `cmd/axiom` (REQ-22.8 + cierre de O-1 del diseño).

## 9. Progreso

| Unidad | Estado | Commit | Verificación observada |
|---|---|---|---|
| F1–F2 (SDD, ya en `main`) | ✅ | `901c8f88`, `ce5e7e85` | 20/20 y 15/15 tests nuevos en verde; cero regresiones; compuertas PASS |
| ODD-0 … ODD-8 | ⬜ pendientes | — | — |

## 10. Próximo paso

Resolver O-2 (§8) y ejecutar **ODD-1 + ODD-2** como una sola unidad de trabajo delegada. ODD-4 y ODD-7 son independientes de O-2 y pueden correr en paralelo o después.

## 11. Racional de cambios aceptados

- **Salida de SDD a ODD (2026-09-22):** el alcance ya estaba especificado al completo; el envoltorio SDD multiplicaba el coste por ~3 sin aportar especificación. Autorizado por el usuario.
- **Desviación sobre el snippet de `GoInstallResolvable` (F2, cerrada):** el snippet literal de D-01 contradecía su propia tabla de aceptación en el caso «prefijo parcial sin `/v3`». Se implementó el comportamiento correcto (rechazar el sufijo de versión mayor) y se documentó el conflicto.
