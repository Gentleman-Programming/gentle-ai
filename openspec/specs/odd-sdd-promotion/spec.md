<!-- Especificación Viva generada a partir de '2026-09-18-inc-19-odd-workflow-and-promotion' -->

# Especificación de Requerimientos: Promoción de Documentos ODD a Propuestas SDD (INC-19)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para la semántica de la frontera entre el carril ágil (ODD) y el carril formal (SDD) en Axiom: el mapeo determinista de un documento vivo ODD hacia una propuesta SDD sembrada, sin fabricar rigor que el documento de origen no respalda, con validación de nombre y colisión, marcado de estado en el documento después de la promoción, y prohibición de segunda promoción.

---

## 1. Capacidad: `odd-sdd-promotion`

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
