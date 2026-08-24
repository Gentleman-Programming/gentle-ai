<!--
PLANTILLA: skills/architecture/<pattern-id>/SKILL.md
Categoría cognitiva: "Architecture Catalog" — cómo queremos construir SISTEMAS NUEVOS.

Diferencia con repo-profiles/: repo-profiles describe un repo que YA EXISTE
("qué tenemos"); architecture/ describe un patrón de referencia para decidir
CÓMO empezar un repo que TODAVÍA NO existe ("cómo queremos construir").
Lo consume solution-architect durante Greenfield Discovery (ver
GenerateContextForAgent's architectureID -> skills/architecture/<id>/SKILL.md)
y, si el blueprint se aprueba, project-bootstrap lo usa para scaffoldear.

Reemplaza cada bloque [ENTRE CORCHETES] con contenido real y específico de
TU stack. Borra este bloque de comentario al terminar. No dejes ninguna
sección con "TODO" o contenido genérico tipo "com.example" — si no sabes
la respuesta todavía, escribe explícitamente "PENDIENTE: <qué falta decidir
y quién debe decidirlo>" en vez de inventar.
-->
---
name: [id-del-patron]                     # ej. spring-rest-service, vue3-microfrontend, event-driven-worker
description: "Trigger: [id-del-patron]. Reference architecture for [una frase: qué tipo de sistema construye este patrón]."
license: Apache-2.0
metadata:
  author: [tu nombre]
  version: "1.0"
---

# Architecture Profile: [Nombre legible del patrón]

## 1. Cuándo usar este patrón
[Bajo qué condiciones Greenfield Discovery debe elegir este patrón y no otro.
Sé específico: tipo de carga esperada, sincronía/asincronía, quién lo consume,
restricciones de equipo/infra. Ejemplo real, no genérico:
"Usar cuando el nuevo servicio expone un CRUD transaccional consumido por un
solo microfrontend, con <100 req/s esperados y sin necesidad de eventos
asíncronos."]

## 2. Stack tecnológico de referencia
| Capa | Tecnología | Versión mínima | Skill de technology/ relacionada |
|---|---|---|---|
| [ej. Backend] | [ej. Spring Boot] | [ej. 3.2] | `technology/[slug]` |
| [ej. Persistencia] | [ej. PostgreSQL] | [ej. 15] | `technology/postgresql` |

## 3. Estructura de carpetas / capas
```
[árbol de directorios REAL de un repo que ya siga este patrón, o el árbol
exacto que project-bootstrap debe generar — no uses "com.example", usa el
namespace/paquete real de tu organización]
```

## 4. Reglas de capas (invariantes arquitectónicos)
1. [Regla verificable, ej. "los controllers SOLO delegan a servicios de
   aplicación, nunca acceden a repositorios directamente"]
2. [Regla verificable]
3. [Qué es lo que un `dev-verifier` debería poder chequear automáticamente]

## 5. Contratos e integración
[Cómo expone este patrón sus APIs/eventos hacia otros sistemas: REST/GraphQL/
gRPC/eventos. Formato de versionado de contrato. Dónde viven los schemas.]

## 6. Persistencia
[Patrón de acceso a datos (repository/ORM/query builder), estrategia de
migraciones (herramienta, convención de nombres de archivo), y qué policy/
y technology/ de base de datos aplica. Enlaza explícitamente:
`policy/[...]`, `technology/[...]`]

## 7. Testing esperado
[Qué tipos de test son obligatorios para este patrón (unit/integration/
contract/e2e), en qué carpeta viven, y qué comando los ejecuta.]

## 8. Checklist de Project Bootstrap
Lo que `project-bootstrap` DEBE generar al scaffoldear un repo nuevo con este patrón:
- [ ] [ej. README con el árbol de la sección 3]
- [ ] [ej. pipeline CI mínimo]
- [ ] [ej. Dockerfile / manifest de despliegue]
- [ ] [ej. entrada en docs/repository-registry.md con `type` = este patrón]

## 9. Ejemplo real de referencia
[Enlace o ruta a un repositorio real que ya implementa este patrón
correctamente. Si no existe todavía ninguno, decláralo:
"PENDIENTE: aún no hay un repo de referencia; el primero que se construya
con este patrón debe registrarse aquí."]
