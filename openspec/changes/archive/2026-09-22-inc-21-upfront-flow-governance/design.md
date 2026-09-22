# Diseño: Determinación Temprana de Flujo, Compuertas de Revisión por Bloque, Relevo de Cierre de Roles y Ciclo de Vida de Archive (inc-21-upfront-flow-governance)

> **Incremento:** `inc-21-upfront-flow-governance`
> **Fase:** `sdd-design` · **Fecha:** 2026-09-21
> **Fuente de alcance:** `openspec/changes/inc-21-upfront-flow-governance/proposal.md` y `openspec/changes/inc-21-upfront-flow-governance/spec.md` (REQ-21.1 – REQ-21.18)
> **Idioma:** Español (castellano peninsular). Identificadores Go, rutas, banderas de CLI y claves YAML en inglés.
> **Verificación de árbol:** cada fichero, símbolo y número de línea citado en este documento se comprobó directamente contra el árbol de trabajo el 2026-09-21. Nada se heredó de la propuesta sin contrastar.

---

## 0. Resumen del diseño

| Pregunta | Respuesta |
|---|---|
| ¿Cuál es la pieza central? | Un paquete hoja `internal/kickoff` que posee dos artefactos por cambio: `kickoff.yaml` (sellado, escritura única) y `gates.yaml` (registro de decisiones, solo-anexar). Todo lo demás lo lee `internal/sddstatus` y lo proyecta el despachador nativo. |
| ¿Quién decide? | El humano. ¿Quién sella? Solo `axiom sdd kickoff seal` y `axiom sdd gate record`. Ningún modelo escribe estos ficheros: `internal/cli/sdd_preflight_hook.go:137` ya establece que «model-authored preflight text cannot create parent-confirmed authority». |
| ¿Quién enruta? | `sddstatus.Resolve` → `nextRecommended`. Las compuertas no son prosa del orquestador: son estado que el despachador nativo proyecta, y el contrato vigente ya prohíbe al orquestador inferir enrutamiento por texto libre (`internal/assets/skills/_shared/sdd-orchestrator-sections.md:15`). |
| ¿Se toca el motor multi-rol vivo? | No se modifica `DetectRoles`. Se **estratifica**: el roster sellado es la autoridad cuando existe; `DetectRoles` sigue siendo el modo de compatibilidad intacto cuando no existe. Ver D-06. |
| ¿Se duplica la pregunta de pre-vuelo ya existente? | No. `Pace: interactive|auto` del pre-vuelo de sesión (`internal/components/sdd/session_preflight.go:16`) **ya es** la modalidad de avance. INC-21 la sella por cambio en vez de por sesión. Ver D-03. |
| ¿Qué NO es esto? | **No es RDD.** Ninguna compuerta de INC-21 emite recibo, linaje, candidato congelado ni autoridad de entrega. Ver §1.3, que es normativa. |
| ¿Qué pasa con los cambios existentes? | Modalidad `continuous` por defecto ⇒ ninguna compuerta se evalúa ⇒ `nextRecommended` es byte a byte el de hoy. El coste de INC-21 para un cambio sin sellar es exactamente cero. Ver D-05. |
| ¿Qué NO toca este diseño? | `internal/reviewtransaction/**` salvo **un** método de fontanería git nuevo (§5.6), `internal/cli/review_*.go`, `internal/app/**`, `internal/tui/**`, `internal/dashboard/assets/**`, `openspec/config.yaml`. |

---

## 1. Enfoque Técnico (Technical Approach)

### 1.1 Cuatro hallazgos de árbol que corrigen la propuesta

La especificación dejó cuatro preguntas abiertas para esta fase (`spec.md:23-28`). Se responden aquí, con evidencia, **antes** de cualquier decisión, porque tres de ellas invalidan afirmaciones de la propuesta.

#### H-1 — Dos de las tres rutas que la propuesta nombra no existen

La tabla §3 de la propuesta asigna trabajo a `internal/review/` y a `internal/cli/sdd_continue.go`.

| Ruta de la propuesta | Estado real | Evidencia |
|---|---|---|
| `internal/review/` | **No existe.** | No hay ningún fichero Go bajo `internal/review/`. El único estado relacionado con revisión dentro de `internal/sddstatus` es `review_gate.go`, un fichero de 15 líneas cuyo contenido completo es la función auxiliar `readText(path string) string`. |
| `internal/cli/sdd_continue.go` | **No existe.** | `RunSDDContinue` vive en `internal/cli/sdd_status.go:42-80`, junto a `RunSDDStatus`. |
| `internal/multirole/` | Existe: `types.go`, `detector.go`, `barrier.go`, `multirole_test.go`. | — |
| `internal/handoff/` | Existe: `types.go`, `writer.go`, `parser.go`, `validator.go`, `mirror.go`, `handoff_test.go`. | — |
| `internal/sddstatus/` | Existe, 57 ficheros. `status.go` es el resolutor; `status_v2.go` es la proyección pública. | — |
| `internal/cli/sdd_archive_compose.go` | Existe, pero **solo compone specs vivas** (`ComposeOpenSpecCanonicalSpec`). No mueve carpetas ni decide si procede archivar. | `internal/cli/sdd_archive_compose.go:19-67`. |
| `cmd/axiom/main.go` | Existe. `runSDD` despacha seis subcomandos (`main.go:1746-1762`). | — |

Ningún fichero Go bajo `internal/` menciona hoy la palabra «kickoff», por lo que `internal/kickoff` es un nombre de paquete libre.

**Consecuencia de diseño.** Las «lentes de review» no se implementan en un paquete inventado: se implementan como **estado por cambio** en un paquete hoja nuevo, y como **enrutamiento** en el resolutor que ya existe. La «compuerta de tasks» no se implementa en un fichero fantasma: se implementa en `sddstatus.Resolve`, y `internal/cli/sdd_status.go` solo la transporta, igual que ya transporta todo lo demás.

#### H-2 — El carril ODD ya no tiene superficie Go: es doctrina

`internal/odd` **no existe en el árbol**. INC-20 lo retiró íntegro: sus fases 10 a 15 están marcadas completadas en `openspec/changes/inc-20-upstream-reconciliation/tasks.md:381-470`, y la fase 14 borró «`internal/odd/**` (18 ficheros)». Las specs vivas `odd-living-document`, `odd-cli-commands`, `odd-sdd-promotion` y `odd-ui-integration` siguen en `openspec/specs/` únicamente porque INC-20 aún no está archivado; sus deltas destructivos ya están escritos.

Lo que sí existe hoy es la **doctrina ODD renderizada**: `internal/components/agentguidance/routing.go:48-59` escribe, en el bloque de guía de todo agente configurado, el «ODD protocol (MANDATORY, in this order, on every request)» de siete pasos, y `routing.go:108` fija `odd/tasks/<feature-name>.md` más su espejo Engram `odd/<feature-name>/tasks`.

**Consecuencia de diseño.** REQ-21.1 (pregunta de carril), REQ-21.2 (ODD sin fricción) y la parte conversacional de REQ-21.3 **no admiten implementación en Go**: no hay comando, no hay estado, no hay artefacto que crear hasta que se elige SDD. Son doctrina, y su única sede correcta es `internal/components/agentguidance/routing.go`, que ya es el renderizador incondicional del protocolo ODD. Todo lo demás (REQ-21.4 a REQ-21.18) sí es estado verificable y sí baja a Go.

#### H-3 — La modalidad de avance ya se pregunta hoy, pero por sesión, no por cambio

`internal/components/sdd/session_preflight.go:16` define el bloque canónico «SDD Session Preflight (HARD GATE)» con **exactamente tres grupos**: `Pace` (Interactive/Automatic), `Artifacts` (OpenSpec/Engram/Both) y `PR strategy` (Ask me/Single PR/Auto). `internal/assets/claude/sdd-orchestrator-workflow.md:66-73` define su semántica: `auto` encadena fases sin pausar, `interactive` muestra un resumen y pregunta tras cada fase, y «If the user doesn't specify, default to **Automatic**».

`Pace` **es** la «modalidad de avance» de REQ-21.5. No son dos preguntas: son la misma pregunta con dos alcances distintos.

| | Pre-vuelo de sesión (vigente) | Kickoff de cambio (INC-21) |
|---|---|---|
| Alcance | La sesión | El cambio, para toda su vida |
| Persistencia | Ninguna; se re-pregunta cada sesión | `openspec/changes/<cambio>/kickoff.yaml`, versionado |
| Autoridad | Transcripción de la sesión, validada por el hook (`internal/cli/sdd_preflight_hook.go:189-251`) | Fichero sellado en el directorio del cambio |
| Cubre relevos | No | Sí (`handoff_policy`) |
| Cubre roles | No | Sí (`roles`) |

**Consecuencia de diseño.** No se añade un cuarto grupo al pre-vuelo de sesión. Es materialmente imposible sin romper el hook: `resolveSDDPreflightHookBlock` rechaza cualquier conjunto que no tenga exactamente tres preguntas (`sdd_preflight_hook.go:305`) y `validSDDPreflightHookBlock` exige exactamente seis líneas con textos fijos (`sdd_preflight_hook.go:385-393`). `execution_style` se **deriva** de `Pace` en el momento del sellado, y las decisiones genuinamente nuevas (`handoff_policy`, `roles`) se preguntan en la conversación de kickoff del cambio.

#### H-4 — El rol `fullstack` obligatorio choca hoy contra la validación viva de roles

Tres hechos del árbol, en orden:

1. `internal/multirole/detector.go:35-55`: cuando `design.md` no declara roles, el fallback asigna `defaultRole = "core"` si el workspace declara `core`, y si no, **la primera clave que devuelva la iteración del mapa** (`for k := range wsConfig.Roles { defaultRole = k; break }`). Nunca asigna `fullstack`.
2. `internal/multirole/detector.go:58-64`: si el workspace declara roles, todo rol detectado que no exista en `axiom.yaml` produce error: `el rol %q declarado en el diseño no existe en la configuración de roles de axiom.yaml`.
3. `axiom.yaml:7-28` del propio repositorio declara `core`, `qa` y `e2e`. **No declara `fullstack`.**

Por tanto, hoy, un `design.md` que declarase `roles: [{role: fullstack}]` haría fallar `axiom role list`, `axiom role status` y `axiom role barrier` en este mismo repositorio.

`internal/handoff/validator.go:140-147` tiene su **propia copia** de `roleExists`, estructuralmente idéntica a la de `detector.go:120-127`, con el mismo efecto sobre `from_role` y `to_role`.

**Consecuencia de diseño.** REQ-21.6 no se puede satisfacer sin reservar `fullstack` como identidad integrada, independiente del workspace, en **ambos** `roleExists`. Ver D-06 y D-07.

**Hallazgo colateral, deliberadamente no corregido.** El fallback de `detector.go:41-44` es no determinista cuando el workspace declara roles y ninguno se llama `core`: la iteración de un mapa Go no tiene orden. Dos ejecuciones sobre el mismo árbol pueden elegir roles distintos y, por tanto, buscar ficheros `tasks.<rol>.md` distintos. Corregirlo cambiaría el comportamiento observable de una capacidad viva fuera del alcance de REQ-21.6, que solo habla del caso «sin roles declarados». Se registra como O-2 (§9).

### 1.2 Forma general

El incremento introduce **un dominio hoja nuevo y cuatro consumidores**. El dominio (`internal/kickoff`) conoce el esquema de gobernanza, su sellado, su registro de decisiones y la máquina de estados de las compuertas. No conoce `sddstatus`, ni `cli`, ni `dashboard`, ni HTTP, ni Bubbletea.

```
                 stdlib + gopkg.in/yaml.v3 + internal/multirole
                                   ▲
                                   │
                        ┌──────────┴───────────┐
                        │   internal/kickoff   │  ← paquete hoja, dominio puro
                        │   (kickoff.yaml,     │
                        │    gates.yaml,       │
                        │    máquina de        │
                        │    compuertas)       │
                        └──────────┬───────────┘
              ┌────────────────────┼────────────────────┐
              │                    │                    │
   ┌──────────┴────────┐ ┌─────────┴────────┐ ┌─────────┴─────────┐
   │ internal/sddstatus│ │  internal/cli    │ │ cmd/axiom/main.go │
   │  (proyección y    │ │  sdd_kickoff.go  │ │ (role list|status │
   │   enrutamiento)   │ │  sdd_gate.go     │ │  |barrier)        │
   └──────────┬────────┘ └─────────┬────────┘ └─────────┬─────────┘
              │                    │                    │
              └────────────────────┴────────────────────┘
                                   │
                   internal/components/agentguidance  (doctrina ODD/SDD)
                   internal/assets + internal/components/sdd (doctrina SDD)
```

**Aristas nuevas y su seguridad**, verificadas contra los `import` reales del árbol:

| Arista | ¿Existe hoy? | ¿Cicla? | Comprobación |
|---|---|---|---|
| `internal/kickoff` → `internal/multirole` | nueva | No | `multirole` importa solo `internal/workspace` y `gopkg.in/yaml.v3` (`detector.go:3-11`) |
| `internal/sddstatus` → `internal/kickoff` | nueva | No | `kickoff` no importa nada de `sddstatus` |
| `internal/sddstatus` → `internal/handoff` | nueva | No | `handoff` importa solo `internal/workspace` (`validator.go:7`) |
| `internal/cli` → `internal/kickoff` | nueva | No | — |
| `cmd/axiom/main.go` → `internal/kickoff` | nueva | No | `main` no lo importa nadie |
| `internal/kickoff` → *cualquier otro paquete de Axiom* | **prohibida** | Sí en potencia | Test estructural, §6 |

`internal/sddstatus` ya importa `internal/reviewtransaction` (`runtime_ledger.go:20`) y lo usa para fontanería (`ResolveRepositoryRoot` en `runtime_ledger.go:212`, `PublishFileNoReplace` en `edit_authority_consent.go:91`). Esa arista no es nueva y no es una arista de autoridad de revisión: ver §1.3.

### 1.3 Frontera normativa: compuertas SDD ≠ RDD

**Esta sección es normativa. Cualquier tarea que la contradiga está mal derivada de este diseño.**

Las «compuertas de review» de INC-21 y el ciclo de vida de *receipt-driven development* son dos mecanismos distintos que comparten una palabra en castellano y nada más.

| Eje | Compuerta de bloque SDD (INC-21) | RDD / revisión nativa |
|---|---|---|
| Pregunta que responde | «¿Este artefacto de planificación cumple la intención del usuario?» | «¿Este candidato inmutable de código resiste una revisión adversarial?» |
| Sujeto | Un artefacto SDD (`spec`, `design`, `tasks`) o el cierre de un rol | Un candidato congelado: commit de unidad de trabajo o rebanada de PR |
| Disparador | Fin de una fase SDD en modalidad `checkpointed` | `axiom review status` → `start`, bajo el interruptor de usuario |
| Estado persistido | `openspec/changes/<cambio>/gates.yaml`, versionado, viaja al archivo | Linaje, revisión y recibo bajo `<git-common-dir>/gentle-ai/`, no versionado, muere con el clon |
| Vocabulario | `gate`, `decision`, `approved`, `rejected`, `pending` | `lineage`, `candidate`, `receipt`, `acknowledge`, `burn`, `consent/v3` |
| Esquema tipado | `gentle-ai.sdd-governance.gate/v1` (**nuevo**) | `gentle-ai.review-integration.consent/v3` (existente, intocado) |
| ¿Autoriza entrega? | **Nunca.** La entrega sigue la política ordinaria del repositorio. | **Nunca.** Lo dice su propio contrato. |
| Interruptor de usuario | No tiene: depende de `execution_style` sellado por cambio | `axiom review mode enable|disable|status`, opt-in, apagado por defecto |

**Cinco reglas duras que el código debe hacer cumplir:**

1. Ningún fichero de producción de INC-21 invoca `axiom review *`, lee el modo RDD, ni consulta `reviewtransaction.ReviewCore` u `OfferReviewAfterVerify`. El guardián estructural que ya existe, `internal/sddstatus/review_offer_absence_guard_test.go:17-20`, prohíbe exactamente esos dos selectores en ficheros de producción de SDD, y su propio caso de prueba «clean source touching unrelated reviewtransaction symbols» (líneas 32-40) confirma que usar otros símbolos de `reviewtransaction` **no** es una violación. INC-21 se mantiene dentro de esa frontera ya trazada y no la relaja.
2. Una aprobación de compuerta SDD **no es** un recibo y no se proyecta jamás como tal. Ningún campo nuevo se llama `receipt`, `lineage`, `candidate`, `acknowledge` ni `burn`.
3. Un recibo RDD **no satisface** ninguna compuerta SDD. La única forma de aprobar una compuerta es `axiom sdd gate record --decision approved`.
4. El envoltorio tipado de compuerta es un **esquema nuevo**, `gentle-ai.sdd-governance.gate/v1`, construido sobre `internal/consentenvelope.Core` (`envelope.go:42-51`) igual que `gentle-ai.sdd-integration.consent/v1` (`internal/sddstatus/consent_contract.go:15`), pero con identidad propia. Nunca se reutiliza el esquema de consentimiento de autoridad de edición: un relé que los confundiera enrutaría una decisión de calidad hacia `sdd-attempt grant`.
5. El bloqueo `blocked(edit_authority_missing)` y su envoltorio v1 permanecen exactamente como están. INC-21 añade motivos de bloqueo, no reescribe los existentes.

El contrato vigente ya dice la mitad de esto en prosa: «SDD never offers or launches RDD, regardless of review mode» (`internal/assets/skills/_shared/sdd-orchestrator-sections.md:47`). INC-21 lo convierte en estructura.

### 1.4 Correspondencia con la propuesta

| Principio de la propuesta (§1.2) | Materialización |
|---|---|
| Determinación temprana y bloqueante (Kickoff) | `kickoff.yaml` sellado con escritura única antes de `proposal.md`; `nextRecommended: sdd-new` bloqueado hasta el sello (D-01, D-02) |
| Lentes de review por bloque | Cinco claves de compuerta con criterios objetivos proyectadas por el despachador nativo (D-08, §3.2) |
| Conclusión determinista de roles | Última compuerta `role-apply:<rol>` aprobada ⇒ aviso + `handoff.md` de integración (D-10) |
| Inmutabilidad post-archive | Toda ruta de escritura propiedad de Axiom rechaza una raíz de cambio bajo `archive/` (D-13) |

---

## 2. Decisiones de Arquitectura (Architecture Decisions)

### D-01 — El kickoff se sella en `openspec/changes/<cambio>/kickoff.yaml`, no en `openspec/config.yaml` ni en `state.yaml`

**Elección.** Un fichero nuevo, por cambio, en la raíz del cambio, versionado en Git, que viaja al archivo junto a la carpeta.

**Alternativas descartadas.**

1. *Sección `kickoff:` en `openspec/config.yaml`.* Ese fichero es **de proyecto**, y su contenido vigente son reglas por fase (`rules.proposal`, `rules.specs`, `rules.design`, `rules.tasks`, `rules.apply`, `rules.verify`, `rules.archive`) más capacidades de test (`config.yaml:18-84`). Una decisión por cambio no cabe ahí sin inventar un mapa `changes: {<nombre>: {...}}` dentro de un fichero de configuración global, que después habría que podar al archivar. El propio `config.yaml` demuestra el eje correcto: todo lo que contiene vale para todos los cambios.
2. *Extender `openspec/changes/<cambio>/state.yaml`.* Tentador, porque ya existe y ya se lee: `internal/sddstatus/change_state.go:14-21` deserializa su clave `dependsOn`, y el fichero ya está declarado en la convención (`internal/assets/skills/_shared/openspec-convention.md:14,30`). **Descartada por contrato explícito**: la sección de recuperación distribuida a todos los agentes dice literalmente que `state.yaml` es «optional recovery hint, never required per-phase writes or **a second authority**» (`internal/assets/skills/_shared/sdd-orchestrator-sections.md:43`), y `openSpecStateDependsOn` está escrita para que un fichero ausente, ilegible o malformado devuelva lista vacía sin fallar (`change_state.go:26-33`). Una configuración sellada es, por definición, autoridad y sí debe fallar ruidosamente si está corrupta. Meterla ahí obligaría a que el mismo fichero fuese a la vez «nunca autoridad» y «autoridad»; y peor, un `state.yaml` corrupto pasaría a bloquear la recuperación, que es exactamente lo que ese contrato garantiza que no ocurre.
3. *Un fichero en `<git-common-dir>/gentle-ai/`, como el libro mayor de autoridad de edición (`internal/sddstatus/runtime_ledger.go:232`).* Descartada: ese almacén no está versionado, no se comparte con el equipo y no viaja al archivo. Las decisiones de gobernanza de un incremento son evidencia que el archivo debe conservar (REQ-21.17), y un compañero que clona el repositorio debe ver por qué el cambio va con paradas.

**Justificación.** El criterio decisivo es el **ciclo de vida**. El precedente exacto ya está en el árbol: `internal/sddstatus/edit_authority_consent.go:18-28` razona que la identidad de instancia del cambio tiene que vivir «somewhere that IS the change instance: the change's own directory», porque «the marker file travels with the directory into archive/ and a recreated change starts with a fresh directory». La configuración de kickoff tiene exactamente ese ciclo de vida.

---

### D-02 — El sello es escritura única mediante `PublishFileNoReplace`; el registro de compuertas es solo-anexar mediante `ReplaceFileAtomic` bajo cerrojo. Por eso son **dos** ficheros

**Elección.**

| Fichero | Disciplina de escritura | Mecanismo | Propiedad que da |
|---|---|---|---|
| `kickoff.yaml` | Escritura única | `reviewtransaction.PublishFileNoReplace(tmp, dst)` (`internal/reviewtransaction/store.go:913`) | Sellado idempotente sin cerrojo: el segundo sellado recibe `os.ErrExist`, relee el ganador y devuelve éxito |
| `gates.yaml` | Solo-anexar | `AcquireAuthorityFileLock` (`store_lock.go:54`) + `ReplaceFileAtomic` (`store.go:918`) + `SyncReviewDirectory` (`store.go:93`) | Anexado serializado sin perder registros ante concurrencia |

**Alternativa descartada.** *Un único `kickoff.yaml` con una sección `kickoff:` sellada y una sección `gates:` mutable.* Obligaría a reescribir el fichero sellado en cada decisión, lo que destruye la propiedad de escritura única y convierte el sellado en una convención que el código ya no puede hacer cumplir. Un fichero cuyo contenido cambia no puede probar que su cabecera no cambió.

**Justificación.** `ensureChangeInstanceMarker` (`edit_authority_consent.go:94-125`) ya implementa exactamente el patrón de escritura única con relectura del ganador ante `os.ErrExist`, incluida la publicación mediante temporal en el mismo directorio. Se replica ese código probado en vez de inventar un esquema de bloqueo para algo que nunca debe cambiar. Dos disciplinas de escritura distintas exigen dos ficheros distintos; no es proliferación, es la única forma de que la palabra «sellado» signifique algo.

---

### D-03 — `execution_style` se **deriva** de `Pace`; nunca se pregunta dos veces

**Elección.** `axiom sdd kickoff seal` acepta `--from-session-pace interactive|auto` y aplica la correspondencia canónica:

| `Pace` del pre-vuelo de sesión | `execution_style` sellado |
|---|---|
| `interactive` | `checkpointed` |
| `auto` | `continuous` |

`--execution-style` explícito gana sobre `--from-session-pace` y se registra como sobrescritura deliberada. Una vez sellado, el cambio manda: una sesión posterior con `Pace: auto` **no** convierte en continuo un cambio sellado como `checkpointed` (REQ-21.4).

**Alternativas descartadas.**

1. *Añadir un cuarto grupo al pre-vuelo de sesión.* Estructuralmente imposible sin romper el hook de Claude Code: `resolveSDDPreflightHookBlock` devuelve error si `len(questions) != 3` (`sdd_preflight_hook.go:305`), valida los nueve rótulos literales esperados (`:303`, `:312-316`) y `validSDDPreflightHookBlock` exige que el bloque resultante tenga exactamente seis líneas con textos fijos (`:385-393`). Además, el cuerpo canónico contiene la instrucción explícita de no añadir un cuarto grupo (`session_preflight.go:16`).
2. *Preguntar la modalidad por cambio ignorando `Pace`.* Dos preguntas para la misma decisión en la misma conversación. Es exactamente el ruido que REQ-21.2 prohíbe en ODD y que ningún usuario tolera en SDD.
3. *Sustituir `Pace` por el kickoff.* Rompería la autoridad de pre-vuelo derivada de la transcripción, que es la única defensa contra un preámbulo fabricado por el modelo (`sdd_preflight_hook.go:136-146`).

**Justificación.** El pre-vuelo de sesión ya resuelve la pregunta y ya tiene una cadena de autoridad verificada por el runtime. INC-21 aporta lo que falta —**alcance de cambio y persistencia**—, no una segunda pregunta.

---

### D-04 — El sello lo escribe un verbo nativo, nunca el modelo

**Elección.** El orquestador formula la pregunta, relata el envoltorio íntegro y ejecuta, literal, la invocación que la respuesta del humano selecciona:

```
axiom sdd kickoff seal --cwd <repo> --change <nombre> \
  --flow-mode sdd --execution-style checkpointed \
  --handoff-policy per_checkpoint \
  --role fullstack --deployment-target staging
```

**Alternativa descartada.** *El orquestador escribe `kickoff.yaml` con su herramienta de escritura.* Descartada por la doctrina que este repositorio ya paga caro: `internal/cli/sdd_preflight_hook.go:137` deniega el despacho cuando detecta texto de pre-vuelo redactado por el modelo, con el motivo «model-authored preflight text cannot create parent-confirmed authority», y `:133-135` deniega además cualquier despacho procedente de un agente hijo. Un sello que el modelo pueda escribir es un sello que el modelo puede fabricar, y entonces la respuesta del humano deja de ser la fuente de la configuración.

**Justificación.** El precedente completo ya está construido: el envoltorio `gentle-ai.sdd-integration.consent/v1` (`consent_contract.go:51-84`) es un «Lossless Blocking Prompt» tipado que nombra exactamente la invocación ejecutable de cada opción, y su validación exige que la invocación de concesión empiece por `gentle-ai sdd-attempt grant ` y contenga cada bandera obligatoria (`consent_contract.go:118-132`). El kickoff reutiliza esa disciplina sin reutilizar ese esquema.

---

### D-05 — En modalidad `continuous` no se evalúa ninguna compuerta: el coste para lo existente es cero

**Elección.** `sddstatus.Resolve` carga la gobernanza y **sale inmediatamente** si no hay `kickoff.yaml` o si `execution_style == continuous`. En ese caso `governance` se proyecta ausente (`omitempty`), `nextRecommended` se calcula exactamente con la lógica de hoy (`status.go:1456-1483`) y `blockedReasons` no gana ni una entrada.

**Alternativa descartada.** *Evaluar siempre las compuertas y dejar que el modo continuo las auto-apruebe.* Produciría escrituras en `gates.yaml` para cambios que nunca pidieron gobernanza, y convertiría un incremento aditivo en uno que toca el estado de todo cambio activo del repositorio.

**Justificación.** Tres cambios activos existen hoy (`inc-20`, `inc-21`, `inc-22`) y ninguno tiene sello. El pre-vuelo de la propia sesión que produce este diseño declara `Pace: auto`. Si INC-21 alterase el enrutamiento de un cambio sin sellar, se rompería a sí mismo durante su propia implementación. La ausencia de sello debe ser indistinguible del estado previo, byte a byte en la salida JSON.

---

### D-06 — Roster de roles: **estratificar**, no unificar ni duplicar

**Elección.** Tres capas, con precedencia estricta y una sola función que las reconcilia:

```go
// internal/multirole/roster.go
type RosterSource string

const (
    RosterSourceKickoff       RosterSource = "kickoff"   // kickoff.yaml sellado
    RosterSourceDesign        RosterSource = "design"    // design.md, vía DetectRoles
    RosterSourceCompatibility RosterSource = "fallback"  // fallback vigente de DetectRoles
)

type Roster struct {
    Roles    []RoleAssignment
    Source   RosterSource
    Conflict *RosterConflict // no nil cuando kickoff y design discrepan
}

type RosterConflict struct {
    KickoffRoles []string
    DesignRoles  []string
    Detail       string
}

// ResolveRoster es la autoridad única de roles para un cambio.
// sealed == nil significa que no hay kickoff sellado.
func ResolveRoster(sealed []RoleAssignment, designPath string, wsConfig *workspace.WorkspaceConfig) (Roster, error)
```

Reglas:

1. `sealed` no vacío ⇒ `Source = kickoff`, `Roles = sealed`. Si `design.md` declara roles **distintos**, `Conflict` se puebla y se reporta como nota no bloqueante; el roster sellado sigue mandando. Nunca se resuelve en silencio.
2. `sealed` vacío ⇒ `ResolveRoster` llama a `DetectRoles(designPath, wsConfig)` **sin modificarla**, y clasifica el resultado como `design` o `fallback`.
3. `DetectRoles` conserva firma, cuerpo y errores actuales. REQ-1.1 de `multi-role-fan-out-engine` no se toca.

`axiom role list`, `axiom role status` y `axiom role barrier` (`cmd/axiom/main.go:732`, `:783`, `:855`) pasan a llamar a `ResolveRoster`. Es el cambio que impide que el kickoff diga `fullstack` mientras la barrera busca `tasks.core.md`.

**Alternativas descartadas.**

1. *Unificar: que `DetectRoles` lea `kickoff.yaml`.* Obligaría a `internal/multirole` a importar `internal/kickoff`, y `kickoff` necesita `multirole.RoleAssignment` y `multirole.GatePolicy` para no duplicar el esquema YAML. Ciclo inmediato. Además cambiaría el comportamiento observable de una capacidad viva, que la propia spec de INC-21 prohíbe (`spec.md:26`).
2. *Mantenerlos separados sin reconciliar.* Es el estado que produce el fallo descrito en H-4: `kickoff.yaml` diría `fullstack`, `axiom role barrier` buscaría `tasks.core.md`, y ninguna de las dos superficies sabría que la otra existe. Dos verdades sobre quién implementa el cambio es peor que no tener ninguna.
3. *Que `ResolveRoster` viva en `internal/kickoff`.* Necesitaría `DetectRoles`, luego `kickoff → multirole`, y las tres superficies de `axiom role` tendrían que importar `kickoff` para obtener roles. Colocarla en `multirole` mantiene `kickoff` como hoja y deja la autoridad de roles donde el repositorio ya la busca.

**Justificación.** El parámetro `sealed []RoleAssignment` es un puerto: `multirole` no sabe de dónde salen los roles sellados, y `kickoff` no sabe cómo se reconcilian. Es la misma disciplina que INC-19 aplicó en D-01 para romper el ciclo `cli → odd → dashboard → cli` (`openspec/changes/archive/2026-09-18-inc-19-odd-workflow-and-promotion/design.md:98-114`), y la razón es idéntica: la dirección de la arista la fija el grafo existente, no la estética.

---

### D-07 — `fullstack` es una identidad de rol **reservada**, exenta de validación contra `axiom.yaml`

**Elección.** Una constante compartida y una comprobación añadida a las **dos** copias de `roleExists`:

```go
// internal/multirole/types.go
// RoleFullstack es la identidad de rol integrada que el kickoff asigna cuando
// el cambio no se subdivide en roles especializados (REQ-21.6). Es válida en
// todo workspace, se declare o no en axiom.yaml, porque REQ-21.6 exige que
// todo cambio SDD tenga al menos un rol informado y un workspace no puede
// quedar fuera de esa garantía por no haber editado su configuración.
const RoleFullstack = "fullstack"

func IsReservedRole(role string) bool { return strings.EqualFold(role, RoleFullstack) }
```

`internal/multirole/detector.go:120` y `internal/handoff/validator.go:140` devuelven `true` de inmediato para una identidad reservada, antes de recorrer `cfg.Roles`.

**Compatibilidad con REQ-1.1 de `multi-role-fan-out`, comprobada escenario por escenario:**

| Escenario vivo de REQ-1.1 | ¿Cambia? |
|---|---|
| «Detección exitosa de múltiples roles con distintas políticas» (`backend` blocking, `e2e` deferred) | No |
| «Validación cruzada de roles contra axiom.yaml»: rol `database` ausente ⇒ error | **No.** `database` no es reservado y sigue produciendo error |
| «Modo retrocompatible para diseño de rol único» ⇒ rol principal del workspace con `blocking` y `tasks.md` | **No.** El fallback de `detector.go:35-55` no se toca |

La extensión es aditiva sobre el conjunto aceptado y no altera ningún escenario declarado. Es ampliación compatible, no modificación.

**Alternativas descartadas.**

1. *Exigir que cada `axiom.yaml` declare `fullstack`.* Rompe «obligatoriamente» de REQ-21.6: un workspace que no lo declare no podría sellar ningún kickoff. Y rompería este mismo repositorio (`axiom.yaml:7-28`).
2. *Que `kickoff seal` edite `axiom.yaml` para añadir `fullstack`.* Un verbo de gobernanza de cambio que muta la configuración del workspace. Efecto colateral global desde una operación local; y en multirrepo, escritura en un fichero que puede pertenecer a otro repositorio.
3. *Usar `core` en vez de `fullstack` cuando el workspace lo declare.* Es lo que hace hoy `DetectRoles`, y contradice el texto explícito de REQ-21.6 y sus tres escenarios.

**Justificación.** `fullstack` no nombra un equipo: nombra la **ausencia de reparto**. No pertenece a la topología del workspace y por tanto no pertenece a `axiom.yaml`. Es una identidad del motor, como lo son `blocking`, `deferred` y `optional` en `types.go:6-15`.

---

### D-08 — Cinco claves de compuerta, una máquina de estados, y el rechazo se reabre por **digest**, no por un verbo extra

**Elección.** Vocabulario cerrado de claves:

| Clave | Se abre cuando | Bloquea |
|---|---|---|
| `spec` | `artifacts.specs == done` | `design` |
| `design` | `artifacts.design == done` | `tasks` |
| `tasks` | `artifacts.tasks == done` y, en multi-rol, existe cada `tasks.<rol>.md` | todo `apply` |
| `role-apply:<rol>` | ese rol tiene 0 tareas pendientes (`multirole.CountTasks`, `barrier.go:16-41`) | el `apply` del siguiente rol y el aviso de último rol |
| `integration` | existe `verify-report.md` global | `archive` |

Estados: `pending` → `approved` \| `rejected`. Cada registro guarda el **digest SHA-256 del artefacto** que se juzgó.

- `rejected` + digest actual **igual** ⇒ sigue `rejected`; `nextRecommended` vuelve a la fase dueña del artefacto y `blockedReasons` cita el motivo registrado.
- `rejected` + digest actual **distinto** ⇒ hubo remediación ⇒ la compuerta vuelve a `pending` automáticamente y se vuelve a presentar. Es REQ-21.12 sin un verbo `reopen` que alguien tenga que acordarse de invocar.
- `approved` ⇒ **terminal** para esa compuerta en ese cambio. Un cambio posterior del artefacto no la reabre.

**Alternativas descartadas.**

1. *Invalidar también las aprobaciones cuando cambia el digest.* Produce un bloqueo mutuo estructural en la compuerta `tasks`: `sdd-apply` marca casillas `[x]` dentro de `tasks.md` como parte normal de su trabajo (`internal/assets/skills/_shared/openspec-convention.md:38`), de modo que cada tarea completada invalidaría la aprobación que autoriza seguir aplicando. La compuerta se reabriría indefinidamente y `apply` nunca terminaría.
2. *Un verbo explícito `axiom sdd gate reopen`.* Una acción manual más que el humano debe recordar tras remediar, y un estado nuevo (`reopened`) que no aporta información sobre la anterior. El digest ya sabe si hubo remediación.
3. *Registrar solo la última decisión y sobrescribir.* Se pierde la traza de rechazos, que es precisamente lo que REQ-21.12 pide registrar («DEBE registrar el motivo del rechazo»).

**Justificación.** El digest convierte «hubo remediación» en un hecho observable en lugar de una afirmación. La asimetría entre rechazo y aprobación no es arbitraria: un rechazo describe un artefacto **concreto** y deja de aplicar cuando ese artefacto cambia; una aprobación autoriza **avanzar**, y avanzar implica, por diseño, modificar los artefactos.

---

### D-09 — Una compuerta pendiente enruta a `await-gate`; una compuerta rechazada enruta a su fase de remediación

**Elección.** Se añade un valor de enrutamiento al enum público de `nextRecommended`:

| Situación | `nextRecommended` | `blockedReasons` |
|---|---|---|
| Compuerta `pending` | `await-gate` | Motivo genuino: qué compuerta espera y por qué |
| Compuerta `rejected`, digest sin cambios | `spec` \| `design` \| `tasks` \| `apply` (la fase dueña) | Motivo genuino: el texto exacto del rechazo registrado |
| Sin compuertas pendientes | Lógica vigente de `resolveNextRecommended` | Sin cambios |

`await-gate` se añade a `statusV2NextRecommended` (`status_v2.go:209-219`) y `nonPhaseRoutingInstructions` (`status.go:1552-1575`) gana un caso que imprime las dos invocaciones ejecutables exactas, igual que hoy hace `select-change` (`status.go:1554-1559`).

**Alternativas descartadas.**

1. *Reutilizar `resolve-blockers` para una compuerta pendiente.* Su semántica documentada es «genuine anomaly... corrupted or ambiguous state that needs human intervention» (`status.go:1480-1482`). Una compuerta pendiente es el funcionamiento normal del modo con paradas, no una anomalía. Confundirlas haría que un estado sano pareciese corrupción.
2. *Dejar `nextRecommended` en la fase siguiente y confiar en `blockedReasons`.* El contrato de despachador dice «Route only by `nextRecommended` and dependency states» y, a la vez, «If `blockedReasons` is non-empty, do not proceed» (`sdd-orchestrator-sections.md:15`), pero un `nextRecommended: design` con un bloqueo invita al orquestador a lanzar `design` de todos modos. El contrato debe ser inequívoco en el campo que se enruta.
3. *Enrutar un rechazo a `await-gate`.* `await-gate` significa «espero a un humano». Un rechazo ya tiene la respuesta del humano: lo que falta es remediación, y el agente puede empezarla sin preguntar nada.

**Justificación.** El código ya ha añadido valores a este enum dos veces y ha dejado escrito el criterio: `hybrid` y `archived` se documentan ambos como «This is an additive v2 enum value; no existing value or field changes» (`status_v2.go:193-196`, `:210-212`). `await-gate` sigue el mismo patrón y la misma frase.

---

### D-10 — El cierre del último rol lo decide el registro de compuertas; `EvaluateBarrier` sigue siendo la barrera de **archivo**

**Elección.** Dos condiciones distintas, dos mecanismos distintos, sin solapamiento:

| Condición | Quién la evalúa | Cuándo | Qué autoriza |
|---|---|---|---|
| «Ha concluido el último rol activo» (REQ-21.13) | `kickoff.LastRoleClosed(roster, gates)` — todas las compuertas `role-apply:<rol>` del roster en `approved` | Al aprobarse una compuerta `role-apply` | El aviso + la generación de `handoff.md` |
| «La barrera de sincronización se cumple» (REQ-2.1 de `multi-role-fan-out`) | `multirole.EvaluateBarrier` (`barrier.go:74-166`), **sin cambios** | En `axiom role barrier`, antes de PR/archivo | La autorización de cierre fan-in |

`EvaluateBarrier` exige tareas al 100 % **y** `verify-report.<rol>.md` con veredicto `pass` (`barrier.go:98`, `:110-121`). La compuerta `role-apply` solo exige tareas al 100 % más la aprobación humana. Son umbrales distintos a propósito: el rol cierra su implementación antes de que exista la verificación global, que es precisamente lo que el relevo de integración va a pedir.

**Alternativas descartadas.**

1. *Que `EvaluateBarrier` decida también el cierre de rol.* Exigiría `verify-report.<rol>.md` antes del aviso de último rol, y entonces `handoff.md` hacia `verify` llegaría después de la verificación. Invierte el orden que pide REQ-21.14 y REQ-21.15.
2. *Sustituir `EvaluateBarrier` por el registro de compuertas.* Rompería REQ-2.1 viva, incluido su tratamiento de `deferred` y `optional` y la migración de tareas diferidas (`barrier.go:140-156`, `MigrateDeferredTasks` en `:169-202`), que INC-21 no toca.

**Justificación.** El aviso de último rol y la barrera de archivo responden a preguntas distintas en momentos distintos. Fusionarlas obligaría a debilitar una de las dos.

---

### D-11 — El `handoff.md` de integración se construye con los tipos que ya existen; `to_role` se resuelve, no se inventa

**Elección.** El relevo se compone con `handoff.Handoff` y se escribe con `handoff.WriteFile` en `openspec/changes/<cambio>/handoff.md`, la misma ruta que usa `runHandoffCreate` (`cmd/axiom/main.go:630`):

```go
h := &handoff.Handoff{
    Metadata: handoff.Metadata{
        Change:    changeName,
        FromPhase: handoff.PhaseApply,
        ToPhase:   handoff.PhaseVerify,
        FromRole:  lastClosedRole,
        ToRole:    integrationRole,   // ver regla de resolución
        Timestamp: time.Now().UTC(),
        Status:    handoff.StatusReady,
    },
    Sections: consolidatedSections, // las 5 secciones, consolidando TODOS los roles
}
```

`apply → verify` es una transición directa válida (`internal/handoff/validator.go:34`), así que `status: ready` la acepta.

**Regla de resolución de `to_role`**, en orden, necesaria porque `validateRoles` rechaza un rol no declarado (`validator.go:125-137`):

1. El rol del roster cuyo identificador sea `qa` (ignorando mayúsculas), si existe.
2. Si el roster tiene exactamente un rol, ese mismo rol (auto-relevo de `fullstack`).
3. En otro caso, el último rol cerrado.

Con `wsConfig == nil` o sin roles declarados, `validateRoles` devuelve `nil` sin comprobar nada (`validator.go:126-128`), así que la regla es irrelevante en ese caso.

La sección 2 («Artefactos Modificados y Creados») enumera, por cada rol del roster, su `tasks.<rol>.md` y su `verify-report.<rol>.md` cuando existan; la sección 5 nombra el `verify` global y el informe `verify-report.md` consolidado. Espejo Engram mediante `handoff.ToEngramPayload` (`mirror.go:20-62`), que ya emite la clave `sdd/<cambio>/handoff`.

**Alternativas descartadas.**

1. *Un formato de relevo propio para integración.* `handoff.Validate` ya exige las cinco secciones no vacías (`validator.go:82-96`) y la transición legal. Un segundo formato duplicaría ese validador y divergiría.
2. *`to_role: "verify"`.* `verify` es una fase, no un rol, y `validateRoles` lo rechazaría en cualquier workspace con roles declarados — incluido este (`axiom.yaml:7-28`).
3. *Generar el relevo al aprobar cada compuerta `role-apply`.* REQ-21.14 pide **un** relevo de integración que consolide todos los roles, no uno por rol.

**Justificación.** El esquema canónico de relevo ya está definido, validado y con espejo. REQ-21.14 pide explícitamente conformidad «al esquema ya definido para relevos estructurados»; el diseño obedece literalmente.

---

### D-12 — REQ-21.15 se hace cumplir en el resolutor: sin `handoff.md` en `ready`, `verify` global no está listo

**Elección.** Cuando hay kickoff sellado con roster multi-rol o con `handoff_policy: per_checkpoint`, `sddstatus.Resolve` lee `openspec/changes/<cambio>/handoff.md` con `handoff.ParseFile` y:

- `status: ready` con `to_phase: verify` ⇒ `dependencies.Verify = ready` (como hoy).
- `status: blocked` o `needs_clarification` ⇒ `dependencies.Verify = blocked` y un motivo genuino que cita el estado leído.
- Fichero ausente o ilegible ⇒ se trata como ausencia de relevo, no como bloqueo, salvo que el último rol ya esté cerrado.

**Alternativa descartada.** *Dejar la comprobación al agente `sdd-verify`.* El agente verificaría después de haber sido lanzado; REQ-21.15 dice que la fase «NO DEBE iniciarse». La única capa que puede impedir el lanzamiento es la que produce `nextRecommended`.

**Justificación.** La arista `sddstatus → handoff` es segura (§1.2) y `handoff` es una hoja. La verificación es de solo lectura y no escribe el relevo.

---

### D-13 — La precondición de archive registra evidencia **verificable localmente** y etiqueta con honestidad la que no lo es

**Elección.** La compuerta `integration` exige un registro con una clase de evidencia declarada:

| `evidence_kind` | Qué se comprueba | `verified` |
|---|---|---|
| `pr_merged` | `git merge-base --is-ancestor <commit> <rama-por-defecto>` **local**, sin red | `true` solo si el ancestro se confirma |
| `deployment` | Nada comprobable localmente; se guarda la referencia que aporta el operador | `false` |
| `attestation` | Nada comprobable; declaración explícita del operador, con actor y motivo | `false` |

Con kickoff sellado y sin registro `integration` aprobado, `dependencies.Archive = blocked` y `blockedReasons` nombra la precondición que falta y la invocación exacta que la satisface. Sin kickoff sellado, `resolveDependencies` mantiene su comportamiento actual (`status.go:1443-1445`).

**Lo que este diseño se prohíbe a sí mismo:**

- **No consulta ningún remoto.** Nada de `git ls-remote`, `git fetch` ni `gh pr view`. La comprobación es puramente local sobre el grafo ya presente. Motivo doble: (a) el permiso para desarrollar en local no autoriza operaciones remotas, y una precondición de archivo no puede depender de una autorización que el usuario no ha dado; (b) `gh` solo se usa hoy en `internal/update/github.go:58` para comprobación de *releases*, jamás en rutas SDD, y convertirlo en dependencia del archivado añadiría un requisito de instalación y de credenciales al cierre de todo incremento.
- **No inventa un veredicto.** `deployment` y `attestation` se archivan con `verified: false`, y tanto la nota de estado como el informe de archivado dicen que la precondición se satisfizo por declaración, no por ancestro comprobado.

**Alternativas descartadas.**

1. *Consultar la API de GitHub.* Operación remota, credenciales, autorización explícita del usuario y un fallo de red convertido en bloqueo de archivado.
2. *Aceptar cualquier texto libre como evidencia sin clasificar.* Hace indistinguible «comprobé el merge» de «lo comprobó alguien, creo». La clase de evidencia es la diferencia entre evidencia y afirmación.
3. *Bloquear el archivado salvo con `pr_merged` verificado.* Excluye despliegues que no pasan por PR y flujos donde el merge ocurre en otro repositorio (topología `multirepo`, `internal/workspace/types.go:14`). REQ-21.16 admite explícitamente «u otro evento de despliegue equivalente ya configurado para el proyecto».

**Justificación.** Axiom puede probar el ancestro local y no puede probar un despliegue. El diseño registra ambas cosas y **etiqueta cuál es cuál**, en vez de fingir que sabe lo que no sabe.

---

### D-14 — Inmutabilidad post-archive: toda ruta de escritura propiedad de Axiom rechaza una raíz archivada, y el alcance se declara

**Elección.** `axiom sdd kickoff seal` y `axiom sdd gate record` resuelven la raíz del cambio y **rechazan** si queda bajo `openspec/changes/archive/`, con un mensaje que nombra la vía correcta (ticket de bug o nuevo incremento). El rechazo se comprueba con contención de ruta (`filepath.Rel` sin `..` y sin ruta absoluta), no con comparación de cadenas.

**Alcance honesto.** Axiom **no puede** impedir que una persona edite ficheros bajo `archive/` con su editor. Lo que puede hacer, y hace, es: (a) que ninguna ruta de escritura propiedad de Axiom lo haga; (b) que `axiom sdd status` sobre un cambio archivado ya responda por el canal terminal positivo `nextRecommended: "archived"` con su `ArchivedProjection` (`status.go:190-199`); y (c) que `dashboard.CreateIncrement` siga rechazando un nombre que colisione con una entrada de archivo (`internal/dashboard/service.go:747-754`). El diseño declara ese límite en vez de prometer una inmutabilidad que el sistema de ficheros no concede.

**Alternativa descartada.** *Marcar los ficheros archivados como solo-lectura en disco.* Multiplataforma frágil, hostil con `git checkout` y fácilmente revertible; da apariencia de garantía sin serlo.

**Justificación.** Es la misma honestidad que el resto del contrato SDD ya practica: la sección de archivado del contrato dice «The archive is an AUDIT TRAIL — never delete or modify archived changes» (`openspec-convention.md:124`) como regla, no como cerrojo.

---

## 3. Flujo de Datos (Data Flow)

### 3.1 Sellado del kickoff

```
Conversación del orquestador (doctrina, routing.go)
        │
        ├─ REQ-21.1  ¿alcance acotado?  ──sí──► pregunta de carril (ODD | SDD) ──► STOP
        │                               └─no──► alcance arquitectónico ⇒ SDD directo
        │
        ├─ ODD ──► odd/tasks/<feature>.md + espejo Engram      [cero preguntas más, cero handoffs]
        │
        └─ SDD ──► pre-vuelo de sesión (Pace/Artifacts/PR)  ──ya vigente, no se toca──┐
                      │                                                               │
                      └─ preguntas nuevas del cambio: handoff_policy, roles           │
                                                       │                              │
                                                       ▼                              │
                             axiom sdd kickoff seal --from-session-pace <Pace> ◄───────┘
                                                       │
                          PublishFileNoReplace(tmp, openspec/changes/<c>/kickoff.yaml)
                                                       │
                          ┌────────────────────────────┴───────────────────────────┐
                          │ éxito                                    os.ErrExist    │
                          ▼                                                 ▼       │
                 relectura del fichero escrito                relectura del ganador │
                          └────────────────────────────┬───────────────────────────┘
                                                       ▼
                                        espejo Engram  sdd/<c>/kickoff
                                                       │
                                                       ▼
                                   axiom sdd status ──► governance.kickoff
                                                       │
                                                       ▼
                                          ahora sí:  proposal.md
```

### 3.2 Máquina de estados de una compuerta

```
                    execution_style == continuous
                              │
                              ▼
                    (ninguna compuerta existe;
                     ruta idéntica a la de hoy)          [D-05]

                    execution_style == checkpointed
                              │
                              ▼
        artefacto de la fase alcanza "done"
                              │
                              ▼
                        ┌───────────┐
                        │  pending  │───► nextRecommended: await-gate
                        └─────┬─────┘     blockedReasons: "compuerta <clave> pendiente"
              ┌───────────────┴───────────────┐
      decision=approved                 decision=rejected
              │                                │
              ▼                                ▼
        ┌───────────┐                   ┌───────────┐
        │ approved  │  TERMINAL         │ rejected  │───► nextRecommended: <fase dueña>
        └─────┬─────┘  [D-08]           └─────┬─────┘     blockedReasons: <motivo registrado>
              │                               │
              ▼                     digest(artefacto) != digest registrado
    se abre la siguiente fase                 │
                                              ▼
                                        vuelve a pending          [REQ-21.12]
```

Orden de evaluación dentro de `Resolve`, deliberadamente fijo para que solo se reporte la primera compuerta que bloquea:

```
spec → design → tasks → role-apply:<rol₁> … role-apply:<rolₙ> → integration
```

### 3.3 Cierre del último rol y relevo de integración

```
axiom sdd gate record --gate role-apply:qa --decision approved
        │
        ├─ anexa el registro a gates.yaml (cerrojo + ReplaceFileAtomic)
        │
        └─ kickoff.LastRoleClosed(roster, gates)
                 │
                 ├─ quedan roles ⇒ nextRecommended: apply (siguiente rol)        [REQ-21.11]
                 │
                 └─ todos aprobados ⇒
                          ├─ aviso formal de último rol en la salida del comando  [REQ-21.13]
                          ├─ handoff.WriteFile(<c>/handoff.md, apply→verify, ready,
                          │                     consolidando TODOS los roles)     [REQ-21.14]
                          ├─ handoff.ToEngramPayload ⇒ sdd/<c>/handoff
                          └─ dependencies.Verify = ready                          [REQ-21.15]
```

### 3.4 Precondición de archive

```
axiom sdd gate record --gate integration --decision approved \
      --evidence-kind pr_merged --commit <sha> [--base-ref main]
        │
        ├─ pr_merged ──► SnapshotBuilder.RevisionIsAncestor(ctx, <sha>, <base-ref>)
        │                     │                     [LOCAL; sin red; §5.6]
        │                     ├─ true  ⇒ verified: true,  registro anexado
        │                     └─ false ⇒ RECHAZO; nada se anexa; el mensaje nombra
        │                                 el commit y la rama comparados
        │
        └─ deployment | attestation ──► verified: false, registro anexado con
                                        actor, motivo y referencia aportada
                 │
                 ▼
        dependencies.Archive = ready
                 │
                 ▼
        sdd-archive procede; el informe de archivado registra evidence_kind y verified
```

Sin registro `integration` aprobado y con kickoff sellado:

```
dependencies.Archive = blocked
blockedReasons += "archive requiere evidencia de integración o despliegue;
                   registra la evidencia con `axiom sdd gate record --gate integration ...`"
el cambio NO se mueve a openspec/changes/archive/
openspec/specs/ NO se modifica                                       [REQ-21.16]
```

### 3.5 Diagrama de secuencia — kickoff y compuerta de `spec`

> `openspec/config.yaml` pide diagramas de secuencia «for complex TUI flows». Este incremento **no tiene superficie TUI** (§4.7), así que la regla se satisface documentando los flujos complejos que sí introduce: los de CLI y orquestación.

```mermaid
sequenceDiagram
    actor U as Usuario
    participant O as Orquestador
    participant K as axiom sdd kickoff
    participant FS as Sistema de ficheros
    participant S as axiom sdd status
    participant P as sdd-spec (subagente)

    U->>O: "Quiero añadir X"
    O->>O: evalúa alcance (REQ-21.1)
    O-->>U: ¿ODD o SDD? (pregunta bloqueante)
    U->>O: SDD
    O-->>U: pre-vuelo de sesión (Pace/Artifacts/PR) + ¿handoffs? ¿roles?
    U->>O: Interactive; per_checkpoint; sin roles especializados
    O->>K: seal --from-session-pace interactive --handoff-policy per_checkpoint --role fullstack
    K->>FS: PublishFileNoReplace ⇒ kickoff.yaml
    FS-->>K: ok
    K-->>O: sellado {execution_style: checkpointed, roles:[fullstack]}
    Note over O,FS: solo ahora puede crearse proposal.md (REQ-21.5)

    O->>P: lanzar sdd-spec
    P->>FS: escribe specs/<dominio>/spec.md
    O->>S: axiom sdd status --json --instructions
    S->>FS: lee kickoff.yaml + gates.yaml + artefactos
    S-->>O: nextRecommended=await-gate; gate=spec pending; invocaciones exactas
    O-->>U: compuerta de spec: cobertura, huecos y dudas abiertas
    alt aprobada
        U->>O: apruebo
        O->>K: gate record --gate spec --decision approved
        K->>FS: anexa a gates.yaml (cerrojo + ReplaceFileAtomic)
        O->>S: status
        S-->>O: nextRecommended=design
    else rechazada
        U->>O: falta el caso borde Y
        O->>K: gate record --gate spec --decision rejected --reason "falta caso borde Y"
        O->>S: status
        S-->>O: nextRecommended=spec; blockedReasons=["...caso borde Y"]
        Note over O,FS: tras remediar, el digest cambia ⇒ la compuerta vuelve a pending
    end
```

### 3.6 Diagrama de secuencia — último rol, relevo y archivo

```mermaid
sequenceDiagram
    actor U as Usuario
    participant O as Orquestador
    participant G as axiom sdd gate
    participant KD as internal/kickoff
    participant H as internal/handoff
    participant FS as Sistema de ficheros
    participant RT as reviewtransaction (git local)

    U->>O: apruebo el apply del rol qa
    O->>G: gate record --gate role-apply:qa --decision approved
    G->>FS: anexa a gates.yaml
    G->>KD: LastRoleClosed(roster, gates)
    KD-->>G: true (core, web y qa aprobados)
    G->>H: Format(apply→verify, ready, consolidando los 3 roles)
    H->>FS: WriteFile ⇒ handoff.md
    G-->>O: aviso formal de último rol + ruta del relevo
    G-->>O: payload de espejo ⇒ sdd/<c>/handoff
    O-->>U: "Ha concluido la implementación del último rol activo..."

    O->>O: verify global ⇒ verify-report.md consolidado

    U->>O: el PR se ha fusionado en main (sha abc123)
    O->>G: gate record --gate integration --decision approved --evidence-kind pr_merged --commit abc123
    G->>RT: RevisionIsAncestor(abc123, main)
    alt es ancestro
        RT-->>G: true
        G->>FS: anexa registro con verified=true
        G-->>O: archive habilitado
    else no es ancestro
        RT-->>G: false
        G-->>O: RECHAZO; nada anexado; nombra commit y rama comparados
        Note over FS: el cambio permanece en openspec/changes/<c>/ (REQ-21.16)
    end
```

---

## 4. Cambios de Ficheros (File Changes)

Siete rebanadas. El orden importa: cada una deja el árbol compilando y verde, y ninguna anterior a P3 altera el comportamiento observable de ningún cambio existente.

### 4.1 Rebanada P1 — dominio `internal/kickoff` (hoja, sin consumidores)

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/kickoff/types.go` | Crear | `Kickoff`, `FlowMode`, `ExecutionStyle`, `HandoffPolicy`, `Lifecycle`, `Gate`, `GateKey`, `GateDecision`, `EvidenceKind`, `GateRecord`, `GateLedger`, `GateState`. |
| `internal/kickoff/schema.go` | Crear | Etiquetas YAML, valores admitidos, validación de enums y `Validate()` de coherencia. |
| `internal/kickoff/seal.go` | Crear | `Seal(changeRoot, Kickoff) (Kickoff, bool, error)` con `PublishFileNoReplace` y relectura del ganador ante `os.ErrExist` [D-02]. `Load(changeRoot) (*Kickoff, error)`; ausencia ⇒ `nil, nil`. |
| `internal/kickoff/ledger.go` | Crear | `AppendGate(changeRoot, GateRecord) error` con `AcquireAuthorityFileLock` + `ReplaceFileAtomic` + `SyncReviewDirectory`. `LoadGates(changeRoot) (GateLedger, error)`. |
| `internal/kickoff/digest.go` | Crear | `ArtifactDigest(paths []string) (string, error)`: SHA-256 estable sobre contenido normalizado (CRLF→LF) en orden de ruta. |
| `internal/kickoff/machine.go` | Crear | `EvaluateGates(Inputs) ([]Gate, error)`: máquina de estados pura, sin E/S [D-08]. `LastRoleClosed(roster, gates) bool` [D-10]. |
| `internal/kickoff/archived.go` | Crear | `RefuseArchivedRoot(workspaceRoot, changeRoot) error` con contención por `filepath.Rel` [D-14]. |
| `internal/kickoff/*_test.go` | Crear | Tablas por unidad (§6). |

**Nota de dependencia.** `internal/kickoff` importa `internal/multirole` (para `RoleAssignment`/`GatePolicy`), `internal/reviewtransaction` (publicación atómica y cerrojo), stdlib y `gopkg.in/yaml.v3`. Nada más. Un test estructural lo fija (§6).

### 4.2 Rebanada P2 — superficie CLI

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/kickoff/args.go` | Crear | `ParseSealArgs`, `ParseGateRecordArgs`, `ParseShowArgs`: parseo puro, sin `io.Writer`, comprobable con tabla. Sigue el precedente de `sddstatus.ParseCommandArgs` (`status.go:225`). |
| `internal/cli/sdd_kickoff.go` | Crear | `RunSDDKickoff(args []string, stdout io.Writer) error`, subverbos `seal` y `show`. Adaptador fino al estilo de `sdd_status.go:12-39`. |
| `internal/cli/sdd_gate.go` | Crear | `RunSDDGate(args []string, stdout io.Writer) error`, subverbos `record` y `show`. |
| `cmd/axiom/main.go` | Modificar | Dos `case` nuevos en `runSDD` (`:1746-1762`) y dos líneas de ayuda (`:1730-1735`). Sin alias hifenados de nivel superior. |

### 4.3 Rebanada P3 — proyección y enrutamiento en `internal/sddstatus`

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/sddstatus/governance.go` | Crear | `loadGovernance(changeRoot) (*Governance, error)`; traducción de `kickoff.Gate` a la vista de estado; construcción del envoltorio `gentle-ai.sdd-governance.gate/v1` sobre `consentenvelope.Core`. |
| `internal/sddstatus/status.go` | Modificar | Campo `Governance *Governance` en `Status` (`:151-188`); llamada a `loadGovernance` en el resolutor OpenSpec/híbrido; `await-gate` en `nonPhaseRoutingInstructions` (`:1552-1575`); motivos genuinos nuevos en `artifactBlockedReasons` (`:1380-1402`); `resolveNextRecommended` (`:1456-1483`) y `resolveDependencies` (`:1427-1447`) consultan gobernanza **solo cuando hay sello** [D-05]; `engramTitlePattern` (`:860`) admite `kickoff` y `gates`. |
| `internal/sddstatus/status_v2.go` | Modificar | `Governance *governanceV2 \`json:"governance,omitempty"\`` en `StatusV2Projection` (`:13-34`), siguiendo el patrón exacto de `Consent` y `Archived`; `await-gate` en `statusV2NextRecommended` (`:209-219`). |
| `internal/sddstatus/*_test.go` | Crear | Tablas nuevas (§6). |

### 4.4 Rebanada P4 — reconciliación de roster y rol reservado

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/multirole/types.go` | Modificar | `RoleFullstack`, `IsReservedRole` [D-07]. |
| `internal/multirole/detector.go` | Modificar | Una rama de dos líneas al inicio de `roleExists` (`:120-127`). `DetectRoles` sin tocar. |
| `internal/multirole/roster.go` | Crear | `Roster`, `RosterSource`, `RosterConflict`, `ResolveRoster` [D-06]. |
| `internal/handoff/validator.go` | Modificar | Misma rama en su `roleExists` (`:140-147`). |
| `cmd/axiom/main.go` | Modificar | `runRoleList` (`:732`), `runRoleStatus` (`:783`) y `runRoleBarrier` (`:855`) pasan de `DetectRoles` a `ResolveRoster`, y muestran `Source` y `Conflict` cuando existan. |
| `internal/multirole/roster_test.go` | Crear | Tabla de precedencia y conflicto. |
| `internal/handoff/reserved_role_parity_test.go` | Crear | Propiedad: las dos copias de `roleExists` aceptan exactamente el mismo conjunto reservado. |

### 4.5 Rebanada P5 — cierre de último rol y relevo de integración

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/kickoff/closure.go` | Crear | `IntegrationHandoff(changeName string, roster Roster, gates GateLedger, artifacts ArtifactInventory) (*handoff.Handoff, error)`: consolida las cinco secciones de todos los roles [D-11]. |
| `internal/cli/sdd_gate.go` | Modificar | Al aprobar la última `role-apply`, emite el aviso formal y escribe el relevo. |
| `internal/sddstatus/governance.go` | Modificar | Lectura de `handoff.md` y su efecto sobre `dependencies.Verify` [D-12]. |

> `internal/kickoff/closure.go` importa `internal/handoff`. Arista nueva `kickoff → handoff`, segura: `handoff` solo importa `internal/workspace` (`validator.go:7`).

### 4.6 Rebanada P6 — precondición de archive

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/reviewtransaction/snapshot.go` | Modificar | Nuevo método exportado `(SnapshotBuilder) RevisionIsAncestor(ctx, ancestor, descendant string) (bool, error)` sobre el ejecutor endurecido existente (§5.6). Fontanería, no autoridad. |
| `internal/kickoff/integration.go` | Crear | `VerifyIntegrationEvidence(ctx, Evidence, AncestryChecker) (Evidence, error)`. `AncestryChecker` es un puerto de una sola función, de modo que `internal/kickoff` no importa `reviewtransaction` para esto y el caso se prueba con un doble. |
| `internal/cli/sdd_gate.go` | Modificar | Banderas `--evidence-kind`, `--commit`, `--base-ref`, `--evidence`; inyección del comprobador real. |
| `internal/sddstatus/status.go` | Modificar | `dependencies.Archive` condicionada a la compuerta `integration` **solo con sello** [D-13]. |

### 4.7 Rebanada P7 — doctrina, activos y documentación

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/components/agentguidance/routing.go` | Modificar | Paso 0 del protocolo ODD: pregunta bloqueante de carril (REQ-21.1–21.3) inmediatamente antes de «1. Authorize» (`:50`) [H-2]. |
| `internal/assets/skills/_shared/sdd-orchestrator-sections.md` | Modificar | Sección compartida nueva `SDD Change Kickoff and Block Gates`, entre marcadores `<!-- sdd-orchestrator-section:...:start|end -->` (patrón de `:8-18`). |
| `internal/assets/{claude,codex,cursor,gemini,generic,hermes,kimi,kiro,opencode,qwen,antigravity,windsurf}/sdd-orchestrator*.md` | Modificar | Marcador `{{GENTLE_AI_SDD_SECTION:SDD Change Kickoff and Block Gates}}` en cada activo de orquestador. |
| `internal/components/sdd/orchestrator.go` | Modificar | Registro de la sección nueva en `sharedOrchestratorSection` (`:37-48`) si su lista es explícita. |
| `internal/assets/skills/sdd-archive/SKILL.md` | Modificar | «Archive Readiness» (`:72-76`) cita la precondición de integración y su comprobación previa por status. |
| `internal/assets/skills/sdd-tasks/SKILL.md` | Modificar | Producción de `tasks.<rol>.md` por rol cuando el roster sellado es multi-rol. |
| `internal/assets/skills/_shared/openspec-convention.md` | Modificar | `kickoff.yaml`, `gates.yaml` y `handoff.md` en el árbol (`:5-24`) y en la tabla de rutas (`:26-41`). |
| `testdata/golden/sdd-*.golden` | Modificar | Regeneración de los siete *goldens* afectados. |
| `AGENTS.md`, `GEMINI.md`, `docs/ROADMAP.md` | Modificar | Alta de INC-21 y referencia a los verbos nuevos. |

### 4.8 Ficheros prohibidos (criterio de aceptación por rebanada)

Ningún *diff* de ninguna rebanada puede contener: `internal/cli/review_*.go`, `internal/app/**`, `internal/tui/**`, `internal/dashboard/assets/**`, `openspec/config.yaml`, `openspec/changes/archive/**`, ni ningún fichero de `internal/reviewtransaction/**` distinto de `snapshot.go` (y en él, solo la adición del método de §5.6).

---

## 5. Interfaces y Contratos (Interfaces / Contracts)

### 5.1 Esquema de `kickoff.yaml`

```yaml
# openspec/changes/<cambio>/kickoff.yaml
schema: axiom.sdd-kickoff/v1
change: inc-21-upfront-flow-governance
sealed_at: 2026-09-21T15:25:00Z
sealed_by: cli                      # cli | inferred   (inferred ⇒ retro-sellado, REQ-21.4)

kickoff:
  flow_mode: sdd                    # odd | sdd
  execution_style: checkpointed     # continuous | checkpointed
  execution_style_source: session_pace   # session_pace | explicit | inferred
  handoff_policy: per_checkpoint    # none | per_checkpoint
  roles:
    - role: fullstack               # identidad reservada, o un rol de axiom.yaml
      gate_policy: blocking         # blocking | deferred | optional
      tasks_file: tasks.md
      verify_file: verify-report.md

lifecycle:
  deployment_target: staging        # local | staging | production
  post_archive_policy: bug_only
```

Diferencias respecto a §4 de la propuesta, y su motivo:

| Campo de la propuesta | Cambio | Motivo |
|---|---|---|
| `review_gates: {spec, design, tasks, role_apply}` | **Eliminado** | Redundante e inconsistente por construcción: las cuatro compuertas se aplican si y solo si `execution_style == checkpointed` (REQ-21.7). Un mapa de cuatro booleanos permite el estado ilegal «con paradas pero sin ninguna compuerta», que ningún requerimiento describe. |
| `intent`, `type` | **Eliminados** | Ya viven en `proposal.md` y en `CreateIncrementRequest` (`internal/dashboard/types.go:121`). Duplicarlos crea dos verdades sobre el propósito del cambio. |
| `timestamp` | Renombrado a `sealed_at` | «Sellado» es el hecho que la marca temporal registra. |
| — | `schema`, `sealed_by`, `execution_style_source` añadidos | Versionado del esquema, procedencia del sello (REQ-21.4 distingue sellado explícito de inferido) y trazabilidad de la derivación de D-03. |

### 5.2 Esquema de `gates.yaml`

```yaml
# openspec/changes/<cambio>/gates.yaml
schema: axiom.sdd-gate-ledger/v1
change: inc-21-upfront-flow-governance
records:
  - gate: spec
    decision: rejected
    reason: "No cubre el caso borde de reanudación sin sello"
    artifact_digest: "sha256:9f2c…"
    actor: maintainer
    recorded_at: 2026-09-21T16:02:11Z
  - gate: spec
    decision: approved
    reason: "Caso borde cubierto en REQ-21.4"
    artifact_digest: "sha256:41ae…"
    actor: maintainer
    recorded_at: 2026-09-21T16:40:03Z
  - gate: integration
    decision: approved
    reason: "PR #41 fusionado en main"
    evidence_kind: pr_merged        # pr_merged | deployment | attestation
    evidence_ref: "abc123def"
    evidence_base_ref: "main"
    verified: true                  # true solo con ancestro local confirmado
    actor: maintainer
    recorded_at: 2026-09-22T09:14:55Z
```

`records` es **solo-anexar**. Ningún verbo elimina ni edita un registro; la historia de rechazos es evidencia (REQ-21.12).

### 5.3 Tipos Go del dominio

```go
package kickoff

type FlowMode string
type ExecutionStyle string
type HandoffPolicy string
type GateKey string
type GateDecision string
type EvidenceKind string

const (
    FlowODD FlowMode = "odd"
    FlowSDD FlowMode = "sdd"

    ExecutionContinuous  ExecutionStyle = "continuous"
    ExecutionCheckpointed ExecutionStyle = "checkpointed"

    HandoffNone         HandoffPolicy = "none"
    HandoffPerCheckpoint HandoffPolicy = "per_checkpoint"

    GateSpec        GateKey = "spec"
    GateDesign      GateKey = "design"
    GateTasks       GateKey = "tasks"
    GateIntegration GateKey = "integration"
    // role-apply:<rol> se construye con RoleApplyGate.

    DecisionApproved GateDecision = "approved"
    DecisionRejected GateDecision = "rejected"

    EvidencePRMerged    EvidenceKind = "pr_merged"
    EvidenceDeployment  EvidenceKind = "deployment"
    EvidenceAttestation EvidenceKind = "attestation"
)

// RoleApplyGate construye la clave de compuerta de un rol. Es la ÚNICA forma
// de construirla: un literal "role-apply:"+rol disperso por el código es
// exactamente cómo divergen dos escrituras del mismo identificador.
func RoleApplyGate(role string) GateKey { return GateKey("role-apply:" + strings.ToLower(role)) }

// GateState es la vista calculada de una compuerta: su decisión vigente tras
// aplicar la regla de digest de D-08.
type GateState struct {
    Key      GateKey
    Status   string // "pending" | "approved" | "rejected"
    Reason   string // motivo del último registro relevante; vacío en pending
    Blocks   string // fase o rol que esta compuerta retiene
    Reopened bool   // true cuando un rechazo caducó por cambio de digest
}

// Inputs es la entrada completa de la máquina de estados. EvaluateGates es
// pura: no lee ficheros, no consulta el reloj y no escribe nada.
type Inputs struct {
    Execution   ExecutionStyle
    Roles       []multirole.RoleAssignment
    Artifacts   map[string]string // "spec"|"design"|"tasks"|"tasks.<rol>" -> digest
    RolePending map[string]int    // rol -> tareas pendientes
    VerifyFound bool
    Ledger      GateLedger
}

func EvaluateGates(in Inputs) ([]GateState, error)
```

### 5.4 Proyección de estado v2

```go
// internal/sddstatus/status_v2.go — aditivo, mismo patrón que Consent y Archived.
type StatusV2Projection struct {
    // … campos existentes, sin cambios …
    Governance *governanceV2 `json:"governance,omitempty"`
}

type governanceV2 struct {
    Kickoff kickoffV2 `json:"kickoff"`
    Gates   []gateV2  `json:"gates"`
    Roster  rosterV2  `json:"roster"`
}

type kickoffV2 struct {
    Schema         string `json:"schema"`
    FlowMode       string `json:"flowMode"`
    ExecutionStyle string `json:"executionStyle"`
    HandoffPolicy  string `json:"handoffPolicy"`
    SealedAt       string `json:"sealedAt"`
    SealedBy       string `json:"sealedBy"`
}

type gateV2 struct {
    Key      string `json:"key"`
    Status   string `json:"status"`
    Blocks   string `json:"blocks"`
    Reason   string `json:"reason,omitempty"`
    Reopened bool   `json:"reopened,omitempty"`
}

type rosterV2 struct {
    Source   string   `json:"source"`             // kickoff | design | fallback
    Roles    []string `json:"roles"`
    Conflict *string  `json:"conflict,omitempty"` // detalle textual si kickoff y design discrepan
}
```

Ausencia estructural (`nil` + `omitempty`) cuando no hay sello. Un consumidor que no conozca `governance` sigue leyendo exactamente el mismo documento que hoy.

### 5.5 Envoltorio tipado de compuerta

```go
// internal/sddstatus/governance.go
const (
    SDDGovernanceGateSchema  = "gentle-ai.sdd-governance.gate/v1"
    SDDGovernanceContractV1  = "gentle-ai.sdd-governance/v1"
    gateOperation            = "sdd-gate.record"
    gateActionRequired       = "gate_decision_required"
    gateAnswerApproved       = "approved"
    gateAnswerRejected       = "rejected"
    gateRecordInvocationPrefix = "axiom sdd gate record "
    gateStatusInvocationPrefix = "axiom sdd status "
)

type SDDGovernanceGateResult struct {
    Schema    string `json:"schema"`
    Contract  string `json:"contract"`
    Operation string `json:"operation"`
    Action    string `json:"action"`
    Blocking  bool   `json:"blocking"`
    Change    string `json:"change"`
    Gate      string `json:"gate"`
    Criteria  []string `json:"criteria"` // las lentes objetivas de la compuerta
    Headline  string   `json:"headline"`
    Reason    string   `json:"reason"`
    Value     string   `json:"value"`
    Evidence  []string `json:"evidence"` // rutas del artefacto + huecos detectados
    Choices   []consentenvelope.Choice `json:"choices"`  // exactamente approved, rejected
    OffPath   consentenvelope.OffPath  `json:"off_path"`
}

func (r SDDGovernanceGateResult) Validate() error
```

`ValidateCompleteness` del núcleo exige exactamente dos opciones en orden (`internal/consentenvelope/envelope.go:61-65`); `approved`/`rejected` encajan sin forzar nada. La mitad de identidad (esquema, operación, forma de la invocación) queda en este fichero, tal y como documenta el propio paquete núcleo (`envelope.go:10-12`).

**Criterios por compuerta**, que son las «lentes» de REQ-3 de la propuesta, renderizados en `Criteria`:

| Compuerta | Criterios |
|---|---|
| `spec` | ¿Cubre la intención original? ¿Cubre casos borde y escenarios funcionales? ¿Qué huecos o dudas abiertas quedan? |
| `design` | ¿Satisface todos los requerimientos de la spec? ¿Respeta las tecnologías de `axiom.yaml` y los patrones del repositorio? ¿Qué desviación arquitectónica introduce? |
| `tasks` | ¿Es coherente el reparto entre roles? ¿Son las tareas suficientemente atómicas y verificables? |
| `role-apply:<rol>` | ¿Conforme al diseño de su rol? ¿Cumple la spec asignada? ¿Compila limpio, pasan sus pruebas, cobertura y estilo? |
| `integration` | ¿Hay evidencia de integración o despliegue? ¿De qué clase y verificable? |

### 5.6 Fontanería git nueva (una sola adición)

```go
// internal/reviewtransaction/snapshot.go
// RevisionIsAncestor informa si ancestor es alcanzable desde descendant en el
// grafo LOCAL. No consulta ningún remoto: no ejecuta fetch ni ls-remote, de
// modo que nunca requiere autorización remota ni credenciales.
// Corre sobre el ejecutor endurecido del paquete, con LocalGitCommandTimeout,
// límite de salida y las guardas de injerto ya vigentes.
func (builder SnapshotBuilder) RevisionIsAncestor(ctx context.Context, ancestor, descendant string) (bool, error)
```

Implementación: `git merge-base --is-ancestor <ancestor> <descendant>` mediante `runGitCapturedRangeWithTimeout` (`snapshot.go:1963`). Salida 0 ⇒ `true`; salida 1 ⇒ `false`; cualquier otra ⇒ error. `ancestor` y `descendant` se pasan como elementos de *slice*, nunca interpolados en una cadena de *shell*.

**Por qué aquí y no en un paquete nuevo.** Es el único ejecutor de git endurecido del repositorio, `internal/sddstatus` ya lo importa (`runtime_ledger.go:20`) y ya lo usa para fontanería (`:212`), y el guardián estructural que protege la frontera RDD prohíbe exactamente dos selectores —`OfferReviewAfterVerify` y `ReviewCore`— declarando explícitamente que tocar otros símbolos de `reviewtransaction` no es una violación (`review_offer_absence_guard_test.go:17-20,32-40`). Duplicar el ejecutor en un paquete propio significaría reimplementar los *timeouts*, los límites de salida, el aislamiento de configuración y las guardas de injerto; sería una segunda superficie de fallo para ahorrar una arista que ya existe.

### 5.7 Contrato de CLI

```
axiom sdd kickoff seal --cwd <ruta> --change <nombre>
      [--flow-mode sdd]
      (--execution-style continuous|checkpointed | --from-session-pace interactive|auto)
      --handoff-policy none|per_checkpoint
      [--role <id>[:blocking|deferred|optional] ...]        (por defecto: fullstack:blocking)
      [--deployment-target local|staging|production]
      [--json]

axiom sdd kickoff show   --cwd <ruta> --change <nombre> [--json]

axiom sdd gate record --cwd <ruta> --change <nombre>
      --gate spec|design|tasks|role-apply:<rol>|integration
      --decision approved|rejected
      --reason "<texto>"
      [--evidence-kind pr_merged|deployment|attestation]
      [--commit <sha>] [--base-ref <ref>] [--evidence "<texto>"]
      [--actor <id>] [--json]

axiom sdd gate show --cwd <ruta> --change <nombre> [--json]
```

| Situación | Salida | Código |
|---|---|---|
| Sellado correcto | Resumen del sello | 0 |
| Sellado de un cambio ya sellado | Configuración ganadora, sin escribir | 0 |
| Sellado o registro sobre una raíz bajo `archive/` | Rechazo nombrando bug o nuevo incremento | 1 |
| Registro con compuerta desconocida | Rechazo enumerando el vocabulario válido | 1 |
| Registro con `role-apply:<rol>` fuera del roster | Rechazo nombrando el roster sellado | 1 |
| `--decision rejected` sin `--reason` | Rechazo: REQ-21.12 exige registrar el motivo | 1 |
| `--evidence-kind pr_merged` sin ancestro confirmado | Rechazo nombrando commit y rama; nada anexado | 1 |
| Cambio inexistente | Rechazo nombrando la ruta resuelta | 1 |

Sin alias hifenados de nivel superior (`axiom sdd-kickoff`, `axiom sdd-gate`): los alias existentes (`main.go:352-363`) existen porque activos ya distribuidos los invocan como comandos de una palabra en un contrato publicado. Estos verbos son nuevos y no tienen ese consumidor.

### 5.8 Espejo Engram

| Artefacto | Clave de tema | Productor |
|---|---|---|
| Kickoff sellado | `sdd/<cambio>/kickoff` | `axiom sdd kickoff seal` emite la carga; el orquestador la persiste |
| Registro de compuertas | `sdd/<cambio>/gates` | `axiom sdd gate record` emite la carga |
| Relevo de integración | `sdd/<cambio>/handoff` | Ya existe: `handoff.ToEngramPayload` (`mirror.go:25`) |

`engramTitlePattern` (`status.go:860`) admite `kickoff` y `gates` como sufijos. Las dos claves nuevas siguen la disciplina vigente: `capture_prompt: false`, `type: architecture`, `topic_key` para *upsert*.

---

## 6. Estrategia de Pruebas (Testing Strategy)

`strict_tdd: true` (`openspec/config.yaml:16`). Todo test listado se escribe en RED, con fallo observado y registrado, **antes** de la producción que lo satisface. Convención: tabla de *structs* anónimos + `t.Run`, `t.Fatalf` para precondiciones y `t.Errorf` para aserciones, sin `os.Exit` ni `panic`.

| Capa | Qué se prueba | Cómo | Rebanada |
|---|---|---|---|
| Unit — esquema | Enums válidos e inválidos de `flow_mode`, `execution_style`, `handoff_policy`, `gate_policy`, `deployment_target`; `schema` desconocido ⇒ error; YAML malformado ⇒ error, no valor cero silencioso | Tabla sobre `Validate` con documentos literales | P1 |
| Unit — sellado | Primer sellado escribe; segundo sellado con contenido distinto **no escribe** y devuelve el ganador; fichero preexistente ilegible ⇒ error; el temporal no sobrevive al retorno | `t.TempDir()` + tabla | P1 |
| Unit — registro | Anexado preserva registros previos; escritura concurrente desde dos *goroutines* no pierde ninguno; fichero ausente ⇒ registro vacío sin error | `t.TempDir()`, `sync.WaitGroup`, `-race` | P1 |
| Unit — digest | Mismo contenido con CRLF y con LF ⇒ mismo digest; orden de rutas no altera el resultado; fichero ausente ⇒ error nombrado | Tabla | P1 |
| **Unit — máquina de compuertas** | **`continuous` ⇒ cero compuertas** [D-05]; apertura por artefacto; `rejected` + digest igual ⇒ sigue rechazada; `rejected` + digest distinto ⇒ `pending` con `Reopened: true`; **`approved` no se invalida al cambiar el digest** [D-08]; orden de evaluación fijo; roster multi-rol ⇒ una `role-apply` por rol | Tabla sobre `EvaluateGates`, función pura, sin E/S | **P1** |
| Unit — último rol | N−1 roles aprobados ⇒ `false`; N aprobados ⇒ `true`; un rol rechazado ⇒ `false` aunque el resto esté aprobado (REQ-21.13, tercer escenario) | Tabla sobre `LastRoleClosed` | P1 |
| Unit — raíz archivada | `openspec/changes/archive/2026-01-01-x` ⇒ rechazo; `openspec/changes/x` ⇒ aceptación; `../` y ruta absoluta ⇒ rechazo por contención | Tabla sobre `RefuseArchivedRoot` | P1 |
| Unit — parseo de argumentos | Banderas conocidas y desconocidas; `--decision rejected` sin `--reason`; `--from-session-pace` y `--execution-style` juntos; `role-apply:` sin rol; `--cwd` relativo y absoluto | Tabla sobre `Parse*Args` | P2 |
| Unit — CLI | Códigos de salida de las ocho filas de §5.7; forma de `--json`; el aviso de último rol aparece exactamente una vez | `bytes.Buffer` como `stdout`, al estilo de `internal/cli/sdd_archive_compose_test.go` | P2, P5 |
| **Regresión — sin sello** | **Un cambio sin `kickoff.yaml` produce un `StatusV2Projection` byte a byte idéntico al de hoy**, incluida la ausencia de la clave `governance` | Comparación de la cadena JSON íntegra contra la salida capturada antes del cambio; **escrito antes de tocar `status.go`** | **P3** |
| Unit — proyección | Con sello: `governance` presente con kickoff, compuertas y roster; `await-gate` en `statusV2NextRecommended`; instrucciones de `nonPhaseRoutingInstructions` nombran las dos invocaciones ejecutables | Tabla sobre `ProjectStatusV2` y `RenderDispatcherMarkdown` | P3 |
| Unit — enrutamiento | Los 37 escenarios BDD de `spec.md` que producen una ruta observable, uno a uno, sobre `Resolve` con árboles temporales | Tabla + `t.TempDir()` | P3 |
| Unit — envoltorio | `Validate` rechaza esquema ajeno, opciones distintas de `approved`/`rejected`, invocación que no empieza por `axiom sdd gate record `, y `off_path` que no reentra por status | Tabla, calcada de `internal/sddstatus/consent_contract_test.go` | P3 |
| Unit — roster | Las tres fuentes y su precedencia; conflicto kickoff/design se reporta y **no** se resuelve en silencio; `sealed` vacío ⇒ resultado idéntico al de `DetectRoles` | Tabla sobre `ResolveRoster` | P4 |
| **Caracterización — REQ-1.1** | **Los tres escenarios vivos de `multi-role-fan-out` REQ-1.1 siguen pasando**: multi-rol con políticas, rol `database` ausente ⇒ error, fallback de rol único | Tabla sobre `DetectRoles`, **escrita antes de tocar `roleExists`** | **P4** |
| Unit — rol reservado | `fullstack` aceptado sin declararlo en `axiom.yaml`; `database` sigue produciendo error; paridad entre las dos copias de `roleExists` sobre un corpus compartido | Tabla en `multirole` y en `handoff` | P4 |
| Unit — relevo | `apply→verify` con `ready` valida; las cinco secciones no vacías; consolida los N roles, no solo el último; `to_role` sigue la regla de tres pasos; `blocked` ⇒ `dependencies.Verify = blocked` | Tabla sobre `IntegrationHandoff` + `handoff.Validate` | P5 |
| Unit — evidencia | `pr_merged` con ancestro ⇒ `verified: true`; sin ancestro ⇒ rechazo y **cero escrituras**; `deployment` y `attestation` ⇒ `verified: false` anexado; comprobador que falla ⇒ error, nunca `verified: true` | Tabla con un `AncestryChecker` doble | P6 |
| Unit — ancestro real | `RevisionIsAncestor` sobre un repositorio git temporal: ancestro, no-ancestro, revisión inexistente, contexto ya cancelado | `t.TempDir()` + `git init` | P6 |
| Integración — archive | Con sello y sin compuerta `integration`: `dependencies.Archive == blocked` y el cambio permanece en `openspec/changes/<c>/`; con ella: `ready` | `t.TempDir()` con árbol OpenSpec completo | P6 |
| Activos | Renderizado determinista de la sección compartida nueva en los 12 activos de orquestador; presencia del paso de carril en `RenderRouting` para cada `AgentID` | Baterías existentes de `internal/assets/assets_test.go` y `internal/components/sdd/orchestrator_shared_sections_test.go` | P7 |
| Estructural — frontera RDD | Ningún fichero de producción de INC-21 nombra `ReviewCore`, `OfferReviewAfterVerify`, `receipt`, `lineage`, `acknowledge` ni `burn`; `internal/kickoff` no importa nada fuera de la lista blanca de §4.1 | Escáner `go/ast`, calcado de `review_offer_absence_guard_test.go` | P1, P3 |

### 6.1 Verificación por rebanada, y la restricción del suite completo

**Restricción declarada.** El orquestador señaló que `go test ./...` puede agotar los 600 s por un test colgado en `internal/sddstatus` que bloquea en `runGitCapturedRangeWithTimeout` (`internal/reviewtransaction/snapshot.go:1963`).

**Lo que se verificó.** El nombre concreto que se citó, `TestRuntimeFinishRecordsTruthfulInterruptedRemediation`, **no existe en el árbol actual**: una búsqueda de `TestRuntimeFinish` en todo el repositorio solo encuentra dos menciones, ambas dentro de `internal/sddstatus/retired_legacy_binding_fixture_absence_test.go:46-47`, que es un test cuyo propósito es afirmar que esas funciones están **ausentes**. No se localizó ningún test con ese nombre exacto. Este diseño **no infiere** de ahí que el problema esté resuelto: infiere que la ruta `sddstatus` → `reviewtransaction` → subproceso git es cara y con riesgo de bloqueo, y que el nombre citado puede corresponder a una observación histórica o a un nombre que ha cambiado.

**Lo que este diseño hace al respecto, y lo que no.**

- **No lo arregla.** Está fuera de alcance y ningún requerimiento de INC-21 lo toca.
- El bucle interno de trabajo usa ejecución **por paquete y con presupuesto explícito**: `go test ./internal/kickoff/... ./internal/multirole/... ./internal/handoff/... ./internal/cli/... -timeout 300s`.
- `internal/sddstatus` se ejecuta **aparte y con presupuesto propio**: `go test ./internal/sddstatus/... -timeout 600s`. Si agota el presupuesto, se reejecuta con `-run` acotado a los tests de P3 y se **registra el agotamiento como resultado real**, jamás como aprobado.
- `go test ./...` se ejecuta **una vez por rebanada, al cierre**, con `-timeout 900s`, que es el presupuesto que INC-20 ya usó para su cierre de fase 14 (`openspec/changes/inc-20-upstream-reconciliation/tasks.md:456`). Es precedente del repositorio, no un número inventado.
- Todo test nuevo de INC-21 que dependa de git es **hermético**: `t.TempDir()` con `git init` local, sin remotos, sin red. Ninguno amplía la superficie del problema existente.
- Si el suite completo agota el presupuesto, el informe de verificación lo declara como **no ejecutado**, con el paquete y el presupuesto, y no se convierte en aprobado por omisión.

---

## 7. Matriz de Amenazas (Threat Matrix)

Aplicable: el diseño cambia el enrutamiento de fases, añade subcomandos de CLI, invoca subprocesos git, construye rutas a partir de entrada del usuario y toca la semántica de archivado y de VCS.

### 7.1 Matriz canónica

| # | Frontera | Casos adversariales mínimos | Aplicabilidad | Respuesta de diseño | Tests RED planificados |
|---|---|---|---|---|---|
| T-1 | Rutas con apariencia de documentación | `requirements.txt`, `CMakeLists.txt`, Markdown/MDX ejecutable, `README.sh` | **N/A** — INC-21 no clasifica ficheros por contenido ni ejecuta ninguno. Lee YAML y Markdown como datos inertes; las extensiones las añade Go. | — | Ninguno (fila N/A) |
| T-2 | Selección de repositorio | `--cwd` relativo, absoluto, inexistente; `--change` con `..` | **Aplicable** — `--cwd` y `--change` determinan la raíz del cambio. | La raíz se resuelve una vez al inicio; toda ruta se construye con `filepath.Join` y se valida con `filepath.Rel` (sin `..`, no absoluta), igual que `RefuseArchivedRoot`. `--cwd` inexistente falla **antes** de cualquier escritura. | `--cwd` relativo, absoluto e inexistente ⇒ cero ficheros creados; `--change ../otro` ⇒ rechazo |
| T-3 | Estado del índice Git | preparado, `commit -a`, índice vacío | **Aplicable, de solo lectura** — `RevisionIsAncestor` consulta el grafo de commits. No lee el índice, no prepara, no confirma. Un árbol sucio no altera el resultado. | El comando es `merge-base --is-ancestor`, que solo mira el grafo. Ninguna ruta de INC-21 ejecuta `add`, `commit`, `stash` ni `checkout`. | Árbol sucio ⇒ mismo veredicto que árbol limpio |
| T-4 | Estado de *push* | rama de seguimiento, primer *push*, *refspec* explícita | **N/A por diseño deliberado** — cero interacción con remotos [D-13]. `--base-ref` nombra una referencia **local**; si no existe localmente, el comando falla nombrándola en vez de intentar traerla. | — | `--base-ref` inexistente ⇒ error explícito, sin `fetch` |
| T-5 | Comandos de PR | `--head` explícito, prefijo de entorno, comandos compuestos | **N/A** — no se invoca `gh` ni ninguna automatización de PR. El único uso de `gh` en el árbol está en `internal/update/github.go:58`, para *releases*, y este diseño no lo toca. | — | Ninguno (fila N/A) |

### 7.2 Fronteras adicionales que este cambio sí introduce

| # | Frontera | Casos adversariales mínimos | Aplicabilidad | Respuesta de diseño | Tests RED planificados |
|---|---|---|---|---|---|
| T-6 | Invocación de subproceso git | binario ausente del `PATH`; salida distinta de 0 y de 1; proceso que no termina; salida desbordada | **Aplicable** | `RevisionIsAncestor` corre sobre el ejecutor endurecido existente: `LocalGitCommandTimeout`, límite de salida, `argv` como *slice* literal, sin cadena de *shell* y sin interpolar entrada del usuario. Salida ≠ {0,1} ⇒ error nombrado, nunca `verified: true`. | `PATH` vacío; salida 128; contexto cancelado; los tres ⇒ error, cero registros anexados |
| T-7 | Segmento de ruta controlado por el usuario → escritura en disco | `--change` con `..`, `/`, `\`, ruta absoluta, `con`, `nul`, nombre de 300 caracteres, cadena vacía | **Aplicable** | El nombre del cambio se resuelve contra los directorios existentes bajo `openspec/changes/` y nunca se usa para **crear** un directorio nuevo: estos verbos escriben dentro de un cambio que ya existe. Comprobación de contención tras el `Join`. | Tabla con los ocho vectores; ningún fichero aparece fuera de `t.TempDir()` |
| T-8 | Enrutamiento de subcomandos de CLI | subcomando desconocido; sin subcomando; `--help`; `-h`; colisión con un `case` existente | **Aplicable** | Los dos `case` nuevos se calcan de `runSDD` (`main.go:1746-1762`): ayuda con salida 1 sin argumentos, 0 con `--help`, mensaje explícito y salida 1 ante subverbo desconocido. Verificado que no existe hoy ningún `case "kickoff"` ni `case "gate"` en ese `switch`. | Tabla de enrutamiento sobre `sdd kickoff`, `sdd gate`, sus `--help` y un subverbo inexistente |
| T-9 | Escalada de autoridad por confusión con RDD | Un relé que trate una aprobación de compuerta como recibo; un envoltorio de compuerta enrutado a `sdd-attempt grant`; una aprobación que autorice entrega | **Aplicable, y es el riesgo de mayor severidad del incremento** | Esquema y contrato distintos (`gentle-ai.sdd-governance.gate/v1`); vocabulario sin `receipt`/`lineage`/`acknowledge`/`burn`; invocación de concesión que debe empezar por `axiom sdd gate record ` y nunca por `sdd-attempt grant `; guardián estructural `go/ast` sobre los ficheros de producción; el contrato distribuido ya declara que SDD no ofrece ni lanza RDD (`sdd-orchestrator-sections.md:47`). | `Validate` rechaza una invocación con prefijo `sdd-attempt`; el escáner falla ante cualquiera de los cinco términos prohibidos; ningún test afirma que una compuerta autorice entrega |
| T-10 | Enrutamiento envenenado por metadatos | `kickoff.yaml` o `gates.yaml` editados a mano con enums inválidos, digest falso o compuerta desconocida | **Aplicable** | Enums cerrados con error nombrado; compuerta desconocida ⇒ error, jamás ignorada en silencio; un digest falsificado solo puede **reabrir** una compuerta rechazada (la invalidación insegura sería lo contrario: aprobar sin decisión), y ninguna aprobación se deriva de un digest. YAML malformado ⇒ error del resolutor, nunca configuración vacía tratada como «sin sello». | Tabla de documentos corruptos; **en particular, un `kickoff.yaml` ilegible NO debe degradarse a «sin sello»**, que sería un desvío silencioso de todas las compuertas |
| T-11 | Fabricación de evidencia de integración | `--evidence-kind pr_merged` con un sha arbitrario; `attestation` con texto que parece una verificación | **Aplicable, mitigación parcial y declarada** | `pr_merged` se comprueba contra el grafo local y se rechaza si el ancestro no se confirma. `deployment` y `attestation` **no son comprobables** y se registran con `verified: false`, valor que el estado y el informe de archivado muestran siempre. Axiom no puede distinguir una declaración honesta de una falsa; puede, y hace, distinguir una declaración de una comprobación. | `attestation` ⇒ `verified: false` en el registro, en `governance` y en la nota de estado; `pr_merged` no verificable ⇒ rechazo |

Toda fila marcada `Aplicable` se traslada **sin modificación** a `tasks.md` como test RED previo a su producción. Las filas `N/A` no generan tarea.

---

## 8. Migración y Despliegue (Migration / Rollout)

### 8.1 Retro-sellado de cambios preexistentes (REQ-21.4) — con una corrección a la especificación

`spec.md:98-103` describe el cambio preexistente con artefactos y sin sello, y propone inferir «modalidad continua, sin `handoff.md` intermedios, rol único `fullstack` si no hay roles ya declarados». Contrastado contra el árbol, **dos tercios se confirman y un tercio debe corregirse**:

| Valor inferido | Veredicto | Evidencia |
|---|---|---|
| `execution_style: continuous` | **Correcto.** Es el comportamiento vigente. | `internal/assets/claude/sdd-orchestrator-workflow.md:73`: «If the user doesn't specify, default to **Automatic**»; `auto` encadena fases sin pausar (`:70`). |
| `handoff_policy: none` | **Correcto.** Hoy nada genera relevos automáticamente. | `axiom handoff create` es enteramente manual (`cmd/axiom/main.go:604-659`); ninguna fase lo invoca. |
| `roles: [fullstack]` | **Incorrecto como regla general.** | `DetectRoles` sobre un cambio con `design.md` y sin roles declarados infiere hoy `core` en este workspace (`detector.go:35-46`, `axiom.yaml:8`). Sellar `fullstack` cambiaría el fichero que la barrera busca, de `tasks.md`/`verify-report.md` con rol `core` a los del rol `fullstack`, sin que nadie lo haya pedido. |

**Regla de retro-sellado corregida**, en este orden:

1. Existe `design.md` y `DetectRoles` devuelve roles **sin error** ⇒ se sellan **esos** roles, con `execution_style_source: inferred` y `sealed_by: inferred`. Preserva byte a byte el comportamiento vigente de la barrera.
2. No existe `design.md` ⇒ se sella `fullstack:blocking` con `tasks.md` y `verify-report.md`. No hay comportamiento previo que preservar: nada lee roles todavía.
3. `DetectRoles` **devuelve error** (rol declarado ausente de `axiom.yaml`) ⇒ **no se sella nada**. Se emite una nota no bloqueante y el error existente sigue apareciendo donde ya aparece. Sellar `fullstack` aquí enmascararía un error real de configuración con una inferencia.

El retro-sellado es **explícito y perezoso**: lo ejecuta `axiom sdd kickoff seal --infer`, no un efecto colateral de `axiom sdd status`. Motivo: `sdd status` es de solo lectura por contrato —«Inspection needs no execution preflight... No recommendation is executed during inspection» (`sdd-orchestrator-sections.md:9`)— y una inspección que escribe en el repositorio rompería esa garantía. Mientras no se selle, el cambio se comporta exactamente como hoy [D-05], que es precisamente lo que REQ-21.4 pide: «sin bloquear el trabajo ni interrogar retroactivamente al usuario».

Esta corrección se eleva como amendment candidato de la especificación (O-1, §9).

### 8.2 Despliegue por rebanadas apiladas

Estrategia de entrega de la sesión: `auto-chain`. Las siete rebanadas se apilan contra `main`.

```
main
 └─ P1 dominio ──► P2 CLI ──► P3 status ──► P4 roster ──► P5 relevo ──► P6 archive ──► P7 doctrina
```

| Rebanada | Estado del árbol al cerrar | Frontera de reversión |
|---|---|---|
| P1 | Compila y verde. Paquete nuevo sin consumidores: el binario no cambia de comportamiento. | Aditivo puro. Revertir después de P2. |
| P2 | `axiom sdd kickoff|gate` operativos y capaces de escribir. Nada los lee todavía: un sello no altera ninguna ruta. | Eliminar los dos ficheros de CLI y los dos `case`. Revertir después de P3. |
| P3 | Las compuertas enrutan. **Puerta de control: la regresión «sin sello» debe seguir verde.** | Revertir `status.go` y `status_v2.go`. Los ficheros ya sellados quedan como datos inertes. |
| P4 | `axiom role *` respeta el roster sellado. **Puerta de control: la caracterización de REQ-1.1 debe seguir verde.** | Revertir `roster.go` y las dos ramas de `roleExists`. |
| P5 | Aviso de último rol y relevo de integración. | Eliminar `closure.go` y su invocación. Independiente de P6. |
| P6 | Precondición de archive activa **solo con sello**. | Revertir la condición de `dependencies.Archive` y el método de ancestro. |
| P7 | Doctrina y *goldens* al día. | Revertir activos y regenerar *goldens*. |

**Orden invertido rechazado.** Poner P3 antes que P2 dejaría una ventana en la que el enrutamiento consulta compuertas que ningún verbo puede registrar: un cambio sellado a mano quedaría bloqueado en `await-gate` sin salida ejecutable.

**Sin banderas de funcionalidad.** El cambio es aditivo y la ausencia de `kickoff.yaml` ya es el interruptor de apagado natural [D-05]. Una bandera añadiría una ruta de código que nadie desactivaría nunca.

**Compatibilidad hacia atrás.** El único contrato existente que se toca es `gentle-ai.sdd-status/v2`, y se toca añadiendo un campo opcional con `omitempty` y un valor de enum en `nextRecommended`. El propio código documenta ese patrón como aditivo dos veces (`status_v2.go:193-196`, `:210-212`). Los tres cambios activos de hoy (`inc-20`, `inc-21`, `inc-22`) no tienen sello y ven un documento idéntico al actual.

**Criterios de aborto.** (1) La regresión «sin sello» de P3 falla. (2) La caracterización de REQ-1.1 de P4 falla. (3) Cualquier *diff* contiene un fichero de §4.8. (4) Cualquier fichero de producción menciona `receipt`, `lineage`, `acknowledge` o `burn`. (5) Cualquier ruta de INC-21 ejecuta una operación git remota.

---

## 9. Preguntas Abiertas (Open Questions)

- [ ] **O-1 — El rol por defecto del retro-sellado contradice la especificación.** `spec.md:102` dice «rol único `fullstack` si no hay roles ya declarados». §8.1 demuestra que, para un cambio con `design.md` y sin roles explícitos, la inferencia vigente es `core` en este workspace, y que sellar `fullstack` cambiaría en silencio los ficheros que la barrera busca. Este diseño adopta la regla corregida de §8.1. **Severidad media; no bloquea ninguna rebanada** — la regla corregida es estrictamente más conservadora que la escrita. Se eleva al orquestador por si prefiere enmendar el segundo escenario de REQ-21.4 antes de `sdd-tasks`.

- [ ] **O-2 — El fallback de roles de `DetectRoles` es no determinista.** `detector.go:41-44` recorre `wsConfig.Roles` con `for k := range` y toma la primera clave cuando el workspace no declara `core`. La iteración de mapas en Go no tiene orden: dos ejecuciones pueden elegir roles distintos y buscar `tasks.<rol>.md` distintos. **Deliberadamente no se corrige aquí**, porque cambiaría el comportamiento observable de una capacidad viva (`multi-role-fan-out` REQ-1.1) fuera del alcance de REQ-21.6, que solo habla del caso «sin roles declarados». En este repositorio el fallo está latente porque `axiom.yaml:8` declara `core`. Registrado como candidato a incremento propio.

- [ ] **O-3 — El test colgado que agota `go test ./...` no se pudo localizar por nombre.** `TestRuntimeFinishRecordsTruthfulInterruptedRemediation` no existe en el árbol; las dos únicas menciones de `TestRuntimeFinish` están en un test de ausencia (`retired_legacy_binding_fixture_absence_test.go:46-47`). El riesgo de agotar el presupuesto en la ruta `sddstatus` → subproceso git sigue siendo real y §6.1 lo gestiona sin repararlo. **No bloquea ninguna rebanada.** Se eleva para que `sdd-verify` no interprete un agotamiento como fallo de INC-21.

- [ ] **O-4 — Los cuatro `openspec/specs/odd-*` siguen vivos mientras INC-20 no se archive.** `internal/odd` ya no existe (H-2), pero sus specs vivas sí, porque los deltas destructivos de INC-20 están escritos y sin promover. Mientras dure ese solape, `openspec/specs/` describe comandos `axiom odd` que el binario no tiene. INC-21 **no** toca esas specs: son propiedad de INC-20. **No bloquea ninguna rebanada**, pero P7 debe evitar referenciarlas.

Ninguna de las cuatro bloquea el diseño ni el inicio de P1.

---

## 10. Trazabilidad

| Requerimiento | Decisiones y secciones que lo cubren |
|---|---|
| REQ-21.1 Pregunta bloqueante de carril | H-2; §4.7 `routing.go`; diagrama §3.5 |
| REQ-21.2 ODD sin fricción adicional | H-2; doctrina en `routing.go:48-59`, sin superficie Go |
| REQ-21.3 Entrada al pre-vuelo SDD | D-03; §3.1 |
| REQ-21.4 Idempotencia del kickoff | D-01, D-02 (escritura única), §8.1 (retro-sellado), O-1 |
| REQ-21.5 Bloqueo sin modalidad y relevos sellados | D-03, D-04; §5.1, §5.7 |
| REQ-21.6 Rol `fullstack` obligatorio | D-06, D-07; H-4; §4.4 |
| REQ-21.7 Aplicabilidad condicionada a la modalidad | **D-05**; §3.2; regresión «sin sello» de §6 |
| REQ-21.8 Compuerta de `spec` | D-08, D-09; criterios en §5.5 |
| REQ-21.9 Compuerta de `design` | D-08, D-09; criterios en §5.5 |
| REQ-21.10 Compuerta de `tasks` | D-08, D-09; `tasks.<rol>.md` multi-rol en §4.7 |
| REQ-21.11 Compuerta de `apply` por rol | D-08, D-10; `RoleApplyGate` en §5.3 |
| REQ-21.12 Rechazo bloquea y exige remediación | **D-08** (reapertura por digest); registro solo-anexar en §5.2 |
| REQ-21.13 Aviso de último rol | D-10; §3.3; `LastRoleClosed` |
| REQ-21.14 `handoff.md` de integración | D-11; §3.3; `handoff.WriteFile` + `ToEngramPayload` |
| REQ-21.15 `verify` global tras el relevo | D-12; §4.5 |
| REQ-21.16 Precondición de archive | **D-13**; §3.4; §5.6; T-11 |
| REQ-21.17 Contenido del archivado | Sin cambios de mecanismo: `sdd-archive` y `livingdoc` vigentes; §4.7 solo documenta la precondición |
| REQ-21.18 Sellado inmutable post-archive | D-14; `RefuseArchivedRoot`; `ArchivedProjection` vigente |
| Frontera RDD (instrucción del orquestador) | **§1.3 normativa**; T-9; guardián estructural en §6 |
| Pregunta abierta 1 de `spec.md` (rutas inexistentes) | H-1 |
| Pregunta abierta 2 de `spec.md` (solape con REQ-1.1) | H-4, D-06, D-07; caracterización de §6 |
| Pregunta abierta 3 de `spec.md` (retrocompatibilidad) | §8.1; O-1 |
| Pregunta abierta 4 de `spec.md` (esquema `kickoff.yaml`) | D-01, D-02, D-04; §5.1, §5.2, §5.8 |
