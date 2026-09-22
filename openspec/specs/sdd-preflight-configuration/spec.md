<!-- Especificación Viva generada a partir de '2026-09-22-inc-21-upfront-flow-governance' -->

# Especificación de Requerimientos: Cuestionario Pre-Vuelo de SDD, Modalidad de Avance y Asignación Obligatoria de Roles (INC-21)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y escenarios BDD para el cuestionario de pre-vuelo que sella, antes de crear la propuesta SDD, la modalidad de avance del cambio (continua o con paradas y reviews por bloque), la política de relevos (sin o con handoffs intermedios) y la asignación de roles, incluyendo la asignación obligatoria del rol `fullstack` cuando no existe subdivisión de roles especializados.

---

## 1. Capacidad: `sdd-preflight-configuration`

El cuestionario de pre-vuelo sella, antes de crear la propuesta, la modalidad de avance del cambio SDD, su política de relevos y la asignación de roles.

### Requirement: Bloqueo de Avance sin Modalidad y Política de Relevos Selladas (REQ-21.5)

Al entrar en el carril SDD, el orquestador DEBE bloquear la creación de `proposal.md` hasta que se definan explícitamente: (1) la modalidad de avance (`continuo` o `con paradas y reviews por bloque`), y (2) la política de relevos (`sin handoffs intermedios` o `con handoff.md en cada parada`). El orquestador DEBE persistir ambas decisiones de forma sellada para el cambio activo y NO DEBE volver a preguntarlas mientras el cambio permanezca activo, salvo lo indicado en REQ-21.4.

#### Scenario: Modalidad y política de relevos selladas antes de crear la propuesta

- **DADO** que el usuario ha elegido el carril SDD
- **CUANDO** responde explícitamente la modalidad de avance y la política de relevos
- **ENTONCES** el orquestador sella ambas decisiones para el cambio
- **Y** procede a crear `proposal.md`

#### Scenario: Respuesta ambigua o ausente no crea la propuesta ni asume un valor por defecto

- **DADO** que el usuario no responde con una modalidad o una política de relevos reconocibles entre las opciones válidas
- **CUANDO** el orquestador evalúa la respuesta
- **ENTONCES** no crea `proposal.md`
- **Y** vuelve a presentar la pregunta pendiente hasta obtener una respuesta válida, sin asumir un valor por defecto no confirmado

---

### Requirement: Asignación Obligatoria del Rol `fullstack` sin Subdivisión Especializada (REQ-21.6)

Si, al concluir el cuestionario de pre-vuelo, ni el usuario ni el diseño previsto subdividen el trabajo en roles especializados (por ejemplo `core`, `web`, `qa`), el orquestador DEBE asignar obligatoriamente el rol único `fullstack`, con política de compuerta `blocking`, fichero de tareas `tasks.md` y fichero de verificación `verify-report.md`. El orquestador NO DEBE dejar un cambio SDD sin al menos un rol informado en su configuración de kickoff.

#### Scenario: Sin roles declarados, se asigna fullstack por defecto

- **DADO** un cuestionario de pre-vuelo en el que el usuario no declara ningún rol especializado
- **CUANDO** concluye el cuestionario
- **ENTONCES** el orquestador asigna el rol único `fullstack` con política `blocking`, `tasks.md` y `verify-report.md`
- **Y** el cambio queda con exactamente un rol informado

#### Scenario: Un único rol especializado declarado no se combina con fullstack

- **DADO** un cuestionario de pre-vuelo en el que el usuario declara exactamente un rol especializado (por ejemplo, únicamente `core`)
- **CUANDO** concluye el cuestionario
- **ENTONCES** el orquestador respeta ese único rol declarado como el rol activo del cambio
- **Y** no asigna adicionalmente el rol `fullstack` junto al rol ya declarado

#### Scenario: Varios roles especializados declarados tampoco activan fullstack

- **DADO** un cuestionario de pre-vuelo en el que el usuario declara varios roles especializados (por ejemplo `core`, `web` y `qa`)
- **CUANDO** concluye el cuestionario
- **ENTONCES** el orquestador no asigna el rol `fullstack`
- **Y** cada rol declarado conserva su propia política de compuerta
