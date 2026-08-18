---
name: ERPPlanillas-profile
description: "Contrato de ejecución para agentes que modifican el backend ERPPlanillas. Trigger: cargar al operar en ERPPlanillas."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
  repo_type: backend
  primary_agent: backend-implementer
  tech_stack: ["C# .NET Core 3.1", "EF Core", "CQRS", "Kafka", "SignalR", "AWS Lambda", "Steeltoe"]
  gitlab_path: SReasonsERP/ERPPlanillas
---

## 1. Execution Role

Eres un sub-agente (`backend-implementer`, `dev-designer`, `dev-verifier`) o un orquestador operando en **ERPPlanillas**.
- Si eres implementador, debes adherirte estrictamente a las reglas arquitectónicas descritas abajo.
- Si el requerimiento altera estructuras de la base de datos subyacente, **debes delegar** a `database-specialist`. No corras migraciones por tu cuenta.

> **⚠️ Atención:** Este repositorio es el más complejo del ecosistema. Contiene **6 proyectos deployables** en una sola solución. Asegúrate de identificar en cuál proyecto estás trabajando antes de escribir código.

## 2. Language Domain Contract

- **Código:** C# (.NET Core 3.1). Nomenclatura en PascalCase para clases/métodos, camelCase para variables locales.
- **Mensajes de Commit:** En español, semánticos (`feat`, `fix`, `refactor`), claros y directos.
- **Artefactos:** Generated technical artifacts (like `apply-progress`) default to English.
- **Comentarios:** Solo para lógica de negocio compleja, no redundantes.

## 3. Architectural Invariants

Este repositorio implementa **CQRS con separación de deployables** — el caso más maduro del ecosistema.

1. **CQRS Estricto (API principal):** En `MSPlanillas.API`, los endpoints de lectura (`GET`) solo residen en `Controllers/Query`, las mutaciones (`POST`, `PUT`, `DELETE`) solo en `Controllers/Command`.
2. **CQRS a nivel de deployment:** Existe un proyecto **separado** `MSPlanillas.Qry` que es una API de solo lectura con sus propios Controllers, Endpoints, HealthChecks y Hubs. No mezclar código de escritura en Qry.
3. **Configuración Externalizada:** Via **Steeltoe Config Server**. Prohibido hardcodear connection strings en appsettings.
4. **Kafka:** El API principal tiene Kafka producer/consumer.
5. **SignalR Hubs:** `MSPlanillas.Qry` y posiblemente `MSPlanillas.API` usan SignalR para comunicación real-time.
6. **AWS Lambda:** Dos funciones Lambda en el repo:
   - `MSPlanillas.Sincro` — Sincronización (AWS Lambda con `Function.cs`)
   - `MSPlanillas.ProcesamientoLambda` — Procesamiento batch (con `src/` y `test/`)
7. **Sub-APIs:** El repo incluye APIs auxiliares dentro de la misma solución:
   - `MSMarcaciones.API` — API de marcaciones/time-tracking
   - `PublicidadWhatsapp.API` — API de WhatsApp (boilerplate)
8. **Unit of Work:** La capa Repo usa `{Domain}Unit.cs`. No inyectar DbContext directamente en controllers.

## 4. Directory Structure Contract

```
ERPPlanillas.sln
├── Entidades/                        → Domain: DTOs, Interfaces, Modelos, Utilitarios
├── Infraestructura/                  → DB Context, Configuraciones
├── Infraestructura.Test/             → Tests de infraestructura
├── MSPlanillas.API/                  → API principal (Command + Query)
│   ├── Controllers/Command/          → Endpoints de mutación
│   ├── Controllers/Query/            → Endpoints de lectura
│   ├── Background/                   → Background tasks
│   ├── Configurations/               → DI config
│   ├── HostedServices/               → Kafka consumer, etc.
│   ├── Hubs/                         → SignalR hubs
│   └── Utilitarios/
├── MSPlanillas.API.Test/             → Tests del API principal
├── MSPlanillas.Qry/                  → API de solo lectura (deployable separado)
│   ├── Controllers/                  → Query controllers
│   ├── Endpoints/                    → Minimal API endpoints
│   ├── HealthChecks/                 → Health check endpoints
│   ├── Hubs/                         → SignalR hubs (real-time)
│   └── chart/                        → Helm chart (K8s deployment)
├── MSPlanillasQry.Test/              → Tests del API de lectura
├── MSPlanillas.Sincro/               → AWS Lambda: sincronización
│   └── Function.cs                   → Entry point Lambda
├── MSPlanillas.ProcesamientoLambda/  → AWS Lambda: procesamiento batch
│   ├── src/                          → Código fuente
│   └── test/                         → Tests
├── MSMarcaciones.API/                → Sub-API de marcaciones
├── PublicidadWhatsapp.API/           → Sub-API WhatsApp (boilerplate)
├── MSPlanillas.Repo/                 → Unit-of-Work + Servicios
├── devops/                           → Deployment config
└── Jenkinsfile
```

## 5. Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Mutaciones** | `MSPlanillas.API/Controllers/Command/EmpleadoController.cs` | Poner un POST en `MSPlanillas.Qry/Controllers/` |
| **Lecturas API principal** | `MSPlanillas.API/Controllers/Query/ReporteController.cs` | Mezclar GET y POST en el mismo controller |
| **Lecturas Qry (separado)** | `MSPlanillas.Qry/Controllers/PlanillaQueryController.cs` | Poner lógica de escritura en MSPlanillas.Qry |
| **SignalR Hub** | `MSPlanillas.Qry/Hubs/NotificacionHub.cs` | WebSocket manual sin SignalR |
| **Lambda** | `MSPlanillas.Sincro/Function.cs` con handler estándar AWS | Script suelto sin Function handler |
| **Acceso a Datos** | `var unit = new EmpleadoUnit(context);` | `await _context.Empleados.ToListAsync()` en controller |

## 6. Testing Contract

- **Tests existentes:** `Infraestructura.Test/`, `MSPlanillas.API.Test/`, `MSPlanillasQry.Test/`, `MSPlanillas.ProcesamientoLambda/test/`.
- **Build:** `dotnet build ERPPlanillas.sln`.
- Si la tarea requiere tests, seguir la convención del proyecto de test correspondiente al proyecto que se modifica.

## 7. Output / Delivery Contract

Al finalizar la implementación, el agente debe retornar:
- **Identificar el proyecto modificado** (MSPlanillas.API, MSPlanillas.Qry, Sincro, etc.).
- Confirmación de que los endpoints obedecen la regla de CQRS en su proyecto correspondiente.
- Evidencia de que el build completo de la solución es exitoso (`dotnet build`).
- Confirmación de que no se expusieron secretos en texto plano.
- Confirmación de que nuevos tipos residen en `Entidades/`.
- Si se tocó Qry: confirmación de que NO se introdujo lógica de escritura.
- Si se tocó Lambda: confirmación de que el handler Function.cs sigue funcional.
