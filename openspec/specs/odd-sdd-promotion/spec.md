<!-- Especificación Viva generada a partir de '2026-09-18-inc-19-odd-workflow-and-promotion' -->

# Especificación de Requerimientos: Promoción de Documentos ODD a Propuestas SDD (INC-19)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para la semántica de la frontera entre el carril ágil (ODD) y el carril formal (SDD) en Axiom: el mapeo determinista de un documento vivo ODD hacia una propuesta SDD sembrada, sin fabricar rigor que el documento de origen no respalda, con validación de nombre y colisión, marcado de estado en el documento después de la promoción, y prohibición de segunda promoción.

---

## 1. Capacidad Modificada (Retirada Destructiva): `odd-sdd-promotion`

**Aviso de delta destructivo**: los cuatro requerimientos siguientes, archivados el 2026-09-18 como parte de INC-19, se retiran en su totalidad junto con `internal/odd/promote.go` (INC-20 F6.3). Corresponde a la fase de archivado de INC-20 decidir si este fichero se elimina por completo o se conserva vacío con esta misma nota de retirada.

## REMOVED Requirements

### Requirement: Mapeo Determinista de Documento ODD a Propuesta SDD (REQ-19.9)

(Motivo: el mapeo determinista de secciones del documento vivo hacia `proposal.md` lo implementaba `internal/odd/promote.go`, retirado por la Decisión D1.)
(Migración: la promoción de un documento vivo ODD a una propuesta SDD pasa a ser un acto de agente: el agente redacta `proposal.md` informado por el documento vivo, sin un mapeo Go determinista. El endpoint `POST /api/increments` con `proposal_body` (capacidad `dashboard-sdd-orchestration`, REQ-15.1) sigue disponible como vía de siembra de la propuesta ya redactada, sin cambios por este incremento.)

### Requirement: Validación de Nombre y Colisión al Promover (REQ-19.10)

(Motivo: la validación de nombre y las guardas de colisión específicas de la promoción ODD→SDD las implementaba `internal/odd/promote.go`, retirado por la Decisión D1.)
(Migración: la validación de nombre y colisión ordinaria de `axiom change create` y del endpoint `POST /api/increments` sigue aplicándose sobre la propuesta resultante, sin cambios por este incremento.)

### Requirement: Prohibición de Fabricar Contenido de Especificación al Promover (REQ-19.11)

(Motivo: el marcador explícito de la sección `## Capacidades` para `sdd-spec` lo generaba `internal/odd/promote.go`, retirado por la Decisión D1.)
(Migración: ninguna. Queda a criterio del agente que redacta la propuesta no fabricar contenido de la sección `## Capacidades`.)

### Requirement: Estado del Documento ODD tras la Promoción (REQ-19.12)

(Motivo: el marcado del documento vivo como `promovido` y el rechazo de una segunda promoción los implementaba `internal/odd/promote.go`, retirado por la Decisión D1.)
(Migración: ninguna. El seguimiento de qué documento vivo ya se promovió pasa a ser una convención de agente sin verificación Go.)

---
