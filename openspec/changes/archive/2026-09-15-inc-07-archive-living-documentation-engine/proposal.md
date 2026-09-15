# Propuesta: Motor de Documentación Viva y Adopción Orgánica en Archive (INC-07)

## Propósito (Intent)

En el desarrollo asistido por IA bajo la metodología *Spec-Driven Development* (SDD), la fase **`Archive`** suele considerarse erróneamente un mero paso final de empaquetado o almacenamiento pasivo (trasladar carpetas a `archive/`). Sin embargo, en la visión empresarial de Axiom, **`Archive` es el custodio activo de la verdad del sistema y el motor de adopción orgánica (*Zero-Doc Cold Start*)**.

Muchos equipos y proyectos reales carecen de documentación o especificaciones iniciales ("brownfield" o código legado sin documentar). Exigirles una especificación completa antes de permitirles iterar genera fricción y parálisis por análisis. Por otro lado, en proyectos con documentación existente, los incrementos archivados a menudo quedan aislados sin reflejar un catálogo maestro unificado del sistema.

Para resolver este desafío fundamental (Pilares 1.3 y 2.7 de `AXIOM_ENTERPRISE_VISION.md`), el **Incremento 7 (INC-07: `archive-living-documentation-engine`)** transforma `Archive` en un motor activo que:
1. **Mantiene un Catálogo Maestro de Especificaciones Vivas (`openspec/specs/` y `openspec/INDEX.md`):** Indexa todas las especificaciones vivas del workspace, sus capacidades, cantidad de requerimientos, escenarios BDD y roles de desarrollo propietarios.
2. **Habilita la Adopción Orgánica ("Zero-Doc Cold Start"):** Permite a proyectos sin documentación previa comenzar de inmediato, sintetizando y consolidando incrementalmente la especificación viva funcional y técnica con cada cierre de incremento (`spec.md` + `verify-report.md` + símbolos del motor semántico Go AST de INC-06).
3. **Provee Comandos CLI de Gobernanza:** `axiom archive sync`, `axiom archive list` y `axiom archive coldstart` para inspeccionar y sincronizar el mapa vivo de documentación sin esfuerzo manual.
4. **Integra un Explorador de Documentación Viva en el Dashboard Web (`axiom ui`):** Endpoints REST y visualizador interactivo del catálogo vivo en el navegador.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Paquete de Dominio en Go (`internal/livingdoc/`):**
   - `types.go`: Modelos de datos para especificaciones vivas (`LivingSpecEntry`), catálogo maestro (`LivingCatalog`), requerimientos consolidados (`RequirementSummary`), roles propietarios y reportes de sincronización (`SyncReport`).
   - `indexer.go`: Analizador de especificaciones vivas en `openspec/specs/`. Parsea cada `spec.md`, extrae capacidades, requerimientos (`### Requirement: ...`), escenarios BDD (`#### Scenario: ...`) y genera el índice unificado.
   - `synthesizer.go`: Motor de adopción orgánica (*Cold Start*). Genera y actualiza incrementalmente especificaciones vivas a partir de los artefactos de un incremento cerrado (`spec.md`, `verify-report.md`) combinados con la información semántica del código (`internal/semantic/`).
   - `service.go`: Capa de servicio que orquesta la indexación, sincronización de `openspec/INDEX.md` y consultas de especificaciones vivas.
   - `livingdoc_test.go`: Batería completa de pruebas unitarias cubriendo indexación, generación de `INDEX.md`, síntesis orgánica de especificaciones y tolerancia a fallos.

2. **Integración CLI en `cmd/axiom/main.go` (`axiom archive`):**
   - `axiom archive sync [--path <dir>]`: Re-escanea `openspec/specs/`, actualiza el catálogo y genera el manifiesto estructurado `openspec/INDEX.md`.
   - `axiom archive list [--path <dir>]`: Lista todas las especificaciones vivas con su estado, requerimientos y roles.
   - `axiom archive show <dominio> [--path <dir>]`: Muestra el contenido detallado y requerimientos de una especificación viva particular.
   - `axiom archive coldstart <cambio> [--domain <dominio>] [--path <dir>]`: Inicializa o consolida la especificación viva de un cambio en un proyecto con adopción orgánica.

3. **Integración con el Dashboard Web Local (`internal/dashboard/`):**
   - Endpoints REST en `internal/dashboard/server.go`:
     - `GET /api/archive/specs`: Catálogo completo de especificaciones vivas.
     - `GET /api/archive/specs/{domain}`: Contenido y requerimientos de una spec viva.
     - `POST /api/archive/sync`: Sincronización y re-indexación desde la UI.
   - Frontend SPA (`assets/`):
     - Nueva pestaña o visor "Especificaciones Vivas" con tarjetas de dominio, conteo de requerimientos, selector y visor de especificaciones con Markdown estructurado.

4. **Suite de Pruebas Unitarias (`internal/livingdoc/livingdoc_test.go` y `internal/dashboard/dashboard_test.go`):**
   - Cobertura 100% de escenarios de parseo de especificaciones, regeneración de índices y endpoints HTTP.

### Fuera de Alcance (Out of Scope)

- Generación de código fuente a partir de la especificación (SDD opera en sentido inverso: el código implementa la spec).
- Sincronización remota en servidores cloud externos (Axiom opera 100% local sobre filesystem y Git).

---

## Capacidades (Capabilities)

### Nuevas Capacidades

- `livingdoc-spec-indexer`: Capacidad de recorrer `openspec/specs/`, parsear especificaciones en Markdown y construir un catálogo unificado de capacidades y requerimientos.
- `livingdoc-organic-coldstart`: Capacidad de adoptar orgánicamente proyectos no documentados, sintetizando y versionando especificaciones vivas con cada incremento completado.
- `axiom-cli-archive`: Comandos CLI `axiom archive sync|list|show|coldstart`.
- `axiom-dashboard-living-specs`: Visor y explorador interactivo del catálogo vivo de especificaciones en la Web UI local.

---

## Enfoque de Implementación (Approach)

1. **Definir el modelo de catálogo vivo (`internal/livingdoc/types.go`):**
   - Estructuras para `LivingSpecEntry`, `LivingCatalog`, `RequirementMeta` y `SyncReport`.
2. **Implementar el indexador de especificaciones (`indexer.go`):**
   - Parser de expresiones regulares y escaneo de árbol en `openspec/specs/` para extraer requerimientos y escenarios.
3. **Implementar el sintetizador orgánico (*Cold Start*) (`synthesizer.go`):**
   - Lógica de consolidación incremental que toma los deltas de un cambio y actualiza o crea la spec viva correspondiente.
4. **Implementar el servicio unificado (`service.go`):**
   - Generación determinista de `openspec/INDEX.md` y métodos de consulta.
5. **Crear batería de pruebas unitarias (`livingdoc_test.go`):**
   - Tests con fixtures simuladas de especificaciones vivas e índices.
6. **Registrar subcomandos en `cmd/axiom/main.go`:**
   - Comandos `axiom archive sync`, `axiom archive list`, `axiom archive show`, `axiom archive coldstart`.
7. **Exponer endpoints y panel en Dashboard Web (`internal/dashboard/`):**
   - Endpoints `/api/archive/*` y actualización de la SPA para explorar especificaciones vivas.
8. **Verificación y Archivado SDD:**
   - Crear `openspec/specs/living-documentation/spec.md`, validar pruebas y archivar INC-07 completando el Roadmap maestro.
