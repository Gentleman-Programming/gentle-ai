---
name: ERPIntegracion-profile
description: "Contrato de ejecución para agentes que modifican el backend ERPIntegracion. Trigger: cargar al operar en ERPIntegracion."
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
  gitlab_path: SmartClic/ERPIntegracion
---

## 1. Execution Role

Eres un sub-agente (`backend-implementer`, `dev-designer`, `dev-verifier`) o un orquestador operando en **ERPIntegracion**.
- Si eres implementador, debes adherirte estrictamente a las reglas arquitectónicas descritas abajo.
- Si el requerimiento altera estructuras de la base de datos subyacente, **debes delegar** a `database-specialist`.

## 2. Language Domain Contract

- **Código:** C# (.NET Core 3.1). Nomenclatura en PascalCase para clases/métodos, camelCase para variables locales.
- **Mensajes de Commit:** En español, semánticos (`feat`, `fix`, `refactor`), claros y directos.
- **Artefactos:** Generated technical artifacts default to English.
- **Comentarios:** Solo para lógica de negocio compleja, no redundantes.

## 3. Architectural Invariants

Backend de integración con **Clean Architecture y CQRS estricto**. Actúa como puente entre módulos del ERP.

1. **CQRS Estricto:** Los endpoints de lectura (`GET`) solo residen en `Controllers/Query`. Las mutaciones (`POST`, `PUT`, `DELETE`) solo en `Controllers/Command`.
2. **Configuración Externalizada:** Via **Steeltoe Config Server**. Prohibido hardcodear connection strings en appsettings.
3. **Unit of Work:** La capa Repo usa `{Domain}Unit.cs`. No inyectar DbContext directamente en controllers.
4. **Sin mensajería:** Este repo NO tiene Kafka ni Redis. Si se necesita comunicación inter-servicio, escalar al orquestador.
5. **Rol de integración:** Este microservicio conecta módulos. Los cambios aquí pueden impactar múltiples consumidores. Evaluar impacto transversal antes de modificar contratos existentes.

## 4. Directory Structure Contract

```
ERPIntegracion.sln
├── Entidades/                 → Domain: DTOs, Interfaces, Modelos, Utilitarios
├── Infraestructura/           → DB Context, Configuraciones
├── MSIntegracion.API/         → API principal
│   ├── Controllers/Command/   → Endpoints de mutación
│   └── Controllers/Query/     → Endpoints de lectura
├── MSIntegracion.Repo/        → Unit-of-Work + Servicios
├── devops/                    → Deployment config
└── Jenkinsfile
```

## 5. Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Mutaciones** | `MSIntegracion.API/Controllers/Command/SyncController.cs` | `MSIntegracion.API/Controllers/SyncController.cs` |
| **Lecturas** | `MSIntegracion.API/Controllers/Query/EstadoController.cs` | Poner un GET en `Controllers/Command/` |
| **Configuración** | Steeltoe Config Server + `IConfiguration` | Hardcodear credenciales en appsettings |
| **Acceso a Datos** | `var unit = new IntegracionUnit(context);` | `await _context.Integraciones.ToListAsync()` en controller |
| **Contratos existentes** | Mantener backward compatibility en endpoints consumidos por otros MS | Romper signature de endpoint sin aviso |

## 6. Testing Contract

- **No hay proyectos de test** detectados en este repo actualmente.
- **Build:** `dotnet build ERPIntegracion.sln`.
- Si la tarea requiere tests, crear `MSIntegracion.API.Test` siguiendo la convención del ecosistema.

## 7. Output / Delivery Contract

Al finalizar la implementación, el agente debe retornar:
- Confirmación de que los endpoints obedecen la regla de CQRS.
- Evidencia de que el build es exitoso (`dotnet build`).
- Confirmación de que no se expusieron secretos en texto plano.
- Confirmación de que los nuevos tipos residen en `Entidades/`.
- **Si se modificó un endpoint existente:** Confirmación de backward compatibility o listado de consumidores impactados.
