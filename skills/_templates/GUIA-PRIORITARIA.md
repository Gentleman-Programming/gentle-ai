# Guía prioritaria de plantillas para dev-orchestrator

Estas son las 4 categorías que más urgen para que `dev-orchestrator` tenga
contexto real al trabajar. Cada una tiene: **dónde vive el archivo**,
**cuántos hacen falta**, **cómo lo consume el código hoy** (verificado
contra `internal/devorchestrator/orchestrator.go` y
`internal/devorchestrator/skill/resolver.go` — no es teoría, es lo que el
código realmente hace), y el link a su plantilla completa.

## Principio general (repetido a propósito, es la regla más importante)

Solo `repo-profiles/` va **por repositorio**. `architecture/`, `domain/` y
`technology/` van **compartidos** — un archivo por patrón/módulo/tecnología,
referenciado por todos los repos que aplique, nunca duplicado.

---

## 1. `repo-profiles/` — "¿Dónde estoy?"

**Vive en**: `skills/repo-profiles/<repo-slug>/SKILL.md` — el `repo-slug`
es exactamente el de `docs/repository-registry.md` (la fuente de verdad de
qué repositorios existen).

**Cuántos**: **exactamente 1 por repositorio real** — hoy son 30, listados
en `docs/repository-registry.md`. Ya 29 de 30 tienen contenido real y
bueno (`ERPFinanzasCore`, `erp-mf-logistica` son los mejores ejemplos).
**Falta 1**: `payments-api` sigue siendo un stub — y es justo el repo que
`docs/architecture-catalog.md` cita como referencia oficial del patrón
Clean Architecture + CQRS. Prioridad #1 de contenido.

**Cómo lo consume el código hoy**: `GenerateContextForAgent` (paso "2.
Repository Resolver") valida cada nombre de repo contra
`docs/repository-registry.md`; si es válido, lee el archivo de `Profile`
completo como texto y lo mete en `combinedRepoProfile`, que termina en
`context.Package.RepoProfile` — el sub-agente lo recibe como contexto de
prosa antes de trabajar. Si el repo no está en el registro, se descarta
silenciosamente (por eso es tan importante que el registro esté al día).

**Plantilla**: [repo-profile.template.md](repo-profile.template.md)

---

## 2. `architecture/` — "¿Cómo construyo algo nuevo?"

**Vive en**: `skills/architecture/<pattern-id>/SKILL.md` — el `pattern-id`
debe coincidir exactamente con el nombre que el código espera recibir como
`architectureID`.

**Cuántos**: **1 por patrón real distinto**, no por repo. Hoy son
**exactamente 2**, ya documentados (sin inventar nada nuevo) en
`docs/architecture-catalog.md`:
- `clean-architecture-cqrs-dotnet` (7 repos backend .NET la siguen)
- `single-spa-microfrontends` (14 repos `erp-mf-*` la siguen)

Ahora mismo existe además `skills/architecture/spring-rest-service/SKILL.md`
— es un ejemplo genérico que **no corresponde a ningún patrón real de este
ecosistema** (usa `com.example`, no CQRS). Debe reemplazarse por los 2 de
arriba, migrando el contenido ya escrito en `docs/architecture-catalog.md`.

**Cómo lo consume el código hoy**: `GenerateContextForAgent` recibe
`architectureID` como parámetro; si no está vacío, lee
`skills/architecture/<architectureID>/SKILL.md` y lo mete en
`ArchitectureProfile` dentro del `context.Package`. Se usa sobre todo
durante Greenfield Discovery (cuando `solution-architect` decide qué patrón
aplicar a un repo nuevo) pero cualquier implementador que trabaje bajo un
patrón aprobado también lo recibe.

**Plantilla**: [architecture.template.md](architecture.template.md)

---

## 3. `domain/` — "¿Qué significa esto para el negocio?"

**Vive en**: `skills/domain/<modulo-negocio>/SKILL.md`.

**Cuántos**: **~6-8**, uno por módulo de negocio real (contexto acotado,
ni por repo ni por entidad individual). Mapeado a tu ecosistema:
```
domain/
├── facturacion-sunat/
├── logistica-inventarios/
├── planillas/
├── punto-de-venta/
├── finanzas-contabilidad/
└── pagos/
```
Si un módulo crece demasiado, se sub-divide en carpetas dentro del mismo
módulo — nunca por defecto, solo cuando de verdad ya no se puede leer junto.

**Cómo lo consume el código hoy**: a diferencia de `repo-profiles/` y
`architecture/`, **hoy no hay resolución automática**. `skill.Resolver`
(`internal/devorchestrator/skill/resolver.go`) solo busca, dentro de
`skills/`, una carpeta cuyo nombre coincida EXACTAMENTE con lo que venga en
`requiredSkills` — un parámetro que hoy alguien debe nombrar a mano al
llamar `GenerateContextForAgent` (ej. `requiredSkills: ["facturacion-sunat"]`).
No existe todavía un mecanismo que, viendo el repo o la tarea, decida solo
qué dominio cargar — ese es un gap de código conocido, documentado, y fuera
de esta ronda de documentación.

**Plantilla**: [domain.template.md](domain.template.md)

---

## 4. `technology/` — "¿Con qué trabajo?"

**Vive en**: `skills/technology/<tech-id>/SKILL.md`.

**Cuántos**: **~5-6**, uno por tecnología real que usan — no por repo, no
por back/front como categoría (aunque en la práctica .NET solo aplica a
backends y Vue 3 solo a frontends, así que queda separado sin que nadie lo
decida a propósito):
```
technology/
├── csharp-dotnet-core/
├── postgresql/
├── sql-server-tunning/
├── vue3-composition-api/
└── (kafka / steeltoe si de verdad amerita su propia skill)
```

**Cómo lo consume el código hoy**: igual mecanismo que `domain/` — vía
`requiredSkills` nombrado a mano, mismo gap de auto-resolución pendiente.
La única excepción parcialmente automática: si el artefacto declara
`db_impact: simple` y el agente es `backend-implementer`, el código agrega
solo `database-specialist` a `requiredSkills` (es la única inferencia real
que existe hoy en todo el sistema).

**Plantilla**: [technology.template.md](technology.template.md)

---

## Resumen para repartir al equipo

| Categoría | Quién escribe | Cuántos archivos | Complejidad esperada |
|---|---|---|---|
| `repo-profiles/` | Dev que conoce ese repo a fondo | 30 (falta 1: `payments-api`) | 1-2 páginas, con árbol de carpetas y tabla ✅/❌ real |
| `architecture/` | Dev senior/arquitecto | 2 (migrar, no inventar) | 1 página, igual de detallado que `docs/architecture-catalog.md` |
| `domain/` | Negocio redacta en BookStack (elicitación) → técnico traduce aquí | 6-8 | Media página: glosario + reglas numeradas + 1 ejemplo |
| `technology/` | Dev que usa esa tecnología todos los días | 5-6 | 1 página: buenas prácticas, sintaxis moderna, anti-patrones, comandos, APIs |

**Gap de código pendiente** (no bloquea escribir contenido ahora, pero
hay que saberlo): `domain/` y `technology/` no se cargan solos todavía —
alguien tiene que nombrarlos al invocar al orquestador. `repo-profiles/` y
`architecture/` sí tienen una ruta de código que los resuelve automático
hoy mismo.
