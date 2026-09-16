# Propuesta: Orquestación Interactiva SDD y Creación de Cambios en Dashboard Web y CLI (INC-15)

## Propósito (Intent)

A lo largo de los incrementos INC-01 a INC-14, la plataforma Axiom ha consolidado su infraestructura de gobierno (topologías de repositorios, handoffs estructurados, concurrencia multi-rol, hub multi-proyecto, catálogo autoskills con minería heurística, conector semántico, motor de especificaciones vivas, neutralización de identidad visual, persona y contrato de idioma español, unificación de estado en `~/.axiom/`, integración nativa de comandos SDD en la CLI `axiom` y unificación de la TUI interactiva).

Sin embargo, el **Dashboard Web local (`axiom ui`)** opera actualmente bajo un esquema de **solo lectura** respecto al ciclo de vida SDD:
1. **Creación manual de cambios:** Para iniciar un nuevo incremento o cambio, el desarrollador o agente debe crear carpetas y plantillas manualmente en `openspec/changes/`, sin asistencia guiada.
2. **Ciclo de vida desconectado de la UI:** El tablero Kanban muestra el progreso de tareas y la fase calculada, pero carece de botones de acción para avanzar de fase (`sdd continue`), validar verificación formal (`verify-validate`) o archivar el cambio directamente desde la interfaz web.
3. **Visor de Handoffs pasivo:** La pestaña de Handoffs solo visualiza documentos existentes; no permite redactar ni registrar un nuevo relevo estructurado entre fases o roles sin recurrir a la terminal o edición manual de Markdown.
4. **Ausencia de comando rápido de creación en la CLI:** La CLI `axiom` posee comandos para inspeccionar y continuar cambios (`axiom sdd status`, `axiom sdd continue`), pero no cuenta con un subcomando explícito e intuitivo para crear un nuevo cambio (`axiom change create <nombre>`).

El **Incremento 15 (INC-15: `dashboard-interactive-sdd-orchestration`)** resuelve esta limitación transformando el Dashboard Web en un **centro de mando operativo interactivo** y dotando a la CLI del comando canónico de creación de cambios, permitiendo orquestar todo el flujo SDD de forma visual y programática.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Creación Interactiva de Incrementos / Cambios en Dashboard Web:**
   - Botón visible *"+ Nuevo Incremento"* en el encabezado del Tablero de Incrementos (`tab-increments`).
   - Modal interactivo con validación de nombre (`kebab-case`), propósito/intent descriptivo y tipo de cambio (`feature`, `fix`, `refactor`, `architecture`).
   - Generación atómica en backend del andamiaje `openspec/changes/<nombre>/proposal.md` con plantilla canónica en español castellano.
   - Refresco reactivo del tablero Kanban y selección automática del nuevo cambio.

2. **Acciones Operativas del Ciclo de Vida SDD en la Web UI:**
   - En el modal de detalle del incremento (`modal-increment-detail`), incorporación de barra de herramientas de acciones:
     - Botón *"▶ Continuar SDD"*: ejecuta la transición autorizada en backend (`RunSDDContinue`) y presenta el resultado o siguiente acción autorizada con feedback visual claro.
     - Botón *"✓ Validar Verificación"*: ejecuta la validación formal de requerimientos y escenarios BDD (`RunSDDVerifyValidate`).
     - Botón *"🤝 Crear Handoff"*: abre el modal de relevo pre-cargando el cambio activo.
   - Panel de resultados/consola colapsable en el modal para visualizar la salida del motor SDD.

3. **Compositor y Editor de Handoffs Estructurados:**
   - Botón *"+ Crear Handoff"* en la pestaña del Visor de Handoffs (`tab-handoffs`).
   - Modal interactivo con campos estructurados: fase origen, fase destino, rol origen, rol destino, estado (`ready`, `blocked`, `needs_clarification`), resumen ejecutivo, decisiones y riesgos.
   - Serialización canónica a Markdown con Frontmatter YAML mediante `handoff.WriteFile` en `openspec/changes/<cambio>/handoff.md`.
   - Sincronización automática con la memoria persistente de Engram (si está activa).

4. **Endpoints REST Seguros en `internal/dashboard/`:**
   - `POST /api/increments`: crea un nuevo cambio e inicializa su propuesta.
   - `POST /api/increments/continue`: ejecuta la siguiente fase SDD y retorna el veredicto/acción.
   - `POST /api/increments/verify`: evalúa la validez formal del reporte de verificación.
   - `POST /api/handoffs`: registra y persiste un nuevo artefacto de relevo.

5. **Subcomando CLI homólogo en `cmd/axiom/main.go`:**
   - `axiom change create <nombre> [--intent <texto>] [--type <tipo>]`
   - Alias directo `axiom change new <nombre>`
   - Documentación en `printHelp()`.

6. **Batería Completa de Pruebas:**
   - Pruebas unitarias de los nuevos endpoints REST con `httptest` en `internal/dashboard/`.
   - Pruebas del servicio de creación de cambios y handoffs.
   - Pruebas de integración CLI para `axiom change create`.

### Fuera de Alcance (Out of Scope)

- Modificación de la especificación base de OpenSpec ni del almacenamiento de la base de datos de revisión RDD.
- Ejecución desatendida de subagentes en la nube (la ejecución se mantiene local, gobernada y supervisada).

---

## Capacidades (Capabilities)

### Nuevas Capacidades

- `dashboard-sdd-orchestration`: Capacidad del Dashboard Web de Axiom (`axiom ui`) para orquestar interactivamente el ciclo de vida SDD (creación de cambios, avance de fases con `sdd continue`, validación formal de verificación y composición de handoffs estructurados).
- `cli-change-creator`: Subcomando nativo en la CLI `axiom change create` para inicializar cambios y propuestas SDD de manera determinista y canónica.

### Capacidades Modificadas

- `local-web-dashboard`: Extensión de la API REST del dashboard con endpoints de mutación segura (`POST /api/increments`, `POST /api/increments/continue`, `POST /api/increments/verify`, `POST /api/handoffs`).

---

## Enfoque de Implementación (Approach)

1. **Modelos de Datos y Capa de Servicio (`internal/dashboard/`):**
   - Definir los DTOs de petición y respuesta en `types.go`.
   - Implementar en `service.go` las funciones `CreateIncrement`, `ContinueIncrement`, `VerifyIncrement` y `CreateHandoff`, reutilizando la lógica canónica de `internal/handoff` y `internal/cli`.
2. **Controladores HTTP y Enrutamiento (`internal/dashboard/server.go`):**
   - Incorporar manejadores para peticiones `POST` en `/api/increments`, `/api/increments/continue`, `/api/increments/verify` y `/api/handoffs`.
3. **Interfaz Web Embebida (`internal/dashboard/assets/`):**
   - Actualizar `index.html` con los botones de acción y nuevos modales flotantes (`modal-new-increment`, `modal-create-handoff`).
   - Implementar en `app.js` la lógica reactiva de captura de eventos, validación de formularios, llamadas `fetch` asíncronas y refresco automático de las vistas.
   - Añadir estilos responsivos en `style.css`.
4. **CLI Canónica (`cmd/axiom/main.go`):**
   - Añadir soporte para el comando `axiom change create` y su ayuda contextual.
5. **Pruebas y Verificación:**
   - Baterías de pruebas unitarias en `internal/dashboard/dashboard_test.go` y `cmd/axiom/main_test.go`.
   - Verificación formal con `axiom sdd verify-validate`.

---

## Áreas Afectadas

| Área / Archivo | Impacto | Descripción |
| :--- | :---: | :--- |
| `internal/dashboard/types.go` | Modificado | Nuevos DTOs para creación de incrementos, ejecución continue y handoffs. |
| `internal/dashboard/service.go` | Modificado | Lógica de negocio para crear cambios en `openspec/changes/`, ejecutar continue y guardar handoff. |
| `internal/dashboard/server.go` | Modificado | Registro de rutas y controladores HTTP `POST`. |
| `internal/dashboard/assets/index.html` | Modificado | Botón "+ Nuevo Incremento", barra de acciones SDD y modales interactivos. |
| `internal/dashboard/assets/app.js` | Modificado | Lógica JS de interacción, envío de formularios y recarga de vistas. |
| `internal/dashboard/assets/style.css` | Modificado | Estilos visuales de la barra de acciones y formularios modales. |
| `cmd/axiom/main.go` | Modificado | Subcomando `axiom change create`. |
| `cmd/axiom/main_test.go` | Modificado | Pruebas de integración para `axiom change create`. |
| `internal/dashboard/dashboard_test.go` | Modificado | Pruebas unitarias de endpoints REST y capa de servicio. |

---

## Plan de Pruebas y Validación

- `go test -v ./internal/dashboard/...` para asegurar que todos los endpoints REST responden adecuadamente ante peticiones válidas y devuelven errores claros con códigos HTTP apropiados ante entradas inválidas.
- `go test -v ./cmd/axiom/...` para verificar el subcomando `axiom change create`.
- `go test ./...` para garantizar cero regresiones en la suite global.
- Validación formal con el arnés SDD (`axiom sdd-verify-validate`).
