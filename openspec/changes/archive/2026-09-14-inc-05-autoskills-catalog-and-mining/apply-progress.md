# Progreso de Implementación: Autoskills (midudev/autoskills) y Minería Heurística (INC-05)

Estado de ejecución del cambio en el runtime de Axiom:

- [x] Fase 1: Motor de Dominio Core (`internal/autoskill`)
  - [x] T-01 `internal/autoskill/types.go`
  - [x] T-02 `internal/autoskill/client.go`
  - [x] T-03 `internal/autoskill/detector.go`
  - [x] T-04 `internal/autoskill/miner.go`
  - [x] T-05 `internal/autoskill/manager.go`
  - [x] T-06 `internal/autoskill/autoskill_test.go`
- [x] Fase 2: Integración con el Dashboard Web Local (`internal/dashboard`)
  - [x] T-07 Actualización de `types.go` y `service.go`
  - [x] T-08 Endpoints REST en `server.go` (`/api/skills/inbox`, `/api/skills/scan`, `/api/skills/approve`, `/api/skills/reject`)
  - [x] T-09 Panel del Buzón en `internal/dashboard/assets/` (`index.html`, `style.css`, `app.js`)
  - [x] T-10 Pruebas unitarias en `dashboard_test.go` (`TestSkillsInboxEndpoints`)
- [x] Fase 3: Integración CLI (`cmd/axiom/main.go`)
  - [x] T-11 Comandos `axiom skill scan`, `axiom skill list [--inbox]`, `axiom skill approve`, `axiom skill reject`
- [x] Fase 4: Verificación, Cierre y Archivado SDD
  - [x] T-12 Ejecución de suite `go test ./...` y compilación de `axiom.exe`
  - [x] T-13 Verificación en vivo por CLI y pruebas de endpoints
  - [x] T-14 Generación de `apply-progress.md`, `verify-report.md` y `archive-report.md`
  - [x] T-15 Promoción de especificación viva a `openspec/specs/autoskills/spec.md`, archivado formal y actualización de `docs/ROADMAP.md`
