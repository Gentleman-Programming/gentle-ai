# Tareas: Servidor HTTP Local Embebido y Dashboard Web (INC-04)

## Fase 1: Capa de Dominio y Servicio de Agregación (`internal/dashboard`)

- [x] T-01 Crear `internal/dashboard/types.go`: Definir los DTOs para la API REST (`WorkspaceDTO`, `RoleMeta`, `IncrementSummaryDTO`, `IncrementDetailDTO`, `SkillDTO`).
- [x] T-02 Crear `internal/dashboard/service.go`: Implementar la capa de servicio para consolidar datos desde `internal/workspace`, `internal/multirole`, `internal/handoff`, `openspec/changes/` y el catálogo de skills.

## Fase 2: Servidor HTTP y Assets Embebidos (`internal/dashboard`)

- [x] T-03 Crear `internal/dashboard/server.go`: Implementar el servidor `net/http`, enrutador REST (`/api/workspace`, `/api/increments`, `/api/roles`, `/api/handoffs`, `/api/skills`), cabeceras CORS/JSON y fallback automático ante puerto ocupado.
- [x] T-04 Crear `internal/dashboard/assets.go` y la interfaz web en `internal/dashboard/assets/` (`index.html`, `style.css`, `app.js`) embebidos mediante `//go:embed`.

## Fase 3: Pruebas Unitarias de Dominio y API (`internal/dashboard`)

- [x] T-05 Crear `internal/dashboard/dashboard_test.go`: Suite completa de pruebas unitarias evaluando lectura de workspace, detección de incrementos, estado multi-rol, handoffs, catálogo de skills, endpoints REST con `httptest` y servido de assets embebidos.

## Fase 4: Integración CLI (`cmd/axiom`) y Verificación en Vivo

- [x] T-06 Integrar subcomando `axiom ui` en `cmd/axiom/main.go`: Soportar banderas `--port`, `--no-browser`, `--path`, apertura automática del navegador del sistema y captura de señales para apagado elegante (*graceful shutdown*).
- [x] T-07 Compilar `axiom.exe`, ejecutar suite completa de pruebas Go y verificar el funcionamiento en vivo del dashboard web local.
