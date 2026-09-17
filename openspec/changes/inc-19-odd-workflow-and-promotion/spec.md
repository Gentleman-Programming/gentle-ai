<!-- Especificación Viva en desarrollo — cambio 'inc-19-odd-workflow-and-promotion' -->

# Especificación de Requerimientos: Flujo ODD y Promoción a SDD (INC-19)

> **Incremento:** `inc-19-odd-workflow-and-promotion`
> **Estado:** En desarrollo (fase de especificación)
> **Idioma:** Español (castellano peninsular)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para el carril ágil de Organic Driven Development (ODD) en Axiom: el documento vivo `odd/tasks/<feature>.md` y su espejo de recuperación en Engram, la superficie de la CLI `axiom odd create|status|promote`, la semántica de la promoción de un documento ODD a una propuesta SDD, la exposición reactiva del estado ODD en el Dashboard Web y en la TUI con conmutación explícita de carril, y la extensión mínima y retrocompatible del motor de creación de incrementos SDD que la promoción reutiliza.

## Alcance de esta Especificación

| # | Capacidad | Tipo | Requerimientos |
|---|---|---|---|
| 1 | `odd-living-document` | Nueva | REQ-19.1 – REQ-19.4 |
| 2 | `odd-cli-commands` | Nueva | REQ-19.5 – REQ-19.8 |
| 3 | `odd-sdd-promotion` | Nueva | REQ-19.9 – REQ-19.12 |
| 4 | `odd-ui-integration` | Nueva | REQ-19.13 – REQ-19.15 |
| 5 | `dashboard-sdd-orchestration` | Modificada (únicamente REQ-15.1) | REQ-15.1 |

Fuera de esta especificación, por decisión de producto ya cerrada en la propuesta: cualquier cambio en `internal/sddstatus/`, `internal/cli/sdd_*.go` o `internal/agents/researchcapability/` (diferido a INC-20), y las capacidades `tui-ui-parity`, `local-web-dashboard`, `organic-agent-trigger-rules`, `sdd-orchestrator-assets`, `axiom-sdd-cli-integration` y `rdd-sdd-receipt-consumption`, revisadas y confirmadas sin necesidad de delta.

---

## 1. Capacidad: `odd-living-document`

Sostiene el carril ágil de Organic Driven Development (ODD) con un único documento por *feature*: la fuente de verdad del trabajo cotidiano, con progreso derivable y un espejo de recuperación que nunca compite con el fichero por la autoridad.

### Requirement: Estructura Canónica del Documento Vivo ODD (REQ-19.1)

El sistema DEBE representar cada documento vivo ODD como un único fichero Markdown en `odd/tasks/<feature>.md`, redactado en castellano peninsular, que contenga exactamente las doce secciones canónicas, en este orden: Objetivo, Problema, Porqué, Alcance, Restricciones, Alcance autorizado, Checklist accionable, Criterios de aceptación, Comprobaciones aplicables, Progreso, Evidencia de verificación y Siguiente paso. El sistema NO DEBE fragmentar el trabajo de una *feature* en un fichero de plan separado ni en un registro de evidencia separado.

#### Scenario: Creación de un documento vivo nuevo con las doce secciones

- **DADO** que no existe `odd/tasks/gestion-inventario.md`
- **CUANDO** se crea el documento vivo para la *feature* `gestion-inventario`
- **ENTONCES** el fichero resultante contiene las doce secciones canónicas, en el orden definido, redactadas en castellano peninsular
- **Y** ninguna sección canónica queda sin su encabezado, aunque su contenido inicial esté pendiente de completar

#### Scenario: Un documento existente conserva su estructura al actualizarse

- **DADO** un documento vivo `odd/tasks/gestion-inventario.md` con sus doce secciones ya redactadas
- **CUANDO** se actualiza su checklist o su progreso tras completar una tarea
- **ENTONCES** las doce secciones canónicas permanecen presentes y en el mismo orden
- **Y** el contenido de las secciones no tocadas por la actualización permanece intacto

---

### Requirement: Identidad Estable de Feature y Ubicación del Fichero (REQ-19.2)

El sistema DEBE identificar cada documento vivo por un nombre de *feature* único, en formato kebab-case, que determina de forma determinista su ruta `odd/tasks/<feature>.md`. El sistema DEBE rechazar de forma explícita la creación de un documento cuyo nombre de *feature* ya esté en uso por otro documento vivo existente, y DEBE rechazar nombres que no cumplan el formato kebab-case.

#### Scenario: Nombre de feature válido crea el fichero en la ruta esperada

- **DADO** un nombre de *feature* válido en kebab-case, por ejemplo `gestion-inventario`
- **CUANDO** se crea el documento vivo para esa *feature*
- **ENTONCES** el fichero se ubica exactamente en `odd/tasks/gestion-inventario.md`

#### Scenario: Colisión de nombre de feature rechazada de forma explícita

- **DADO** un documento vivo existente en `odd/tasks/gestion-inventario.md`
- **CUANDO** se intenta crear un nuevo documento vivo con el mismo nombre de *feature* `gestion-inventario`
- **ENTONCES** el sistema rechaza la operación con un mensaje explícito de colisión
- **Y** el documento vivo existente permanece sin modificar

---

### Requirement: Identificadores Estables de Tarea y Derivación de Progreso (REQ-19.3)

Cada tarea del checklist accionable DEBE llevar un identificador estable (`T1`, `T2`, …) que el sistema NO DEBE reasignar ante una reordenación o edición del checklist. El sistema DEBE derivar la sección Progreso del documento vivo reutilizando `multirole.CountTasks` sobre las casillas del checklist accionable, sin introducir un mecanismo de cómputo alternativo.

#### Scenario: El progreso se deriva contando casillas marcadas

- **DADO** un documento vivo con un checklist accionable de 5 tareas identificadas `T1`–`T5`, de las cuales 2 están marcadas como completadas
- **CUANDO** se solicita el progreso del documento
- **ENTONCES** el sistema reporta 2 de 5 tareas completadas, calculado mediante `multirole.CountTasks`

#### Scenario: Reordenar el checklist conserva los identificadores de tarea

- **DADO** un documento vivo con tareas `T1`, `T2` y `T3` en ese orden
- **CUANDO** el checklist se reordena de forma que `T3` aparece antes que `T1`
- **ENTONCES** cada tarea conserva su identificador original (`T1`, `T2`, `T3`)
- **Y** ningún identificador se reutiliza para una tarea distinta

---

### Requirement: Contrato del Espejo de Recuperación en Engram y Política de Divergencia (REQ-19.4)

El fichero `odd/tasks/<feature>.md` es la única fuente de verdad autoritativa. El sistema DEBE mantener un espejo de recuperación en Engram bajo el *topic* `odd/<feature>/tasks`, con ámbito del proyecto actual; ese espejo lo actualiza el agente por MCP, y el lado Go del sistema NO DEBE escribir en Engram ni resolver una divergencia detectada entre el fichero y su espejo: DEBE únicamente informarla. Ante la indisponibilidad de Engram, el sistema DEBE declarar el espejo como *pendiente* y NO DEBE reclamar una sincronización exitosa ni bloquear el trabajo local por esa causa.

#### Scenario: El fichero manda cuando el espejo está disponible pero difiere

- **DADO** un documento vivo cuyo contenido en `odd/tasks/gestion-inventario.md` difiere del espejo `odd/gestion-inventario/tasks` en Engram
- **CUANDO** se compara explícitamente el fichero contra su espejo
- **ENTONCES** el sistema informa el estado `divergente`
- **Y** el sistema no modifica ni el fichero ni el espejo para forzar la reconciliación

#### Scenario: Engram no disponible degrada el espejo sin bloquear ni fallar

- **DADO** que el binario o servicio de Engram no responde
- **CUANDO** se intenta actualizar o comparar el espejo de un documento vivo
- **ENTONCES** el sistema declara el espejo como *pendiente* o `no disponible`
- **Y** el trabajo local sobre el fichero continúa sin bloqueo
- **Y** el sistema no reclama en ningún momento una sincronización exitosa

---

## 2. Capacidad: `odd-cli-commands`

Permite a desarrolladores y agentes crear, consultar y promover documentos vivos ODD desde la línea de comandos canónica `axiom`, replicando el patrón manual ya empleado por `axiom sdd` y `axiom change`.

### Requirement: Subcomando axiom odd create (REQ-19.5)

La CLI `axiom` DEBE proveer el subcomando `axiom odd create <nombre>`, que crea el documento vivo descrito en REQ-19.1 y REQ-19.2. Al completarse con éxito, DEBE informar en la salida estándar la ruta del fichero creado y DEBE finalizar con código de salida `0`. Ante un nombre inválido o en colisión, DEBE finalizar con código de salida distinto de `0` y un mensaje de error descriptivo en la salida de error estándar.

#### Scenario: Creación exitosa desde la CLI

- **DADO** el binario `axiom`
- **CUANDO** el usuario ejecuta `axiom odd create gestion-inventario`
- **ENTONCES** el comando crea `odd/tasks/gestion-inventario.md`
- **Y** informa la ruta creada en stdout con código de salida `0`

#### Scenario: Nombre inválido o en colisión rechazado desde la CLI

- **DADO** un documento vivo existente en `odd/tasks/gestion-inventario.md`
- **CUANDO** el usuario ejecuta `axiom odd create gestion-inventario` de nuevo
- **ENTONCES** el comando finaliza con código de salida distinto de `0`
- **Y** emite un mensaje de error descriptivo en stderr sin modificar el fichero existente

---

### Requirement: Subcomando axiom odd status con Salida en Texto y JSON (REQ-19.6)

La CLI `axiom` DEBE proveer el subcomando `axiom odd status`, que lee siempre y solo los documentos vivos existentes bajo `odd/tasks/` para informar su progreso, DEBE ofrecer salida legible en texto por defecto y salida estructurada mediante la bandera `--json`, y en ningún caso DEBE escribir en Engram.

#### Scenario: Consulta de estado en texto legible

- **DADO** uno o más documentos vivos en `odd/tasks/` con progreso parcial
- **CUANDO** el usuario ejecuta `axiom odd status`
- **ENTONCES** el comando imprime en stdout el progreso de cada documento, en formato legible, leído únicamente de los ficheros
- **Y** no se produce ninguna escritura en Engram

#### Scenario: Consulta de estado en formato estructurado

- **DADO** los mismos documentos vivos
- **CUANDO** el usuario ejecuta `axiom odd status --json`
- **ENTONCES** el comando imprime en stdout un objeto JSON con el progreso y los metadatos de cada documento

---

### Requirement: Bandera --check-mirror en axiom odd status (REQ-19.7)

El subcomando `axiom odd status` DEBE aceptar la bandera opcional `--check-mirror`, que activa la comparación explícita descrita en REQ-19.4 entre cada documento vivo y su espejo en Engram. Sin esa bandera, el comando NO DEBE invocar el espejo. Los tres estados informables son `sincronizado`, `divergente` y `no disponible`; el estado `no disponible` NO DEBE alterar el código de salida del comando.

#### Scenario: Estado por defecto no invoca el espejo

- **DADO** un documento vivo con espejo divergente en Engram
- **CUANDO** el usuario ejecuta `axiom odd status` sin `--check-mirror`
- **ENTONCES** el comando informa el progreso leído del fichero
- **Y** no se invoca ninguna comparación contra Engram

#### Scenario: --check-mirror informa divergencia explícita

- **DADO** el mismo documento vivo con espejo divergente
- **CUANDO** el usuario ejecuta `axiom odd status --check-mirror`
- **ENTONCES** el comando informa el estado `divergente` para ese documento

#### Scenario: Engram no disponible no altera el código de salida

- **DADO** que el binario o servicio de Engram no responde
- **CUANDO** el usuario ejecuta `axiom odd status --check-mirror`
- **ENTONCES** el comando informa el estado `no disponible`
- **Y** conserva el mismo código de salida que habría tenido sin `--check-mirror`

---

### Requirement: Subcomando axiom odd promote: Banderas y Presencia en la Ayuda (REQ-19.8)

La CLI `axiom` DEBE proveer el subcomando `axiom odd promote <feature>`, con las banderas opcionales `--dry-run` (previsualiza sin escribir) y `--name <nombre>` (sobrescribe el nombre de cambio derivado del *feature*). El grupo `axiom odd` completo (`create`, `status`, `promote`) DEBE figurar en la salida de `axiom --help` / `printHelp()`.

#### Scenario: --dry-run previsualiza sin escribir

- **DADO** un documento vivo `odd/tasks/gestion-inventario.md` listo para promover
- **CUANDO** el usuario ejecuta `axiom odd promote gestion-inventario --dry-run`
- **ENTONCES** el comando imprime en stdout el contenido que tendría el `proposal.md` sembrado
- **Y** no crea ningún directorio ni fichero bajo `openspec/changes/`

#### Scenario: --name sobrescribe el nombre de cambio derivado

- **DADO** el mismo documento vivo
- **CUANDO** el usuario ejecuta `axiom odd promote gestion-inventario --name modulo-inventario-v2`
- **ENTONCES** el cambio SDD se crea en `openspec/changes/modulo-inventario-v2/`

#### Scenario: El grupo odd aparece en la ayuda de la CLI

- **DADO** el binario `axiom`
- **CUANDO** el usuario ejecuta `axiom --help`
- **ENTONCES** la salida incluye una entrada para `axiom odd` junto a sus subcomandos `create`, `status` y `promote`

---

## 3. Capacidad: `odd-sdd-promotion`

Materializa la frontera entre el carril ágil (ODD) y el carril formal (SDD): transporta lo ya conocido de un documento vivo hacia una propuesta SDD sembrada, sin fabricar rigor que el documento de origen no respalda.

### Requirement: Mapeo Determinista de Documento ODD a Propuesta SDD (REQ-19.9)

Al promover, el sistema DEBE mapear el documento vivo ODD a un `proposal.md` sembrado siguiendo esta correspondencia: Objetivo + Problema + Porqué → Propósito (Intent); Alcance (lo que entra) + Alcance autorizado → Dentro de Alcance; Alcance (lo que no entra) + Restricciones → Fuera de Alcance; Checklist accionable con su estado → Enfoque (Approach) y un apéndice "Estado heredado de ODD"; Criterios de aceptación → Criterios de Éxito; Comprobaciones aplicables y Evidencia de verificación → Criterios de Éxito y el apéndice de estado heredado. La propuesta sembrada DEBE declarar en su cabecera que procede de una promoción ODD y de qué documento procede.

#### Scenario: Promoción exitosa siembra la propuesta con el mapeo completo

- **DADO** un documento vivo `odd/tasks/gestion-inventario.md` con sus doce secciones redactadas
- **CUANDO** se ejecuta `axiom odd promote gestion-inventario`
- **ENTONCES** se crea `openspec/changes/gestion-inventario/proposal.md`
- **Y** su cabecera declara que procede de la promoción del documento vivo `odd/tasks/gestion-inventario.md`
- **Y** sus secciones Propósito, Dentro de Alcance, Fuera de Alcance, Enfoque y Criterios de Éxito contienen la información mapeada desde el documento de origen

#### Scenario: Ausencia de evidencia de verificación no se fabrica en el mapeo

- **DADO** un documento vivo cuya sección "Evidencia de verificación" está vacía porque el trabajo aún no se ha comprobado
- **CUANDO** se ejecuta la promoción
- **ENTONCES** el apéndice "Estado heredado de ODD" de la propuesta sembrada refleja la ausencia de evidencia sin inventar resultados de comprobación

---

### Requirement: Validación de Nombre y Colisión al Promover (REQ-19.10)

El sistema DEBE validar el nombre de cambio derivado del *feature* (o suministrado mediante `--name`) reutilizando la misma expresión regular de nombre válido y las mismas guardas de colisión contra cambios activos y archivados que ya emplea la creación de incrementos existente. Ante un nombre inválido o en colisión, el sistema DEBE rechazar la promoción con un fallo explícito y NO DEBE crear ningún fichero.

#### Scenario: Colisión con un cambio SDD activo rechazada

- **DADO** un cambio SDD activo existente en `openspec/changes/gestion-inventario/`
- **CUANDO** se ejecuta `axiom odd promote gestion-inventario` sin `--name`
- **ENTONCES** la promoción se rechaza con un mensaje explícito de colisión
- **Y** no se modifica el contenido de `openspec/changes/gestion-inventario/`

#### Scenario: Nombre de feature inválido como nombre de cambio rechazado

- **DADO** un documento vivo cuyo nombre de *feature* no es un kebab-case válido como nombre de cambio SDD
- **CUANDO** se ejecuta la promoción sin `--name`
- **ENTONCES** la promoción se rechaza con un mensaje explícito
- **Y** no se crea ningún directorio bajo `openspec/changes/`

---

### Requirement: Prohibición de Fabricar Contenido de Especificación al Promover (REQ-19.11)

La propuesta sembrada NO DEBE rellenar la sección `## Capacidades` con contenido inventado. El sistema DEBE dejar en su lugar un marcador explícito dirigido a la fase `sdd-spec`, indicando que las capacidades deben identificarse en esa fase.

#### Scenario: La sección de Capacidades queda marcada, no inventada

- **DADO** cualquier documento vivo válido para promover
- **CUANDO** se ejecuta la promoción
- **ENTONCES** la sección `## Capacidades` de la propuesta sembrada contiene el marcador explícito para `sdd-spec`
- **Y** no contiene nombres de capacidad inventados por la promoción

#### Scenario: El checklist con lenguaje técnico no se reinterpreta como capacidades

- **DADO** un documento vivo cuyo checklist menciona nombres de módulos o componentes técnicos
- **CUANDO** se ejecuta la promoción
- **ENTONCES** esos nombres se transportan literalmente al apéndice "Estado heredado de ODD" dentro de Enfoque
- **Y** no se copian ni reinterpretan como entradas de la sección `## Capacidades`

---

### Requirement: Estado del Documento ODD tras la Promoción (REQ-19.12)

Tras una promoción exitosa, el sistema DEBE conservar el documento vivo en `odd/tasks/<feature>.md` sin borrarlo ni moverlo, DEBE añadirle la referencia al cambio SDD creado y DEBE marcar su estado como `promovido`. Una segunda promoción del mismo documento ya marcado como `promovido` DEBE rechazarse de forma explícita, sin crear un cambio SDD duplicado.

#### Scenario: El documento se conserva y se marca como promovido

- **DADO** un documento vivo recién promovido a `openspec/changes/gestion-inventario/`
- **CUANDO** se consulta el documento vivo `odd/tasks/gestion-inventario.md`
- **ENTONCES** el fichero sigue existiendo en la misma ruta
- **Y** contiene la referencia al cambio SDD creado y el estado `promovido`

#### Scenario: Segunda promoción del mismo documento rechazada

- **DADO** un documento vivo ya marcado como `promovido`
- **CUANDO** se ejecuta `axiom odd promote gestion-inventario` de nuevo
- **ENTONCES** la promoción se rechaza de forma explícita
- **Y** no se crea un segundo directorio de cambio SDD

#### Scenario: axiom odd status refleja el documento promovido como cerrado

- **DADO** un documento vivo marcado como `promovido`
- **CUANDO** se ejecuta `axiom odd status`
- **ENTONCES** el documento se reporta como `promovido` (cerrado), no como trabajo en curso
- **Y** se informa la referencia al cambio SDD asociado

---

## 4. Capacidad: `odd-ui-integration`

Expone el estado del carril ODD de forma reactiva y con paridad de información en el Dashboard Web y en la TUI interactiva, con una conmutación de carril explícita y visible en ambas superficies.

### Requirement: Exposición del Estado ODD en el Dashboard Web (REQ-19.13)

El Dashboard Web (`axiom ui`) DEBE exponer el estado de los documentos vivos ODD (lista de documentos, progreso de cada uno y cuáles están marcados como `promovido`) a través de la API REST local, y el frontend DEBE renderizar esa información en la interfaz visual sin requerir recarga manual de página.

#### Scenario: Listado de documentos vivos con su progreso en el Dashboard

- **DADO** uno o más documentos vivos en `odd/tasks/`
- **CUANDO** se consulta el estado ODD desde el Dashboard Web
- **ENTONCES** la interfaz muestra cada documento con su progreso y, si aplica, su marca de `promovido`

#### Scenario: Sin documentos vivos, el Dashboard informa un estado vacío claro

- **DADO** que no existe ningún documento en `odd/tasks/`
- **CUANDO** se consulta el estado ODD desde el Dashboard Web
- **ENTONCES** la interfaz informa que no hay documentos vivos, sin reportar error

---

### Requirement: Exposición del Estado ODD en la TUI (REQ-19.14)

La TUI interactiva de Axiom DEBE implementar una pantalla dedicada que lea y muestre los documentos vivos ODD (lista, progreso y estado de promoción), accesible desde el menú de Gobernanza existente, con la misma información que expone REQ-19.13 para el Dashboard Web.

#### Scenario: Consulta del estado ODD desde la TUI

- **DADO** uno o más documentos vivos en `odd/tasks/`
- **CUANDO** el usuario navega a la pantalla ODD de la TUI desde el menú de Gobernanza
- **ENTONCES** la pantalla muestra la misma lista de documentos, progreso y estado de promoción que el Dashboard Web

#### Scenario: Sin documentos vivos, la TUI informa un estado vacío claro

- **DADO** que no existe ningún documento en `odd/tasks/`
- **CUANDO** el usuario navega a la pantalla ODD de la TUI
- **ENTONCES** la pantalla informa que no hay documentos vivos, sin reportar error

---

### Requirement: Conmutación Visible entre Carril ODD y Carril SDD (REQ-19.15)

Tanto el Dashboard Web como la TUI DEBEN ofrecer una acción visible y explícita para conmutar entre el carril ágil (ODD) y el carril formal (SDD), y DEBEN reflejar cuándo un documento ODD ha sido promovido, ofreciendo el salto directo al cambio SDD correspondiente.

#### Scenario: Salto desde un documento ODD promovido a su cambio SDD

- **DADO** un documento vivo marcado como `promovido` con referencia a `openspec/changes/gestion-inventario/`
- **CUANDO** el usuario selecciona ese documento en el Dashboard Web o en la TUI
- **ENTONCES** la interfaz ofrece una acción visible que conduce a la vista del cambio SDD `gestion-inventario`

#### Scenario: Conmutación entre carriles desde cualquiera de las dos interfaces

- **DADO** la pantalla de gobernanza en la TUI o la vista equivalente en el Dashboard Web
- **CUANDO** el usuario activa la acción de conmutación de carril
- **ENTONCES** la interfaz cambia entre la vista del carril ODD y la vista del carril SDD sin perder el contexto del proyecto activo

---

## 5. Capacidad Modificada: `dashboard-sdd-orchestration`

Esta capacidad ya existe en `openspec/specs/dashboard-sdd-orchestration/spec.md`. El bloque siguiente es una **modificación completa** del requerimiento `REQ-15.1` en esa especificación viva: sustituye el bloque entero, encabezado y escenarios incluidos. Los requerimientos `REQ-15.2`, `REQ-15.3` y `REQ-15.4` (misma capacidad) y `REQ-15.5` (capacidad `cli-change-creator`, mismo fichero) no se modifican y DEBEN conservarse sin cambios al fusionar este delta.

### Requirement: Creación de Incrementos y Andamiaje SDD vía API (REQ-15.1)

El servidor HTTP local DEBE exponer el endpoint `POST /api/increments` para recibir peticiones de creación de un nuevo incremento. La petición DEBE incluir el nombre del cambio (`name`, en formato kebab-case), el propósito o intención (`intent`), opcionalmente el tipo de cambio (`type`), y opcionalmente un cuerpo de propuesta ya renderizado (`proposal_body`) que sustituye el contenido de la plantilla generada por defecto. El servidor DEBE validar que el nombre no contenga caracteres inválidos ni coincida con un cambio existente en `openspec/changes/` o `openspec/changes/archive/`. Al validarse, el servidor DEBE crear el directorio del cambio y generar el archivo `proposal.md`: si se aportó `proposal_body` con contenido no vacío, DEBE escribir ese contenido de forma literal; si no se aportó o está vacío, DEBE generar la plantilla canónica vigente redactada en español castellano, byte a byte idéntica a la que producía antes de admitir este campo.

(Previamente: el endpoint aceptaba únicamente `name`, `intent` y `type`, y siempre generaba la plantilla canónica incrustada sin posibilidad de sustituir su contenido.)

#### Scenario: Creación exitosa de un nuevo incremento con plantilla en español

- **DADO** el servidor HTTP del Dashboard Web activo en un workspace Axiom
- **CUANDO** se envía una petición `POST /api/increments` con el cuerpo JSON:
  ```json
  {
    "name": "nuevo-modulo-auth",
    "intent": "Implementar autenticación basada en JWT con rotación atómica de tokens",
    "type": "feature"
  }
  ```
- **ENTONCES** el servidor responde con código de estado HTTP `201 Created`
- **Y** la respuesta JSON contiene `"success": true` y `"name": "nuevo-modulo-auth"`
- **Y** se crea el directorio `openspec/changes/nuevo-modulo-auth/`
- **Y** se genera el archivo `openspec/changes/nuevo-modulo-auth/proposal.md` conteniendo el título en español, el propósito y las secciones canónicas de alcance y capacidades

#### Scenario: Rechazo de creación ante nombre inválido o colisión

- **DADO** un workspace con un incremento existente denominado `auth-core`
- **CUANDO** se envía una petición `POST /api/increments` con `"name": "auth-core"` o `"name": "Nombre Con Espacios!"`
- **ENTONCES** el servidor responde con código de estado HTTP `400 Bad Request`
- **Y** la respuesta JSON contiene un mensaje de error descriptivo en `"error"`
- **Y** no se altera el sistema de archivos

#### Scenario: Creación de incremento con cuerpo de propuesta ya renderizado

- **DADO** el servidor HTTP del Dashboard Web activo
- **CUANDO** se envía una petición `POST /api/increments` con `"name": "gestion-inventario"`, un `"intent"` descriptivo y un campo `"proposal_body"` que contiene un documento de propuesta ya redactado, por ejemplo sembrado por `axiom odd promote`
- **ENTONCES** el servidor responde con código de estado HTTP `201 Created`
- **Y** `openspec/changes/gestion-inventario/proposal.md` contiene exactamente el contenido de `proposal_body`, sin sustituirlo por la plantilla generada por defecto

#### Scenario: Ausencia de cuerpo sembrado preserva la plantilla vigente

- **DADO** el servidor HTTP del Dashboard Web activo
- **CUANDO** se envía una petición `POST /api/increments` sin el campo `proposal_body`, igual que antes de admitir este campo
- **ENTONCES** el `proposal.md` generado es byte a byte idéntico al que el servidor producía antes de admitir `proposal_body`
- **Y** el comportamiento observable de `axiom change create` y de `POST /api/increments` permanece sin cambios para las peticiones que no aportan `proposal_body`
