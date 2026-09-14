# Roadmap de Incrementos — Axiom

> **Visión y Arquitectura:** [AXIOM_ENTERPRISE_VISION.md](architecture/AXIOM_ENTERPRISE_VISION.md)  
> **Estado General:** Fase 1 — Fundación de la Plataforma e Identidad  
> **Última Actualización:** 2026-09-14

---

## Leyenda de Estados

- `⏳ En progreso`: Incremento actualmente en desarrollo activo dentro de su ciclo SDD.
- `📋 Planificado`: Incremento definido con alcance y listo para iniciar cuando corresponda.
- `✅ Archivado`: Incremento completado, verificado al 100% y formalmente archivado en la especificación viva.
- `⏸️ Pausado / En espera`: Incremento a la espera de dependencias previas o decisiones de producto.

---

## Catálogo de Incrementos

| ID | Incremento | Estado | Responsabilidad | Descripción Resumida |
| :--- | :--- | :---: | :---: | :--- |
| **INC-01** | `axiom-identity-workspace-topology` | ✅ Archivado | Arquitectura / Core | Creación del binario `axiom`, esquema `axiom.yaml`, validación de topologías (monorepo-embedded, monorepo-decoupled, multirepo) y repositorio canónico de specs. |
| **INC-02** | `structured-handoffs-lifecycle` | ✅ Archivado | Core SDD / Workflow | Esquema canónico `handoff.md`, motor Go `internal/handoff`, subcomandos CLI `axiom handoff show/create/validate` y espejo Engram. |
| **INC-03** | `multi-role-sdd-fan-out` | 📋 Planificado | Core SDD / Roles | Declaración de roles en `Design`, división de `tasks.<rol>.md`, ejecución `Apply`/`Verify` por rol y barrera de sincronización en `Archive`. |
| **INC-04** | `axiom-local-web-dashboard` | 📋 Planificado | UI / Experiencia | Servidor HTTP local embebido en Go con dashboard web: tablero de incrementos, estado de roles, visor de handoffs y buzón de skills. |
| **INC-05** | `autoskills-catalog-and-mining` | 📋 Planificado | Skills / Inteligencia | Catálogo de skills por tecnología detectada y minería heurística de código por repositorio con pantalla de aprobación humana previa. |
| **INC-06** | `semantic-code-serena-codegraph` | 📋 Planificado | Semántica / Herramientas | Conector local con Serena MCP y CodeGraph para navegación y consultas semánticas de código en `Explore` y `Design`. |
| **INC-07** | `archive-living-documentation-engine` | 📋 Planificado | Documentación / SDD | `Archive` como mantenedor continuo de especificaciones existentes y generador incremental de documentación viva en proyectos no documentados. |

---

## Registro de Cambios y Entregables

### [INC-01] axiom-identity-workspace-topology (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-14-inc-01-axiom-identity-workspace-topology/`
- **Especificación viva:** `openspec/specs/workspace-topology/spec.md`
- **Entregables clave:**
  1. Punto de entrada `cmd/axiom/main.go` con comandos CLI (`axiom --version`, `axiom workspace validate`).
  2. Especificación canónica del esquema `axiom.yaml` con roles (1 a N) y mapeo a rutas locales.
  3. Validación de la regla de topologías (comprobación de carpeta maestra y presencia de repo de specs en multirepo/monorepo desacoplado).
  4. Suite de pruebas unitarias en Go para el validador de topología de workspace.

### [INC-02] structured-handoffs-lifecycle (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-14-inc-02-structured-handoffs-lifecycle/`
- **Especificación viva:** `openspec/specs/structured-handoffs/spec.md`
- **Entregables clave:**
  1. Paquete de dominio Go `internal/handoff/`: tipos, parser bidireccional, formateador canónico, motor de validación semántica de transiciones de fase y exportador de espejo para Engram MCP.
  2. Subcomandos CLI `axiom handoff show`, `create` y `validate` en `cmd/axiom/main.go`.
  3. Batería completa de pruebas unitarias para `internal/handoff/` con cobertura exhaustiva de casos válidos y rechazo de errores.
  4. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 7/7 requerimientos, 13/13 escenarios BDD).
