<!-- Especificación Viva generada a partir de '2026-09-18-inc-19-odd-workflow-and-promotion' -->

# Especificación de Requerimientos: Documento Vivo ODD y Espejo de Recuperación en Engram (INC-19)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para el soporte del carril ágil de Organic Driven Development (ODD) en Axiom: el documento vivo `odd/tasks/<feature>.md` como fuente de verdad del trabajo cotidiano, la estructura canónica de doce secciones, la identidad estable de *feature*, los identificadores estables de tarea y su derivación de progreso, y el espejo de recuperación en Engram con su política de autoridad y manejo de divergencia.

---

## 1. Capacidad Modificada (Retirada Destructiva): `odd-living-document`

**Aviso de delta destructivo** (conforme a `rules.archive` de `openspec/config.yaml`, aplicado por INC-20 F6.4): la Decisión D1 resuelve la sustitución completa del ODD del fork por el ODD de upstream, sin conservar la capa Go. Los cuatro requerimientos siguientes, archivados el 2026-09-18 como parte de INC-19, se retiran en su totalidad. Esta capacidad queda sin ningún requerimiento vivo; corresponde a la fase de archivado de INC-20 decidir si este fichero se elimina por completo o se conserva vacío con esta misma nota de retirada.

## REMOVED Requirements

### Requirement: Estructura Canónica del Documento Vivo ODD (REQ-19.1)

(Motivo: la Decisión D1 retira por completo el paquete Go `internal/odd` —incluidos `document.go`, `render.go`, `template.go` y `store.go`, que implementaban esta estructura de doce secciones— y lo sustituye por el protocolo ODD de upstream, basado en instrucciones de agente inyectadas en `internal/components/agentguidance/routing.go`.)
(Migración: los documentos `odd/tasks/<feature>.md` y sus espejos Engram `odd/<feature>/tasks` ya existentes no se modifican ni se eliminan; son datos, no código. El contrato de forma del documento continúa como convención de facto sostenida por instrucciones de agente, fuera del alcance de `openspec/specs/`; no existe una capacidad Go sucesora verificable por test para esta estructura.)

### Requirement: Identidad Estable de Feature y Ubicación del Fichero (REQ-19.2)

(Motivo: la validación de kebab-case y el rechazo de colisión de nombre de *feature* los implementaba `internal/odd/name.go`, retirado junto con el resto del paquete por la Decisión D1.)
(Migración: ninguna. La resolución de nombre de *feature* pasa a ser una convención de agente sin verificación Go.)

### Requirement: Identificadores Estables de Tarea y Derivación de Progreso (REQ-19.3)

(Motivo: la derivación de progreso mediante `multirole.CountTasks` sobre el checklist accionable estaba implementada en `internal/odd/parse.go` y `internal/odd/document.go`, retirados por la Decisión D1.)
(Migración: el protocolo ODD de upstream reporta continuidad de *feature*, TDD configurado y commit por unidad de trabajo como doctrina de agente, sin un mecanismo Go equivalente y verificable por test en este fork.)

### Requirement: Contrato del Espejo de Recuperación en Engram y Política de Divergencia (REQ-19.4)

(Motivo: la comparación fichero-vs-espejo y la política de autoridad las implementaba `internal/odd/mirror.go`, retirado por la Decisión D1.)
(Migración: el espejo Engram `odd/<feature>/tasks` sigue siendo la convención que el agente mantiene por MCP, ahora sin ninguna verificación del lado Go; no hay comprobación automática de divergencia equivalente a `--check-mirror` tras esta retirada.)

---
