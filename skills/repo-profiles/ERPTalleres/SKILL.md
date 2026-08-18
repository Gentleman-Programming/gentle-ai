---
name: ERPTalleres-profile
description: "Contrato de ejecución para agentes que modifican el backend ERPTalleres. Trigger: cargar al operar en ERPTalleres."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
  repo_type: backend
  primary_agent: backend-implementer
  tech_stack: ["C# .NET Core 3.1", "EF Core", "CQRS", "Steeltoe"]
  gitlab_path: GP-GCG/erptalleres
---

## 1. Execution Role

Eres un sub-agente (`backend-implementer`, `dev-designer`, `dev-verifier`) o un orquestador operando en **ERPTalleres**.
- Si eres implementador, debes adherirte estrictamente a las reglas arquitectónicas descritas abajo.
- Si el requerimiento altera estructuras de la base de datos subyacente, **debes delegar** a `database-specialist`.

> **Nota:** Este repo pertenece al grupo `GP-GCG` (no `SmartClic` ni `SReasonsERP`). Verificar permisos de acceso antes de operar.

## 2. Language Domain Contract

- **Código:** C# (.NET Core 3.1). Nomenclatura en PascalCase para clases/métodos, camelCase para variables locales.
- **Mensajes de Commit:** En español, semánticos (`feat`, `fix`, `refactor`), claros y directos.
- **Artefactos:** Generated technical artifacts default to English.
- **Comentarios:** Solo para lógica de negocio compleja, no redundantes.

## 3. Architectural Invariants

Backend simple de **Clean Architecture con CQRS estricto**. No tiene Kafka, Redis, ni proyectos auxiliares.

1. **CQRS Estricto:** Los endpoints de lectura (`GET`) solo residen en `Controllers/Query`. Las mutaciones (`POST`, `PUT`, `DELETE`) solo en `Controllers/Command`.
2. **Configuración Externalizada:** Via **Steeltoe Config Server**. Prohibido hardcodear connection strings en appsettings.
3. **Unit of Work:** La capa Repo usa `{Domain}Unit.cs`. No inyectar DbContext directamente en controllers.
4. **Sin mensajería:** Este repo NO tiene Kafka ni Redis. Si se necesita comunicación inter-servicio, escalar al orquestador.
5. **Versiones más antiguas:** Elastic APM 1.11.0, Steeltoe 3.2.3 (vs 3.2.6 en otros repos). No actualizar versiones sin aprobación explícita.

## 4. Directory Structure Contract

```
ERPTalleres.sln
├── Entidades/               → Domain: DTOs, Interfaces, Modelos, Utilitarios
├── Infraestructura/         → DB Context, Configuraciones
├── MSTalleres.API/          → API principal
│   ├── Controllers/Command/ → Endpoints de mutación
│   └── Controllers/Query/   → Endpoints de lectura
├── MSTalleres.Repo/         → Unit-of-Work + Servicios
├── devops/                  → Deployment config
└── Jenkinsfile
```

## 5. Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Mutaciones** | `MSTalleres.API/Controllers/Command/OrdenTrabajoController.cs` | `MSTalleres.API/Controllers/OrdenTrabajoController.cs` |
| **Lecturas** | `MSTalleres.API/Controllers/Query/TallerController.cs` | Poner un GET en `Controllers/Command/` |
| **Configuración** | Steeltoe Config Server + `IConfiguration` | Hardcodear credenciales en appsettings |
| **Acceso a Datos** | `var unit = new OrdenTrabajoUnit(context);` | `await _context.OrdenesTrabajo.ToListAsync()` en controller |

## 6. Testing Contract

- **No hay proyectos de test** detectados en este repo actualmente.
- **Build:** `dotnet build ERPTalleres.sln`.
- Si la tarea requiere tests, crear `MSTalleres.API.Test` siguiendo la convención del ecosistema.

## 7. Output / Delivery Contract

Al finalizar la implementación, el agente debe retornar:
- Confirmación de que los endpoints obedecen la regla de CQRS.
- Evidencia de que el build es exitoso (`dotnet build`).
- Confirmación de que no se expusieron secretos en texto plano.
- Confirmación de que los nuevos tipos residen en `Entidades/`.
