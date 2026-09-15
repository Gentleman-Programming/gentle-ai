# Informe de Archivado: Autoskills (midudev/autoskills) y Minería Heurística (INC-05)

**Identificador**: `inc-05-autoskills-catalog-and-mining`  
**Fecha de Archivado**: 2026-09-15  
**Estado Final**: Conforme y Archivado  

---

## 1. Resumen de Capacidades Entregadas

1. **Motor Core de Catálogo y Minería en Go (`internal/autoskill`)**:
   - `types.go`: Esquema canónico de tecnologías (`SKILLS_MAP`), reglas de detección (`DetectConfig`), manifiestos del registro oficial de `midudev/autoskills` (`RegistryIndex`, `RegistrySkillEntry`), metadatos del buzón (`ProposalMetadata`, `SkillProposal`) y reportes de escaneo (`ScanReport`).
   - `client.go`: Cliente HTTP nativo en Go con descarga de índice y skills, verificación criptográfica estricta de sumas **SHA-256** (`crypto/sha256`), soporte para caché local en `.axiom/cache/autoskills/` y modo offline sin dependencias de Node.js.
   - `detector.go`: Motor de detección multi-rol que inspecciona repositorios asociados en `axiom.yaml` buscando dependencias (`package.json`, `go.mod`, `Cargo.toml`), configuraciones clave (`next.config.*`, `tailwind.config.*`, etc.) y extensiones.
   - `miner.go`: Analizador heurístico de código que descubre patrones idiomáticos locales en Go (`table-driven-tests`, `internal-layering`, `idiomatic-error-wrapping`) y formula directrices canónicas `SKILL.md`.
   - `manager.go`: Gestor de gobernanza del buzón transitorio (`.axiom/skills/inbox/`), orquestando escaneo, listado de propuestas, aprobación atómica (`Approve` a `skills/<nombre>/SKILL.md`) y rechazo (`Reject`).
   - `autoskill_test.go`: 5 pruebas unitarias exhaustivas con 100% PASS.

2. **Integración con el Dashboard Web Local (`internal/dashboard`)**:
   - `types.go`: DTOs `SkillProposalDTO` y `SkillActionDTO`.
   - `service.go`: Integración de `autoskill.Manager` en `Service` exponiendo `GetSkillsInbox`, `ScanSkills`, `ApproveSkill` y `RejectSkill`.
   - `server.go`: Endpoints REST `/api/skills/inbox` (GET), `/api/skills/scan` (POST), `/api/skills/approve` (POST) y `/api/skills/reject` (POST).
   - Frontend SPA en `assets/` (`index.html`, `style.css`, `app.js`): Panel interactivo del Buzón de Autoskills en la pestaña de Skills con insignias de procedencia (`midudev auditado` vs `minería local`), validación SHA-256, visor de directrices y botones de acción.
   - `dashboard_test.go`: Prueba unitaria `TestSkillsInboxEndpoints` con 100% PASS (9/9 pruebas del dashboard exitosas).

3. **Integración CLI en `cmd/axiom/main.go`**:
   - Comandos `axiom skill scan`, `axiom skill list [--inbox]`, `axiom skill approve` y `axiom skill reject`.
   - Banderas `--role`, `--path`, `--offline`.

4. **Gobernanza y Especificación Viva**:
   - Especificación viva promovida a: [`openspec/specs/autoskills/spec.md`](file:///c:/repos/axiom/openspec/specs/autoskills/spec.md).
   - Verificación formal con 13/13 requerimientos y 17/17 escenarios BDD aprobados.
   - Binario nativo `axiom.exe` compilado y probado en vivo.
