# Tareas de Implementación: Orquestación Interactiva SDD y Creación de Cambios en Dashboard Web y CLI (INC-15)

## Fase 1: DTOs y Capa de Servicio en Go (`internal/dashboard/`)

- [x] T-01 Añadir DTOs de petición y respuesta en `internal/dashboard/types.go` (`CreateIncrementRequest`, `CreateIncrementResponse`, `IncrementActionRequest`, `IncrementActionResponse`, `CreateHandoffRequest`).
- [x] T-02 Implementar método `CreateIncrement` en `internal/dashboard/service.go` con validación de nombre kebab-case, detección de colisiones y generación de `proposal.md` canónico en español.
- [x] T-03 Implementar métodos `ContinueIncrement` y `VerifyIncrement` en `internal/dashboard/service.go` integrando las operaciones de despacho de `internal/cli`.
- [x] T-04 Implementar método `CreateHandoff` en `internal/dashboard/service.go` utilizando el motor de serialización atómica de `internal/handoff`.

## Fase 2: Controladores HTTP y Endpoints REST (`internal/dashboard/server.go`)

- [x] T-05 Extender `handleIncrements` en `internal/dashboard/server.go` para soportar `POST /api/increments`.
- [x] T-06 Implementar `handleIncrementContinue` (`POST /api/increments/continue`) y `handleIncrementVerify` (`POST /api/increments/verify`) en `internal/dashboard/server.go`.
- [x] T-07 Extender `handleHandoffs` en `internal/dashboard/server.go` para soportar `POST /api/handoffs`.

## Fase 3: Interfaz Web Embebida (`internal/dashboard/assets/`)

- [x] T-08 Añadir botón "+ Nuevo Incremento" y modal interactivo `#modal-new-increment` en `internal/dashboard/assets/index.html`.
- [x] T-09 Incorporar barra de herramientas de acciones SDD (Continuar SDD, Validar Verificación, Redactar Handoff) y panel de resultados en el modal de detalle de incremento.
- [x] T-10 Añadir botón "+ Crear Handoff" y modal interactivo `#modal-create-handoff` en la pestaña de relevos de `index.html`.
- [x] T-11 Implementar en `internal/dashboard/assets/app.js` la captura de eventos, validaciones, peticiones fetch asíncronas y refresco automático de Kanban y Handoffs.
- [x] T-12 Añadir estilos visuales para barras de herramientas, modales de formulario y panel de salida de consola en `internal/dashboard/assets/style.css`.

## Fase 4: Subcomando en la CLI canónica `axiom` (`cmd/axiom/`)

- [x] T-13 Implementar el subcomando `axiom change create <nombre>` (con alias `axiom change new`) en `cmd/axiom/main.go`.
- [x] T-14 Actualizar la ayuda global en `printHelp()` documentando los nuevos comandos de gestión de cambios.

## Fase 5: Pruebas, Verificación Formal y Documentación Viva

- [x] T-15 Crear y ejecutar pruebas unitarias con `httptest` en `internal/dashboard/dashboard_test.go` cubriendo los nuevos endpoints REST.
- [x] T-16 Crear pruebas de integración en `cmd/axiom/main_test.go` para el comando `axiom change create`.
- [x] T-17 Ejecutar la suite completa de pruebas unitarias (`go test ./...`) garantizando cero regresiones.
- [x] T-18 Redactar `verify-report.md` y validar conformidad formal con `axiom sdd-verify-validate`.
- [x] T-19 Redactar `archive-report.md`, consolidar la especificación viva `openspec/specs/dashboard-sdd-orchestration/spec.md` y sincronizar el catálogo con `axiom archive sync`.
