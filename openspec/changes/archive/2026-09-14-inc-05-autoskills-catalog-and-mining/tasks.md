# Tareas: Autoskills (midudev/autoskills) y Minería Heurística con Gobernanza Human-in-the-Loop (INC-05)

## Fase 1: Motor de Dominio Core (`internal/autoskill`)

- [x] T-01 Crear `internal/autoskill/types.go`: Definir estructuras de datos para tecnologías (`Technology`, `DetectConfig`, `SKILLS_MAP`), manifiesto de registro (`RegistryIndex`, `RegistrySkillEntry`), propuestas de skill (`SkillProposal`, `ProposalMetadata`), minería (`MiningProposal`) y reporte de escaneo (`ScanReport`).
- [x] T-02 Crear `internal/autoskill/client.go`: Implementar cliente HTTP nativo en Go para descargar manifiestos y skills de `midudev/autoskills`, cálculo y verificación criptográfica estricta de hash SHA-256 (`crypto/sha256`) y soporte para caché local/modo offline.
- [x] T-03 Crear `internal/autoskill/detector.go`: Implementar motor de detección de tecnologías evaluando dependencias (`package.json`, `go.mod`, etc.) y archivos de configuración contra `SKILLS_MAP` para cada rol configurado en `axiom.yaml`.
- [x] T-04 Crear `internal/autoskill/miner.go`: Implementar analizador heurístico de código local para extraer patrones Go (`table-driven-tests`, `internal-layering`, `idiomatic-error-handling`) y formular borradores canónicos de `SKILL.md`.
- [x] T-05 Crear `internal/autoskill/manager.go`: Implementar gestor del buzón de gobernanza transitoria (`.axiom/skills/inbox/`), orquestando escaneo, listado de pendientes, aprobación atómica (`Approve` ➔ `skills/<name>/`) y rechazo (`Reject`).
- [x] T-06 Crear `internal/autoskill/autoskill_test.go`: Suite completa de pruebas unitarias cubriendo cliente HTTP simulado, detección de corrupción de hash SHA-256, reglas de detección, minería sintáctica y ciclo de vida del buzón.

## Fase 2: Integración con el Dashboard Web Local (`internal/dashboard`)

- [x] T-07 Actualizar `internal/dashboard/types.go` y `service.go`: Incorporar DTOs para el buzón de skills y métodos de servicio para interactuar con `autoskill.Manager`.
- [x] T-08 Actualizar `internal/dashboard/server.go`: Exponer endpoints REST `/api/skills/inbox` (GET), `/api/skills/scan` (POST), `/api/skills/approve` (POST) y `/api/skills/reject` (POST).
- [x] T-09 Actualizar `internal/dashboard/assets/` (`index.html`, `style.css`, `app.js`): Crear el panel interactivo del Buzón de Autoskills en la pestaña de Skills con insignias visuales de procedencia (`midudev` vs `mined`), verificación SHA-256 y botones de Aprobación y Rechazo.
- [x] T-10 Actualizar `internal/dashboard/dashboard_test.go`: Incorporar pruebas unitarias para los nuevos endpoints de skills y buzón.

## Fase 3: Integración CLI (`cmd/axiom/main.go`)

- [x] T-11 Implementar comandos `axiom skill scan`, `axiom skill list [--inbox]`, `axiom skill approve` y `axiom skill reject` en `cmd/axiom/main.go` con soporte para banderas `--offline`, `--role` y `--path`.

## Fase 4: Verificación, Cierre y Archivado SDD

- [x] T-12 Ejecutar suite completa `go test ./...` y compilar `axiom.exe`.
- [x] T-13 Verificación en vivo por CLI y pruebas del servidor UI local.
- [x] T-14 Generar artefactos de trazabilidad: `apply-progress.md`, `verify-report.md` y `archive-report.md`.
- [x] T-15 Promover la especificación viva a `openspec/specs/autoskills/spec.md`, archivar formalmente en `openspec/changes/archive/2026-09-14-inc-05-autoskills-catalog-and-mining/` y actualizar `docs/ROADMAP.md`.

