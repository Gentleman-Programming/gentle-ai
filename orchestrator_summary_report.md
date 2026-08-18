# Resumen de Implementación: Dev Orchestrator (Arquitectura SDD)

Este documento resume el progreso y las implementaciones realizadas en el motor del `Dev Orchestrator` para alinear el código con la **Arquitectura de nuestros agentes** (documentada en `Arquitectura de nuestros agentes 3bf93a9a6f8d80669647c73471dd668b.md`).

---

## 1. Mapeo Arquitectónico (Documentación vs Código)

La documentación definía que el `Dev Orchestrator` (Paso 8 y 9) no era un monolito, sino un conjunto de componentes con responsabilidades únicas que debían ensamblar el contexto para los Agentes y gestionar el ciclo de vida del SDD (State Machine).

Hemos construido este orquestador bajo el paquete `internal/devorchestrator/...` con la siguiente correspondencia:

| Componente (Según Doc) | Ubicación en Código (`internal/`) | Misión |
|------------------------|----------------------------------|--------|
| **Intent Router** | `devorchestrator/intent/router.go` | **Punto de Entrada:** Toma un TDR o requerimiento crudo y lo convierte en un `changeID`, inicializando la carpeta `openspec/changes/<id>` y generando el `explore.md` o `proposal.md` con su frontmatter YAML. |
| **Skill Resolver** | `devorchestrator/skill/resolver.go` | **Mapeo de Habilidades:** Recibe un listado de tecnologías (ej. `["angular"]`) y busca dinámicamente en los directorios `skills/` para extraer las rutas absolutas de los `SKILL.md` requeridos. |
| **Repository Resolver**| `repository/registry.go` | **Contexto de Repo:** Lee el `repository-registry.md` para extraer qué perfil de repositorio (`skills/repo-profiles/...`) inyectar según el proyecto afectado. |
| **Agent Router** | `devorchestrator/router/router.go` | **Ensamblaje XML:** Utiliza plantillas `text/template` para inyectar de forma segura todo el contexto (`<repo_profiles>`, `<architecture_profile>`, etc) en el prompt del Agente. |
| **Context Builder** | `devorchestrator/context/builder.go`| **Estructura Híbrida:** Construye el `Scope` y `Package` de la petición. Soporta inyección dual (Repo Profile + Architecture Profile) para abarcar proyectos Greenfield. |
| **State / Gate Mgr** | `sddstatus/status.go` | **State Machine:** Evalúa qué artefactos (proposal, specs, tasks, etc.) existen y decide qué fase sigue (`PhasePropose`, `PhaseApply`, etc.), emulando el diagrama de estados documentado. |
| **Architecture Catalog**| `skills/architecture/` | **Blueprints (Paso 10):** Taxonomía añadida para instanciar repositorios desde cero. Se agregó `spring-rest-service/SKILL.md` como proxy funcional. |

---

## 2. Archivos Creados / Modificados

A lo largo de la implementación, se han modificado o creado los siguientes archivos clave:

### Componentes Core (Nuevos)
- `[NEW]` `internal/devorchestrator/intent/router.go`: Lógica de enrutamiento inicial.
- `[NEW]` `internal/devorchestrator/intent/router_test.go`: Tests unitarios.
- `[NEW]` `internal/devorchestrator/skill/resolver.go`: Lógica de resolución de rutas.
- `[NEW]` `internal/devorchestrator/skill/resolver_test.go`: Tests unitarios.
- `[NEW]` `skills/architecture/spring-rest-service/SKILL.md`: Catálogo de arquitectura base.

### Modificaciones Estructurales
- `[MODIFY]` `internal/devorchestrator/orchestrator.go`: Ahora envuelve al `IntentRouter` y al `SkillResolver`, actuando como una verdadera "Fachada" (Facade Pattern). `GenerateContextForAgent` y `GenerateAgentPrompt` fueron actualizados para recibir el identificador de arquitectura Greenfield.
- `[MODIFY]` `internal/devorchestrator/context/builder.go`: Se añadieron los campos `Architecture` y `ArchitectureProfile` a las estructuras de transporte.
- `[MODIFY]` `internal/devorchestrator/router/router.go`: Se extendió el `promptTemplate` para pintar de forma condicional el bloque `<architecture_profile>`.
- `[MODIFY]` `internal/devorchestrator/orchestrator_test.go`: Mockeada la inyección de arquitecturas y skills locales.

---

## 3. Trazabilidad de Commits

Para mantener el historial ordenado como se solicitó, los cambios se empaquetaron lógicamente en los siguientes commits locales:

1. **`feat(orchestrator): implement Greenfield Routing and Architecture Catalog support`**
   - *Impacto:* Resolvió el Punto 10 (Architecture Catalog). Permitió al `Agent Router` y al `Context Builder` entender y empaquetar directivas abstractas para proyectos que no existen en el Registry (o proyectos híbridos que requieren un módulo nuevo dentro de un repo existente).
2. **`feat(orchestrator): implement Intent Router and Skill Resolver`**
   - *Impacto:* Cerró el GAP de la arquitectura (Pasos 8 y 9). Agregó la entrada principal para la toma de decisiones basada en TDR (Intent) y la inferencia dinámica de carpetas de Skills, eliminando las dependencias estáticas que había.

---

## Conclusión

El `Dev Orchestrator` está ahora **completamente maduro en su fase de Planificación y Desarrollo**. La inyección de dependencias (Skills, Arquitecturas, Repositorios) opera de manera dinámica, y la máquina de estados reacciona a los artefactos generados. 

El único bloque restante para cerrar el ciclo End-to-End P1 es **Finalize Delivery / Handoff** (la integración post-Verify que enviará la orden a GitLab para crear el Merge Request mediante la API o CLI y llamará a las herramientas de CI).
