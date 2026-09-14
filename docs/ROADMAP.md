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
| **INC-02** | `structured-handoffs-lifecycle` | 📋 Planificado | Core SDD / Workflow | Esquema y artefacto `handoff.md`, comandos de transición formal entre fases y registro en Engram. |
| **INC-03** | `multi-role-sdd-fan-out` | 📋 Planificado | Core SDD / Roles | Declaración de roles en `Design`, división de `tasks.<rol>.md`, ejecución `Apply`/`Verify` por rol y barrera de sincronización en `Archive`. |
| **INC-04** | `axiom-local-web-dashboard` | 📋 Planificado | UI / Experiencia | Servidor HTTP local embebido en Go con dashboard web: tablero de incrementos, estado de roles, visor de handoffs y buzón de skills. |
| **INC-05** | `autoskills-catalog-and-mining` | 📋 Planificado | Skills / Inteligencia | Catálogo de skills por tecnología detectada y minería heurística de código por repositorio con pantalla de aprobación humana previa. |
| **INC-06** | `semantic-code-serena-codegraph` | 📋 Planificado | Semántica / Herramientas | Conector local con Serena MCP y CodeGraph para navegación y consultas semánticas de código en `Explore` y `Design`. |
| **INC-07** | `archive-living-documentation-engine` | 📋 Planificado | Documentación / SDD | `Archive` como mantenedor continuo de especificaciones existentes y generador incremental de documentación viva en proyectos no documentados. |

---

## Registro de Cambios y Entregables

### [INC-01] axiom-identity-workspace-topology (⏳ En progreso)
- **Directorio de cambio SDD:** `openspec/changes/inc-01-axiom-identity-workspace-topology/`
- **Entregables clave:**
  1. Punto de entrada `cmd/axiom/main.go` con comandos CLI (`axiom --version`, `axiom workspace validate`).
  2. Especificación canónica del esquema `axiom.yaml` con roles (1 a N) y mapeo a rutas locales.
  3. Validación de la regla de topologías (comprobación de carpeta maestra y presencia de repo de specs en multirepo/monorepo desacoplado).
  4. Suite de pruebas unitarias en Go para el validador de topología de workspace.
