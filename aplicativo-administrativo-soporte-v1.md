# Elicitación de requerimientos — Aplicativo Web de Administración (Soporte)

## 1. Información General

| | |
|---|---|
| **Funcionalidad** | **Administración de Cobranza / Pagos — Soporte** |
| **Responsable** | *(pendiente)* |
| **Fecha** | 13/08/2026 |
| **Stakeholder** | *(pendiente)* |
| **Estado** | **v1 — desglosado desde v4 unificado** |

> **Nota:** este documento cubre exclusivamente las herramientas y reglas del **aplicativo web de administración**, usado por el equipo de Soporte para intervenir en casos que el cliente no puede resolver por autoservicio. El comportamiento del autoservicio del cliente se documenta en `elicitacion-modulo-pagos-cliente-v1.md`. Ambos aplicativos son proyectos de interfaz separados, pero comparten la misma API de Cobranza y la misma base de datos.

---

## 2. Problema

El ciclo de facturación (emisión, cobro, reintentos, bloqueo, reactivación) ocurre de forma automática y autoservicio para el cliente, pero existen casos excepcionales que el sistema no puede resolver por sí solo: comprobantes rechazados por SUNAT, órdenes que requieren corrección manual, empresas con condiciones comerciales especiales (GCG, precios especiales, extensiones de gratuidad), y transferencias de saldo entre empresas. El negocio necesita una vía manual, controlada y auditable para estos casos.

---

## 3. Objetivo

Dar al equipo de Soporte una herramienta que permita intervenir sobre el estado financiero y comercial de cualquier empresa cuando el autoservicio no cubre el caso, garantizando que toda intervención quede **trazada y auditada**, sin comprometer la integridad de los pagos ya confirmados.

---

## 4. Enlaces

- Vistas de Figma: *(pendiente)*
- Mapas de flujo: *(pendiente)*
- Manual de componentes: *(pendiente)*

---

## 5. Actores

- **Soporte** — usuario interno que opera el aplicativo administrativo. Por ahora, todos los usuarios de Soporte tienen el **mismo nivel de acceso** a todas las acciones (sin roles diferenciados; la segmentación de permisos se define en una iteración posterior).
- **Cliente** — no interactúa directamente con este aplicativo; sus solicitudes llegan a Soporte por canales externos (ej. mesa de ayuda) y motivan las acciones administrativas.
- **Sistema de Cobranza** — expone la información y ejecuta las reglas de negocio sobre las cuales actúa Soporte (misma API y base de datos que el módulo de cliente).
- **SUNAT** — relevante para la acción de reemisión de comprobantes.

---

## 6. Conceptos clave (resumen — ver detalle completo en el documento de cliente)

### 6.1 Estados de la Cuenta

| Estado unificado | Descripción |
|---|---|
| `PENDIENTE_ACTIVACION` | Suscripción creada, aún no inicia el periodo gratuito |
| `PERIODO_GRATUITO` | Cliente dentro de su periodo gratuito inicial |
| `ACTIVA` | Cuenta al día, sin deuda |
| `MOROSA` | Orden vencida sin pagar (0 a umbral configurable, 10 días por defecto) |
| `SUSPENDIDA` | Morosidad superó el umbral configurado — sin acceso |
| `CANCELACION_PROGRAMADA` | Baja solicitada, vigente hasta fin del periodo cubierto |
| `CANCELADA` | Baja voluntaria efectiva |
| GCG / Gratuita | Exención de cobro activada manualmente por Soporte |

### 6.2 Saldo a favor

Pertenece a la empresa que lo generó. Se pierde en baja inmediata, en suspensión por morosidad, y al activarse GCG. **No es transferible por autoservicio** — la transferencia entre empresas es exclusivamente una acción de Soporte (ver sección 7.4).

### 6.3 Órdenes de pago

Una orden **pagada es inmutable** — ni siquiera Soporte puede modificarla. Solo las órdenes no pagadas (`Deuda Pendiente` o `Deuda Vencida`) pueden ser corregidas manualmente.

---

## 7. Acciones de Soporte

Consolidado de todo lo que requiere intervención manual — ninguna de estas acciones es autoservicio del cliente ni ocurre de forma automática. Todas se ejecutan desde el aplicativo web de administración.

| # | Acción | Cuándo se necesita |
|---|---|---|
| 1 | Activar el régimen GCG/Gratuita | La empresa pertenece a un corporativo con acuerdo, o el negocio decide eximirla de cobro |
| 2 | Desactivar el régimen GCG/Gratuita | Se decide que la empresa vuelva a facturación normal |
| 3 | Reemitir un comprobante | SUNAT rechazó el comprobante de un pago ya cobrado exitosamente |
| 4 | Modificar una orden ya generada (no pagada) | Casos excepcionales no cubiertos por las actualizaciones automáticas del sistema |
| 5 | Consultar histórico completo de órdenes/pagos | Investigación de casos, auditoría, soporte al cliente |
| 6 | Forzar cambio de estado de la cuenta/suscripción | Corregir un estado inconsistente u otro caso excepcional |
| 7 | Consultar bitácora de cambios | Trazabilidad de todo cambio comercial y financiero de una empresa |
| 8 | Ejecutar cambios de plan y periodos en nombre del cliente | El cliente solicita el cambio a través de Soporte |
| 9 | Transferir saldo a favor entre empresas | El cliente solicita mover su saldo (ej. cambio de razón social, fusión) |
| 10 | Consultar y gestionar saldo a favor de una empresa | Soporte al cliente, resolución de reclamos |
| 11 | Aplicar precios especiales | Negociaciones comerciales particulares |
| 12 | Otorgar días adicionales de periodo gratuito | Extensiones comerciales, casos de retención de cliente |

### 7.1 Activar / Desactivar GCG (Empresa Gratuita)

**Flujo — Activar**
1. Soporte activa manualmente la condición desde el aplicativo, sobre una empresa ya existente.
2. Si había una orden pendiente al momento de activar, **se anula**.
3. Si había un periodo pagado activo, se respeta hasta que termine; recién ahí arranca el periodo gratuito indefinido.
4. Si la empresa tenía saldo a favor, **lo pierde** al activarse GCG.
5. Mientras esté activo: periodo gratuito se repite indefinidamente, sin generar órdenes. Complementos también quedan exentos de cobro.
6. La cuenta sigue respetando los límites de recursos del plan base.

**Flujo — Desactivar**
1. Soporte desactiva la condición.
2. La facturación se reanuda de forma normal según el ciclo de cobranza estándar.

### 7.2 Reemitir comprobante

1. SUNAT rechaza un comprobante de un pago ya confirmado (el dinero no se ve afectado).
2. Soporte identifica la orden/pago afectado desde el histórico.
3. Soporte ejecuta la reemisión del documento fiscal.
4. Se actualiza el sub-estado del comprobante.

### 7.3 Modificar una orden ya generada

1. Soporte localiza una orden en estado `Deuda Pendiente` o `Deuda Vencida` (nunca una orden `Pagada`).
2. Aplica la corrección necesaria (monto, conceptos, etc.).
3. El cambio queda registrado en la bitácora de la orden, indicando el origen administrativo del cambio.

### 7.4 Transferir saldo a favor entre empresas

1. El cliente de la Empresa A solicita a Soporte transferir su saldo a favor a la Empresa B.
2. Soporte valida la solicitud y ejecuta la transferencia desde el aplicativo.
3. El saldo se descuenta de la Empresa A y se acredita a la Empresa B.
4. El saldo transferido sigue las mismas reglas generales de saldo a favor (aplicación automática, no autoservicio, pérdida ante baja/suspensión/GCG).

### 7.5 Aplicar precios especiales

1. Soporte configura un precio especial para una empresa (plan y/o complemento).
2. El precio especial **tiene vigencia definida** (fecha de inicio y fin) — no es permanente por defecto.
3. Mientras esté vigente, las órdenes de esa empresa usan el precio especial en lugar del precio contratado/catálogo estándar.

> ⏳ **Pendiente de definir**: comportamiento exacto al vencer la vigencia — si vuelve al precio de catálogo estándar o al precio contratado previo del cliente (ver sección 9).

### 7.6 Otorgar días adicionales de gratuidad

1. Soporte otorga días adicionales de periodo gratuito a una empresa puntual.

> ⏳ **Pendiente de definir**: si estos días se **suman** al periodo gratuito estándar (extendiendo la fecha de corte) o si **reemplazan** la duración total con un nuevo valor (ver sección 9).

### 7.7 Forzar cambio de estado de cuenta/suscripción

1. Soporte identifica una cuenta con un estado inconsistente o un caso excepcional que requiere forzar una transición de estado no cubierta por el flujo automático.
2. Ejecuta el cambio desde el aplicativo.
3. Queda registrado en la bitácora administrativa, indicando el estado anterior, el nuevo estado, el usuario de Soporte y el motivo.

### 7.8 Consultas (histórico, bitácora, saldo)

Vistas de solo lectura para investigación y soporte al cliente:
- Histórico completo de órdenes y pagos por empresa.
- Bitácora de cambios comerciales y financieros de una empresa.
- Saldo a favor disponible y su historial de movimientos.

---

## 8. Reglas de Negocio

1. Todos los usuarios de Soporte tienen, por ahora, el **mismo nivel de acceso** a todas las acciones administrativas.
2. El acceso es a **todas las empresas sin restricción** (sin segmentación por región o nivel de soporte, por ahora).
3. Ninguna acción administrativa puede modificar un **pago ya confirmado** — son inmutables incluso para Soporte.
4. Solo las órdenes **no pagadas** (`Deuda Pendiente` o `Deuda Vencida`) pueden modificarse manualmente.
5. Toda acción ejecutada desde el aplicativo administrativo debe quedar registrada en una **bitácora de cambios administrativos**: qué usuario de Soporte la realizó, cuándo, sobre qué empresa/entidad, y con qué valores anteriores/nuevos.
6. La activación/desactivación de GCG es exclusivamente manual y exclusivamente vía este aplicativo — no existe activación automática por otros sistemas.
7. Al activar GCG, la empresa pierde cualquier saldo a favor existente y no genera saldo nuevo mientras dure la condición.
8. El saldo a favor pertenece a la empresa que lo generó; su transferencia a otra empresa es exclusivamente una acción de Soporte, nunca autoservicio.
9. Los precios especiales otorgados por Soporte tienen **vigencia definida** (fecha de inicio y fin).
10. El motivo de rechazo de un cobro (relevante para diagnóstico de casos de Soporte) es específico solo si la pasarela (Culqi) lo entrega; si no, se usa un catálogo de mensajes genéricos — nunca se inventa un motivo.
11. El comprobante fiscal se genera después de la confirmación del pago; su reemisión (cuando SUNAT lo rechaza) no afecta el estado del dinero ya cobrado.

---

## 9. Configuraciones del sistema relevantes para Soporte

| Configuración | Valor |
|---|---|
| Ventana de estado `MOROSA` | Configurable — 10 días por defecto |
| Umbral para pasar a `SUSPENDIDA` | Configurable — 10 días por defecto |
| Vigencia de precios especiales | Configurable por caso (fecha inicio/fin) — **regla exacta de expiración pendiente de definir** |
| Días adicionales de gratuidad | Configurable por caso — **si se suman o reemplazan la duración estándar, pendiente de definir** |
| Pasarela de pagos | Culqi |

---

## 10. Escenarios Gherkin

### Feature: Aplicativo administrativo — Soporte

### 10.1 GCG / Gratuita

```gherkin
Escenario: Activar GCG con orden pendiente
  Dado que una empresa tiene una Orden de pago en estado "Deuda Pendiente"
  Cuando Soporte activa la condición de Empresa Gratuita
  Entonces la Orden pendiente se anula
  Y el periodo gratuito se repite indefinidamente sin generar nuevas órdenes

Escenario: Activar GCG con periodo pagado activo
  Dado que una empresa tiene un periodo pagado activo
  Cuando Soporte activa la condición de GCG
  Entonces el periodo pagado se respeta hasta que termine
  Y recién ahí arranca el periodo gratuito indefinido

Escenario: Activar GCG con saldo a favor existente
  Dado que una empresa tiene saldo a favor disponible
  Cuando Soporte activa la condición de GCG
  Entonces la empresa pierde el saldo a favor existente

Escenario: Desactivar GCG
  Dado que una empresa está bajo el régimen GCG
  Cuando Soporte desactiva la condición
  Entonces la facturación se reanuda de forma normal
```

### 10.2 Transferencia de saldo entre empresas

```gherkin
Escenario: Transferencia de saldo gestionada por Soporte
  Dado que el cliente de la Empresa A solicita transferir su saldo a favor a la Empresa B
  Cuando un usuario de Soporte ejecuta la transferencia desde el aplicativo de administración
  Entonces el saldo se descuenta de la Empresa A
  Y se acredita a la Empresa B
  Y se registra el movimiento en la bitácora administrativa
```

### 10.3 Modificación de órdenes y reemisión de comprobantes

```gherkin
Escenario: Intento de modificar una orden pagada
  Dado que una orden se encuentra en estado "Pagado"
  Cuando Soporte intenta modificarla desde el aplicativo administrativo
  Entonces el sistema impide la modificación

Escenario: Modificar una orden pendiente
  Dado que una orden se encuentra en estado "Deuda Pendiente"
  Cuando Soporte aplica una corrección manual
  Entonces la orden se actualiza
  Y se registra el cambio en la bitácora, incluyendo el origen administrativo

Escenario: Reemitir comprobante rechazado por SUNAT
  Dado que un pago está en estado "Pagado" con Comprobante "Rechazado"
  Cuando Soporte ejecuta la reemisión del comprobante
  Entonces se genera un nuevo comprobante para ese pago
  Y el dinero ya cobrado no se ve afectado
```

### 10.4 Precios especiales y gratuidad adicional

```gherkin
Escenario: Aplicar un precio especial con vigencia
  Dado que Soporte necesita otorgar una condición comercial particular a una empresa
  Cuando configura un precio especial con fecha de inicio y fin
  Entonces las órdenes generadas dentro de esa vigencia usan el precio especial

Escenario: Otorgar días adicionales de periodo gratuito
  Dado que Soporte decide extender la gratuidad de una empresa
  Cuando otorga días adicionales desde el aplicativo
  Entonces la empresa permanece en "PERIODO_GRATUITO" por el tiempo adicional otorgado
```

### 10.5 Forzar cambio de estado

```gherkin
Escenario: Forzar cambio de estado por caso excepcional
  Dado que una cuenta presenta un estado inconsistente con su situación real
  Cuando Soporte fuerza el cambio a un nuevo estado desde el aplicativo
  Entonces la cuenta queda en el estado indicado
  Y se registra el cambio en la bitácora administrativa con el motivo
```

---

## 11. Dependencias

### 11.1 Dependencias funcionales

- Módulo de Pagos / Cobranza (cliente) — mismo dominio de datos: empresas, suscripciones, órdenes, pagos, saldo a favor.
- Integración con SUNAT (para reemisión de comprobantes).
- Sistema de notificaciones (para informar al cliente de acciones administrativas que lo afecten, cuando corresponda).

### 11.2 Dependencias técnicas

- Este aplicativo es un **proyecto de interfaz separado** del módulo de cliente, pero consume la **misma API de Cobranza** y opera sobre la **misma base de datos**.
- Pasarela de pagos **Culqi** — relevante para diagnóstico de casos (motivos de rechazo) aunque Soporte no ejecuta cobros directamente salvo lo cubierto en este documento.

---

## 12. Pendientes / Temas Abiertos

- [ ] Reglas exactas de expiración de los precios especiales: ¿vuelve al precio de catálogo estándar o al precio contratado previo del cliente?
- [ ] Días de gratuidad adicionales: ¿se suman al periodo gratuito estándar (extienden la fecha de corte) o reemplazan la duración total?
- [ ] Nombre de categoría **"Eliminación"** en la Bitácora del Contrato (aplica a todo el sistema, no solo pagos).
- [ ] Segmentación de roles y permisos dentro del equipo de Soporte (queda para iteración posterior — hoy todos tienen el mismo acceso).
- [ ] Segmentación de alcance por empresa/región dentro de Soporte (queda para iteración posterior — hoy es acceso total sin restricción).
- [ ] Definir si el forzado de cambio de estado (acción 6/7.7) tiene restricciones de transición (ej. ¿se puede forzar cualquier estado desde cualquier estado, o hay una matriz de transiciones válidas?).

---

*Documento vivo — versión 1 (desglosado del documento unificado v4). Para el comportamiento de autoservicio del cliente, ver `elicitacion-modulo-pagos-cliente-v1.md`.*
