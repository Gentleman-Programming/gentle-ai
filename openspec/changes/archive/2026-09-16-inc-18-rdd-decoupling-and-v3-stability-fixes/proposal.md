# Propuesta: Desacoplamiento de RDD y Parches de Estabilidad Upstream v3 (inc-18-rdd-decoupling-and-v3-stability-fixes)

> **Incremento:** `inc-18-rdd-decoupling-and-v3-stability-fixes`  
> **Fase:** Fase 4 — Evolución Arquitectónica, Flujo Orgánico (ODD) y Absorción Upstream v3  
> **Responsabilidad:** Core SDD / Estabilidad & Plataforma  
> **Fecha:** 2026-09-16  

---

## 1. Propósito (Intent)

El propósito del Incremento 18 es doble:
1. **Desacoplar limpiamente RDD (*Review-Driven Development*) del flujo central de SDD:** Retirar el bloque acoplado `reviewOffer` y los bindings forzados en las proyecciones de estado (`sdd status`), asegurando que las transiciones de fase de SDD y la compuerta de archivado dependan única y exclusivamente de la verificación formal independiente (`verify-report.md`) y el cumplimiento de tareas. RDD continuará existiendo en Axiom como un conjunto de herramientas de auditoría opt-in y bajo demanda (`axiom review ...`), sin condicionar la máquina de estados de SDD.
2. **Absorber parches críticos de estabilidad de upstream Gentle-AI v3 (v3.0.2+):**
   - **Corrección de rutas en Windows (`1a2f6775`):** Decodificación e inspección estructurada JSON de `sdd status` para evitar fallos producidos por el escape de barras invertidas (`\`) en Windows.
   - **Aislamiento de CWD en sincronización de artefactos (`8c078527`):** Cálculo de rutas de configuración y agentes desacoplado del directorio de trabajo (`CWD`) actual del proceso CLI.
   - **Saneamiento del catálogo de skills (`11f6c000`):** Separación nítida entre las skills internas de desarrollo del repositorio (flujos de pull request, triaje y defects) y el catálogo predeterminado para usuarios.
   - **Robustez en memoria persistente Engram (`59e6705f`, `90992285`):** Clarificación y soporte resiliente ante la respuesta `ambiguous_project` en el protocolo de inicialización de sesión de Engram MCP.

---

## 2. Alcance (Scope)

### Dentro de Alcance (In Scope)
- **Desacoplamiento de RDD en `internal/sddstatus` y CLI:**
  - Retiro de llamadas activas a `reviewOfferForVerify` y eliminación de la dependencia forzada de transacciones de revisión en la proyección de estado `sdd status`.
  - Eliminación de la inyección forzada de comandos sugeridos `gentle-ai review start` en los artefactos y comandos slash.
  - Asegurar que `review` permanezca accesible de manera independiente mediante los subcomandos canónicos de la CLI de Axiom (`axiom review ...`).
- **Absorción de Parches Upstream v3:**
  - Incorporar la decodificación tolerante a Windows en las aserciones y proyecciones JSON de estado SDD.
  - Actualizar la lógica de resolución de rutas en `internal/cli/run.go` y `internal/cli/sync.go` para usar rutas absolutas de configuración independientemente del `CWD`.
  - Aislar `foundationSkills` frente a las skills de colaboradores en `internal/components/skills/presets.go`.
  - Reforzar el protocolo de Engram MCP en `internal/assets/engram/protocol.md` y suite de pruebas para tolerar `ambiguous_project`.
- **Actualización de Documentación y ROADMAP:**
  - Actualizar `docs/ROADMAP.md` marcando el avance del incremento 18.

### Fuera de Alcance (Out of Scope)
- Integración de RTK (`rtk-ai/rtk`): Queda expresamente descartada por diseño arquitectónico y de seguridad (inyección de hooks intrusivos y falta de soporte oficial en Windows).
- Poda profunda de attempts/budgets y creación del motor ODD: Corresponde a la siguiente iteración (INC-19).

---

## 3. Capacidades (Capabilities)

- `sdd-rdd-decoupling`: Motor de estados SDD libre de ataduras o bloqueos condicionados a ofertas o recibos de RDD.
- `windows-path-resilience`: Tratamiento robusto de rutas con backslashes en respuestas JSON de comandos SDD.
- `cwd-isolated-sync`: Sincronización determinista de componentes de agentes sin importar desde qué subdirectorio se ejecute `axiom`.
- `preset-hygiene`: Distribución de presets de skills limpia, sin exponer workflows de contribución interna a usuarios finales.
- `engram-session-resilience`: Protocolo y arnés de pruebas de Engram robusto ante escenarios multi-proyecto.

---

## 4. Enfoque de Implementación (Approach)

1. **Especificación (`spec.md`):** Definición detallada de requerimientos canónicos y escenarios BDD en castellano.
2. **Diseño (`design.md`):** Modelado de desacoplamiento de paquetes, aislamiento de paths en runtime y saneamiento de presets.
3. **Plan de Tareas (`tasks.md`):** Desglose ordenado con verificación unitaria continua (`go test`).
4. **Verificación Formal y Archivado:** Generación de `verify-report.md` y promoción a especificación viva en `openspec/specs/`.
