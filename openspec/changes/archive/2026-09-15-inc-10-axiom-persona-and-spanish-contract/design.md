# Diseño Arquitectónico: Persona Axiom y Contrato de Idioma Español (INC-10)

## Contexto y Motivación

En Gentle AI, el sistema imponía una personalidad única ("Gentleman") basada en el voseo rioplatense argentino y una regla estricta en los orquestadores SDD que forzaba la redacción de artefactos técnicos en inglés ("Generated technical artifacts default to English").

Axiom establece como principio rector la soberanía técnica, la excelencia arquitectónica y la comunicación nativa en **castellano de España (peninsular)**, requiriendo que la documentación y los artefactos de ingeniería generados durante el ciclo SDD se escriban rigurosamente en español, reservando el idioma inglés exclusivamente para identificadores de código fuente.

Este diseño define la arquitectura técnica para materializar la persona `PersonaAxiom`, convertirla en la identidad rectora por defecto del ecosistema, proveer los assets adaptados por runtime, y armonizar el contrato de lenguaje de los orquestadores SDD.

---

## Decisiones de Diseño

### Decisión 1: Identificador de Persona y Prioridad por Defecto
- **Elección:** Definir `model.PersonaAxiom = "axiom"` en `internal/model/types.go`.
- **Comportamiento:** `PersonaAxiom` se sitúa como primera opción en `PersonaOptions()` en la TUI y como valor por defecto en `normalizePersona()` cuando no se especifica el flag `--persona`.
- **Compatibilidad:** `PersonaGentleman`, `PersonaNeutral` y `PersonaCustom` se mantienen operativas para preservar la compatibilidad retroactiva.

### Decisión 2: Tono Lingüístico y Voz de la Persona Axiom
- **Registro:** Castellano de España (peninsular) con tuteo técnico profesional ("tienes", "haz", "revisa", "fíjate", "analiza").
- **Prohibiciones Explícitas:** Queda estrictamente prohibido el voseo rioplatense ("tenés", "hacé", "mirá", "fijate", "dale", "listo" como muletilla informal) y la jerga regional informal.
- **Enfoque Pedagógico y Rigor:** Arquitecto de software principal con más de 15 años de experiencia. Principios rectores:
  - *Validación antes de afirmación:* No aceptar afirmaciones sin verificar en código o documentación.
  - *Conceptos antes de código:* Entender el problema estructural antes de escribir líneas de código.
  - *Pruebas deterministas:* Verificación exhaustiva antes de dar una tarea por finalizada.
  - *Respuestas concisas:* Máximo una pregunta a la vez, esperando la respuesta del usuario.

### Decisión 3: Contrato Lingüístico de Artefactos SDD
- **Regla:** En proyectos gobernados por Axiom (o con reglas de idioma castellano declaradas en `GEMINI.md`, `AGENTS.md` o configuración de proyecto), **todos los artefactos técnicos de SDD** (`proposal.md`, `spec.md`, `design.md`, `tasks.md`, `verify-report.md`, `archive-report.md`, `walkthrough.md`) se redactan obligatoriamente en **español (castellano)**.
- **Excepción acotada:** El idioma inglés se preserva únicamente para identificadores sintácticos de código (nombres de paquetes, funciones, structs, métodos, variables y palabras clave del lenguaje).
- **Alineación de Orquestadores:** La sección compartida `skills/_shared/sdd-orchestrator-sections.md` bajo `Language Domain Contract` se actualiza para explicitar esta jerarquía: en proyectos Axiom rige el español para artefactos; en proyectos genéricos se mantiene la convención del proyecto.

### Decisión 4: Arquitectura de Assets y Output Styles
- Se crean los assets de prompt especializados:
  - `internal/assets/generic/persona-axiom.md`: Plantilla universal con la voz y reglas de Axiom.
  - `internal/assets/claude/persona-axiom.md`: Versión ligera para Claude Code.
  - `internal/assets/claude/output-style-axiom.md`: Output style formal (`axiom.md`) para Claude Code.
  - `internal/assets/opencode/persona-axiom.md`: Asset para OpenCode y Kilocode.
  - `internal/assets/kiro/persona-axiom.md`: Asset adaptado para Kiro IDE.
  - `internal/assets/hermes/persona-axiom.md`: Asset para Hermes Agent con directivas de doble memoria (Engram + Hermes native).
  - `internal/assets/kimi/output-style-axiom.md`: Output style para Kimi IDE.
- El plan de recursos (`ResourcePlanFor` en `internal/components/persona/resources.go`) gestiona la escritura de `axiom.md` y la retirada limpia de estilos previos (`gentleman.md` y `neutral.md`).

---

## Diagrama de Flujo de Selección e Inyección

```
+---------------------------------------------------------+
|                  Entrada de Usuario                     |
|      (TUI, flag --persona=axiom, o por defecto)         |
+---------------------------------------------------------+
                            |
                            v
+---------------------------------------------------------+
|              internal/cli/validate.go                   |
|               normalizePersona() -> axiom               |
+---------------------------------------------------------+
                            |
                            v
+---------------------------------------------------------+
|             internal/components/persona                 |
|  - ResourcePlanFor(PersonaAxiom)                        |
|    -> Write: ~/.claude/output-styles/axiom.md           |
|    -> Retire: gentleman.md, neutral.md                  |
|  - personaContent(adapter, PersonaAxiom)                |
|    -> Lee internal/assets/<adapter>/persona-axiom.md    |
+---------------------------------------------------------+
                            |
                            v
+---------------------------------------------------------+
|         Entrega Atómica en Adaptadores de Agentes       |
|    (Claude, OpenCode, Kiro, Hermes, Kimi, etc.)         |
+---------------------------------------------------------+
```

---

## Riesgos y Mitigaciones

1. **Ruptura de Golden Tests existentes:**
   - *Riesgo:* Al actualizar la sección compartida `Language Domain Contract`, los archivos golden de SDD diferirán en el diff.
   - *Mitigación:* Identificar todos los tests que comparan con `.golden` y actualizar sistemáticamente las referencias a la sección compartida.
2. **Compatibilidad con instalaciones previas:**
   - *Riesgo:* Instalaciones con `state.json` conteniendo `"persona": "gentleman"` o `"neutral"`.
   - *Mitigación:* Se preservan íntegramente las rutas de resolución para `gentleman` y `neutral`, permitiendo una coexistencia limpia.
