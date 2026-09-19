<!-- Especificación Viva generada a partir de '2026-09-16-inc-13-sdd-commands-axiom-cli-integration' -->

# Especificación de Requerimientos: Integración de Comandos SDD en la CLI axiom (INC-13)

## Propósito

Definir de forma exhaustiva los requerimientos y escenarios de prueba para la exposición nativa de los comandos de ciclo de vida SDD bajo la CLI unificada `axiom` y la actualización de los contratos de invocación en los prompts de agentes.

---

## 1. Capacidad: `axiom-sdd-cli-integration`

Exposición de las operaciones de estado, despacho y presupuesto SDD directamente en el ejecutable `axiom`.

### Requirement: Subcomando axiom sdd status (REQ-13.1)
El binario `axiom` DEBE soportar la invocación `axiom sdd status [change] [flags]` (y su alias `axiom sdd-status`), retornando el informe de estado estructurado de la fase actual con idéntica fidelidad de contrato JSON que el motor v2.

#### Scenario: Consulta de estado mediante axiom sdd status
- **DADO** un repositorio con un cambio SDD activo en `openspec/changes/<change>`
- **CUANDO** el usuario o agente ejecuta `axiom sdd status <change> --json`
- **ENTONCES** la salida estándar emite el JSON del estado del cambio con código de salida `0`
- **Y** los campos `activeChange`, `artifactStatuses` y `nextRecommended` se resuelven correctamente

### Requirement: Subcomando axiom sdd continue (REQ-13.2)
El binario `axiom` DEBE soportar `axiom sdd continue [change]` para ejecutar el cálculo del despachador y emitir la siguiente acción autorizada del ciclo.

#### Scenario: Enrutamiento mediante axiom sdd continue
- **DADO** un cambio con fase de diseño completada y tareas pendientes
- **CUANDO** se ejecuta `axiom sdd continue <change>`
- **ENTONCES** la salida emite la instrucción correspondiente a la siguiente fase autorizada (`sdd-tasks` o `sdd-apply`)

### Requirement: Subcomando axiom sdd attempt tras la retirada de la gobernanza de presupuesto (REQ-13.3)
El binario `axiom` NO DEBE soportar las operaciones `acquire` y `settle` del libro mayor de intentos de ejecución bajo `axiom sdd attempt`, siguiendo la retirada de la gobernanza de *attempts* absorbida de upstream (commit `18fa04fb`). El binario `axiom` DEBE conservar sin cambio de comportamiento la operación `axiom sdd attempt grant`, que registra la autoridad de edición por raíz (`--root`) para el cambio activo.

(Previamente: el binario `axiom` DEBÍA soportar las operaciones `acquire` y `settle` del libro mayor de intentos de ejecución, `axiom sdd attempt acquire` y `axiom sdd attempt settle`.)

#### Scenario: acquire y settle dejan de estar disponibles
- **DADO** un cambio en fase `sdd-apply` tras absorber la tanda F4
- **CUANDO** se invoca `axiom sdd attempt acquire --change <cambio> ...` o `axiom sdd attempt settle --token <token> ...`
- **ENTONCES** el binario rechaza la operación como no reconocida
- **Y** no retorna ningún token de sesión ni registra ningún resultado de presupuesto

#### Scenario: grant sigue emitiendo autoridad de edición sin cambios
- **DADO** un cambio activo con raíces de edición pendientes de autorizar
- **CUANDO** se invoca `axiom sdd attempt grant --root <ruta> --change-instance <token> --request-id <id> --actor <actor> --reason <motivo>`
- **ENTONCES** el sistema registra la autoridad de edición para esa raíz
- **Y** el comportamiento de `grant` no cambia respecto al contrato vigente antes de esta absorción

---

## 2. Capacidad: `orchestrator-prompt-call-alignment`

Instrucciones de los agentes configuradas para utilizar la CLI `axiom`.

### Requirement: Referencia exclusiva a la CLI axiom en prompts (REQ-13.4)
Los prompts de orquestadores, comandos slash y guías de agentes NO DEBEN instruir a ejecutar `gentle-ai`, sino que deben prescribir la sintaxis `axiom sdd ...` (o `axiom sdd-*`).

#### Scenario: Verificación de sintaxis de comandos en sdd-orchestrator-sections.md
- **DADO** la sección `Native SDD Dispatcher Guard` del activo de orquestación
- **CUANDO** se evalúa el comando de inspección prescrito
- **ENTONCES** el comando inicia con `axiom sdd status` (o `axiom sdd-status`)
- **Y** no contiene ninguna mención a `gentle-ai sdd-status`

#### Scenario: Comandos slash de OpenCode apuntando a axiom
- **DADO** el comando `/sdd-status` en OpenCode
- **CUANDO** el agente ejecuta la tarea descrita en el archivo markdown del comando
- **ENTONCES** invoca `axiom sdd status` con su herramienta bash
