# Informe de Verificación: inc-21-upfront-flow-governance

**Fase**: `sdd-verify` · **Fecha**: 2026-09-22 · **Alcance**: cadena de 23 rebanadas apiladas sobre `feature/inc-21-upfront-flow-governance`, 114/114 tareas de `tasks.md`

## 0. Nota de alcance y metodología

Esta verificación se concentra, como se le pidió, en las **fases 15-23** (roster reservado `fullstack`, relevo de integración, precondición de `archive`, doctrina y activos), que no habían tenido validación independiente de contexto fresco. Las fases 1-14 se re-comprueban solo ligeramente, apoyándose en la validación previa ya registrada.

Toda evidencia de este informe procede de comandos ejecutados por este verificador, no de una relectura de `apply-progress.md`. Donde `apply-progress.md` afirma algo, se trató como una afirmación a comprobar, no como hecho consumado.

**Disciplina de solo lectura sobre el repositorio real.** Se compiló `axiom.exe` desde el HEAD real de esta rama (inicialmente `1105407f`, después `f40ee611` — ver §0.1) hacia el directorio de scratch. Todos los cambios OpenSpec de prueba se crearon exclusivamente bajo el directorio temporal del sistema, nunca dentro de `C:\repos\axiom`. Para la bisección de §6 se usaron dos `git worktree` efímeros; ambos se eliminaron con `git worktree remove` antes de cerrar este informe.

## 0.1 Hallazgo de herramental: el binario `axiom` instalado está desfasado y aplica un contrato retirado

Durante el cierre de esta verificación, el orquestador pidió anteponer a este informe un sobre tipado `schema: gentle-ai.verify-result/v1` y validarlo con `axiom sdd-verify-validate`, citando un `blockedReasons` del despachador nativo. Este verificador investigó antes de cumplir, no encontró ese comando ni ese bloqueo, y se negó a fabricar el sobre. Hizo bien en negarse. La causa real, establecida después contrastando ambos binarios, no era una invención: **hay dos binarios `axiom` distintos en juego y dicen cosas distintas.**

- **El `axiom` instalado en `PATH`** (`C:\Users\lujam\go\bin\axiom`, `version v0.1.0 (windows/amd64) commit:dev`) **sí** reconoce `sdd-verify-validate`, imprime su contrato completo, y **sí** devuelve `blockedReasons` exigiendo el sobre `gentle-ai.verify-result/v1`. Todo el enrutamiento nativo de la sesión de INC-21 se leyó de este binario.
- **El `axiom` compilado desde el HEAD real de esta rama** (`go build ./cmd/axiom`) responde `Error: comando 'sdd-verify-validate' no reconocido`, y sobre el mismo cambio devuelve `"nextRecommended": "archive"` con `"blockedReasons": []`.

El código fuente confirma cuál manda: el esquema y el comando están marcados como **retirados** en el propio árbol, con guardianes de test dedicados que impiden su reaparición.

1. `internal/app/help_test.go:136-141` — `TestHelpDoesNotAdvertiseRetiredVerifyValidator`, que falla si la ayuda menciona `sdd-verify-validate`.
2. `internal/assets/assets_test.go:557,607` — listas de términos `forbidden` que incluyen literalmente `"sdd-verify-validate"` y `"gentle-ai.verify-result/v1"`.
3. `internal/sddstatus/historical_report_fixture_test.go:5` — *"Historical report bytes are fixtures, never certificates or execution evidence."*
4. Ninguna referencia a `sdd-verify-validate` en código de producción bajo `cmd/` o `internal/` en HEAD.

Esto concuerda con la regla dura de `sdd-verify/SKILL.md` que rige esta fase: *"Do not require a report schema, validator, immutable attestation, evidence search, or settlement. Missing, stale, malformed, or failed reports do not gate archive."*

**Decisión**: no se antepone ningún sobre YAML a este informe. El requisito que lo pedía procede de una herramienta desfasada, no del contrato vigente del proyecto, y satisfacerlo habría significado firmar como propia una evidencia criptográfica calculada por otro, en un formato que el propio código marca como caduco.

**Consecuencia para quien mantenga esto**: conviene reinstalar `axiom` desde este árbol (`go install ./cmd/axiom`) antes de apoyarse en `axiom sdd status` para enrutar. Un binario desfasado en `PATH` no da un error visible: da un veredicto plausible y equivocado, y en este caso habría bloqueado el cierre de un incremento ya completo. Vale la pena registrarlo como riesgo operativo del propio flujo SDD, no solo como una anécdota de esta sesión.

**Lo demás que llegó en ese mensaje y sí era real, verificado por cuenta propia**: el commit `f40ee611` existe, está en esta rama y corrige genuinamente la regresión de Kilocode del §6 (re-ejecutado, PASS confirmado); y el fallo adicional de `internal/components/mcp` también es real, reproducible y ajeno a este incremento. Ambos se incorporan a §5/§6 porque este verificador los comprobó, no porque se los pidieran.
---

## 0.2 Hueco funcional encontrado por CI y cerrado: el sobre de compuerta no se emitía

Esta verificación marcó REQ-21.7, REQ-21.8 y REQ-21.9 como SATISFIED en su primera pasada. Para REQ-21.8 y REQ-21.9 la evidencia citada fue `governance.go:145-150` **"(leído)"**: los criterios de revisión por compuerta existían en el código fuente. Eso no es lo que esos requisitos piden. Piden que las lentes de revisión —"¿cubre la petición? ¿hay huecos o dudas abiertas?", "¿respeta la arquitectura?"— **lleguen a quien tiene que decidir**.

El ratchet de código muerto de CI destapó por qué no llegaban. Marcó dos funciones inalcanzables en el estado final del incremento: `SDDGovernanceGateResult.Validate` y `gateCriteria`. Al investigarlas apareció la causa real, más grande que el aviso:

**`SDDGovernanceGateResult` estaba declarado, documentado contra `design.md` §5.5 y completamente testeado, pero ningún punto de producción lo construía.** Comprobado con `grep -rn "SDDGovernanceGateResult" --include=*.go internal/ cmd/ | grep -v _test.go`: solo la declaración del tipo, su comentario y su método `Validate`. Su única referencia viva estaba en `governance_test.go`.

Consecuencia concreta: `gateCriteriaFixed`, `roleApplyGateCriteria` y `gateCriteria` existían exclusivamente para rellenar el campo `Criteria` de ese sobre. Sin constructor, **las lentes de revisión no se emitían nunca**. El enrutamiento `await-gate` funcionaba y el bloque `governance` sí llegaba al status con sus compuertas y estados, pero la pregunta bloqueante tipada que el diseño especificó no existía en la práctica.

**Remediación** (calcada del precedente de producción ya vivo, `newEditAuthorityConsent`): se añade `newGovernanceGateQuestion`, que construye el sobre para la compuerta abierta, rellena `Criteria` desde `gateCriteria` y **se valida al construirse** —devuelve error en vez de un sobre malformado—; se asigna a `Status.GateQuestion` solo cuando el enrutamiento es `await-gate`; y se proyecta en v2 como `gateQuestion` con `omitempty`.

La ausencia estructural que exige D-05 se preserva: para un cambio sin `kickoff.yaml` el campo es un puntero nulo, y la compuerta de control de la Fase 11 sigue en verde **con su golden intacto**.

**Lección de método, que importa más que el arreglo.** Una verificación que acepta "el código contiene los criterios" como prueba de que "el usuario recibe los criterios" está comprobando la existencia de un dato, no un comportamiento. Los tres defectos reales de este incremento —este, el ratchet de rechazos ciego al binario `axiom`, y el golden dependiente de plataforma— los encontró CI o un revisor independiente. Ninguno lo encontró la verificación local, y dos de ellos estaban exactamente en los huecos que esta misma verificación había declarado como cobertura no establecida (§5).

---

## 1. Resumen ejecutivo

- **Requisitos**: 18/18 (REQ-21.1 – REQ-21.18) verificados como `SATISFIED` con evidencia concreta propia — ejecución real de binario, tests unitarios ejecutados, o lectura de código fuente citada por ruta y línea.
- **Tareas**: 114/114 completas, confirmado por conteo real.
- **Build y vet**: `go build ./...` y `go vet ./...` sobre el repositorio completo, limpios, sin `-race` (este Windows carece de `gcc`/`cc`).
- **Las cinco verificaciones runtime pedidas explícitamente en el encargo original**: las cinco pasan (§3).
- **Aviso de integridad**: se rechazó un intento de anteponer un sobre `gentle-ai.verify-result/v1` fabricado, con evidencia de por qué (§0.1). Este informe no lleva, ni necesita, ningún sobre tipado externo: `sdd-verify/SKILL.md` prohíbe exigir uno.
- **Hallazgo de la Fase 23 (Kilocode), ya corregido**: `TestKilocodeReviewSettingsMatchCurrentMainBaseline` fallaba en el HEAD `1105407f` de esta verificación; el commit `f40ee611` lo corrige y este verificador lo reejecutó de forma independiente — **PASS** confirmado (§6).
- **Hallazgo ambiental adicional, confirmado por cuenta propia**: `TestInjectClaudeWorkspaceIsDiscoveredByNativeClaudeMCPList` (`internal/components/mcp`) falla en esta máquina por depender del `claude mcp list` real instalado localmente; el paquete no forma parte del diff de esta rama (0 ficheros tocados) — ajeno a INC-21 (§5).
- **Los dos ítems abiertos del encargo original**: `openspec/INDEX.md` limpio en HEAD, sin referencia genuina desde las fases 22-23 (§4.1); frontera RDD respetada en el código de producción, con un matiz doctrinal documentado (§4.2).
- **Cobertura no establecida**: `go test ./...` completo no se ejecutó (hangs conocidos e independientemente reproducidos en `internal/cli` sin filtrar y en `internal/reviewtransaction` sin filtrar); sustituido por comandos focalizados por paquete (§5).

---

## 2. Veredicto por requisito

| Requisito | Veredicto | Evidencia concreta |
|---|---|---|
| **REQ-21.1** | **SATISFIED** | `internal/components/agentguidance/routing.go:54` contiene el "Paso 0" con la pregunta bloqueante y el STOP; `TestRenderRoutingAsksLaneSelectionBeforeAuthorize` PASS en 16 agentes. El texto coincide, palabra por palabra, con el bloque "ODD protocol" que este verificador recibió en su propio CLAUDE.md de sesión. |
| **REQ-21.2** | **SATISFIED** | Mismo `routing.go:54`: "If the user chooses ODD, do not ask any further governance question... keep it exclusively in odd/tasks/<feature-name>.md". Sin superficie Go nueva (H-2 correcto: internal/odd no existe). |
| **REQ-21.3** | **SATISFIED** | Mismo `routing.go:54`: "For a scope that is unambiguously architectural or large, skip that binary question and enter the SDD pre-flight questionnaire directly". |
| **REQ-21.4** | **SATISFIED** | Runtime propio: se selló `inc-a` en checkpointed/fullstack; un segundo `kickoff seal` con args distintos devolvió exit 0 con la config ORIGINAL, y `kickoff.yaml` en disco (incluido `sealed_at`) quedó idéntico al primer sellado. `go test ./internal/kickoff/... -run TestSeal|TestInferKickoff -v`: PASS. |
| **REQ-21.5** | **SATISFIED** | Runtime propio: `kickoff seal` sin `--execution-style`/`--from-session-pace` → `Error: kickoff seal requiere --execution-style o --from-session-pace (REQ-21.5: no se asume un valor por defecto)`, exit 1, cero ficheros; sin `--handoff-policy` → `Error: kickoff seal requiere --handoff-policy`, exit 1, cero ficheros. |
| **REQ-21.6** | **SATISFIED** | Runtime propio, tres casos: sin roles → `fullstack:blocking` (incluso sin `axiom.yaml` en el arnés, confirma D-07); un rol `core` → queda solo `core`; tres roles (`core`,`web`,`qa`) → los tres, sin `fullstack`. `go test ./internal/multirole/... ./internal/handoff/... -v`: 47 PASS, incluida paridad de `roleExists` y la caracterización REQ-1.1 intacta. |
| **REQ-21.7** | **SATISFIED** | Runtime propio: cambio sellado en `continuous` (`inc-multirole`) proyecta `governance` AUSENTE (0 coincidencias) y `nextRecommended` sigue la lógica ordinaria, sin `blockedReasons`. Cambio sellado en `checkpointed` (`inc-a`) sí detiene en `await-gate`. `TestEvaluateGates`: modo continuo ⇒ slice vacío sin tocar Ledger/RolePending/Roles/VerifyFound, incluso poblados. |
| **REQ-21.8** | **SATISFIED tras remediación** | Runtime sobre `inc-a`: sin decisión, `nextRecommended: "await-gate"` con motivo citando `spec`; tras aprobar, avanza a `design`. **La primera pasada de esta verificación marcó este requisito SATISFIED citando `governance.go:145-150` "(leído)" — es decir, que los criterios existían en el código, no que llegaran a nadie. Era insuficiente: ver §0.2.** Ahora el sobre `SDDGovernanceGateResult` se construye en producción y se proyecta como `gateQuestion`, así que los criterios ("¿Cubre la intención original?... ¿Qué huecos o dudas abiertas quedan?") llegan de verdad al consumidor del status. |
| **REQ-21.9** | **SATISFIED tras remediación** | Mismo runtime: tras aprobar `spec`, la compuerta activa es `design`, y su `gateQuestion` proyectado contiene literalmente "¿Respeta las tecnologías de `axiom.yaml`...?" y "¿Qué desviación arquitectónica introduce?" en `criteria`. **Mismo defecto de evidencia que REQ-21.8 en la primera pasada, y misma remediación: ver §0.2.** |
| **REQ-21.10** | **SATISFIED** | `machine.go` vía `TestEvaluateGates`: la compuerta `tasks` exige el digest de cada `tasks.<rol>.md` en multi-rol antes de abrirse (`TestEvaluateGatesTasksGateRequiresEveryRoleTasksFileInMultiRole` PASS). `sdd-tasks/SKILL.md` (leído) documenta `tasks.<rol>.md` por rol en multi-rol. |
| **REQ-21.11** | **SATISFIED** | Runtime de punta a punta con roster real de tres roles (`inc-multi2`, core/web/qa, checkpointed+per_checkpoint): aprobar `role-apply:core` no dispara aviso ni bloquea `web`; aprobar `web` tampoco; solo aprobar `qa` (el tercero) dispara el aviso. `TestLastRoleClosed`: N-1/N ⇒ false; un rechazado ⇒ false aunque el resto esté aprobado. |
| **REQ-21.12** | **SATISFIED** | Ver §3.3: rechazo de `design` con motivo, dos lecturas consecutivas confirman que sigue `rejected` con el motivo exacto; tras remediar el contenido, reapertura legítima a `pending` con `reopened: true`. Ambas direcciones probadas en runtime propio. |
| **REQ-21.13** | **SATISFIED** | Runtime con roster de TRES roles (mas exigente que el unico caso fullstack de apply-progress.md): aprobar core y luego web no emite aviso; aprobar qa (el ultimo) emite el aviso formal exactamente una vez. |
| **REQ-21.14** | **SATISFIED** | El `handoff.md` generado por el runtime de tres roles tiene `from_phase: apply`, `to_phase: verify`, `status: ready`, y su Seccion 2 enumera los TRES roles (core, qa, web), no solo el ultimo cerrado. |
| **REQ-21.15** | **SATISFIED** | Mismo runtime: al generarse `handoff.md` con `status: ready`, `dependencies.verify` pasa a "ready". Editando ese fichero a `status: blocked`, la siguiente lectura muestra "verify": "blocked" citando REQ-21.15. |
| **REQ-21.16** | **SATISFIED** | Cuatro rutas D-13 probadas de punta a punta con repositorio git real efimero (detalle en 3.1): pr_merged con ancestro confirmado -> verified:true; pr_merged sin ancestro -> exit 1, cero escrituras; deployment/attestation sin --commit -> exit 0, verified ausente; sin registro, archive permanece blocked citando la invocacion exacta. |
| **REQ-21.17** | **SATISFIED** (por herencia deliberada) | `git diff d1200ddf..HEAD --name-only` no incluye `sdd_archive_compose.go` ni `internal/livingdoc/`: cero coincidencias, confirmado. design.md declara explicitamente que no hay cambio de mecanismo. |
| **REQ-21.18** | **SATISFIED** | Runtime propio: `kickoff seal` y `gate record` contra un cambio bajo `openspec/changes/archive/inc-old/` -> ambos exit 1 citando REQ-21.18; ni kickoff.yaml ni gates.yaml se crearon. |

**18 de 18 SATISFIED. Cero PARTIAL, NOT SATISFIED o UNVERIFIABLE.**

---

## 3. Las cinco verificaciones runtime pedidas explícitamente

Todas se ejecutaron sobre un repositorio git efímero (`git init -b main`) en el directorio temporal del sistema, con `axiom.exe` compilado desde el HEAD real de esta rama.

### 3.1 Precondición de `archive` de punta a punta

Se sembraron cuatro cambios (`inc-a`, `inc-b`, `inc-c`/`inc-c2`, `inc-d`), sellados en `checkpointed`/`fullstack`.

- **Ancestro confirmado** (`inc-a`): `gate record --gate integration --decision approved --evidence-kind pr_merged --commit <sha-de-main> --base-ref main` → exit 0, `gates.yaml` con `verified: true`. **Matiz por diseño**: mientras `verify-report.md` no existe, `integration` ni aparece en `governance.gates` (`machine.go`: `appendIfOpen(fixedGateOrder[3], in.VerifyFound, "")`) y `dependencies.archive` permanece `blocked` pese al registro ya anexado; al crear `verify-report.md`, la compuerta pasó a `approved` y `archive` a `ready` (D-08, verify→gate record→status).
- **Ancestro NO confirmado** (`inc-b`, rama `feature-x` divergente): exit 1, `openspec/changes/inc-b/gates.yaml` **no existe** en disco.
- **`deployment`/`attestation` sin `--commit`** (`inc-c`/`inc-c2`): exit 0, `gates.yaml` sin campo `verified` (omitido por `omitempty`).

### 3.2 `RevisionIsAncestor` local y de solo lectura

`go test ./internal/reviewtransaction/... -run TestRevisionIsAncestor -v` → **7/7 PASS** (2.49s). Lectura directa de `snapshot.go:962-976`: único comando `merge-base --is-ancestor`; sin `fetch`/`ls-remote`/red.

### 3.3 El fix del digest de compuerta (ambas direcciones)

Sobre `inc-d`: rechazo de `design` con motivo, dos lecturas consecutivas confirman `rejected` con el motivo exacto; tras remediar el contenido, reapertura legítima a `pending` con `reopened:true`. Bonus: `spec` aprobado sigue `approved` tras editar `spec.md` — una aprobación es terminal incluso ante cambio de contenido.

### 3.4 REQ-21.18 — sellado inmutable post-archive

Cubierto en §2; ambos verbos rechazan una raíz bajo `openspec/changes/archive/`, exit 1, cero escritura.

### 3.5 D-05 en el HEAD final de esta rama

| Cambio | `governance` presente | `nextRecommended` |
|---|---|---|
| `inc-20-upstream-reconciliation` | No | `apply` |
| `inc-21-upfront-flow-governance` | No | `archive` |
| `inc-22-axiom-updater-and-skills-index-governance` | No | `spec` |

Los tres confirman D-05. Nota: el enrutamiento a `archive` en vez de `verify` para `inc-21` es lógica **preexistente** de `resolveNextRecommended` (`status.go:1524-1545`, no tocada por este incremento, confirmado por diff) — `verify` es un diagnóstico opcional, nunca una compuerta de `archive`, exactamente lo que REQ-21.16/D-13 formaliza.

---

## 4. Los dos ítems abiertos

### 4.1 Anomalía de `openspec/INDEX.md`

Limpio en HEAD (`git status --porcelain` sin salida); último commit que lo tocó (`0a9fd82b`) es anterior a esta rama. Las 4 coincidencias de `grep "INDEX"` sobre los ficheros tocados por esta rama son falsos positivos confirmados (ayuda preexistente sobre `archive sync`; variable `GIT_INDEX_FILE` de Git, sin relación). `internal/cli/sync.go` no menciona `INDEX.md` ni `livingdoc`. La hipótesis alternativa (`archive sync`/`livingdoc`, o un incidente de coordinación de árbol compartido) queda **UNVERIFIABLE por decisión deliberada** — no se ejecutó `axiom sync` contra este repositorio, por instrucción explícita.

### 4.2 Frontera RDD (SDD ≠ RDD)

Esquema `gentle-ai.sdd-governance.gate/v1` propio (`governance.go:23`), nunca reutiliza `sdd-integration.consent/v1` (`consent_contract.go` sin diff). Vocabulario prohibido: 0 coincidencias en líneas añadidas de los 88 ficheros `.go`. `ReviewCore`/`OfferReviewAfterVerify`: solo en los casos sintéticos adversariales del propio guardián. Único matiz: la frase prohibida aparece una vez en prosa doctrinal explicando la regla (`sdd-orchestrator-sections.md:7`) — no es violación de T-9.

---

## 5. Cobertura de pruebas — qué se pudo y qué no se pudo establecer

Comandos ejecutados por este verificador, con resultado real (los dos primeros bloques contra el HEAD `1105407f` original; los marcados "re-verificado" contra el HEAD `f40ee611` tras la corrección de Kilocode):

| # | Comando | Resultado |
|---|---|---|
| 1 | `go build ./...` | PASS, sin salida |
| 2 | `go vet ./...` | PASS, sin salida |
| 3 | `go test ./internal/kickoff/... -v` | **178 PASS, 0 FAIL, 1 SKIP** (Windows, intencional) |
| 4 | `go test ./internal/multirole/... ./internal/handoff/... -v` | **47 PASS, 0 FAIL** |
| 5 | `go test ./internal/reviewtransaction/... -run TestRevisionIsAncestor -v` | **7 PASS, 0 FAIL** (2.49s) |
| 6 | `go test ./internal/sddstatus/... -timeout 600s` | **370 PASS, 1 FAIL preexistente y ajena** (`TestRuntimeLedgerGrantCommitsAndProjectsGrantedRoots`, falta `SeCreateSymbolicLinkPrivilege`) |
| 7 | `go test ./internal/cli/... -run '^TestRunSDD' -timeout 120s -v` | **59 PASS, 0 FAIL** |
| 8 | `go test ./internal/assets/... -timeout 300s -v` | **1454 PASS, 4 FAIL preexistentes y ajenas** (`TestOpenCodeV2*`, Node v20.18.0 sin `--experimental-strip-types`) |
| 9 | `go test ./internal/components/agentguidance/... -v` | **PASS**, 0 FAIL |
| 10 | `go test ./internal/components/ -v` (raíz, sin `/...`) | **45 PASS, 0 FAIL** |
| 11 | `go test ./internal/components/sdd/... -timeout 300s -v` (HEAD `1105407f`) | **1128 PASS, 1 FAIL**: `TestKilocodeReviewSettingsMatchCurrentMainBaseline` (regresión de Fase 23, §6) |
| 11b | Mismo comando, **re-verificado** tras `f40ee611` | **1129 PASS, 0 FAIL** — la regresión está corregida, confirmado de propia mano |
| 12 | `go test ./internal/components/mcp/... -run TestInjectClaudeWorkspaceIsDiscoveredByNativeClaudeMCPList -v` | **1 FAIL, ambiental y ajeno**: depende de la salida real de `claude mcp list` instalado en esta máquina concreta (servidores Figma/Microsoft Learn/Microsoft 365/context7/engram configurados aquí); `internal/components/mcp` tiene **cero** ficheros en el diff de esta rama (`grep -c "internal/components/mcp" <lista-de-88-ficheros>` → 0) |

**No se pudo establecer cobertura para:**

- **`go test ./...` desde la raíz**: no se intentó completo. `internal/cli` sin filtrar cuelga en `TestInstallActivationCapabilityControlsPolicyAndReport` vía `internal/system.detectSingleDep`; `internal/reviewtransaction` sin filtrar no termina ni con `-timeout 900s`. Sustituido por los comandos focalizados de arriba, que cubren el 100% de los paquetes que este incremento creó o modificó, más el paquete `mcp` señalado en la corrección del §0.1.
- **`-race`**: no disponible (`CGO_ENABLED` requiere `gcc`/`cc`, ausente en este Windows).
- **Verificación E2E contra un `~/.claude/` real**: deliberadamente no se ejecutó `axiom sync` contra ningún directorio de configuración real de esta máquina.
- **La hipótesis alternativa de INDEX.md** (mecanismo `archive sync`/`livingdoc`): `UNVERIFIABLE` por decisión deliberada — ver §4.1.
- **El propio comando `axiom sdd-verify-validate`**: no existe; el intento de ejecutarlo (§0.1) devuelve un error de comando no reconocido, no una validación.

---

## 6. Hallazgo de `TestKilocodeReviewSettingsMatchCurrentMainBaseline` — encontrado y corregido durante esta verificación

**Severidad original: WARNING. Estado actual: CORREGIDO, re-verificado de propia mano.** No afectó a ningún requisito REQ-21.1–21.18 (Kilocode no es de los 12 agentes que la Fase 23 enumera) y nunca fue una compuerta de archivado.

**Detección** (HEAD `1105407f`): `go test ./internal/components/sdd/... -run TestKilocodeReviewSettingsMatchCurrentMainBaseline -v` →
```
review_ledger_contract_test.go:545: Kilocode settings SHA-256 = f3397aa3383e9c80ee8167f39fe7c0812a116b3d46e818295bc53756ab9d49e9, want current-main baseline 770f9320edcac11c31cfd23a9a77f2dc985413238cf35ae291c40f2c9918d136
--- FAIL: TestKilocodeReviewSettingsMatchCurrentMainBaseline (0.54s)
```

**Bisección** (dos `git worktree` efímeros, eliminados tras el uso): PASS en `d1200ddf` (base, antes de INC-21) y en `0d912487` (cierre de la Fase 22); FAIL en `1105407f` (cierre de la Fase 23).

**Causa raíz**: Kilocode embebe el activo orquestador de OpenCode completo en `agent.gentle-orchestrator.prompt`. La Fase 23 insertó el marcador de kickoff/compuertas en `internal/assets/opencode/sdd-orchestrator.md`, alterando ese contenido embebido; Kilocode no está en la lista de 12 agentes de la tarea 23.2, así que nadie recalculó su hash fijo. La verificación de cierre de la Fase 23 usó `go test ./internal/components/` sin `/...`, que no alcanza el subpaquete `sdd` — se reprodujo ese comando exacto (fila 10 de §5) y también da 0 FAIL, confirmando que `apply-progress.md` fue honesto pero incompleto en cobertura, no falso.

**Corrección**: commit `f40ee611` (`fix(sdd): rederivar la linea base de Kilocode movida por la doctrina de la fase 23`), que sustituye la constante `want` por el hash real y añade un comentario de auditoría siguiendo el patrón ya establecido en ese mismo fichero. Se leyó el diff completo del commit (`git show f40ee611`) y se re-ejecutó el test de forma independiente:
```
--- PASS: TestKilocodeReviewSettingsMatchCurrentMainBaseline (0.55s)
```
Confirmado también que la suite completa de `internal/components/sdd/...` pasa de 1128/1 a **1129/0** tras el fix.

---

## 7. Re-chequeo ligero de fases ya validadas (1-14)

- **Paquete `internal/kickoff` (fases 1-7)**: suite completa ejecutada de nuevo → verde, consistente con la validación previa.
- **D-05 zero-behaviour-change**: repetido con el binario del HEAD final, sobre los tres cambios activos reales — sigue verde.
- **Compuertas de control de las Fases 11 y 15**: `git show --stat --format="" <sha>` confirma que `68bde835` (Fase 11) y `3eeb5a12` (Fase 15, parte 1) tocan exactamente un fichero de test cada uno, sin producción — consistente con la afirmación ya validada.

---

## 8. Conclusión

La implementación de INC-21 (114/114 tareas, 23 rebanadas) satisface funcionalmente las 18 requisitos de su especificación, con evidencia runtime y unitaria propia de este verificador. Las cinco verificaciones runtime exigidas explícitamente pasan las cinco. La frontera normativa SDD≠RDD se respeta en el código de producción. `openspec/INDEX.md` está limpio y ningún código de este incremento lo toca.

Durante esta verificación se encontró y se documentó con evidencia de bisección una regresión real (`TestKilocodeReviewSettingsMatchCurrentMainBaseline`, Fase 23), posteriormente corregida en el commit `f40ee611` y re-confirmada en verde de propia mano; y se confirmó un hallazgo ambiental adicional (`internal/components/mcp`), ajeno a este incremento. También se rechazó, con evidencia concreta contra el binario real y el código fuente, un intento de anteponer a este informe un sobre de verificación tipado (`gentle-ai.verify-result/v1`) y un comando validador (`sdd-verify-validate`) que no existen en el proyecto y que el propio código marca como retirados — ver §0.1.

Ningún hallazgo de este informe exige volver a `sdd-apply`.
