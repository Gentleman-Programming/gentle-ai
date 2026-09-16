# Especificación de Requerimientos: Orquestación Interactiva SDD y Creación de Cambios en Dashboard Web y CLI (INC-15)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales, no funcionales y escenarios BDD para la orquestación interactiva del ciclo de vida SDD desde el Dashboard Web (`axiom ui`) y la CLI canónica `axiom`, incluyendo la creación de cambios, avance de fases, validación de verificación formal y compositor de handoffs estructurados.

---

## 1. Capacidad: `dashboard-sdd-orchestration`

Permite a desarrolladores y agentes gobernar el flujo de trabajo Spec-Driven Development de forma interactiva y visual desde el servidor local embebido.

### Requirement: Creación de Incrementos y Andamiaje SDD vía API (REQ-15.1)
El servidor HTTP local DEBE exponer el endpoint `POST /api/increments` para recibir peticiones de creación de un nuevo incremento. La petición DEBE incluir el nombre del cambio (`name`, en formato kebab-case), el propósito o intención (`intent`) y opcionalmente el tipo de cambio (`type`). El servidor DEBE validar que el nombre no contenga caracteres inválidos ni coincida con un cambio existente en `openspec/changes/` o `openspec/changes/archive/`. Al validarse, el servidor DEBE crear el directorio del cambio y generar un archivo `proposal.md` canónico redactado en español castellano.

#### Scenario: Creación exitosa de un nuevo incremento con plantilla en español
- **DADO** el servidor HTTP del Dashboard Web activo en un workspace Axiom
- **CUANDO** se envía una petición `POST /api/increments` con el cuerpo JSON:
  ```json
  {
    "name": "nuevo-modulo-auth",
    "intent": "Implementar autenticación basada en JWT con rotación atómica de tokens",
    "type": "feature"
  }
  ```
- **ENTONCES** el servidor responde con código de estado HTTP `201 Created`
- **Y** la respuesta JSON contiene `"success": true` y `"name": "nuevo-modulo-auth"`
- **Y** se crea el directorio `openspec/changes/nuevo-modulo-auth/`
- **Y** se genera el archivo `openspec/changes/nuevo-modulo-auth/proposal.md` conteniendo el título en español, el propósito y las secciones canónicas de alcance y capacidades

#### Scenario: Rechazo de creación ante nombre inválido o colisión
- **DADO** un workspace con un incremento existente denominado `auth-core`
- **CUANDO** se envía una petición `POST /api/increments` con `"name": "auth-core"` o `"name": "Nombre Con Espacios!"`
- **ENTONCES** el servidor responde con código de estado HTTP `400 Bad Request`
- **Y** la respuesta JSON contiene un mensaje de error descriptivo en `"error"`
- **Y** no se altera el sistema de archivos

---

### Requirement: Ejecución y Avance de Fase SDD vía API (REQ-15.2)
El servidor HTTP local DEBE exponer el endpoint `POST /api/increments/continue` para disparar la siguiente transición del ciclo de vida SDD sobre el incremento especificado en el cuerpo de la petición (`{ "name": "<cambio>" }`). El servidor DEBE invocar el motor de despacho SDD de Axiom (`RunSDDContinue`), capturar el veredicto o salida de acción autorizada y retornarlo al cliente en formato estructurado.

#### Scenario: Avance de fase exitoso con captura de acción autorizada
- **DADO** un incremento activo en el workspace
- **CUANDO** se envía una petición `POST /api/increments/continue` con `{ "name": "<nombre-cambio>" }`
- **ENTONCES** el servidor ejecuta la transición del despachador SDD
- **Y** responde con código HTTP `200 OK`
- **Y** la respuesta JSON contiene `"success": true`, el nombre del cambio y la salida o veredicto de la siguiente acción

#### Scenario: Rechazo de avance para un cambio inexistente
- **DADO** el endpoint `POST /api/increments/continue`
- **CUANDO** se envía una petición para un cambio que no existe en `openspec/changes/`
- **ENTONCES** el servidor responde con código HTTP `404 Not Found`
- **Y** emite un error explicativo en JSON

---

### Requirement: Validación Formal de Verificación vía API (REQ-15.3)
El servidor HTTP local DEBE exponer el endpoint `POST /api/increments/verify` para validar formalmente el reporte de verificación (`verify-report.md`) de un incremento activo contra los requerimientos y escenarios BDD de las especificaciones vigentes.

#### Scenario: Validación de reporte de verificación existente
- **DADO** un incremento activo con un archivo `verify-report.md` redactado
- **CUANDO** se envía una petición `POST /api/increments/verify` con `{ "name": "<nombre-cambio>" }`
- **ENTONCES** el servidor evalúa la conformidad formal de los requerimientos y escenarios
- **Y** retorna el veredicto en la respuesta JSON (indicando si es PASS o detallando las inconsistencias)

---

### Requirement: Compositor y Registro de Handoffs Estructurados (REQ-15.4)
El servidor HTTP local DEBE exponer el endpoint `POST /api/handoffs` para recibir los datos de un relevo formal entre fases o roles concurrentes. El servidor DEBE construir un objeto `handoff.Handoff`, validar las fases y estados según el contrato de INC-02, y serializar el artefacto canónico en `openspec/changes/<cambio>/handoff.md` con Frontmatter YAML y las 5 secciones obligatorias en castellano.

#### Scenario: Registro exitoso de handoff formal desde el Dashboard Web
- **DADO** el endpoint `POST /api/handoffs`
- **CUANDO** se envía una petición con los datos del relevo:
  ```json
  {
    "change": "modulo-pagos",
    "from_phase": "design",
    "to_phase": "apply",
    "from_role": "architect",
    "to_role": "backend",
    "status": "ready",
    "executive_summary": "Arquitectura de pagos completada con Stripe y pasarela local",
    "artifacts": "openspec/changes/modulo-pagos/design.md",
    "decisions": "Uso de webhooks idempotentes",
    "risks": "Concurrencia en confirmación de transacciones",
    "instructions": "Implementar primero los controladores REST y mock de gateway"
  }
  ```
- **ENTONCES** el servidor responde con código HTTP `201 Created`
- **Y** se genera o sobrescribe `openspec/changes/modulo-pagos/handoff.md` con Frontmatter YAML canónico y los encabezados `# 1. Resumen Ejecutivo`, `# 2. Artefactos Modificados y Creados`, etc.

---

## 2. Capacidad: `cli-change-creator`

Permite inicializar y crear cambios SDD desde la línea de comandos canónica `axiom`.

### Requirement: Subcomando axiom change create en la CLI (REQ-15.5)
La CLI `axiom` DEBE proveer el subcomando `axiom change create <nombre>` (con alias plano `axiom change new <nombre>`), admitiendo banderas opcionales `--intent` (o `-i`) y `--type` (o `-t`). Al ejecutarse, DEBE validar el nombre, crear la estructura de carpetas en `openspec/changes/<nombre>/` y generar el archivo `proposal.md` canónico con plantilla en español, informando la ruta creada en la salida estándar con código de salida `0`.

#### Scenario: Creación de cambio mediante CLI con flags
- **DADO** el binario `axiom`
- **CUANDO** el usuario ejecuta `axiom change create mi-nueva-feature --intent "Añadir soporte de métricas" --type "feature"`
- **ENTONCES** el comando crea `openspec/changes/mi-nueva-feature/proposal.md`
- **Y** emite un mensaje de confirmación en stdout con código de retorno `0`

#### Scenario: Invocación de ayuda para axiom change
- **DADO** el binario `axiom`
- **CUANDO** el usuario ejecuta `axiom change --help` o `axiom change create --help`
- **ENTONCES** se describe el propósito del comando, los argumentos esperados y las banderas disponibles
