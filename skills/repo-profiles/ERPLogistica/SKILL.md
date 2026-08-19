---
name: ERPLogistica-profile
description: "Contrato de ejecución para agentes que modifican el backend ERPLogistica. Trigger: cargar al operar en ERPLogistica."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
  repo_type: backend
  primary_agent: backend-implementer
  tech_stack: ["C# .NET Core 3.1", "EF Core", "CQRS", "Kafka", "Redis", "Quartz", "Steeltoe"]
  gitlab_path: SReasonsERP/ERPLogistica
---

## 1. Execution Role

Eres un sub-agente (`backend-implementer`, `dev-designer`, `dev-verifier`) o un orquestador operando en **ERPLogistica**.
- Si eres implementador, debes adherirte estrictamente a las reglas arquitectónicas descritas abajo.
- Si el requerimiento altera estructuras de la base de datos subyacente, **debes delegar** a `database-specialist`. No corras migraciones por tu cuenta.

## 2. Language Domain Contract

- **Código:** C# (.NET Core 3.1). Nomenclatura en PascalCase para clases/métodos, camelCase para variables locales.
- **Mensajes de Commit:** En español, semánticos (`feat`, `fix`, `refactor`), claros y directos.
- **Artefactos:** Generated technical artifacts (like `apply-progress`) default to English.
- **Comentarios:** Solo para lógica de negocio compleja, no redundantes.

## 3. Architectural Invariants

Esta API obedece a una **Clean Architecture con CQRS estricto** y es uno de los backends más complejos del ecosistema.

1. **CQRS Estricto:** Los endpoints de lectura (`GET`) solo pueden residir en `Controllers/Query`. Los endpoints de mutación (`POST`, `PUT`, `DELETE`) solo en `Controllers/Command`.
2. **Configuración Externalizada:** Toda configuración sensible se obtiene via **Steeltoe Config Server** (Spring Cloud Config). Prohibido hardcodear connection strings o credenciales en `appsettings.json`.
3. **Kafka Producer + Consumer:** Este repo tiene **producción y consumo de eventos Kafka**. Los topics se configuran en `KafkaTopics` (appsettings). El consumer es un `HostedService` (`LogisticaConsumerService`). El producer es `LogisticaProducer`.
4. **Redis:** Cache distribuido via `StackExchange.Redis`. Se usa `IRedisService` para cacheo.
5. **Outbox Processor:** El proyecto `MSLogistica.OutboxProcessor` es un **deployable separado** que procesa mensajes outbox. Tiene sus propios controllers y background services.
6. **Unit of Work:** La capa Repo usa clases `{Domain}Unit.cs` (ej: `PedidoUnit`, `ItemUnit`, `InventarioUnit`). No crear repositorios genéricos ni inyectar DbContext directamente en controllers.
7. **Quartz Jobs:** Jobs programados via Quartz (ej: `AjusteInventarioJobSetup`, `SyncProcessorJobSetup`). Nuevos jobs deben seguir el mismo patrón.

## 4. Directory Structure Contract

```
ERPLogistica.sln
├── Entidades/                     → Domain: DTOs, Interfaces, Modelos, Shared, Utilitarios
├── Infraestructura/               → DB Context (MSLogisticaCmdContext), Configuraciones, Features, Interceptors, Resiliencia
├── Infraestructura.Test/          → Tests de infraestructura
├── MSLogistica.API/               → API principal
│   ├── Controllers/Command/       → Endpoints de mutación
│   ├── Controllers/Query/         → Endpoints de lectura
│   ├── Background/                → Background tasks
│   ├── Configurations/            → Swagger, DI, VPN Auth
│   ├── HostedServices/            → Kafka consumer, hosted services
│   └── Utilitarios/               → Helpers (StringToNumberConverter, etc.)
├── MSLogistica.API.Test/          → Tests de la API
├── MSLogistica.OutboxProcessor/   → Deployable separado para outbox messaging
│   ├── Controllers/               → Health/status endpoints
│   ├── Background/                → Outbox processing background tasks
│   └── Configurations/            → Config del outbox processor
├── MSLogistica.Repo/              → Unit-of-Work classes + Servicios + MensajeriaInterna
│   ├── Implementacion/            → Implementaciones concretas
│   ├── Builders/                  → Builder patterns
│   ├── MensajeriaInterna/         → Internal messaging
│   ├── Servicios/                 → SyncOffline, SyncJobOffline
│   └── {Domain}Unit.cs            → PedidoUnit, ItemUnit, InventarioUnit, etc.
├── devops/                        → Deployment config
└── Jenkinsfile
```

## 5. Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Ubicación de Mutaciones** | `MSLogistica.API/Controllers/Command/PedidoController.cs` | `MSLogistica.API/Controllers/PedidoController.cs` |
| **Ubicación de Lecturas** | `MSLogistica.API/Controllers/Query/ItemController.cs` | `MSLogistica.API/Controllers/Command/ItemController.cs` (para GET) |
| **Configuración** | `Configuration.GetConnectionString("SRLogisticaCmd")` via Steeltoe | Hardcodear `"Server=10.0.1.5;..."` en appsettings |
| **Eventos Kafka** | `await logisticaProducer.ProducirAsync(topicConfig.Topic, evt);` | `await httpClient.PostAsync("http://otro-ms/api", ...)` |
| **Cache Redis** | `await redisService.GetAsync<T>(key);` | Cache in-memory sin Redis |
| **Acceso a Datos** | `var pedidoUnit = new PedidoUnit(context); await pedidoUnit.ObtenerPedidos();` | `await _context.Pedidos.ToListAsync()` directo en controller |
| **Jobs Programados** | Nuevo `IConfigureOptions<QuartzOptions>` registrado en Startup | `Task.Delay` loop en background service |

## 6. Testing Contract

- **Tests existentes:** `Infraestructura.Test/` y `MSLogistica.API.Test/`.
- **Build:** `dotnet build ERPLogistica.sln`.
- Si la tarea requiere tests, seguir la convención de los proyectos de test existentes.

## 7. Output / Delivery Contract

Al finalizar la implementación, el agente debe retornar:
- Confirmación de que los endpoints agregados obedecen a la regla de CQRS (Command vs Query).
- Evidencia de que el build es exitoso (`dotnet build`).
- Confirmación explícita de que no se expusieron secretos en texto plano.
- Confirmación de que los nuevos tipos residen en `Entidades/` y no en la API.
- Si se tocó Kafka: confirmación de topic name y que el producer/consumer están correctamente registrados.
- Si se tocó el OutboxProcessor: confirmación de que el deployable separado sigue compilando.
