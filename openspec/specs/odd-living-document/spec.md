<!-- Especificación Viva generada a partir de '2026-09-18-inc-19-odd-workflow-and-promotion' -->

# Especificación de Requerimientos: Documento Vivo ODD y Espejo de Recuperación en Engram (INC-19)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para el soporte del carril ágil de Organic Driven Development (ODD) en Axiom: el documento vivo `odd/tasks/<feature>.md` como fuente de verdad del trabajo cotidiano, la estructura canónica de doce secciones, la identidad estable de *feature*, los identificadores estables de tarea y su derivación de progreso, y el espejo de recuperación en Engram con su política de autoridad y manejo de divergencia.

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
