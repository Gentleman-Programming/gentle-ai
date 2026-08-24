<!--
Esta es la plantilla REAL que tu equipo ya usa en BookStack (confirmada
buscando "elicitación" ahí mismo — 14 páginas reales la siguen: Pagos de
Suscripción, Proyecto Notificaciones, Restaurantes). No es una plantilla
nueva inventada — es una copia fiel de la que ya existe, en Markdown.

Organización en BookStack (según lo que ya hacen, no algo nuevo a montar):
- Un LIBRO por proyecto/módulo grande (ej. "Pagos de Suscripción",
  "Proyecto Notificaciones", "Restaurantes"). El libro ES la página
  principal que agrupa todo — BookStack ya lista sus páginas/capítulos
  automáticamente, no hace falta un índice manual aparte.
- Opcionalmente, CAPÍTULOS dentro del libro para sub-módulos (ej.
  "Restaurantes" tiene capítulos separados para "Creación de salones" y
  "Acciones de salón").
- Cada PÁGINA dentro = UN requerimiento pequeño, con esta plantilla
  completa. Nunca mezclar dos requerimientos en una sola página.

Quién la llena: quien pide el requerimiento (negocio/PM), con Dev y QA
validando en la sección 10 antes de pasar a dev-orchestrator.

Documento padre (opcional, NO bloqueante): si esta página vive dentro de
un libro/proyecto más grande, es buena práctica anotar el enlace en el
campo "Documento padre" de abajo — sirve como trazabilidad/auditoría de
"de dónde viene esto". Pero esto NUNCA es un requisito para que el
requerimiento avance: dev-orchestrator trabaja con el contenido real de la
página (problema, objetivo, reglas, escenarios) sin necesitar este campo.
Si no aplica o no se sabe todavía, déjalo como "N/A" y sigue adelante.
-->

## 1. Información General

| Campo | Valor |
|---|---|
| **Funcionalidad** | [nombre corto y específico del requerimiento] |
| **Documento padre** (opcional) | [enlace al Libro/Capítulo/página de BookStack del que depende este requerimiento, o "N/A"] |
| **Responsable** | [nombre] |
| **Fecha** | [fecha] |
| **Stakeholder** | [área/persona que lo pide] |
| **Estado** | [Borrador / En revisión / Validado con Dev / Validado con QA] |

## 2. Problema
👉 Qué está pasando actualmente (sin solución)

**Ejemplo:**
> El proceso de creación de órdenes de compra es manual y genera errores en montos y proveedores.

## 3. Objetivo
👉 Qué se quiere lograr

**Ejemplo:**
> Estandarizar y automatizar la creación de órdenes de compra para reducir errores y mejorar trazabilidad.

## 4. Actores
- [ej. Usuario]
- [ej. Sistema]
- [ej. Proveedor (si aplica)]

## 5. Flujo General
👉 Mapa de flujos

**Ejemplo:**
1. Usuario inicia registro
2. Ingresa datos
3. Sistema valida
4. Sistema responde

## 6. Reglas de Negocio
👉 Base para TODO lo demás

- [ej. El proveedor debe existir en el sistema]
- [ej. El monto total no puede ser negativo]
- [ej. La orden debe tener al menos un ítem]
- [ej. Solo usuarios autorizados pueden crear órdenes]

> Si ya existe una skill de `domain/` para este proceso de negocio, enlázala
> aquí en vez de repetir las reglas desde cero.

## 7. Escenarios en Gherkin

### Feature: [nombre de la funcionalidad completa]
👉 Nivel alto, agrupa todos los escenarios, responde "¿qué estamos construyendo?"

```gherkin
Feature: [nombre]
  Como [rol de usuario]
  Quiero [acción]
  Para [beneficio de negocio]
```

### Background (opcional)
👉 Contexto común a todos los escenarios — evita repetir lo mismo en cada uno

```gherkin
Background:
  Given [precondición común]
  And [precondición común 2]
```

### Escenarios principales (happy path)
👉 Todo sale bien

```gherkin
Escenario: [nombre del caso exitoso]
  When [acción del usuario]
  And [acción adicional]
  Then [resultado esperado]
  And [efecto secundario esperado]
```

### Escenarios de validación
👉 Cuando el usuario se equivoca — 💡 validan reglas de negocio

```gherkin
Escenario: [nombre del caso de error de negocio]
  Given [precondición de error]
  When [acción]
  Then [el sistema bloquea/valida]
  And [mensaje esperado]
```

### Escenarios edge
👉 Casos menos comunes pero importantes — 💡 aquí viven los bugs si no los defines

```gherkin
Scenario: [caso límite]
  When [condición inusual]
  Then [comportamiento esperado, no un crash]
```

## 8. Dependencias
- [ej. API]
- [ej. Servicios externos]
- [ej. Datos necesarios]

## 9. Criterios de Éxito
- [métrica 1, ej. "Reducción de errores en órdenes"]
- [métrica 2, ej. "Tiempo promedio de creación"]
- [métrica 3, ej. "% de órdenes correctamente registradas"]

## 10. Checklist de validación
- [ ] Problema claro
- [ ] Reglas de negocio definidas
- [ ] Escenarios Gherkin completos
- [ ] Casos edge considerados
- [ ] Validado con Dev
- [ ] Validado con QA

---
*A partir de aquí (marcado "Validado con Dev" y "Validado con QA"), esta
página es lo que se le entrega como intent a dev-orchestrator. Él decide
solo, técnicamente, a qué repositorio(s) toca, si es existing/greenfield, y
en qué fase SDD entra — eso no se decide en esta página.*
