# Especificación de Requerimientos: Persona Axiom y Contrato de Idioma Español (INC-10)

## Propósito

Definir de forma verificable los requerimientos y escenarios de comportamiento para la persona `axiom`, el dialecto en castellano de España (peninsular) y el contrato de redacción de artefactos técnicos en español.

---

## 1. Capacidad: `axiom-persona-definition`

Provisión de la personalidad oficial de Axiom para agentes de IA asistida.

### Requirement: Definición del modelo PersonaAxiom (REQ-10.1)
El sistema DEBE definir el identificador de persona `model.PersonaAxiom = "axiom"` y soportarlo en todos los adaptadores y pantallas de selección TUI.

#### Scenario: Selección de persona Axiom en la TUI
- **DADO** la pantalla de selección de persona del instalador interactivo
- **CUANDO** el usuario visualiza las opciones disponibles
- **ENTONCES** la opción "Axiom" está presente
- **Y** su descripción indica "Conversación en castellano peninsular; artefactos en español"
- **Y** figura seleccionada por defecto

### Requirement: Tono e instrucciones de la persona Axiom (REQ-10.2)
El prompt de `persona-axiom.md` DEBE especificar que las respuestas en español utilizan castellano de España con tuteo profesional, prohibiendo explícitamente el voseo rioplatense forzado y la jerga regional informal.

#### Scenario: Contrato de dialecto en persona Axiom
- **DADO** el archivo `persona-axiom.md` generado para cualquier adaptador
- **CUANDO** se inspecciona la sección de lenguaje
- **ENTONCES** contiene la directiva de uso de castellano peninsular de España
- **Y** NO contiene términos como "Rioplatense", "voseo", "che", "tenés" ni "podés"

---

## 2. Capacidad: `sdd-language-domain-alignment`

Alineación del contrato lingüístico de los orquestadores con la política oficial de Axiom.

### Requirement: Generación de artefactos SDD en español (REQ-10.3)
El contrato compartido de orquestación (`Language Domain Contract`) DEBE exigir que los artefactos SDD (`proposal.md`, `spec.md`, `design.md`, `tasks.md`, `verify-report.md`, `archive-report.md`) se redacten íntegramente en español castellano.

#### Scenario: Contrato lingüístico en orquestadores SDD
- **DADO** el archivo `sdd-orchestrator-sections.md` bajo la sección `Language Domain Contract`
- **CUANDO** se evalúa la regla para proyectos Axiom
- **ENTONCES** establece que las propuestas, especificaciones, diseños y tareas deben escribirse en español (castellano)
- **Y** limita el uso del inglés a los identificadores de código fuente (nombres de funciones, variables, structs, métodos)

### Requirement: Supresión de la regla "default to English" para documentación (REQ-10.4)
El sistema NO DEBE inyectar en las instrucciones de los agentes la directiva "Generated technical artifacts default to English" cuando el entorno pertenezca a un proyecto gobernado por Axiom.

#### Scenario: Validación de ausencia de default to English incondicional
- **DADO** un proyecto con archivo `axiom.yaml` o reglas de proyecto en español
- **CUANDO** se compila el prompt del orquestador SDD
- **ENTONCES** la regla de generación de artefactos prioriza el español peninsular sobre el inglés
