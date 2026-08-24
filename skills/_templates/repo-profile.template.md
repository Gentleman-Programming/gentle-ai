<!--
PLANTILLA: skills/repo-profiles/<repo-slug>/SKILL.md
Categoría cognitiva: "El Dónde" — ÚNICA capa que va por repositorio (1:1),
nunca compartida. Basada en los 2 mejores ejemplos reales de este repo:
ERPFinanzasCore (backend .NET) y erp-mf-logistica (frontend Vue).

Antes de llenar esto:
1. Confirma la fila exacta en docs/repository-registry.md (gitlab_path,
   repo-slug, Owner, Type). Si el repo no está ahí, agrégalo primero — el
   registro es la fuente de verdad, este perfil no debe contradecirlo.
2. Revisa docs/architecture-catalog.md y elige el `architecture_pattern`
   que le corresponde (o "greenfield-pending" si aún no se decide).
3. Identifica a qué contexto(s) de negocio de skills/domain/ pertenece este
   repo — si el dominio todavía no existe como skill, créalo primero con
   domain.template.md (o pide a negocio que llene
   business-rules-capture.bookstack-template.md).

Los campos `architecture_pattern` y `domains` son el ENLACE que hoy falta
en los 30 repo-profiles existentes (ninguno lo declara todavía) — sin ellos,
ni un humano ni dev-orchestrator pueden saber de forma consistente qué
skills compartidas aplican a este repo. NO los dejes vacíos: si de verdad no
aplica ninguno, escribe `[]` explícitamente, nunca omitas el campo.
-->
---
name: [repo-slug]-profile                       # ej. payments-api-profile
description: "Contrato de ejecución para agentes que modifican [repo-slug]. Trigger: cargar al operar en [repo-slug]."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: [tu nombre]
  version: "1.0"
  delegate_only: true
  gitlab_path: [org/nombre-exacto-en-gitlab]     # DEBE coincidir con docs/repository-registry.md
  repo_type: [backend | frontend | single-spa-mfe | platform | legacy]
  primary_agent: [backend-implementer | frontend-implementer | database-specialist | ...]
  tech_stack: ["[lenguaje+versión]", "[framework]", "[ORM/estado]", "[mensajería si aplica]"]
  architecture_pattern: [id exacto de docs/architecture-catalog.md, ej. "clean-architecture-cqrs-dotnet", o "greenfield-pending"]
  domains: ["[slug de skills/domain/<id> que aplica]"]   # [] explícito si de verdad no aplica ninguno todavía
---

## 1. Execution Role
[Confirma el rol: "Eres un sub-agente (backend-implementer / frontend-implementer
/ database-specialist / dev-verifier) o un orquestador operando en [repo-slug]."
Indica explícitamente cuándo debe delegar en vez de actuar directo — ej.
"si la tarea altera la base de datos, DEBES delegar a database-specialist,
no correr migraciones por tu cuenta."]

## 2. Language Domain Contract
- **Código:** [lenguaje + convención de nombres exacta, ej. "C# .NET 8,
  PascalCase clases/métodos, camelCase variables locales"]
- **Commits/PRs:** [idioma + convención semántica exacta]
- **Artefactos técnicos:** [idioma por defecto para apply-progress/reportes]
- **Comentarios:** [cuándo sí/no comentar código]

## 3. Architectural Invariants
[Copia/adapta las invariantes del `architecture_pattern` elegido arriba
(ver docs/architecture-catalog.md) y hazlas específicas a ESTE repo con
nombres de carpeta/clase reales. Numeradas, verificables, sin ambigüedad.]

1. [Invariante 1, con la ruta exacta donde se aplica]
2. [Invariante 2]

## 4. Directory Structure Contract
```
[árbol REAL de este repo, no genérico — copiado de `ls`/`tree` real,
anotando qué va en cada carpeta]
```

## 5. Code Writing Rules
| Criterio | Ejemplo ✅ | Anti-ejemplo ❌ |
|---|---|---|
| [ej. Ubicación de mutaciones] | [ruta/código real correcto] | [ruta/código real incorrecto] |
| [criterio 2] | [✅] | [❌] |

## 6. Testing Contract
- [Comando de build exacto]
- [Comando de test exacto, o "No hay test projects detectados — si la tarea
  los requiere, crear siguiendo el patrón de [otro repo del mismo patrón]"]

## 7. Reglas de negocio que aplican aquí
[NO copies aquí las reglas completas — enlaza a la(s) skill(s) de
skills/domain/ declaradas en `domains:` del frontmatter. Ejemplo:
"Este repo implementa `domain/reglas-facturacion-sunat` — ver esa skill
para las reglas completas. Aquí solo la nota de cómo se traduce a este
repo específicamente, si hace falta."]

## 8. Excepciones locales a skills compartidas
[Si este repo se desvía deliberadamente de una `technology/`, `policy/` o
`architecture/` compartida, decláralo aquí explícitamente con el motivo —
nunca en silencio. Si no hay ninguna excepción, escribe "Ninguna conocida."]

## 9. Output / Delivery Contract
Al finalizar, el agente debe retornar:
- [Confirmación específica de esta arquitectura, ej. "endpoints nuevos
  obedecen CQRS"]
- Evidencia de build/test exitoso.
- Confirmación de que no se expusieron secretos.
