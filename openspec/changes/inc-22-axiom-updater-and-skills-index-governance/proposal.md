# Propuesta: Actualizador Autónomo de Axiom, Sincronización y Gobernanza del Índice Unificado de Skills (inc-22-axiom-updater-and-skills-index-governance)

> **Incremento:** `inc-22-axiom-updater-and-skills-index-governance`  
> **Fase del Roadmap:** Fase 4 — Gobernanza de Agentes, Flujo Dual y Ecosistema Autónomo  
> **Responsabilidad:** Ciclo de Vida / Ecosistema / Actualizaciones / Gobernanza de Skills  
> **Fecha:** 2026-09-21  
> **Idioma:** Español (castellano peninsular)  
> **Estado:** Propuesta Formal (Listo para Especificación y Diseño en Siguiente Sesión)  

---

## 0. Resumen Ejecutivo

Este incremento resuelve dos pilares fundamentales para la autonomía operativa y la experiencia de desarrollo en Axiom:

1. **Pipeline de Actualización Integral y Autónomo (`update` + `upgrade` + `sync`):**
   - Garantiza que la comprobación de versiones (`update`) y la ejecución de la actualización (`upgrade`) apunten de forma robusta y nativa al repositorio del fork (`IGutierrezZ/axiom`), resolviendo el bloqueo crítico existente en Windows debido a la discrepancia entre el import path de Go y el nombre del módulo en `go.mod`.
   - Asegura la paridad entre interfaces: unifica la Web UI (`axiom ui`) con la TUI para que la acción de actualización encadene automáticamente la sincronización (`sync`) de agentes, prompts y configuraciones, garantizando que el nuevo binario proyecte sus assets embebidos preservando el código propio mediante `filemerge`.
   - Limpia de forma definitiva cualquier residuo de texto y branding de Gentle-AI en las vistas de actualización.

2. **Gobernanza del Índice Unificado y Dinámico de Skills:**
   - Proporciona a los agentes un índice vivo y fiable que les permita saber qué skills existen en el proyecto y entorno de usuario, cuándo invocarlas y qué rutas exactas leer, evitando lecturas a ciegas y sobrecarga de contexto.
   - Expone canónicamente en la CLI el subcomando `axiom skill index [refresh|list]`.
   - Conecta la regeneración del índice (`.atl/skill-registry.md` y memoria Engram `topic_key: skill-registry`) con la tabla canónica `## Skills` de `AGENTS.md`, manteniéndola actualizada sin tocar el resto del documento.
   - Integra un gancho (*hook*) automático en `autoskill` (`axiom skill approve`) para que cualquier skill aprobada desde el buzón transitorio pase inmediatamente a formar parte del índice visible para los agentes.

3. **Política Ligera de Rastreo Upstream (Gentle-AI):**
   - Registra formalmente la versión base de upstream contra la que Axiom está sincronizado (fijada inicialmente en `v3.4.0`) en un fichero de configuración/estado rastreable, permitiendo barridos periódicos manuales de novedades (skills, templates, prompts) sin incurrir en sobreingeniería ni automatizaciones frágiles.

---

## 1. Propósito y Justificación Técnica (Intent)

### 1.1 El Problema Actual

1. **Bloqueo en la actualización en Windows:**
   - La comprobación `axiom update` consulta correctamente a `IGutierrezZ/axiom` y detecta nuevos releases (por ejemplo, `v3.5.0` publicado hoy).
   - Sin embargo, en Windows la estrategia enrutada invoca `go install github.com/IGutierrezZ/axiom/cmd/axiom@vX.Y.Z`. Dado que el archivo `go.mod` de la raíz del proyecto declara `module github.com/gentleman-programming/gentle-ai/v3`, el compilador de Go aborta con un error de discrepancia de módulo (`unexpected module path`).
   - Además, GitHub Releases de Axiom no dispone de binarios compilados `.zip`/`.exe` para Windows, dejando al usuario sin vía de auto-actualización operativa.
2. **Desconexión entre Upgrade y Sync en la Web UI:**
   - La TUI cuenta con la pantalla combinada *Upgrade + Sync*. Sin embargo, el endpoint `/api/ecosystem/upgrade` del Dashboard Web únicamente dispara `upgrade`, requiriendo un paso manual posterior para que los nuevos templates y skills embebidos se proyecten en los agentes.
3. **Invisibilidad de Skills Dinámicas y Ceguera de Agentes:**
   - Los agentes leen `AGENTS.md` al arrancar. La tabla de skills en `AGENTS.md` es actualmente estática y manual.
   - El motor de escaneo e indexación existe en `internal/skillregistry`, pero no está enrutado en `cmd/axiom/main.go` (el comando `axiom skill-registry` no responde).
   - Cuando se utiliza `axiom skill scan` y se aprueba una skill con `axiom skill approve`, la skill se copia a `skills/`, pero ni `AGENTS.md`, ni `.atl/skill-registry.md` ni Engram se actualizan. Los agentes desconocen que la nueva skill existe.
4. **Falta de Referencia Clara sobre el Suelo de Upstream:**
   - No existe un campo explícito y controlado donde consultar qué versión de Gentle-AI sirvió de base a la última sincronización, dificultando auditorías de diferencias periódicas.

---

## 2. Alcance (Scope)

### 2.1 Dentro de Alcance (In Scope)

1. **Actualizador Resiliente:**
   - Adaptación de la estrategia de actualización en `internal/update/upgrade/` para que en entornos Windows donde el módulo de Go difiera del import path, o bien descargue un binario precompilado válido de Axiom o bien ejecute un proceso de compilación local/clon controlado.
   - Sustitución de textos heredados en `internal/tui/screens/upgrade_sync.go` sustituyendo referencias a `gentle-ai` por `axiom`.
   - Ajuste de la versión build-time local en `cmd/axiom/main.go` para coherencia con los tags de release.
2. **Encadenamiento Web UI (Upgrade ➔ Sync):**
   - Actualización de `internal/dashboard/service.go` (`RunUpgrade`) para ejecutar la secuencia completa `upgrade` seguida de `sync`, emitiendo un reporte unificado a la interfaz web.
3. **Comando y Motor de Índice de Skills (`axiom skill index`):**
   - Enrutado formal en `cmd/axiom/main.go` de `axiom skill index` con subcomandos `refresh` (regenerar) y `list` (consultar).
   - Extensión de `skillregistry.Regenerate()` para:
     1. Generar `.atl/skill-registry.md`.
     2. Actualizar el tópico `skill-registry` en Engram MCP (`type: config`).
     3. Actualizar la sección `## Skills` en `AGENTS.md` utilizando `filemerge` para respetar el resto del documento.
4. **Gancho en `autoskill`:**
   - Invocación automática de `skillregistry.Regenerate()` dentro de `Manager.Approve()` tras la promoción exitosa de una skill desde el buzón `.axiom/skills/inbox/` hacia `skills/`.
5. **Metadatos de Versión Upstream:**
   - Inclusión del registro `upstream_version: "3.4.0"` en el estado del ecosistema (`state.json` / configuración de gobernanza) para referencia de auditoría manual.

### 2.2 Fuera de Alcance (Out of Scope)

- **Framework automatizado de sincronización con Gentle-AI:** Descartado expresamente por preferencia del usuario; la absorción de novedades de upstream seguirá un proceso manual/guiado bajo demanda, evaluando caso a caso.
- **Modificaciones en el motor de fusión `filemerge`:** Se reutiliza íntegramente la lógica existente de secciones y formatos JSON/YAML/TOML.

---

## 3. Capacidades (Capabilities)

### 3.1 Nuevas Capacidades
- `axiom-skills-index-governance`: Indexación viva, unificada y pedagógica de skills locales, globales y de autoskill, expuesta vía CLI (`axiom skill index`) y reflejada en `AGENTS.md`, `.atl/skill-registry.md` y Engram.

### 3.2 Capacidades Modificadas
- `axiom-updater-resilience`: Robustecimiento de la detección, descarga/compilación y aplicación de nuevas versiones de Axiom en Windows y Unix, asegurando el ciclo completo con `sync` automático en Web UI y TUI.

---

## 4. Requerimientos de Comportamiento (BDD)

### REQ-1: Ejecución Segura de Actualización en Windows
* **DADO QUE** el usuario ejecuta `axiom upgrade` en un entorno Windows,
* **CUANDO** el actualizador detecta una nueva versión en `IGutierrezZ/axiom`,
* **ENTONCES** debe ejecutar una estrategia de actualización que no falle por discrepancia de ruta de módulo en `go.mod`, reemplazando el binario de forma segura y transparente.

### REQ-2: Paridad y Encadenamiento en Web UI
* **DADO QUE** el usuario pulsa *"Actualizar Herramientas"* en el Dashboard Web (`axiom ui`),
* **CUANDO** el servicio completa la actualización binaria con éxito,
* **ENTONCES** debe ejecutar inmediatamente la sincronización (`sync`) de agentes y reportar el estado consolidado de ambas operaciones en la interfaz.

### REQ-3: Indexación Unificada de Skills
* **DADO QUE** se ejecuta `axiom skill index refresh` (o se aprueba una skill),
* **CUANDO** el motor escanea las carpetas de skills de proyecto y usuario,
* **ENTONCES** debe:
  1. Escribir `.atl/skill-registry.md` con todos los metadatos y rutas exactas.
  2. Actualizar el tópico persistente `skill-registry` en Engram.
  3. Reemplazar la tabla `## Skills` de `AGENTS.md` de forma atómica sin alterar ninguna otra sección del archivo.

### REQ-4: Disparo Automático desde Autoskill
* **DADO QUE** el usuario ejecuta `axiom skill approve <nombre>` para validar una skill del buzón,
* **CUANDO** la skill se mueve a `skills/<nombre>/`,
* **ENTONCES** el sistema debe regenerar automáticamente el índice de skills, haciendo que la nueva skill esté disponible de inmediato en `AGENTS.md` para todos los agentes.

---

## 5. Plan de Fases para la Siguiente Sesión

1. **Fase `spec`:** Detalle de contratos CLI, JSON de respuesta en Web UI y estructura exacta de reemplazo en `AGENTS.md`.
2. **Fase `design`:** Arquitectura de la estrategia de actualización en Windows, desacoplamiento de branding y cableado del gancho de autoskill con el registro.
3. **Fase `tasks`:** Desglose atómico de tareas con TDD.
4. **Fase `apply` & `verify`:** Implementación y batería de pruebas unitarias y de integración.
