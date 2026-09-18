<!-- Especificación Viva generada a partir de '2026-09-18-inc-19-odd-workflow-and-promotion' -->

# Especificación de Requerimientos: Integración de ODD en Dashboard Web y TUI (INC-19)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para la exposición reactiva del estado del carril ágil de Organic Driven Development (ODD) en el Dashboard Web (`axiom ui`) y en la TUI interactiva de Axiom, con paridad de información entre ambas superficies, conmutación explícita y visible entre carril ODD y carril formal (SDD), y navegación directa desde documentos promovidos hacia sus cambios SDD correspondientes.

---

## 1. Capacidad: `odd-ui-integration`

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
