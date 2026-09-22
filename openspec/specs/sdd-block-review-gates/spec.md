<!-- Especificación Viva generada a partir de '2026-09-22-inc-21-upfront-flow-governance' -->

# Especificación de Requerimientos: Compuertas de Revisión por Bloque (spec, design, tasks, apply) (INC-21)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y escenarios BDD para las cuatro compuertas de revisión por bloque que operan cuando un cambio SDD se sella en modalidad "con paradas y reviews": validación de `spec` (cobertura funcional), validación de `design` (conformidad arquitectónica), validación de `tasks` (repartición y granularidad), y validación de `apply` por rol (calidad e implementación).

---

## 1. Capacidad: `sdd-block-review-gates`

Cuando el cambio se sella en modalidad "con paradas", el orquestador se detiene al concluir cada bloque del ciclo de vida SDD y exige una aprobación explícita antes de abrir la siguiente fase.

### Requirement: Aplicabilidad de las Compuertas Condicionada a la Modalidad Sellada (REQ-21.7)

Las compuertas descritas en REQ-21.8 a REQ-21.11 DEBEN aplicarse única y exclusivamente cuando la modalidad de avance sellada en el pre-vuelo (REQ-21.5) es "con paradas". Cuando la modalidad sellada es "continua", el orquestador NO DEBE detenerse entre fases ni presentar ninguna de esas compuertas, y DEBE encadenar las fases automáticamente en cuanto cada una cumpla sus propios criterios de completitud.

#### Scenario: Modalidad con paradas detiene el flujo entre spec y design

- **DADO** un cambio sellado con modalidad "con paradas"
- **CUANDO** concluye la fase `spec`
- **ENTONCES** el orquestador se detiene y presenta la compuerta de `spec` antes de iniciar `design`

#### Scenario: Modalidad continua encadena las fases sin compuertas

- **DADO** un cambio sellado con modalidad "continua"
- **CUANDO** concluye la fase `spec`
- **ENTONCES** el orquestador continúa directamente a `design` sin presentar ninguna compuerta de revisión por bloque

---

### Requirement: Compuerta de `spec` (REQ-21.8)

En modalidad "con paradas", al completarse la fase `spec` el orquestador DEBE detenerse y presentar al usuario el spec producido, evaluando explícitamente si cubre la intención original de la petición y si existen huecos o dudas abiertas. El orquestador NO DEBE iniciar la fase `design` mientras esta compuerta no reciba una aprobación explícita del usuario.

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

En modalidad "con paradas", al completarse la fase `design` el orquestador DEBE detenerse y evaluar explícitamente si el diseño satisface todos los requerimientos del spec y si respeta las directrices de arquitectura, las tecnologías declaradas en `axiom.yaml` y los patrones existentes del repositorio. El orquestador NO DEBE iniciar la fase `tasks` mientras esta compuerta no reciba una aprobación explícita del usuario.

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

En modalidad "con paradas", al completarse la fase `tasks` el orquestador DEBE presentar el documento `tasks.md` (o cada `tasks.<rol>.md` cuando existan varios roles) y DEBE detenerse antes de iniciar cualquier fase `apply`, evaluando si la repartición de responsabilidades entre roles es coherente y si el plan de trabajo es suficientemente granular y verificable. El orquestador NO DEBE iniciar ninguna fase `apply` mientras esta compuerta no reciba una aprobación explícita del usuario.

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

En modalidad "con paradas", al finalizar la implementación de cada rol activo (incluido el rol único `fullstack`), el orquestador DEBE detenerse y evaluar: conformidad con el diseño asignado a ese rol, cumplimiento de las funcionalidades de la `spec` asignadas a ese rol, y estándares de calidad (compilación limpia, pruebas unitarias ejecutadas, cobertura y estilo). El orquestador NO DEBE marcar el rol como concluido, ni permitir que otro rol pendiente inicie su propia fase `apply`, mientras esta compuerta no reciba una aprobación explícita.

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
