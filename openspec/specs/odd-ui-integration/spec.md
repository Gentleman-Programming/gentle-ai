<!-- Especificación Viva generada a partir de '2026-09-18-inc-19-odd-workflow-and-promotion' -->

# Especificación de Requerimientos: Integración de ODD en Dashboard Web y TUI (INC-19)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para la exposición reactiva del estado del carril ágil de Organic Driven Development (ODD) en el Dashboard Web (`axiom ui`) y en la TUI interactiva de Axiom, con paridad de información entre ambas superficies, conmutación explícita y visible entre carril ODD y carril formal (SDD), y navegación directa desde documentos promovidos hacia sus cambios SDD correspondientes.

---

## 1. Capacidad Modificada (Retirada Destructiva): `odd-ui-integration`

**Aviso de delta destructivo**: los tres requerimientos siguientes, archivados el 2026-09-18 como parte de INC-19, se retiran en su totalidad junto con `internal/dashboard/odd_service.go` y `internal/tui/screens/odd_features.go`, así como la entrada de ODD en `internal/tui/screens/governance.go` y su enrutamiento en `internal/tui/router.go` (INC-20 F6.2b, F6.2c). Corresponde a la fase de archivado de INC-20 decidir si este fichero se elimina por completo o se conserva vacío con esta misma nota de retirada.

## REMOVED Requirements

### Requirement: Exposición del Estado ODD en el Dashboard Web (REQ-19.13)

(Motivo: `internal/dashboard/odd_service.go`, que exponía el estado ODD vía API REST local, se retira por la Decisión D1.)
(Migración: ninguna. El Dashboard Web deja de mostrar el estado de los documentos vivos ODD.)

### Requirement: Exposición del Estado ODD en la TUI (REQ-19.14)

(Motivo: `internal/tui/screens/odd_features.go` se retira por la Decisión D1.)
(Migración: ninguna. El progreso ODD deja de ser visible desde la TUI; solo es legible leyendo directamente el contenido Markdown del documento vivo.)

### Requirement: Conmutación Visible entre Carril ODD y Carril SDD (REQ-19.15)

(Motivo: la acción de conmutación de carril y el salto directo desde un documento promovido hacia su cambio SDD dependían de `internal/dashboard/odd_service.go` y de la entrada de ODD en `internal/tui/screens/governance.go`, retirados por la Decisión D1.)
(Migración: ninguna. No existe una conmutación de carril visible equivalente en el Dashboard Web ni en la TUI tras esta retirada; el carril SDD sigue siendo accesible por sus propias pantallas y endpoints, sin cambios por este incremento.)

---
