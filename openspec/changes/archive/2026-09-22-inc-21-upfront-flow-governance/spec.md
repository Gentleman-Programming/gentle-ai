# Especificación de Requerimientos: Determinación Temprana de Flujo, Compuertas de Revisión por Bloque, Relevo de Cierre de Roles y Ciclo de Vida de Archive (INC-21)

> **Incremento:** `inc-21-upfront-flow-governance`
> **Fase del Roadmap:** Fase 4 — Gobernanza de Agentes, Flujo Dual ODD/SDD y Automatización de Ciclo de Vida
> **Responsabilidad:** Gobernanza / Orquestación SDD / Motor Multi-Rol / Experiencia de Usuario
> **Estado:** En desarrollo (fase de especificación)
> **Idioma:** Español (castellano peninsular)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para: (1) la compuerta temprana y bloqueante de selección de carril ODD/SDD en el kickoff de cualquier solicitud de trabajo; (2) el cuestionario de pre-vuelo de SDD que sella la modalidad de avance, la política de relevos y la asignación de roles, incluida la asignación obligatoria del rol `fullstack` cuando no hay subdivisión especializada; (3) las cuatro compuertas de revisión por bloque (`spec`, `design`, `tasks` y `apply` por rol) que operan cuando el cambio se configura con paradas; (4) el aviso formal y el relevo de integración que se disparan al concluir el último rol activo, junto con la ejecución de la fase `verify` global de la solución; y (5) la semántica estricta del ciclo de vida de `archive`, que exige evidencia de integración o despliegue antes de sellar la especificación viva, y que convierte cualquier corrección posterior en un ticket de bug o en un nuevo incremento.

## Alcance de esta Especificación

| # | Capacidad | Tipo | Requerimientos |
|---|---|---|---|
| 1 | `sdd-lane-kickoff` | Nueva | REQ-21.1 – REQ-21.4 |
| 2 | `sdd-preflight-configuration` | Nueva | REQ-21.5 – REQ-21.6 |
| 3 | `sdd-block-review-gates` | Nueva | REQ-21.7 – REQ-21.12 |
| 4 | `sdd-role-closure-handoff` | Nueva | REQ-21.13 – REQ-21.15 |
| 5 | `sdd-archive-lifecycle` | Nueva | REQ-21.16 – REQ-21.18 |

### Notas de Alcance y Preguntas Abiertas para Diseño

- La propuesta de este incremento asocia parte de este trabajo a `internal/review/` y a `internal/cli/sdd_continue.go`. Ninguno de los dos existe en el árbol actual del repositorio (verificado contra `internal/` antes de escribir esta especificación). Esta especificación describe únicamente el comportamiento observable de las cuatro compuertas de revisión y de la parada obligatoria de `tasks`; corresponde a `sdd-design` decidir en qué paquete, fichero o punto de extensión concreto se implementa cada pieza, sin presuponer los nombres citados en la propuesta.
- El rol `fullstack` obligatorio (REQ-21.6) se sella en el cuestionario de pre-vuelo, antes de que exista `design.md`. La capacidad ya viva `multi-role-fan-out-engine` (`openspec/specs/multi-role-fan-out/spec.md`, Requirement REQ-1.1) define, de forma independiente, un modo de compatibilidad al analizar `design.md` cuando este no declara roles explícitos. Esta especificación NO modifica ese requerimiento ya vivo: queda como pregunta abierta para `sdd-design` si el pre-vuelo y el análisis de `design.md` deben unificarse en un único mecanismo, o si deben coexistir como capas independientes que en la práctica casi siempre coinciden.
- Ningún fichero Go bajo `internal/` menciona hoy la palabra "kickoff" (verificado). El vocabulario "kickoff" empleado en esta especificación es conceptual y describe un momento del flujo, no presupone un identificador de paquete, tipo o función con ese nombre literal.
- Los paquetes verificados como existentes y potencialmente relevantes para estas capacidades son `internal/multirole/` (incluye `barrier.go`, `detector.go`, `types.go`), `internal/handoff/`, `internal/sddstatus/`, `internal/cli/sdd_archive_compose.go` y `cmd/axiom/main.go`; su uso concreto en cada requerimiento queda a discreción de `sdd-design`.

---

## 1. Capacidad: `sdd-lane-kickoff`

El orquestador determina el carril de ejecución (ODD o SDD) en el primer contacto con cualquier solicitud de trabajo, antes de crear ningún artefacto, y sella esa decisión para el resto del ciclo de vida del cambio.

### Requirement: Evaluación de Alcance y Pregunta Bloqueante de Selección de Carril (REQ-21.1)

El orquestador DEBE evaluar el alcance de toda solicitud de trabajo nueva antes de crear cualquier artefacto de trabajo (documento vivo ODD o propuesta SDD). Si el alcance detectado es acotado y admite razonablemente ambos carriles, el orquestador DEBE formular explícitamente la pregunta bloqueante de selección de carril (“¿Deseas abordarla mediante vía ágil ODD o vía formal SDD?”) y DEBE detenerse a esperar la respuesta del usuario antes de continuar. El orquestador NO DEBE crear `odd/tasks/<feature>.md` ni `openspec/changes/<cambio>/proposal.md` mientras la pregunta esté pendiente de respuesta.

#### Scenario: Alcance acotado formula la pregunta y bloquea hasta obtener respuesta

- **DADO** una solicitud de trabajo de alcance acotado que razonablemente admite tanto ODD como SDD
- **CUANDO** el orquestador recibe la solicitud
- **ENTONCES** formula la pregunta de selección de carril y se detiene a esperar respuesta
- **Y** no crea ningún artefacto de trabajo antes de recibir la respuesta

#### Scenario: Alcance inequívocamente arquitectónico evita la pregunta binaria

- **DADO** una solicitud cuyo alcance es inequívocamente de arquitectura o de gran envergadura
- **CUANDO** el orquestador evalúa el alcance
- **ENTONCES** entra directamente al cuestionario de pre-vuelo de SDD (capacidad `sdd-preflight-configuration`) sin necesidad de formular la pregunta binaria ODD/SDD

---

### Requirement: Carril ODD sin Fricción Adicional (REQ-21.2)

Si el usuario elige el carril ODD, el orquestador NO DEBE formular ninguna pregunta adicional de gobernanza, DEBE ejecutar el trabajo “del tirón” sin dividirlo en roles ni generar ningún `handoff.md`, y DEBE mantener el trabajo exclusivamente en `odd/tasks/<feature>.md`.

#### Scenario: Elección de ODD sin preguntas adicionales

- **DADO** que el usuario elige el carril ODD en respuesta a la pregunta de REQ-21.1
- **CUANDO** el orquestador inicia el trabajo
- **ENTONCES** no formula ninguna pregunta adicional de configuración de gobernanza
- **Y** el trabajo se registra exclusivamente en `odd/tasks/<feature>.md`

#### Scenario: Trabajo ODD multi-área sigue sin roles ni handoff

- **DADO** un trabajo bajo el carril ODD que toca varias áreas o módulos del repositorio
- **CUANDO** se ejecuta el trabajo hasta su cierre
- **ENTONCES** el orquestador no genera ningún artefacto `handoff.md`
- **Y** no introduce una división de roles aunque el trabajo abarque varias áreas

---

### Requirement: Entrada al Cuestionario de Pre-Vuelo de SDD (REQ-21.3)

Si el usuario elige el carril SDD en respuesta a la pregunta de REQ-21.1, o si el alcance detectado ya es inequívocamente de arquitectura o gran alcance (REQ-21.1, segundo escenario), el orquestador DEBE entrar al cuestionario de pre-vuelo de SDD descrito en la capacidad `sdd-preflight-configuration` antes de crear `proposal.md`.

#### Scenario: Entrada al pre-vuelo tras elección explícita o alcance ya determinado

- **DADO** que el usuario elige el carril SDD, o que el alcance ya se determinó inequívocamente como de arquitectura
- **CUANDO** el orquestador continúa el flujo
- **ENTONCES** entra al cuestionario de pre-vuelo de SDD antes de crear `proposal.md`

---

### Requirement: Idempotencia del Kickoff ante un Cambio con Artefactos o Configuración Ya Sellada (REQ-21.4)

El orquestador NO DEBE volver a formular la pregunta de selección de carril (REQ-21.1) ni el cuestionario de pre-vuelo de SDD (capacidad `sdd-preflight-configuration`) para un cambio que ya tiene al menos un artefacto persistido (`proposal.md`, un `spec.md` bajo `specs/`, `design.md` o `tasks.md`) o cuya configuración de kickoff ya está sellada. En ese caso, el orquestador DEBE continuar respetando el carril y la configuración ya selladas en el primer kickoff, sin interrogar de nuevo al usuario.

(Previamente: esta especificación indicaba que, al retro-sellar un cambio preexistente sin kickoff sellado, el rol por defecto congelado era el rol único `fullstack`. Se corrige tras la pregunta abierta O-1 planteada en `sdd-design` y resuelta explícitamente por el usuario con evidencia de código —`internal/multirole/detector.go:35-55` cae siempre a `core`, nunca a `fullstack`; `internal/multirole/detector.go:58-64` rechaza cualquier rol ausente de `axiom.yaml`; y `axiom.yaml:7-28` no declara `fullstack`—: el retro-sello DEBE congelar el rol que la detección de roles ya vigente infiere para ese cambio, nunca introducir la identidad `fullstack`.)

#### Scenario: Reanudación de un cambio con kickoff ya sellado

- **DADO** un cambio SDD activo con `proposal.md` existente y su configuración de kickoff ya sellada (carril `sdd`, modalidad y política de relevos definidas)
- **CUANDO** se reanuda el trabajo sobre ese cambio en una sesión posterior
- **ENTONCES** el orquestador no repite la pregunta de selección de carril ni el cuestionario de pre-vuelo
- **Y** continúa el flujo respetando la modalidad y la política de relevos ya selladas

#### Scenario: Cambio preexistente con artefactos pero sin kickoff sellado infiere un valor por defecto sin bloquear

- **DADO** un cambio SDD activo con artefactos existentes (por ejemplo `proposal.md` y `design.md`) pero sin ninguna configuración de kickoff sellada previamente
- **CUANDO** el orquestador retoma ese cambio
- **ENTONCES** infiere y sella la configuración por defecto documentada (modalidad continua, sin `handoff.md` intermedios, y el rol que la detección de roles ya vigente infiere para ese cambio —hoy, `core`, cuando existe un `design.md` sin roles explícitos—) sin bloquear el trabajo ni interrogar retroactivamente al usuario
- **Y** el retro-sello no introduce la identidad `fullstack` ni ningún rol nuevo: es conservador por definición y se limita a congelar el rol y el comportamiento que la detección de roles ya aplica hoy para ese cambio, sin redefinirlo
- **Y** dicha configuración inferida queda registrada para no volver a inferirse en la siguiente reanudación

#### Scenario: El retro-sello no altera qué `tasks.<rol>.md` consulta la barrera

- **DADO** un cambio SDD preexistente sin kickoff sellado, cuya barrera de sincronización ya consulta `tasks.core.md` porque la detección de roles vigente infiere el rol `core` para su `design.md`
- **CUANDO** el orquestador retro-sella la configuración de kickoff de ese cambio conforme al escenario anterior
- **ENTONCES** la barrera de sincronización sigue consultando exactamente `tasks.core.md`, el mismo fichero que consultaba antes del sellado
- **Y** el retro-sello no crea, renombra ni redirige ningún fichero `tasks.<rol>.md`, ni introduce `tasks.fullstack.md` como fuente de verdad nueva para ese cambio

---

## 2. Capacidad: `sdd-preflight-configuration`

El cuestionario de pre-vuelo sella, antes de crear la propuesta, la modalidad de avance del cambio SDD, su política de relevos y la asignación de roles.

### Requirement: Bloqueo de Avance sin Modalidad y Política de Relevos Selladas (REQ-21.5)

Al entrar en el carril SDD, el orquestador DEBE bloquear la creación de `proposal.md` hasta que se definan explícitamente: (1) la modalidad de avance (`continuo` o `con paradas y reviews por bloque`), y (2) la política de relevos (`sin handoffs intermedios` o `con handoff.md en cada parada`). El orquestador DEBE persistir ambas decisiones de forma sellada para el cambio activo y NO DEBE volver a preguntarlas mientras el cambio permanezca activo, salvo lo indicado en REQ-21.4.

#### Scenario: Modalidad y política de relevos selladas antes de crear la propuesta

- **DADO** que el usuario ha elegido el carril SDD
- **CUANDO** responde explícitamente la modalidad de avance y la política de relevos
- **ENTONCES** el orquestador sella ambas decisiones para el cambio
- **Y** procede a crear `proposal.md`

#### Scenario: Respuesta ambigua o ausente no crea la propuesta ni asume un valor por defecto

- **DADO** que el usuario no responde con una modalidad o una política de relevos reconocibles entre las opciones válidas
- **CUANDO** el orquestador evalúa la respuesta
- **ENTONCES** no crea `proposal.md`
- **Y** vuelve a presentar la pregunta pendiente hasta obtener una respuesta válida, sin asumir un valor por defecto no confirmado

---

### Requirement: Asignación Obligatoria del Rol `fullstack` sin Subdivisión Especializada (REQ-21.6)

Si, al concluir el cuestionario de pre-vuelo, ni el usuario ni el diseño previsto subdividen el trabajo en roles especializados (por ejemplo `core`, `web`, `qa`), el orquestador DEBE asignar obligatoriamente el rol único `fullstack`, con política de compuerta `blocking`, fichero de tareas `tasks.md` y fichero de verificación `verify-report.md`. El orquestador NO DEBE dejar un cambio SDD sin al menos un rol informado en su configuración de kickoff.

#### Scenario: Sin roles declarados, se asigna fullstack por defecto

- **DADO** un cuestionario de pre-vuelo en el que el usuario no declara ningún rol especializado
- **CUANDO** concluye el cuestionario
- **ENTONCES** el orquestador asigna el rol único `fullstack` con política `blocking`, `tasks.md` y `verify-report.md`
- **Y** el cambio queda con exactamente un rol informado

#### Scenario: Un único rol especializado declarado no se combina con fullstack

- **DADO** un cuestionario de pre-vuelo en el que el usuario declara exactamente un rol especializado (por ejemplo, únicamente `core`)
- **CUANDO** concluye el cuestionario
- **ENTONCES** el orquestador respeta ese único rol declarado como el rol activo del cambio
- **Y** no asigna adicionalmente el rol `fullstack` junto al rol ya declarado

#### Scenario: Varios roles especializados declarados tampoco activan fullstack

- **DADO** un cuestionario de pre-vuelo en el que el usuario declara varios roles especializados (por ejemplo `core`, `web` y `qa`)
- **CUANDO** concluye el cuestionario
- **ENTONCES** el orquestador no asigna el rol `fullstack`
- **Y** cada rol declarado conserva su propia política de compuerta

---

## 3. Capacidad: `sdd-block-review-gates`

Cuando el cambio se sella en modalidad “con paradas”, el orquestador se detiene al concluir cada bloque del ciclo de vida SDD y exige una aprobación explícita antes de abrir la siguiente fase.

### Requirement: Aplicabilidad de las Compuertas Condicionada a la Modalidad Sellada (REQ-21.7)

Las compuertas descritas en REQ-21.8 a REQ-21.11 DEBEN aplicarse única y exclusivamente cuando la modalidad de avance sellada en el pre-vuelo (REQ-21.5) es “con paradas”. Cuando la modalidad sellada es “continua”, el orquestador NO DEBE detenerse entre fases ni presentar ninguna de esas compuertas, y DEBE encadenar las fases automáticamente en cuanto cada una cumpla sus propios criterios de completitud.

#### Scenario: Modalidad con paradas detiene el flujo entre spec y design

- **DADO** un cambio sellado con modalidad “con paradas”
- **CUANDO** concluye la fase `spec`
- **ENTONCES** el orquestador se detiene y presenta la compuerta de `spec` antes de iniciar `design`

#### Scenario: Modalidad continua encadena las fases sin compuertas

- **DADO** un cambio sellado con modalidad “continua”
- **CUANDO** concluye la fase `spec`
- **ENTONCES** el orquestador continúa directamente a `design` sin presentar ninguna compuerta de revisión por bloque

---

### Requirement: Compuerta de `spec` (REQ-21.8)

En modalidad “con paradas”, al completarse la fase `spec` el orquestador DEBE detenerse y presentar al usuario el spec producido, evaluando explícitamente si cubre la intención original de la petición y si existen huecos o dudas abiertas. El orquestador NO DEBE iniciar la fase `design` mientras esta compuerta no reciba una aprobación explícita del usuario.

#### Scenario: Spec completo aprobado habilita design

- **DADO** un spec recién producido que cubre la intención original sin huecos detectados
- **CUANDO** se presenta la compuerta de `spec`
- **Y** el usuario la aprueba explícitamente
- **ENTONCES** el orquestador inicia la fase `design`

#### Scenario: Huecos detectados se enumeran explícitamente en la compuerta

- **DADO** un spec que deja al menos un escenario funcional o caso borde sin cubrir
- **CUANDO** se presenta la compuerta de `spec`
- **ENTONCES** el orquestador enumera explícitamente los huecos o dudas abiertas detectados como parte de la presentación de la compuerta
- **Y** no inicia `design` hasta que el usuario apruebe explícitamente, con o sin remediación previa de esos huecos

---

### Requirement: Compuerta de `design` (REQ-21.9)

En modalidad “con paradas”, al completarse la fase `design` el orquestador DEBE detenerse y evaluar explícitamente si el diseño satisface todos los requerimientos del spec y si respeta las directrices de arquitectura, las tecnologías declaradas en `axiom.yaml` y los patrones existentes del repositorio. El orquestador NO DEBE iniciar la fase `tasks` mientras esta compuerta no reciba una aprobación explícita del usuario.

#### Scenario: Diseño conforme aprobado habilita tasks

- **DADO** un `design.md` que cubre todos los requerimientos del spec aprobado y respeta la arquitectura declarada
- **CUANDO** se presenta la compuerta de `design` y el usuario la aprueba
- **ENTONCES** el orquestador inicia la fase `tasks`

#### Scenario: Desviación arquitectónica se señala explícitamente en la compuerta

- **DADO** un `design.md` que introduce una tecnología o patrón no declarado en `axiom.yaml` ni presente en el repositorio
- **CUANDO** se presenta la compuerta de `design`
- **ENTONCES** el orquestador señala explícitamente esa desviación como parte de la presentación de la compuerta
- **Y** no inicia `tasks` hasta recibir una aprobación explícita

---

### Requirement: Compuerta de `tasks` (REQ-21.10)

En modalidad “con paradas”, al completarse la fase `tasks` el orquestador DEBE presentar el documento `tasks.md` (o cada `tasks.<rol>.md` cuando existan varios roles) y DEBE detenerse antes de iniciar cualquier fase `apply`, evaluando si la repartición de responsabilidades entre roles es coherente y si el plan de trabajo es suficientemente granular y verificable. El orquestador NO DEBE iniciar ninguna fase `apply` mientras esta compuerta no reciba una aprobación explícita del usuario.

#### Scenario: Tasks de un único rol aprobadas habilitan apply

- **DADO** un cambio con el rol único `fullstack` y su `tasks.md` recién generado
- **CUANDO** se presenta la compuerta de `tasks`
- **Y** el usuario la aprueba
- **ENTONCES** el orquestador inicia la fase `apply` del rol `fullstack`

#### Scenario: Tasks multi-rol se presentan juntas antes de iniciar cualquier apply

- **DADO** un cambio con varios roles, cada uno con su propio `tasks.<rol>.md`
- **CUANDO** se presenta la compuerta de `tasks`
- **ENTONCES** el orquestador presenta la repartición de responsabilidades entre todos los roles a la vez, no solo el desglose de un rol individual
- **Y** no inicia `apply` para ningún rol hasta que el usuario apruebe explícitamente la compuerta

---

### Requirement: Compuerta de `apply` por Rol (REQ-21.11)

En modalidad “con paradas”, al finalizar la implementación de cada rol activo (incluido el rol único `fullstack`), el orquestador DEBE detenerse y evaluar: conformidad con el diseño asignado a ese rol, cumplimiento de las funcionalidades de la `spec` asignadas a ese rol, y estándares de calidad (compilación limpia, pruebas unitarias ejecutadas, cobertura y estilo). El orquestador NO DEBE marcar el rol como concluido, ni permitir que otro rol pendiente inicie su propia fase `apply`, mientras esta compuerta no reciba una aprobación explícita.

#### Scenario: Rol concluido y aprobado habilita el siguiente rol pendiente

- **DADO** un cambio con los roles `core` y `web`, donde `core` termina su implementación primero
- **CUANDO** se presenta la compuerta de `apply` del rol `core` y el usuario la aprueba
- **ENTONCES** el orquestador marca `core` como concluido y habilita el inicio de la fase `apply` del rol `web`

#### Scenario: Incumplimiento de calidad bloquea la conclusión del rol

- **DADO** un rol cuya implementación no supera sus pruebas unitarias o no cumple la funcionalidad de spec asignada
- **CUANDO** se presenta la compuerta de `apply` de ese rol
- **ENTONCES** el orquestador la rechaza en el criterio de calidad correspondiente y no marca el rol como concluido
- **Y** ese rol no puede disparar el aviso de último rol (REQ-21.13) hasta que su compuerta se apruebe

---

### Requirement: Rechazo de una Compuerta Bloquea el Avance y Exige Remediación (REQ-21.12)

Si el usuario rechaza o solicita cambios en cualquiera de las compuertas descritas en REQ-21.8 a REQ-21.11, el orquestador NO DEBE avanzar a la siguiente fase o rol, DEBE registrar el motivo del rechazo, y DEBE mantener el artefacto de la fase o rol actual abierto a remediación. Tras la remediación, el orquestador DEBE volver a presentar la misma compuerta antes de reintentar el avance.

#### Scenario: Rechazo de la compuerta de spec exige remediación antes de reintentar

- **DADO** que el usuario rechaza la compuerta de `spec` señalando un caso borde no cubierto
- **CUANDO** el orquestador registra el rechazo
- **ENTONCES** permanece en la fase `spec` para remediación y no inicia `design`
- **Y**, tras remediar el spec, vuelve a presentar la compuerta de `spec` antes de reintentar el avance

#### Scenario: Rechazo de la compuerta de apply de un rol impide el cierre de ese rol

- **DADO** que el usuario rechaza la compuerta de `apply` de un rol por incumplimiento de un requerimiento de spec
- **CUANDO** el orquestador registra el rechazo
- **ENTONCES** ese rol permanece abierto a remediación y no se marca como concluido
- **Y** no se dispara el aviso de último rol (REQ-21.13) para ese rol hasta que su compuerta de `apply` se apruebe explícitamente

---

## 4. Capacidad: `sdd-role-closure-handoff`

Al concluir el último rol activo del cambio, el orquestador cierra la fase de implementación con un aviso formal, un relevo de integración y la apertura de la verificación global de toda la solución.

### Requirement: Aviso Formal de Conclusión del Último Rol Activo (REQ-21.13)

Al aprobarse la compuerta de `apply` (REQ-21.11) del último rol pendiente entre los roles activos del cambio —incluido el caso de un único rol `fullstack`— el orquestador DEBE notificar formalmente al usuario que ha concluido la implementación del último rol activo y que se genera el relevo de integración para la verificación global de la solución.

#### Scenario: Último de varios roles activa el aviso y el relevo

- **DADO** un cambio con los roles `core`, `web` y `qa`, donde `core` y `web` ya están concluidos y `qa` es el único rol pendiente
- **CUANDO** se aprueba la compuerta de `apply` del rol `qa`
- **ENTONCES** el orquestador emite el aviso formal de conclusión del último rol
- **Y** genera el relevo de integración descrito en REQ-21.14

#### Scenario: Rol único fullstack satisface la condición de último rol de inmediato

- **DADO** un cambio con el rol único `fullstack`
- **CUANDO** se aprueba la compuerta de `apply` de ese único rol
- **ENTONCES** esa misma aprobación satisface la condición de “último rol activo”
- **Y** el orquestador emite el aviso formal y genera el relevo de integración inmediatamente, sin esperar ningún otro rol

#### Scenario: Compuerta rechazada no dispara el cierre aunque sea el único rol pendiente

- **DADO** un cambio en el que el único rol pendiente tiene su compuerta de `apply` rechazada (REQ-21.12)
- **CUANDO** se registra el rechazo
- **ENTONCES** el orquestador no emite el aviso de último rol ni genera el relevo de integración
- **Y** espera a que esa compuerta se apruebe explícitamente antes de reevaluar la condición de cierre

---

### Requirement: Generación del `handoff.md` de Integración (REQ-21.14)

Junto con el aviso de REQ-21.13, el orquestador DEBE generar un artefacto `handoff.md` canónico, conforme al esquema ya definido para relevos estructurados (frontmatter YAML con `from_phase`, `to_phase`, `from_role`, `to_role`, `timestamp` y `status`, seguido de las cinco secciones descriptivas), con `status: ready`, consolidando los cambios de todos los roles participantes y especificando las instrucciones de prueba para la fase `verify`.

#### Scenario: Handoff de integración consolida todos los roles participantes

- **DADO** el aviso de conclusión del último rol activo de un cambio con varios roles
- **CUANDO** el orquestador genera el relevo de integración
- **ENTONCES** el `handoff.md` resultante declara `from_phase: apply`, `to_phase: verify` y `status: ready`
- **Y** su contenido consolida los artefactos y decisiones de todos los roles participantes, no solo del último rol concluido

---

### Requirement: Ejecución de la Fase `verify` Global tras el Handoff (REQ-21.15)

Recibido el relevo de integración en estado `ready` (REQ-21.14), la fase `verify` DEBE ejecutar la suite completa de pruebas de la solución (integración, regresión y compuertas de seguridad aplicables) considerando el trabajo consolidado de todos los roles, y DEBE emitir un informe consolidado `verify-report.md`, distinto de los informes individuales `verify-report.<rol>.md` de cada rol. La fase `verify` global NO DEBE iniciarse si el `handoff.md` de integración no tiene `status: ready`.

#### Scenario: Verify global emite un informe consolidado distinto de los informes por rol

- **DADO** un `handoff.md` de integración con `status: ready` para un cambio con varios roles ya verificados individualmente
- **CUANDO** se ejecuta la fase `verify` global
- **ENTONCES** se genera `verify-report.md` consolidado, distinto de los `verify-report.<rol>.md` de cada rol
- **Y** el informe refleja la suite completa de pruebas de la solución, no solo la de un rol

#### Scenario: Handoff bloqueado impide iniciar el verify global

- **DADO** un `handoff.md` de integración con `status: blocked` o `needs_clarification`
- **CUANDO** se evalúa si procede iniciar la fase `verify` global
- **ENTONCES** el orquestador no la inicia
- **Y** exige resolver el bloqueo del relevo antes de reintentar

---

## 5. Capacidad: `sdd-archive-lifecycle`

El archivado es la entrega formal del incremento a un entorno preproductivo o productivo, no un paso inmediato tras las pruebas locales; una vez ejecutado, el alcance del incremento queda congelado.

### Requirement: Precondición de Integración o Despliegue para Ejecutar `archive` (REQ-21.16)

La fase `archive` NO DEBE ejecutarse como un paso inmediato tras la finalización de las pruebas locales de desarrollo, aunque `verify-report.md` (o el consolidado de REQ-21.15) tenga veredicto favorable. `archive` SOLO DEBE ejecutarse cuando exista evidencia de que el incremento se ha integrado o desplegado hacia un entorno preproductivo o productivo, mediante un PR formal fusionado hacia la rama principal (u otro evento de despliegue equivalente ya configurado para el proyecto). Si se solicita `archive` sin esa evidencia, el orquestador DEBE rechazar la operación de forma explícita, indicando la precondición concreta que falta, y el cambio DEBE permanecer sin mover a `openspec/changes/archive/`.

#### Scenario: PR fusionado habilita el archivado

- **DADO** un cambio con `verify-report.md` en veredicto favorable y un PR fusionado hacia la rama principal que integra ese incremento
- **CUANDO** se solicita `archive` para ese cambio
- **ENTONCES** la fase `archive` procede

#### Scenario: Archive solicitado sin integración ni despliegue se rechaza explícitamente

- **DADO** un cambio con `verify-report.md` en veredicto favorable pero sin ningún PR fusionado ni evidencia de despliegue hacia preproducción o producción
- **CUANDO** se solicita `archive` para ese cambio
- **ENTONCES** el orquestador rechaza la operación de forma explícita, señalando la ausencia de integración o despliegue como motivo
- **Y** el cambio permanece en `openspec/changes/<cambio>/` sin moverse a `openspec/changes/archive/`
- **Y** no se actualiza ninguna especificación viva en `openspec/specs/`

---

### Requirement: Contenido del Archivado (REQ-21.17)

Al ejecutarse `archive` con su precondición satisfecha (REQ-21.16), el sistema DEBE: transferir el incremento a `openspec/changes/archive/`; actualizar la especificación viva correspondiente en `openspec/specs/`; y actualizar el inventario maestro (`openspec/INDEX.md`) y los roadmaps declarados que referencien el incremento.

#### Scenario: Archivado exitoso actualiza incremento, especificación viva e inventario

- **DADO** un cambio cuya precondición de archivado ya está satisfecha
- **CUANDO** se ejecuta `archive`
- **ENTONCES** el incremento se transfiere a `openspec/changes/archive/`
- **Y** la especificación viva en `openspec/specs/` incorpora los requerimientos `ADDED`/`MODIFIED`/`REMOVED` del cambio
- **Y** `openspec/INDEX.md` y los roadmaps afectados reflejan el incremento como archivado

---

### Requirement: Sellado Inmutable Post-Archive y Gestión Exclusiva vía Bug o Nuevo Incremento (REQ-21.18)

Una vez archivado un incremento, su alcance queda formalmente congelado. El sistema NO DEBE reabrir un incremento ya archivado para aplicar un ajuste, un comportamiento anómalo o una regresión detectados con posterioridad. Cualquier corrección posterior DEBE gestionarse mediante un ticket de bug independiente o un nuevo incremento de evolución que referencie la especificación viva ya sellada, nunca modificando directamente el contenido bajo `openspec/changes/archive/`.

#### Scenario: Intento de reabrir un incremento archivado se rechaza

- **DADO** un incremento ya presente en `openspec/changes/archive/`
- **CUANDO** se intenta reabrirlo o modificarlo directamente para aplicar un ajuste
- **ENTONCES** el sistema rechaza la operación
- **Y** señala que la vía correcta es un ticket de bug o un nuevo incremento

#### Scenario: Regresión post-archive se gestiona mediante bug o nuevo incremento

- **DADO** que se detecta una regresión en un comportamiento definido por un incremento ya archivado
- **CUANDO** se gestiona esa regresión
- **ENTONCES** se abre un ticket de bug o un nuevo incremento de evolución que referencia la especificación viva afectada
- **Y** el contenido del incremento archivado original permanece sin modificaciones
