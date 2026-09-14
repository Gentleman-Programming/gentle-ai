# Axiom Enterprise — Visión Funcional y Arquitectura Maestra

> **Proyecto:** Axiom (Fork y evolución paralela de Gentle-AI)  
> **Propósito:** Transformar el flujo Spec-Driven Development (SDD) individual en un entorno de ingeniería colaborativo, multi-rol, multi-repositorio y con adopción orgánica de proyectos legados.

---

## 1. Visión y Objetivos Fundacionales

Axiom amplía Gentle-AI para que su metodología SDD pueda ser utilizada por equipos con responsabilidades diferenciadas, múltiples repositorios, distintas herramientas de IA y proyectos con diversos grados de madurez documental.

El objetivo no es sustituir la esencia de Gentle-AI ni reinventar un framework SDD completo desde cero, sino conservar sus fortalezas deterministas y añadir:
1. **Trabajo colaborativo y multi-rol:** Desacoplo por responsabilidades y ejecución paralela de tareas de implementación.
2. **Topologías de Workspace Flexibles:** Soporte nativo de monorrepositorios y multirrepositorios bajo una carpeta maestra común.
3. **Adopción Orgánica ("Zero-Doc Cold Start"):** Capacidad de entrar en proyectos sin documentación previa e ir generando la especificación viva conforme se cierran incrementos.
4. **Comprensión Semántica Profunda:** Integración con herramientas de análisis de código (Serena / CodeGraph).
5. **Autoskills Supervisadas:** Generación y sugerencia de skills técnicas y de proyecto con aprobación humana previa.
6. **Experiencia de Usuario Simplificada:** Servidor local embebido con Web Dashboard (`axiom ui`) agnóstico a cualquier IDE.

---

## 2. Los Pilares de Axiom

### 2.1. Workflow por Responsabilidades y Fan-Out Multi-Rol

El flujo SDD se divide en fases asignadas a responsabilidades claras:
- **Responsabilidad Funcional / Producto:** `Explore` y `Propose`.
- **Responsabilidad de Arquitectura / Liderazgo Técnico:** `Design`.
- **Responsabilidades de Implementación:** `Tasks`, `Apply` y `Verify` (por cada rol).
- **Responsabilidad de Validación Final / Cierre:** `Archive`.

```text
Explore ➔ Propose ➔ Design
                      ├── Tasks [frontend] ➔ Apply [frontend] ➔ Verify [frontend]
                      ├── Tasks [backend]  ➔ Apply [backend]  ➔ Verify [backend]
                      └── Tasks [devops]   ➔ Apply [devops]   ➔ Verify [devops]
                                             ↓
                                          Archive
```

- En `Design` se identifican los roles participantes (de 1 a N).
- Cada rol recibe un alcance concreto, dependencias, entregables esperados y criterios de validación.
- `Archive` no cierra el cambio hasta que todos los roles obligatorios hayan superado su verificación.

### 2.2. Continuidad mediante Handoffs Estructurados

Cada cambio de fase o relevo entre personas/agentes genera un artefacto estructurado de traspaso (`handoff.md`) que incluye:
- Trabajo realizado.
- Artefactos creados o modificados.
- Decisiones tomadas.
- Riesgos y bloqueos identificados.
- Preguntas abiertas.
- Roles afectados.
- Siguiente acción recomendada.

### 2.3. Topologías de Repositorio y Repositorio Canónico de Especificaciones

Todo proyecto gestionado por Axiom reside en una **carpeta maestra local común** (`workspace_root`). Se soportan tres topologías canónicas:

1. **Monorepo Embebido (`monorepo-embedded`):**
   - Un único repositorio Git donde coexisten código y especificaciones (`openspec/`).
2. **Monorepo Desacoplado (`monorepo-decoupled`):**
   - El código vive en un repositorio Git monorrepo, pero las especificaciones y contratos residen en un repositorio Git independiente dentro de la carpeta maestra.
3. **Multirepo Federado (`multirepo`):**
   - Múltiples repositorios de código divididos por tecnologías o dominios.
   - **Regla estricta obligatoria:** Es indispensable la existencia de un repositorio canónico de especificaciones dentro de la carpeta maestra común (ej. `especificacion/`). Si el proyecto no lo tiene, Axiom lo inicializa automáticamente.

### 2.4. Mapeo Roles ➔ Repositorios (1 a N)

La configuración `axiom.yaml` vincula de 1 a N roles con sus respectivos repositorios locales:
```yaml
workspace:
  name: "PlataformaEnterprise"
  topology: "multirepo"       # monorepo-embedded | monorepo-decoupled | multirepo
  specs_repository: "especificacion"
  root: "."

roles:
  frontend:
    name: "Equipo Frontend"
    repositories:
      - path: "frontend"
    tech: ["react", "typescript", "tailwind"]

  backend:
    name: "Equipo Backend"
    repositories:
      - path: "backend"
    tech: ["go", "postgresql"]
```

### 2.5. Comprensión Semántica de Código (Serena & CodeGraph)

Para evitar que los agentes dependan únicamente de búsquedas léxicas (`grep`) o de documentación perfecta, Axiom integra conectores semánticos locales (Serena MCP con LSP/Tree-sitter y CodeGraph) para consultar jerarquías de tipos, llamadas y dependencias entre paquetes y repositorios en las fases de `Explore` y `Design`.

### 2.6. Autoskills en Dos Capas (Con Aprobación Humana)

- **Capa 1 (Catálogo Estándar):** Al detectar las tecnologías de un rol, propone skills estándar curadas.
- **Capa 2 (Minería Heurística de Repositorio):** Un analizador examina el código del rol en busca de patrones repetidos y propone borradores de `SKILL.md`.
- **Revisión Humana:** Ninguna skill se vuelve obligatoria sin la validación previa de un humano en la Web UI o vía CLI.

### 2.7. Archive como Motor de Documentación Viva

- **Proyectos con documentación existente:** `Archive` mantiene sincronizados los documentos canónicos reflejando los deltas del cambio.
- **Proyectos sin documentación inicial ("Cold Start"):** `Archive` genera y consolida incrementalmente especificaciones vivas funcionales y técnicas en cada cierre, construyendo el mapa completo del sistema de manera orgánica.

### 2.8. Interfaz Web Local Agnóstica (`axiom ui`)

Servidor HTTP ligero en Go embebido (`//go:embed`) que despliega un dashboard web en el navegador del usuario:
- Visor de repositorios y salud del workspace.
- Tablero Kanban de incrementos y estado de los roles.
- Visor y aprobación de handoffs.
- Buzón de revisión de nuevas skills propuestas.

---

## 3. Principios de Gobernanza y Límites Iniciales

- **Idioma Oficial:** Español (castellano) en todas las comunicaciones, artefactos, planes y especificaciones.
- **Sin Dependencia Cloud Prematura:** Todo opera en local sobre el sistema de ficheros, Git y CLI/servidor HTTP local.
- **Determinismo y Verificación:** Se mantiene la premisa de Gentle-AI: *verificar vence a generar*.
