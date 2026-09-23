# Especificación Viva: Gobernanza del Índice Unificado de Skills

> **Dominio:** `axiom-skills-index-governance`  
> **Versión Canónica:** 1.0.0  
> **Estado:** Vigente  
> **Idioma:** Español (Castellano peninsular)

---

## 1. Capacidad: `axiom-skills-index-governance`

Gobernanza del catálogo de habilidades (skills) de agentes en tres destinos sincronizados desde un único escaneo (`.atl/skill-registry.md`, sección gestionada `## Skills` en `AGENTS.md` y tópico persistente en Engram MCP), con soporte de adopción atómica mediante marcadores canónicos y gancho no transaccional tras promociones en `autoskill`.

### Requirement: Superficie CLI del índice de skills (REQ-22.10)

El sistema DEBE exponer el subcomando `axiom skill index <refresh|list>` dentro del grupo `axiom skill`, diferenciando claramente en ayuda y ejecución entre `skill list` (skills activas y buzón de autoskill) y `skill index list` (inventario unificado indexado).

#### Scenario: Consulta del índice desde CLI
- **DADO** un workspace con skills instaladas y globales
- **CUANDO** el usuario ejecuta `axiom skill index list`
- **ENTONCES** se muestra la lista formateada con nombre, ámbito (`project`/`user`) y ruta de cada skill

---

### Requirement: Regeneración unificada en tres destinos desde un único escaneo (REQ-22.11)

La ejecución de `axiom skill index refresh` DEBE escanear las skills una sola vez y actualizar atómicamente:
1. El registro local en disco `.atl/skill-registry.md`.
2. La sección delimitada `## Skills` en el archivo `AGENTS.md` de la raíz del workspace (con rutas relativas para skills de proyecto).
3. El tópico de persistencia `skill-registry` en Engram MCP mediante cliente stdio acotado.

Un fallo de conexión con Engram MCP NO DEBE impedir la actualización de los destinos locales en disco ni producir un código de salida fallido.

#### Scenario: Regeneración exitosa en los tres destinos
- **DADO** el workspace con agentes y servidor Engram activo
- **CUANDO** se invoca `axiom skill index refresh`
- **ENTONCES** los tres destinos reflejan el conjunto actual de skills
- **Y** el comando retorna código de salida `0`

---

### Requirement: Reemplazo atómico y marcado de ## Skills en AGENTS.md (REQ-22.12)

La sección `## Skills` de `AGENTS.md` DEBE delimitarse por los marcadores canónicos:
```markdown
<!-- axiom:skills-index -->
## Skills

| Skill | Trigger / description | Scope | Path |
| --- | --- | --- | --- |
...
<!-- /axiom:skills-index -->
```
El motor `filemerge.InjectMarkdownSection` DEBE operar de forma atómica sobre el bloque, preservando intacto el resto de secciones del documento y elevando marcadores legados si estuvieran presentes.

#### Scenario: Idempotencia en la sustitución de marcadores
- **DADO** un `AGENTS.md` con marcadores canónicos
- **CUANDO** se regenera el índice consecutivamente sin cambios de skills
- **ENTONCES** el archivo permanece byte a byte idéntico
- **Y** ninguna sección circundante se altera

---

### Requirement: Disparo automático del índice desde autoskill (REQ-22.13)

Cuando el gestor de autoskills apruebe una propuesta (`Manager.Approve`), el sistema DEBE disparar automáticamente la regeneración del índice unificado para que la nueva skill esté disponible de inmediato en todos los destinos. El gancho DEBE ser no transaccional: un eventual fallo de indexación DEBE emitirse como aviso y no debe revertir la promoción completada. Descartar una propuesta (`Reject`) NO DEBE disparar la regeneración.

#### Scenario: Aprobación de skill promueve e indexa
- **DADO** una skill candidata en el buzón transitorio
- **CUANDO** se aprueba con `axiom skill approve <nombre>`
- **ENTONCES** la skill se mueve a `skills/<nombre>/`
- **Y** el índice se actualiza automáticamente

---

### Requirement: Compatibilidad del verbo skill-registry (REQ-22.14)

El verbo `axiom skill-registry <refresh|list>` DEBE conservarse plenamente operativo y con compatibilidad byte a byte en flags, salida primaria y formato JSON, delegando en el motor unificado de `skill index`.

#### Scenario: Invocación por scripts externos
- **DADO** un plugin que invoca `axiom skill-registry refresh --quiet --no-gitignore`
- **CUANDO** se ejecuta la orden
- **ENTONCES** el comando se procesa con éxito con código de salida `0`
