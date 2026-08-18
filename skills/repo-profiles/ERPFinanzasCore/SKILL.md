---
name: ERPFinanzasCore-profile
description: "Contrato de ejecución para agentes que modifican el backend ERPFinanzasCore. Trigger: cargar al operar en ERPFinanzasCore."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
  repo_type: backend
  primary_agent: backend-implementer
  tech_stack: ["C# .NET Core 3.1", "EF Core", "CQRS", "Kafka", "Steeltoe"]
  gitlab_path: SmartClic/ERPFinanzasCore
---

## 1. Execution Role

Eres un sub-agente (`backend-implementer`, `dev-designer`, `dev-verifier`) o un orquestador operando en **ERPFinanzasCore**.
- Si eres implementador, debes adherirte estrictamente a las reglas arquitectónicas descritas abajo.
- Si el requerimiento altera estructuras de la base de datos subyacente, **debes delegar** a `database-specialist`. No corras migraciones por tu cuenta.

## 2. Language Domain Contract

- **Código:** C# (.NET Core 3.1). Nomenclatura en PascalCase para clases/métodos, camelCase para variables locales.
- **Mensajes de Commit:** En español, semánticos (`feat`, `fix`, `refactor`), claros y directos.
- **Artefactos:** Generated technical artifacts (like `apply-progress`) default to English.
- **Comentarios:** Solo para lógica de negocio compleja, no redundantes.

## 3. Architectural Invariants

Esta API obedece a una **Clean Architecture con CQRS estricto**.

1. **CQRS Estricto:** Los endpoints de lectura (`GET`) solo pueden residir en `Controllers/Query`. Los endpoints de mutación (`POST`, `PUT`, `DELETE`) solo en `Controllers/Command`.
2. **Secretos Segregados:** Prohibido inyectar credenciales, tokens o DB strings directamente en `appsettings.json`. Configuración externalizada via **Steeltoe Config Server** (Spring Cloud Config).
3. **Comunicaciones Inter-servicio:** Toda transacción que impacte módulos contiguos debe emitir un evento al tópico `erp_finanza` en **Kafka**.
4. **Unit of Work:** La capa Repo usa clases `{Domain}Unit.cs` como Unit-of-Work. No crear repositorios genéricos ni inyectar DbContext directamente en controllers.

## 4. Directory Structure Contract

```
ERPFinanzas.sln
├── Entidades/               → Domain: DTOs, Interfaces, Modelos, Utilitarios
├── Infraestructura/         → DB Context, Configuraciones, Interceptors, Resiliencia
├── MSFinanzas.API/          → API principal
│   ├── Controllers/Command/ → Endpoints de mutación (POST, PUT, DELETE)
│   ├── Controllers/Query/   → Endpoints de lectura (GET)
│   ├── Background/          → Background services
│   ├── Configurations/      → Swagger, DI config
│   └── Utilitarios/         → Helpers específicos de la API
├── MSFinanzas.Repo/         → Implementaciones de repositorio, {Domain}Unit.cs, Servicios
├── devops/                  → Jenkinsfile, deployment config
└── Jenkinsfile
```

## 5. Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Ubicación de Mutaciones** | `MSFinanzas.API/Controllers/Command/CajaController.cs` | `MSFinanzas.API/Controllers/CajaController.cs` |
| **Ubicación de Lecturas** | `MSFinanzas.API/Controllers/Query/CuentaController.cs` | `MSFinanzas.API/Controllers/Command/CuentaController.cs` (para un GET) |
| **Gestión de Configuración** | Steeltoe Config Server + `IConfiguration` | `"ConnectionStrings": { "DB": "Server=...;" }` hardcoded en appsettings |
| **Emisión de Eventos** | `await kafkaPublisher.PublishAsync("erp_finanza", evt);` | `await httpClient.PostAsync("http://otro-ms/api", ...)` |
| **Acceso a Datos** | `var unit = new CajaUnit(context); await unit.Method();` | `await _context.Cajas.ToListAsync()` directo en controller |

## 6. Testing Contract

- No hay test projects detectados en este repo actualmente.
- Build: `dotnet build ERPFinanzas.sln`.
- Si la tarea requiere tests, crearlos en un nuevo proyecto `MSFinanzas.API.Test` siguiendo el patrón de otros repos del ecosistema.

## 7. Output / Delivery Contract

Al finalizar la implementación, el agente debe retornar:
- Confirmación de que los endpoints agregados obedecen a la regla de CQRS.
- Evidencia de que el build es exitoso (`dotnet build`).
- Confirmación explícita de que no se expusieron secretos en texto plano.
- Confirmación de que los nuevos tipos residen en `Entidades/` y no en la API.
