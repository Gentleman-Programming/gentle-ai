<!-- Especificación Viva generada a partir de '2026-09-18-inc-19-odd-workflow-and-promotion' -->

# Especificación de Requerimientos: Comandos CLI para ODD (INC-19)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para la exposición del carril ágil de Organic Driven Development (ODD) a través de la línea de comandos canónica de Axiom: `axiom odd create`, `axiom odd status` y `axiom odd promote`, replicando el patrón manual ya empleado por `axiom sdd` y `axiom change`, con salida en texto y JSON, gestión de espejos en Engram y presencia explícita en la ayuda de la CLI.

---

## 1. Capacidad Modificada (Retirada Destructiva): `odd-cli-commands`

**Aviso de delta destructivo**: los cuatro requerimientos siguientes, archivados el 2026-09-18 como parte de INC-19, se retiran en su totalidad junto con `internal/cli/odd_create.go`, `internal/cli/odd_status.go`, `internal/cli/odd_promote.go` y el despacho `case "odd":` de `cmd/axiom/main.go` (INC-20 F6.2a). Tras esta retirada, el grupo de comandos `axiom odd` deja de existir; corresponde a la fase de archivado de INC-20 decidir si este fichero se elimina por completo o se conserva vacío con esta misma nota de retirada.

## REMOVED Requirements

### Requirement: Subcomando axiom odd create (REQ-19.5)

(Motivo: `internal/cli/odd_create.go` y el despacho `case "odd":` que lo invocaba desde `cmd/axiom/main.go` se retiran por la Decisión D1.)
(Migración: ninguna. La creación de un documento vivo ODD pasa a ser un acto de agente con herramientas de fichero, sin subcomando de CLI.)

### Requirement: Subcomando axiom odd status con Salida en Texto y JSON (REQ-19.6)

(Motivo: `internal/cli/odd_status.go` se retira por la Decisión D1.)
(Migración: ninguna. La consulta de progreso ODD pasa a leerse directamente del contenido Markdown del documento vivo, sin salida `--json` estructurada.)

### Requirement: Bandera --check-mirror en axiom odd status (REQ-19.7)

(Motivo: la bandera `--check-mirror` de `internal/cli/odd_status.go` se retira junto con el subcomando completo.)
(Migración: ninguna. No existe una comprobación de divergencia equivalente tras esta retirada.)

### Requirement: Subcomando axiom odd promote: Banderas y Presencia en la Ayuda (REQ-19.8)

(Motivo: `internal/cli/odd_promote.go` y la entrada del grupo `axiom odd` en `printHelp()` de `cmd/axiom/main.go` se retiran por la Decisión D1.)
(Migración: ver el delta de la capacidad `odd-sdd-promotion` para el reemplazo funcional de la promoción.)

---
