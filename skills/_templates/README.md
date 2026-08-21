# Plantillas de `skills/`

Cada carpeta de `skills/` representa una dimensión distinta del contexto
cognitivo que `dev-orchestrator` compone dinámicamente para un sub-agente.
Fuente de la taxonomía: Notion — "Taxonomía Cognitiva de `skills/`" y
"Arquitectura de nuestros agentes".

| Carpeta | Dimensión | Pregunta que responde | Granularidad | Plantilla |
|---|---|---|---|---|
| `agents/` | Identidad | ¿Quién soy? | Compartido (1 por rol) | (no aplica — thin pointers a `internal/assets/claude/agents/*.md`) |
| `repo-profiles/` | Contexto | ¿Dónde estoy? | **Por repo (1:1)** — la ÚNICA capa que va por repositorio | [repo-profile.template.md](repo-profile.template.md) |
| `architecture/` | Patrón de referencia | ¿Cómo construyo un sistema nuevo? | Compartido (1 patrón : N repos) | [architecture.template.md](architecture.template.md) |
| `methodology/` | Proceso | ¿Cómo trabajo? | Compartido (global) | [methodology.template.md](methodology.template.md) |
| `technology/` | Capacidad técnica | ¿Con qué trabajo? | Compartido (1 tecnología : N repos) | [technology.template.md](technology.template.md) |
| `policy/` | Restricciones | ¿Qué límites nunca puedo romper? | Compartido (global) | [policy.template.md](policy.template.md) |
| `domain/` | Negocio | ¿Qué significa esto funcionalmente? | Compartido (1 contexto de negocio : N repos) | [domain.template.md](domain.template.md) |
| `legacy/` | Compatibilidad | ¿Qué comportamiento antiguo debo soportar? | Compartido (caso por caso) | [legacy.template.md](legacy.template.md) |

**Regla de oro**: solo `repo-profiles/` se crea uno por repositorio. Todo lo
demás es compartido — si dos repos necesitan la misma regla de negocio o
tecnología, referencian la MISMA skill, nunca se duplica. Lo específico de
un repo va como una sección dentro de su propio `repo-profiles/<repo>/SKILL.md`
(sección 8 de la plantilla: "Excepciones locales"), no como una copia
completa del archivo compartido.

## Estado real (auditado en este repo)

- **29 de 30** `repo-profiles/` ya tienen contenido específico y real.
  **Pendiente**: `payments-api` sigue siendo un stub genérico — y es el repo
  que `docs/architecture-catalog.md` cita como la referencia oficial del
  patrón Clean Architecture + CQRS para los otros 6 backends .NET. Prioridad #1.
- Solo 7 de 29 repo-profiles reales declaran `tech_stack:`/`repo_type:` en
  el frontmatter (los backends .NET). Los 14 `erp-mf-*` y el resto todavía
  no tienen esos campos estructurados, aunque su prosa sea buena.
- **Ningún** repo-profile (ni los 7 buenos) declara `architecture_pattern:`
  ni `domains:` — el enlace que conecta un repo con su patrón de
  arquitectura y su(s) contexto(s) de negocio no existe como campo
  estructurado en ninguno todavía. `repo-profile.template.md` ya lo incluye;
  falta rellenarlo en los 30 repos existentes.
- `domain/`, `technology/`, `policy/`, `methodology/` siguen siendo stubs
  100% genéricos (excepto `architecture/spring-rest-service`, que tiene
  contenido real pero mínimo).

## Regla de uso

1. Copia la plantilla correspondiente a `skills/<categoría>/<id>/SKILL.md`.
2. Reemplaza cada bloque `[ENTRE CORCHETES]` con contenido real y específico
   de tu organización — nunca dejes texto genérico tipo "stub capability"
   o `com.example`.
3. Si algo no está decidido todavía, escríbelo explícitamente como
   `PENDIENTE: ...` — nunca inventes una regla de negocio, arquitectura o
   policy que no esté confirmada.
4. Borra el bloque de comentario HTML del inicio de la plantilla al terminar.

## Jerarquía de autoridad (para resolver conflictos entre skills)

```
policy > repo-profile > domain > methodology > technology > preferencia del agente
```

Una skill de `technology/` nunca puede justificar violar una `policy/`; una
recomendación genérica de `technology/` nunca gana sobre una decisión
específica del `repo-profile`.

## Plantilla de elicitación de requerimientos (BookStack, no técnica)

`elicitacion-requerimiento.md` NO es un `SKILL.md` — es copia fiel de la
plantilla **real** que tu equipo ya usa en BookStack (confirmada ahí mismo:
14 páginas reales en los libros "Pagos de Suscripción", "Proyecto
Notificaciones" y "Restaurantes" la siguen). La llena quien pide el
requerimiento (negocio/PM), con Dev y QA validando al final.

Organización en BookStack (la que ya usan, no algo nuevo):
- **1 Libro por proyecto/módulo grande** — el libro mismo es la "página
  principal" que agrupa todo; BookStack ya lista sus páginas automáticamente.
- Opcionalmente, **capítulos** dentro del libro para sub-módulos.
- **1 página por requerimiento pequeño**, con esta plantilla completa.

Cuando una página queda "Validada con Dev" y "Validada con QA" (checklist
final de la plantilla), es lo que se entrega como intent a
`dev-orchestrator` — él decide solo, técnicamente, a qué repo(s) toca y en
qué fase SDD entra.
