# Especificación Viva: Handoffs Estructurados y Ciclo de Vida de Transición

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y escenarios BDD para el artefacto canónico `handoff.md`, el paquete de dominio `internal/handoff` y los subcomandos de la CLI `axiom handoff` en el runtime de Axiom.

---

## 1. Capacidad: `structured-handoff-engine`

El paquete `internal/handoff` provee el modelo de datos, la deserialización/serialización y la validación semántica de los relevos de fase en el ciclo de vida de SDD.

### Requirement: Esquema y Formato Canónico del Handoff (REQ-1.1)

El artefacto `handoff.md` DEBE componerse de un bloque de metadatos frontal en formato YAML delimitado por `---` seguido obligatoriamente por cinco secciones descriptivas en español.

Los campos del Frontmatter YAML requeridos son:
- `change`: nombre del cambio activo.
- `from_phase`: fase emisora (`explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `verify`).
- `to_phase`: fase receptora (`propose`, `spec`, `design`, `tasks`, `apply`, `verify`, `archive`).
- `from_role`: identificador del rol emisor.
- `to_role`: identificador del rol receptor.
- `timestamp`: fecha y hora en formato ISO 8601 UTC.
- `status`: estado del relevo (`ready`, `blocked`, `needs_clarification`).

Las secciones Markdown requeridas son:
1. `## 1. Resumen Ejecutivo`
2. `## 2. Artefactos Modificados y Creados`
3. `## 3. Decisiones Técnicas y Acuerdos`
4. `## 4. Riesgos, Bloqueos y Preguntas Abiertas`
5. `## 5. Instrucciones Directas para el Siguiente Rol`

#### Scenario: Handoff canónico válido y completo
- **DADO** un archivo `handoff.md` con frontmatter YAML que contiene todos los campos obligatorios
- **Y** que contiene las cinco secciones markdown requeridas no vacías
- **CUANDO** el parser analiza el archivo
- **ENTONCES** se genera una estructura `Handoff` válida en memoria
- **Y** no se reporta ningún error de esquema

#### Scenario: Handoff inválido por metadatos faltantes en frontmatter
- **DADO** un archivo `handoff.md` al que le falta el campo `to_role` o `status` en el frontmatter
- **CUANDO** el parser intenta deserializarlo
- **ENTONCES** la operación retorna un error tipificado indicando el campo faltante

#### Scenario: Handoff inválido por sección markdown ausente
- **DADO** un archivo `handoff.md` que carece de la sección `## 3. Decisiones Técnicas y Acuerdos`
- **CUANDO** se ejecuta la validación del documento
- **ENTONCES** se retorna un error indicando que la sección obligatoria no está presente

---

### Requirement: Parser y Serializador Bidireccional (REQ-1.2)

El paquete `internal/handoff` DEBE ser capaz de leer un archivo o flujo `io.Reader` con formato markdown/YAML y convertirlo a una estructura Go tipificada (`Handoff`), así como serializar dicha estructura a un documento markdown idéntico y canónico (*round-trip* sin pérdida de información).

#### Scenario: Lectura y escritura isomórfica (Round-trip)
- **DADO** una estructura `Handoff` en Go con metadatos y contenido en las cinco secciones
- **CUANDO** se invoca `Format(h)` y posteriormente `Parse(bytes)`
- **ENTONCES** el objeto resultante es equivalente al original en todos sus campos de metadatos y secciones

---

### Requirement: Motor de Validación Semántica de Transiciones (REQ-1.3)

El validador de `internal/handoff` DEBE verificar que la transición entre fases declarada en el handoff sea coherente con la máquina de estados de SDD y con la topología de workspace declarada en `axiom.yaml`.

Las reglas de transición válidas hacia adelante son:
- `explore` ➔ `propose`
- `propose` ➔ `spec`
- `spec` ➔ `design`
- `design` ➔ `tasks`
- `tasks` ➔ `apply`
- `apply` ➔ `verify`
- `verify` ➔ `archive`

Se admiten transiciones de retroceso explícito por remediación únicamente si `status` es `blocked` o `needs_clarification`:
- `verify` ➔ `apply` (remediación de pruebas fallidas)
- `verify` ➔ `tasks` (remediación de cobertura o desglose)
- `apply` ➔ `design` (bloqueo arquitectónico imprevisto)

#### Scenario: Transición válida hacia la siguiente fase
- **DADO** un handoff con `from_phase: "design"`, `to_phase: "tasks"` y `status: "ready"`
- **CUANDO** se valida la transición
- **ENTONCES** la regla aprueba la transición como válida

#### Scenario: Rechazo de salto de fase no permitido
- **DADO** un handoff que intenta transicionar de `propose` a `apply` directamente
- **CUANDO** se ejecuta la validación
- **ENTONCES** se rechaza la transición indicando que omite las fases obligatorias `spec`, `design` y `tasks`

#### Scenario: Validación de existencia de roles según axiom.yaml
- **DADO** un handoff que declara `to_role: "mobile-lead"`
- **Y** una configuración `axiom.yaml` que solo contiene los roles `core` y `qa`
- **CUANDO** se valida el handoff en el contexto del espacio de trabajo
- **ENTONCES** se reporta un error indicando que el rol destinatario no está declarado en la topología

---

### Requirement: Formato de Espejo para Engram MCP (REQ-1.4)

El paquete `internal/handoff` DEBE proporcionar la función `ToEngramObservation(h)` que serializa el relevo en el payload exacto esperado para registrarlo en Engram bajo la clave `topic_key: sdd/{change}/handoff`, con tipo `architecture` y título normalizado.

#### Scenario: Generación de payload para Engram
- **DADO** un handoff validado para el cambio `auth-jwt`
- **CUANDO** se llama a `ToEngramPayload(h)`
- **ENTONCES** el payload contiene `topic_key: "sdd/auth-jwt/handoff"`
- **Y** el contenido resume claramente la transición de fase, roles y próximas instrucciones

---

## 2. Capacidad: `axiom-cli-handoff`

La CLI `axiom` proporciona comandos dedicados para gestionar, inspeccionar y validar los relevos de manera interactiva o automatizada.

### Requirement: Subcomando axiom handoff show (REQ-2.1)

El comando `axiom handoff show [--change <nombre>]` DEBE mostrar en terminal un resumen formateado del relevo activo del cambio, incluyendo estado, roles, fases y la última sección de instrucciones directas.

#### Scenario: Visualización de handoff existente
- **DADO** que existe un archivo `openspec/changes/auth-jwt/handoff.md`
- **CUANDO** el usuario ejecuta `axiom handoff show --change auth-jwt`
- **ENTONCES** la terminal imprime el estado, origen, destino y resumen del relevo
- **Y** el código de salida es `0`

#### Scenario: Error cuando no existe handoff
- **DADO** un cambio sin handoff generado
- **CUANDO** se ejecuta `axiom handoff show --change cambio-inexistente`
- **ENTONCES** la salida notifica que no existe relevo registrado para ese cambio
- **Y** el código de salida es `1`

---

### Requirement: Subcomando axiom handoff create (REQ-2.2)

El comando `axiom handoff create` DEBE generar una plantilla canónica de `handoff.md` con los metadatos suministrados por argumentos de línea de comandos.

Parámetros requeridos:
- `--change`: identificador del cambio.
- `--from`: fase de origen.
- `--to`: fase de destino.
- `--from-role`: rol que entrega.
- `--to-role`: rol que recibe.
- `--status`: (opcional, por defecto `ready`).

#### Scenario: Creación exitosa de plantilla de relevo
- **DADO** los parámetros de cambio y fases válidas
- **CUANDO** se ejecuta `axiom handoff create --change test-feature --from spec --to design --from-role tech-lead --to-role architect`
- **ENTONCES** se crea el archivo `openspec/changes/test-feature/handoff.md`
- **Y** contiene el frontmatter inicializado con los metadatos y las plantillas de las cinco secciones
- **Y** el código de salida es `0`

---

### Requirement: Subcomando axiom handoff validate (REQ-2.3)

El comando `axiom handoff validate [--change <nombre>]` DEBE analizar el archivo `handoff.md` del cambio activo y verificar el cumplimiento de todas las reglas del esquema y de transición de fases.

#### Scenario: Validación exitosa de handoff
- **DADO** un handoff bien estructurado y con transición legítima
- **CUANDO** se ejecuta `axiom handoff validate --change test-feature`
- **ENTONCES** la salida reporta `HANDOFF VALID: ready`
- **Y** el código de salida es `0`

#### Scenario: Validación fallida de handoff incompleto
- **DADO** un handoff al que le falta una sección obligatoria o con transición inválida
- **CUANDO** se ejecuta `axiom handoff validate --change test-feature`
- **ENTONCES** la salida reporta `HANDOFF INVALID` detallando el error específico
- **Y** el código de salida es `1`
