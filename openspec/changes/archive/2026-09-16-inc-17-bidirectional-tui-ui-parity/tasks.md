# Tareas de Implementación: Paridad Bidireccional entre TUI y Web UI (INC-17)

## Fase 1: DTOs y Capa de Servicio en Go (`internal/dashboard/`)

- [x] T-01 Añadir DTOs para el ecosistema en `internal/dashboard/types.go` (`DoctorCheck`, `DoctorReport`, `BackupItem`, `BackupActionRequest`, `EcosystemActionResponse`).
- [x] T-02 Implementar métodos de servicio en `internal/dashboard/service.go`: `GetDoctorDiagnostics`, `RunSync`, `RunUpgrade`, `GetBackups`, `CreateBackup`, `RestoreBackup` y `GetModelAssignments`.

## Fase 2: Controladores HTTP y Endpoints REST (`internal/dashboard/server.go`)

- [x] T-03 Registrar e implementar rutas de diagnóstico y mantenimiento (`GET /api/ecosystem/doctor`, `POST /api/ecosystem/sync`, `POST /api/ecosystem/upgrade`) en `internal/dashboard/server.go`.
- [x] T-04 Registrar e implementar rutas de respaldos y modelos (`GET /api/ecosystem/backups`, `POST /api/ecosystem/backups/create`, `POST /api/ecosystem/backups/restore`, `GET /api/ecosystem/models`) en `internal/dashboard/server.go`.

## Fase 3: Interfaz Web Embebida (`internal/dashboard/assets/`)

- [x] T-05 Añadir pestaña `⚙️ Ecosistema & Herramientas` y sus secciones (Doctor, Mantenimiento, Respaldos, Modelos) en `internal/dashboard/assets/index.html`.
- [x] T-06 Implementar funciones JavaScript reactivas (`loadEcosystem()`, `triggerSync()`, `triggerUpgrade()`, `loadBackups()`, `createBackup()`, `restoreBackup()`) en `internal/dashboard/assets/app.js`.
- [x] T-07 Añadir estilos visuales en `internal/dashboard/assets/style.css` para diagnósticos de salud, tarjetas de respaldo y consola de logs.

## Fase 4: Pantallas de Gobernanza SDD y Proyectos en TUI (`internal/tui/`)

- [x] T-08 Añadir opción de submenú `📁 Proyectos y Gobernanza SDD ➔` en `internal/tui/screens/welcome.go`.
- [x] T-09 Implementar pantalla de submenú de gobernanza en `internal/tui/screens/governance.go`.
- [x] T-10 Implementar pantalla de gestión multi-proyecto `ScreenHubProjects` en `internal/tui/screens/hub_projects.go` para listar, conmutar, registrar e inicializar proyectos.
- [x] T-11 Implementar pantalla de ciclo de vida SDD `ScreenSDDIncrements` en `internal/tui/screens/sdd_increments.go` para listar incrementos, ver fases y avanzar (`continue`).
- [x] T-12 Implementar pantalla de monitor multi-rol `ScreenMultiRole` en `internal/tui/screens/multi_role.go` para ver roles, compuertas y barrera Fan-In.
- [x] T-13 Implementar pantalla de visor de relevos `ScreenHandoffs` en `internal/tui/screens/handoffs.go`.
- [x] T-14 Implementar pantalla de catálogo de especificaciones vivas `ScreenLivingDoc` en `internal/tui/screens/living_doc.go` con sincronización en caliente.
- [x] T-15 Conectar enrutamiento y despacho de estados en `internal/tui/router.go` y `internal/tui/model.go`.

## Fase 5: Pruebas, Verificación Formal y Documentación Viva

- [x] T-16 Crear pruebas unitarias con `httptest` en `internal/dashboard/dashboard_test.go` para todos los nuevos endpoints de ecosistema.
- [x] T-17 Crear pruebas unitarias para el renderizado y opciones de las pantallas TUI en `internal/tui/screens/` y `internal/tui/model_test.go`.
- [x] T-18 Ejecutar suite completa `go test ./...` y verificar compilación de `cmd/axiom`.
- [x] T-19 Redactar `verify-report.md` y validar conformidad formal con arnés SDD (`axiom sdd-verify-validate`).
- [x] T-20 Redactar `archive-report.md`, consolidar especificación viva en `openspec/specs/tui-ui-parity/spec.md`, sincronizar catálogo maestro y actualizar `ROADMAP.md`.
