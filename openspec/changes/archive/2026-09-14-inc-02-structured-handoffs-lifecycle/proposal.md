# Propuesta: Handoffs Estructurados y Ciclo de Vida de Transición (INC-02)

## Propósito (Intent)

En equipos de ingeniería reales o en sistemas con múltiples agentes de IA especializados, una persona o agente finaliza una fase del ciclo SDD (por ejemplo, el diseño técnico) y otra persona o agente diferente asume la siguiente (como la implementación frontend o backend). 

Actualmente, este traspaso se realiza de forma implícita o dependiendo de la memoria volátil de la conversación, lo que provoca pérdida de contexto, asunciones no validadas o necesidad de releer manualmente cientos de líneas de documentación.

El objetivo de este incremento es dotar a Axiom de un mecanismo formal y determinista de **relevo continuo**:
1. El artefacto canónico `handoff.md` dentro de la carpeta del cambio activo.
2. Un modelo de datos desacoplado en Go (`internal/handoff`) que parsee, valide y gestione los traspasos entre roles y fases.
3. Subcomandos CLI en `axiom`: `axiom handoff show`, `axiom handoff create` y `axiom handoff validate`.
4. Espejo y sincronización automática con **Engram MCP** bajo la clave `topic_key: sdd/<cambio>/handoff`, permitiendo que cualquier agente que despierte en una nueva sesión retome el trabajo con contexto completo al instante.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)
- **Esquema del artefacto `handoff.md`:**
  - Encabezado con metadatos estructurados (Frontmatter YAML o bloque de propiedades): cambio, fase origen, fase destino, rol emisor, rol receptor, timestamp y estado (`ready`, `blocked`, `needs_clarification`).
  - Secciones canónicas en español:
    1. Resumen del trabajo realizado.
    2. Artefactos creados o modificados con sus rutas relativas.
    3. Decisiones técnicas y de negocio tomadas.
    4. Riesgos, bloqueos y preguntas abiertas.
    5. Instrucciones directas y siguiente acción esperada para el rol receptor.
- **Paquete de dominio en el runtime de Axiom (`internal/handoff/`):**
  - `types.go`: Modelos Go para representar el handoff, metadatos, artefactos involucrados y estados de transición.
  - `parser.go` y `writer.go`: Serialización y deserialización bidireccional entre el archivo `handoff.md` y la estructura Go.
  - `validator.go`: Reglas de validación semántica del traspaso (verificación de que la transición de fases sea válida según el ciclo SDD, que los roles existan en `axiom.yaml` y que los artefactos referenciados existan en disco).
  - `mirror.go`: Exportador de payload formateado para registro en Engram MCP o almacenamiento local persistente.
- **Integración CLI en el binario de Axiom (`cmd/axiom/main.go`):**
  - `axiom handoff show [--change <nombre>]`: Muestra en terminal el estado del último relevo del cambio indicado.
  - `axiom handoff create --change <nombre> --from <fase> --to <fase> --from-role <rol> --to-role <rol>`: Generador asistido del archivo `handoff.md`.
  - `axiom handoff validate [--change <nombre>]`: Valida la integridad del handoff antes de autorizar el avance a la siguiente fase.
- **Suite de pruebas unitarias exhaustiva:** Pruebas en `internal/handoff/...` cubriendo creación, parseo, validación y transiciones válidas e inválidas.

### Fuera de Alcance (Out of Scope)
- Modificaciones a la capa de instalación o al arnés metodológico de Gentle AI utilizado para gobernar este repositorio (se mantiene desacoplado e inmutable).
- Ejecución paralela de múltiples roles simultáneos en `Tasks`/`Apply` (fan-out) — objeto del **INC-03**.
- Renderizado visual en dashboard web local (`axiom ui`) — objeto del **INC-04**.
- Notificaciones externas a Slack, Teams o Webhooks.

---

## Capacidades (Capabilities)

### Nuevas Capacidades
- `structured-handoff-engine`: Motor en `internal/handoff` para parsear, escribir y validar artefactos de relevo `handoff.md`.
- `axiom-cli-handoff`: Subcomandos CLI `axiom handoff [show|create|validate]` para inspeccionar y gobernar los traspasos de fase.

### Capacidades Modificadas
- `workspace-topology-engine`: Interoperabilidad para validar que los roles emisores y receptores declarados en un handoff existan en la configuración `axiom.yaml`.

---

## Enfoque de Implementación (Approach)

1. **Definición del contrato de datos (`internal/handoff/types.go`):**
   - Estados de handoff: `StatusReady`, `StatusBlocked`, `StatusNeedsClarification`.
   - Fases válidas de SDD: `explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `verify`, `archive`.
   - Estructura `Handoff` con metadatos, listas de artefactos, decisiones, bloqueos e instrucciones.
2. **Parser y Generador Markdown (`internal/handoff/parser.go` y `writer.go`):**
   - Parseo robusto de bloque YAML frontal delimitado por `---` seguido del contenido markdown estructurado.
3. **Motor de Validación de Transición (`internal/handoff/validator.go`):**
   - Validación de la máquina de estados: la fase destino debe ser sucesora lógica de la fase origen (o vuelta atrás explícita por remediación).
   - Validación de consistencia con `axiom.yaml` mediante la integración con `internal/workspace`.
4. **Comandos CLI (`cmd/axiom/main.go`):**
   - Extensión del despachador de comandos para admitir el grupo `axiom handoff`.
5. **Pruebas y Verificación:**
   - Pruebas unitarias guiadas por tabla para parsing, generación y casos límite de validación.
   - Verificación de ejecución del binario `axiom handoff`.

---

## Áreas Afectadas (Affected Areas)

| Área / Archivo | Impacto | Descripción |
| :--- | :--- | :--- |
| `internal/handoff/types.go` | Nuevo | Estructuras de datos, fases y estados del handoff |
| `internal/handoff/parser.go` | Nuevo | Parser de `handoff.md` (frontmatter + secciones markdown) |
| `internal/handoff/writer.go` | Nuevo | Generador y formateador de `handoff.md` |
| `internal/handoff/validator.go` | Nuevo | Reglas de validación de transición y consistencia con workspace |
| `internal/handoff/handoff_test.go` | Nuevo | Suite completa de pruebas unitarias |
| `cmd/axiom/main.go` | Modificado | Incorporación de subcomandos `axiom handoff show/create/validate` |

---

## Riesgos y Mitigaciones (Risks)

- **Riesgo:** Inconsistencia de formato si los desarrolladores o agentes editan `handoff.md` manualmente.  
  *Mitigación:* El parser debe ser tolerante a espacios y variaciones menores, y el validador `axiom handoff validate` debe reportar con precisión la línea o sección faltante.
- **Riesgo:** Transiciones circulares o saltos de fase indebidos (ej. pasar de `propose` a `apply` saltándose `spec` y `design`).  
  *Mitigación:* La máquina de estados en `validator.go` rechaza cualquier handoff que intente saltarse fases obligatorias del ciclo SDD.
