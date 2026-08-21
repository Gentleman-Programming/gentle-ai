<!--
PLANTILLA: skills/technology/<tech-id>/SKILL.md
Categoría cognitiva: "El Con Qué" — conocimiento técnico experto de una
tecnología concreta. Se combina con repo-profiles/: el repo-profile dice
QUÉ tecnología corresponde usar en ese repo; esta skill dice CÓMO usarla
bien. NO es por repositorio ni por back/front como categoría — es por
tecnología puntual (aunque, en la práctica, .NET solo aplica a backends y
Vue 3 solo a frontends, así que queda separado sin que nadie lo decida).

Estructura alineada 1:1 con la definición oficial de Notion ("Taxonomía
Cognitiva de skills/", sección technology/): buenas prácticas, sintaxis
moderna, patrones recomendados, anti-patterns, comandos, APIs,
convenciones — cada uno tiene su propia sección abajo, ninguno se mezcla
con otro.

Quién la llena mejor: el dev que YA usa esta tecnología todos los días en
este equipo — no un resumen genérico de la documentación oficial, sino lo
que este equipo ya aprendió a las malas (incidentes, code review, deuda
técnica evitada).

Reemplaza cada bloque [ENTRE CORCHETES]. Si una sección no aplica a esta
tecnología, escribe "No aplica" explícitamente — no la borres en silencio.
-->
---
name: [tech-id]                     # ej. postgresql, vue3-composition-api, csharp-dotnet-core, typescript, playwright
description: "Trigger: [tech-id]. Technology rules for [una frase: qué tecnología y para qué]."
license: Apache-2.0
metadata:
  author: [tu nombre]
  version: "1.0"
  version_range: "[ej. PostgreSQL 14-16]"
---

# Technology: [Nombre legible + versión mínima]

## 1. Cuándo aplica
[Qué debe declarar el repo-profile (`tech_stack:`) para que esta skill se
cargue. Ejemplo: "Se carga cuando repo-profile.tech_stack incluye
'PostgreSQL'".]

## 2. Matriz de versiones soportadas
| Versión | Estado en este equipo | Notas |
|---|---|---|
| [ej. 16] | [Recomendada / Soportada / En pruebas] | [nota] |
| [ej. 14] | [Legacy — no usar en repos nuevos] | [por qué sigue viva en algunos repos] |

## 3. Buenas prácticas obligatorias
1. [Práctica concreta, con snippet correcto]
   ```[lenguaje]
   [snippet real y correcto, sacado de un repo real de este equipo]
   ```
2. [Práctica 2]
3. [Práctica 3]

## 4. Sintaxis moderna
[Features/sintaxis de la versión actual que este equipo SÍ debe usar en
código nuevo — y la sintaxis vieja que debe dejar de escribirse. Ejemplo:
"Usar `record` en vez de clases DTO planas desde C# 9+"; "Composition API
con `<script setup>` en vez de Options API para componentes nuevos".]
```[lenguaje]
[snippet mostrando la sintaxis moderna recomendada]
```

## 5. Patrones recomendados
[Patrones de diseño específicos de esta tecnología que este equipo adoptó
— distinto de "buenas prácticas" (que son reglas puntuales): esto son
formas de estructurar soluciones completas. Ejemplo: "Repository +
Unit-of-Work para acceso a datos"; "Composables para lógica compartida
entre componentes Vue".]

## 6. Anti-patrones prohibidos
- ❌ [snippet o patrón incorrecto real — idealmente uno que YA causó un
  incidente o code review rechazado en este equipo]
  ```[lenguaje]
  [snippet incorrecto]
  ```
  **Por qué está prohibido**: [razón concreta — performance, seguridad,
  incidente real, deuda técnica ya sufrida]
- ✅ [snippet corregido equivalente]

## 7. Comandos esenciales
| Acción | Comando exacto |
|---|---|
| [ej. correr migraciones] | `[comando real]` |
| [ej. ejecutar tests] | `[comando real]` |
| [ej. lint / formato] | `[comando real]` |
| [ej. ver logs / debug local] | `[comando real]` |

## 8. APIs / SDKs principales
[Las APIs o SDKs que este equipo usa de esta tecnología día a día — no la
API completa oficial, solo la porción real que usan, con el patrón de uso
correcto. Ejemplo: "Entity Framework Core `DbContext` — nunca inyectarlo
directo en un controller, ver sección Anti-patrones".]
| API/SDK | Para qué la usamos | Ejemplo de uso correcto |
|---|---|---|
| [ej. Npgsql] | [driver de conexión a Postgres] | `[snippet corto]` |

## 9. Convenciones de nombres / estructura
[Convenciones específicas de esta tecnología en tu organización — naming de
archivos, carpetas, migraciones, branches de esquema, componentes, etc.]

## 10. Rendimiento y escalabilidad
[Qué SÍ y qué NO hacer para que esto rinda a la escala real de tu
ecosistema. Ejemplo: "Todo índice nuevo en tablas >1M filas requiere
EXPLAIN ANALYZE antes de mergear — ver plantilla de PR."]

## 11. Seguridad específica de esta tecnología
[Riesgos propios de esta tecnología (ej. inyección SQL, XSS en templates,
deserialización insegura) y la mitigación exacta que usa este equipo.
Enlaza con `policy/security` si corresponde, sin repetir su contenido.]

## 12. Errores comunes y cómo diagnosticarlos
| Síntoma | Causa probable | Cómo confirmarlo | Solución |
|---|---|---|---|
| [ej. timeout intermitente en queries] | [ej. falta de índice / lock] | [comando/log a revisar] | [fix] |

## 13. Migración / upgrade de versión
[Si este equipo ya migró de una versión a otra, qué aprendieron. Si nunca
lo han hecho, escribe "Sin experiencia de migración todavía — documentar la
primera vez que ocurra".]

## 14. Cuándo NO usar esta tecnología
[Casos donde, aunque esté disponible, este equipo decidió NO usarla — y
qué usar en su lugar. Si no hay ninguno, "Ninguno conocido".]

## 15. Checklist de validación antes de terminar la tarea
- [ ] [Verificación específica de esta tecnología]
- [ ] [Verificación 2]
- [ ] Sin credenciales/secretos hardcodeados relacionados a esta tecnología

## 16. Referencias cruzadas
[Enlaza explícitamente qué `policy/` y qué `architecture/` suelen
combinarse con esta tecnología, ej. `policy/security`,
`architecture/clean-architecture-cqrs-dotnet`.]
