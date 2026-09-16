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

### Requirement: Subcomando axiom sdd attempt (REQ-13.3)
El binario `axiom` DEBE soportar las operaciones `acquire` y `settle` del libro mayor de intentos de ejecución (`axiom sdd attempt acquire` y `axiom sdd attempt settle`).

#### Scenario: Reserva y liquidación de presupuesto de ejecución
- **DADO** un cambio en fase `sdd-apply`
- **CUANDO** se invoca `axiom sdd attempt acquire --change <change> ...`
- **ENTONCES** retorna el token de sesión con estado `proceed`
- **Y** tras la ejecución, `axiom sdd attempt settle --token <token> ...` registra el resultado de forma determinista

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
