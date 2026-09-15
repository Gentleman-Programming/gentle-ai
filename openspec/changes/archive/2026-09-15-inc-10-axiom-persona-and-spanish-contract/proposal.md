# Propuesta: Nueva Persona Axiom, Contrato Lingüístico en Español Peninsular y Desactivación del Voseo (INC-10)

## Propósito (Intent)

El modelo de comportamiento de agentes heredado de Gentle AI impone actualmente dos restricciones que contradicen frontalmente los principios fundacionales de Axiom:
1. **Voseo rioplatense obligatorio en conversación:** La persona predeterminada (`PersonaGentleman`) prescribe taxativamente: *"When replying to the user in Spanish, use warm natural Rioplatense Spanish (voseo)"*, forzando construcciones como "tenés", "hacé", "mirá", "fijate", "dale", lo cual resulta artificial e incómodo para usuarios que trabajan en castellano de España o estándar profesional.
2. **Imposición de inglés en artefactos técnicos:** El contrato compartido de los orquestadores (`sdd-orchestrator-sections.md` y `Language Domain Contract`) establece: *"Generated technical artifacts default to English regardless of the active persona or conversation language"*. Esto choca directamente con la **Regla Suprema** de Axiom (`GEMINI.md` y `AGENTS.md`): *"TODO EN ESPAÑOL: Toda la comunicación, explicaciones y artefactos de SDD (proposal.md, spec.md, design.md, tasks.md, verify-report.md, archive-report.md) DEBEN generarse y redactarse estrictamente en español (castellano)"*.
3. **Ausencia de una persona propia de Axiom:** Actualmente solo se ofrecen `gentleman` (voseo rioplatense) y `neutral` (sin identidad). Falta una persona institucional representativa de la filosofía de ingeniería de Axiom.

El **Incremento 10 (INC-10: `axiom-persona-and-spanish-contract`)** introduce la persona canónica de **Axiom**, establece el **castellano peninsular de España** (tuteo técnico riguroso, sobrio y constructivo) como dialecto nativo, y armoniza el contrato lingüístico de los orquestadores para que los artefactos SDD se redacten por defecto en español castellano.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Creación de la Persona `axiom` (`PersonaAxiom`):**
   - Incorporar `model.PersonaAxiom = "axiom"` en `internal/model/types.go`.
   - Definir los activos de prompt correspondientes:
     - `internal/assets/generic/persona-axiom.md`
     - `internal/assets/claude/persona-axiom.md`
     - `internal/assets/opencode/persona-axiom.md`
     - `internal/assets/kiro/persona-axiom.md`
     - `internal/assets/hermes/persona-axiom.md`
     - `internal/assets/claude/output-style-axiom.md`
   - **Tono e Identidad:**
     - Arquitecto de sistemas principal, riguroso, pedagógico y centrado en la calidad de diseño.
     - Dialecto español: **Castellano de España (peninsular)** con tuteo profesional ("tienes", "haz", "revisa", "analiza", "fíjate"), sin voseo rioplatense ni jerga regional informal.
     - Enfoque directo: validación antes de afirmación, conceptos antes de código, pruebas deterministas antes de entrega.

2. **Persona Axiom como Predeterminada en Presets:**
   - Actualizar `internal/model/presets.go` y la TUI (`internal/tui/screens/persona.go`) para que `PersonaAxiom` sea la opción seleccionada por defecto en lugar de `PersonaGentleman`.

3. **Reforma del Contrato Lingüístico de Artefactos SDD:**
   - Modificar la sección `Language Domain Contract` en `internal/assets/skills/_shared/sdd-orchestrator-sections.md` y `generic/sdd-orchestrator.md`:
     - Establecer que para proyectos Axiom o con reglas de idioma castellano declaradas, **todos los artefactos de SDD** (`proposal.md`, `spec.md`, `design.md`, `tasks.md`, informes de verificación y archivo) se generen obligatoriamente en **español (castellano)**.
     - Preservar el idioma inglés únicamente para los identificadores de código (nombres de tipos, funciones, structs, variables y palabras clave del lenguaje de programación).

4. **Desactivación y Desacoplamiento del Voseo:**
   - La persona `gentleman` puede permanecer como opción secundaria o archivada para compatibilidad, pero deja de ser la identidad rectora del sistema.

### Fuera de Alcance (Out of Scope)

- Renombrado del orquestador (`gentle-orchestrator` -> `axiom-orchestrator`), abordado en INC-11.
- Modificación de comandos CLI ni de la estructura de carpetas `openspec/`.

---

## Plan de Pruebas y Validación

- Pruebas unitarias de selección de persona en `internal/components/persona/` verificando que `PersonaAxiom` se resuelve correctamente y no contiene referencias a `voseo` ni `Rioplatense`.
- Pruebas de contrato de lenguaje en `internal/assets/language_contract_test.go` verificando las reglas de español peninsular para la persona Axiom y español para los artefactos SDD.
