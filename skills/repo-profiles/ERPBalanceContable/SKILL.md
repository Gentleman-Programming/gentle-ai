---
name: ERPBalanceContable-profile
description: "Contrato de ejecución para agentes que modifican el backend ERPBalanceContable. Trigger: cargar al operar en ERPBalanceContable."
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
  gitlab_path: SmartClic/ERPBalanceContable
---

## 1. Execution Role

Eres un sub-agente (`backend-implementer`, `dev-designer`, `dev-verifier`) o un orquestador operando en **ERPBalanceContable**.
- Si eres implementador, debes adherirte estrictamente a las reglas arquitectónicas descritas abajo.
- Si el requerimiento altera estructuras de la base de datos subyacente, **debes delegar** a `database-specialist`.

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
4. **Sin mensajería:** Este repo NO tiene Kafka ni Redis. Si se necesita comunicación inter-servicio, escalar al orquestador para evaluar si corresponde agregar Kafka.

## 4. Directory Structure Contract

```
ERPBalanceContable.sln
├── Entidades/                  → Domain: DTOs, Interfaces, Modelos, Utilitarios
├── Infraestructura/            → DB Context, Configuraciones
├── MSBalanceContable.API/      → API principal
│   ├── Controllers/Command/   → Endpoints de mutación
│   └── Controllers/Query/     → Endpoints de lectura
├── MSBalanceContable.Repo/     → Unit-of-Work + Servicios
├── devops/                     → Deployment config
└── Jenkinsfile
```

## 5. Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Mutaciones** | `MSBalanceContable.API/Controllers/Command/AsientoController.cs` | `MSBalanceContable.API/Controllers/AsientoController.cs` |
| **Lecturas** | `MSBalanceContable.API/Controllers/Query/BalanceController.cs` | Poner un GET en `Controllers/Command/` |
| **Configuración** | Steeltoe Config Server + `IConfiguration` | Hardcodear credenciales en appsettings |
| **Acceso a Datos** | `var unit = new AsientoUnit(context);` | `await _context.Asientos.ToListAsync()` en controller |

## 6. Testing Contract

- **No hay proyectos de test** detectados en este repo actualmente.
- **Build:** `dotnet build ERPBalanceContable.sln`.
- Si la tarea requiere tests, crear `MSBalanceContable.API.Test` siguiendo la convención del ecosistema.

## 7. Output / Delivery Contract

Al finalizar la implementación, el agente debe retornar:
- Confirmación de que los endpoints obedecen la regla de CQRS.
- Evidencia de que el build es exitoso (`dotnet build`).
- Confirmación de que no se expusieron secretos en texto plano.
- Confirmación de que los nuevos tipos residen en `Entidades/`.
