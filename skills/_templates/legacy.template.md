<!--
PLANTILLA: skills/legacy/<old-skill-id>/SKILL.md
Categoría cognitiva: "El Cementerio / Transición" — skills antiguas,
deprecated, o mantenidas solo por compatibilidad temporal. Nunca debe usarse
para desarrollo nuevo salvo razón explícita. Ciclo de vida esperado:
legacy -> migración -> methodology/technology/policy moderna -> eliminación.

Reemplaza cada bloque [ENTRE CORCHETES]. Si esta skill no tiene todavía un
reemplazo moderno, decláralo explícitamente en la sección 5 en vez de
dejarla vacía — un legacy sin plan de salida es deuda técnica invisible.
-->
---
name: [old-skill-id]
description: "Trigger: [old-skill-id]. DEPRECATED — [una frase: qué hacía y por qué ya no es la forma recomendada]."
license: Apache-2.0
metadata:
  author: [tu nombre]
  version: "1.0"
  deprecated_since: [fecha o versión]
  migration_status: [not-started | in-progress | blocked]
---

# Legacy: [Nombre legible]

## 1. Por qué existe
[Contexto histórico: qué resolvía originalmente y por qué se dejó de
recomendar. 2-3 frases, sin juicios de valor — solo el hecho.]

## 2. Estado de migración
| Parte del sistema | Ya migrada | Pendiente |
|---|---|---|
| [ej. repo X] | [sí/no] | [qué falta exactamente] |

## 3. Cuándo SÍ usar esto todavía
[Casos legítimos y ACOTADOS de compatibilidad — si no hay ninguno, escribe
"Ninguno — solo existe por compatibilidad histórica, no debe invocarse en
trabajo nuevo bajo ninguna circunstancia."]

## 4. Cuándo NO usar esto
[Para todo lo demás. Sé explícito: "Para cualquier tarea nueva, usar
[reemplazo] en su lugar — ver sección 5."]

## 5. Reemplazo moderno
[Enlace directo a la skill de methodology/technology/policy que sustituye a
esta. Si todavía no existe reemplazo, escribe explícitamente:
"PENDIENTE: no existe reemplazo moderno todavía; no eliminar esta skill
hasta que exista uno."]

## 6. Plan de eliminación
[Condición exacta y verificable bajo la cual esta skill puede borrarse —
ej. "cuando 0 repos activos referencien este flujo en su repo-profile".]
