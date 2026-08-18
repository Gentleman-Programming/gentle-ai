# Elicitación de requerimientos — Módulo de Pagos (Cliente / Autoservicio)

## 1. Información General

| | |
|---|---|
| **Funcionalidad** | **Pagos / Suscripciones — Autoservicio del Cliente** |
| **Responsable** | *(pendiente)* |
| **Fecha** | 13/08/2026 |
| **Stakeholder** | *(pendiente)* |
| **Estado** | **v1 — desglosado desde v4 unificado** |

> **Nota:** este documento cubre exclusivamente la experiencia y las reglas del **cliente en autoservicio**. Las acciones que ejecuta el equipo de Soporte (activación de GCG, modificación de órdenes, transferencias de saldo, precios especiales, etc.) se documentan por separado en `aplicativo-administrativo-soporte-v1.md`. Ambos módulos comparten la misma API de Cobranza y la misma base de datos.

---

## 2. Problema

El negocio necesita cobrar de forma recurrente y automática el uso de la plataforma (plan + complementos), gestionar periodos gratuitos, cambios de plan, compra/baja de recursos adicionales, y manejar de forma controlada los casos de no-pago — todo esto sin perder trazabilidad de cada orden, cada intento de cobro y cada comprobante emitido.

---

## 3. Objetivo

Permitir que el ciclo completo de facturación (emisión, cobro, reintentos, bloqueo, reactivación) ocurra de forma **automática y autoservicio** para el cliente, garantizando la integridad del dinero cobrado y la trazabilidad de cada intento de pago. Los casos excepcionales que el cliente no puede resolver por sí mismo se derivan a Soporte (ver documento del aplicativo administrativo).

---

## 4. Enlaces

- Vistas de Figma: *(pendiente)*
- Mapas de flujo: *(pendiente)*
- Manual de componentes: *(pendiente)*

---

## 5. Actores

- **Cliente** — dueño/administrador de la cuenta de la empresa. Ve su estado de cuenta, paga, compra/da de baja complementos, cambia de plan o ciclo.
- **Pasarela de pagos (Culqi)** — procesa los cobros (manuales y automáticos), confirma la entrega del dinero, entrega (cuando puede) el motivo de un rechazo.
- **SUNAT** — acepta o rechaza el comprobante fiscal emitido después de un pago exitoso.
- **Sistema** — ejecuta las reglas de emisión de órdenes, reintentos automáticos, bloqueo/desbloqueo, y actualizaciones de recursos.
- **Soporte** *(fuera de este documento)* — interviene solo en casos excepcionales no resolubles por autoservicio. Ver `aplicativo-administrativo-soporte-v1.md`.

---

## 6. Conceptos clave

### 6.1 Terminología de fechas

- **Fecha de Emisión**: cuando se genera la orden de pago. Siempre **5 días antes** de la Fecha de Vencimiento.
- **Fecha de Vencimiento**: fecha de corte del periodo; último día válido para pagar.
- **Cierre del plazo**: a las **00:00 del día siguiente** a la Fecha de Vencimiento. El mismo día del vencimiento todavía es un plazo válido para pagar.

### 6.2 Duración de ciclo

| Ciclo | Duración usada en fórmulas |
|---|---|
| Mensual | 30 días fijos |
| Anual | 365 días fijos |

No se usan los días reales del mes calendario para el cálculo de prorrateo — siempre 30 o 365, sin importar el mes.

### 6.3 Estados de la Orden de Pago

| Estado | Cuándo aplica | Comprobante existe? |
|---|---|---|
| **Pagado** | La pasarela confirmó la entrega del dinero | Sí (con su propio sub-estado, ver 6.3.1) |
| **Deuda Pendiente** | Orden generada, dentro del plazo (Emisión ≤ Hoy ≤ Vencimiento), sin pagar | No |
| **Deuda Vencida** | Pasó la Fecha de Vencimiento sin pago | No |

*Sub-estado opcional en Deuda Pendiente/Vencida:* **Pago rechazado** — hubo al menos un intento (automático o manual) que fue rechazado.

#### 6.3.1 Sub-estado del Comprobante (solo si Estado = Pagado)

| Estado del comprobante | Descripción |
|---|---|
| Aceptado | SUNAT validó el comprobante |
| Rechazado | SUNAT lo rechazó — el dinero ya está cobrado, solo se reemite el documento (esto lo resuelve Soporte, ver documento administrativo) |
| En Proceso | Esperando la validación de SUNAT |

El comprobante se genera **después** de la confirmación del pago por parte de Culqi: *pago confirmado → generación de comprobante → validación SUNAT*.

### 6.4 Estados de la Cuenta

| Estado unificado | Acceso | Se generan órdenes nuevas? |
|---|---|---|
| `PENDIENTE_ACTIVACION` | Sin acceso operativo — pendiente de completar configuración inicial | No |
| `PERIODO_GRATUITO` | Total | No (mientras dure la gratuidad) |
| `ACTIVA` | Total | Sí, normal |
| `MOROSA` | Total, pero **ninguna acción** — solo pagar | No |
| `SUSPENDIDA` | Ninguno — solo pantalla de pago | No |
| `CANCELACION_PROGRAMADA` | Total hasta fin del periodo vigente | No |
| `CANCELADA` | Total hasta fin del periodo vigente, luego ninguno | No |
| GCG / Gratuita | Total (respeta límites del plan) | No — periodo gratuito indefinido |

El umbral de días para pasar de `MOROSA` a `SUSPENDIDA` es de **10 días por defecto**, configurable a nivel de sistema.

> La activación/desactivación de GCG es una acción exclusiva de Soporte — desde la perspectiva del cliente, este documento solo describe cómo se ve y se comporta la cuenta una vez que el régimen ya está activo.

### 6.5 Complementos

Recursos adicionales a los incluidos en el plan: **usuarios, almacenes, tiendas virtuales, cajas de venta, sucursales**. Cantidad **ilimitada**. Costo aparte del plan, con tarifa propia (mensual y anual).

### 6.6 Saldo a favor (crédito)

Saldo único de cuenta que se genera cuando un cambio (downgrade de plan, cambio de ciclo) produce un valor negativo. Se aplica automáticamente contra la siguiente orden real que se genere, sea de plan o de complementos, **sin que el cliente lo active manualmente**. No puede coexistir saldo a favor sobrante con deuda/bloqueo simultáneo.

No existe un documento formal tipo "Nota de crédito" — el registro vive únicamente como saldo interno, visible en la tabla de créditos/descuentos de la interfaz.

**Pérdida del saldo:** se pierde cuando la empresa se da de baja inmediatamente, cuando pasa a `SUSPENDIDA` por morosidad, o cuando se activa la condición GCG/Gratuita.

**Transferencia entre empresas:** el saldo a favor **no es transferible por autoservicio**. Si el cliente necesita mover su saldo a otra empresa, debe comunicarse con Soporte (ver documento administrativo).

---

## 7. Flujos por caso

### 7.1 Happy Path (flujo básico)

**Flujo**
1. Cliente selecciona un plan → arranca el periodo gratuito (`PERIODO_GRATUITO`).
2. **5 días antes** del corte → se genera la Orden + notificación "debes pagar".
3. Cliente paga dentro del plazo → notificación de pago exitoso, se renueva el periodo (`ACTIVA`).
4. Se repite cada ciclo.

### 7.2 Pago adelantado

**Flujo**
1. El cliente ingresa a "Gestión de suscripción" antes de que exista una orden generada para el siguiente periodo.
2. Hace clic en el botón **"Pagar por adelantado"**.
3. El sistema muestra el detalle, especificando para qué periodo es ese pago.
4. El cliente confirma y paga.
5. El sistema no genera una orden duplicada en la fecha automática (ya quedó cubierto).

**Regla**
Si el cliente paga **antes** de la fecha de Emisión, no se genera una orden duplicada en la fecha automática. El nuevo periodo se activa recién en la fecha de corte (Vencimiento actual), no antes.

**UI**
El botón de pago debe decir **"Pagar por adelantado"** (no solo el monto) mientras no exista una orden generada, y el detalle debe indicar para qué periodo es.

### 7.3 Compra de complementos

**Flujo**
1. El cliente entra a la sección "Complementos" y selecciona el recurso que quiere agregar (almacén, caja, usuario, sucursal o tienda virtual) y la cantidad.
2. Confirma la intención de compra.
3. El sistema identifica en cuál de las 4 condiciones está la cuenta (ver tabla) y calcula el cobro correspondiente.
4. Si la condición ofrece 2 opciones (A/B), el sistema se las presenta al cliente antes de confirmar el pago.
5. El cliente elige una opción (si aplica) y confirma.
6. El sistema ejecuta el cobro (esperando confirmación de la pasarela antes de aplicar el cambio) y/o actualiza la orden correspondiente, y dispara notificación + correo.

**Reglas de compra (según el momento)**

| # | Condición | Qué pasa |
|---|---|---|
| 1 | No existe orden del periodo, y el plan no está prepagado | Cobro inmediato prorrateado (hasta el próximo corte), condicionado a confirmación de pago. Se combina automáticamente en la orden que se genere después. Sin opciones. |
| 2 | La orden del siguiente periodo **ya fue generada** | Cobro del prorrateo del periodo actual (aparte, siempre). 2 opciones para el complemento del siguiente periodo: **(A, default)** dejar pendiente → se suma a la orden ya existente; **(B)** pagar todo de inmediato junto con la orden ya generada. |
| 3 | El plan ya está pagado por adelantado, y aún queda fecha automática pendiente | Cobro del prorrateo (aparte). 2 opciones para el periodo siguiente: (A) dejar pendiente → orden nueva **solo de complementos** en la fecha automática; (B) pagar también por adelantado. |
| 4 | El plan ya está pagado por adelantado y **no** queda fecha automática pendiente | Única opción: pagar también por adelantado. |

**Fórmula de prorrateo**
```
Monto = (precio del complemento / 30 o 365) × días restantes hasta el próximo corte
```

### 7.4 Baja de complementos

> ⚠️ v1/provisional — en revisión con el PO por posible cambio de UX (dar opción de elegir antes en vez de solo auto-seleccionar).

**Flujo**
1. Cliente cancela el complemento desde la sección **Planes** (autoservicio).
2. Sigue usándolo hasta fin del periodo ya pagado — sin corte inmediato, sin reembolso.
3. Al llegar el corte, si hay exceso sobre el nuevo límite, el sistema **autodeshabilita la instancia más nueva** (almacén, caja, usuario o sucursal).
4. Notificación (in-app + correo) de cuál fue deshabilitada.
5. El cliente puede reasignar libremente después desde el listado del recurso (swap).

**Advertencia previa**
Antes de confirmar la baja, se muestra cuál instancia se autodeshabilitará.

**Tiendas virtuales**
Nunca se eliminan — solo se deshabilitan (mismo patrón que almacenes, cajas, usuarios y sucursales).

### 7.5 Cambio de Plan (Upgrade / Downgrade)

**Flujo**
1. El cliente entra a "Mejorar mi plan actual" (o la opción equivalente de cambio de plan).
2. Selecciona el plan nuevo.
3. Si el cambio reduce algún límite de recursos, el sistema muestra la advertencia previa (qué recursos se verán afectados).
4. El cliente confirma el cambio.
5. El sistema calcula el cobro o crédito correspondiente, según la matriz de upgrade/downgrade.
6. Si el cambio genera un cobro, **el cambio no se hace efectivo hasta que la pasarela confirme el pago**. Si genera saldo a favor o diferencia cero, se aplica de inmediato.
7. La orden se actualiza o se genera según corresponda.

**Matriz de Upgrade**

| | Orden NO generada | Orden YA generada |
|---|---|---|
| **Periodo PAGADO** | Cobro del ajuste prorrateado; el cambio se aplica al confirmarse el pago; luego orden normal en la fecha automática | Cobro del ajuste; al confirmarse, la orden ya generada se actualiza al plan nuevo |
| **Periodo GRATUITO** | Cobro del precio completo del plan nuevo (adelantado obligatorio); el cambio se aplica al confirmarse el pago | Solo se actualiza la orden ya generada, sin cobro inmediato |

**Downgrade**
1 sola opción — el resultado negativo se convierte en saldo a favor automático y se aplica **de inmediato**.

**Impacto en recursos**
Acceso se mantiene hasta fin del periodo actual; los nuevos límites se aplican al iniciar el nuevo periodo (auto-deshabilitar el más nuevo + advertencia previa obligatoria).

**Independencia con Complementos**
Cambiar de plan **nunca** toca los complementos ya contratados.

> ⏳ **Pendiente de confirmar**: si la lógica de opciones A/B de la Regla 2 de complementos (7.3) también aplica al caso análogo "Periodo PAGADO + Orden YA generada" de esta sección — hoy no se ofrece alternativa.

### 7.6 Cambio de Ciclo (Mensual ↔ Anual)

**Flujo**
1. El cliente entra a "Gestión de suscripción" y elige cambiar su ciclo de facturación.
2. El sistema calcula el crédito por lo no usado del ciclo actual y el costo completo del ciclo nuevo.
3. Se calcula el monto neto a cobrar (o se genera saldo a favor si el resultado es negativo).
4. Si el resultado es un cobro, el sistema **espera la confirmación de la pasarela** antes de aplicar el cambio de ciclo. Si es saldo a favor, se aplica de inmediato.
5. El nuevo ciclo arranca al confirmarse el pago (o de inmediato si no requiere cobro).

**Fórmula**
```
Crédito = (precio_viejo / días_ciclo_viejo) × días_restantes
Cobro neto = precio_nuevo_completo − Crédito
```
Los complementos cambian automáticamente a la tarifa del nuevo ciclo.

### 7.7 Pago Automático (tarjetas guardadas)

**Flujo**
1. Cliente registra tarjeta(s); una es predeterminada. Registrar = activación automática.
2. **3 intentos, uno por día, durante los últimos 3 días antes e incluyendo la Fecha de Vencimiento** (ej. vencimiento=30 → intentos en los días 28, 29 y 30).
3. Orden en cada intento: predeterminada primero, luego las demás.
4. Notificación al cliente en cada intento fallido.
5. Si el cliente paga manualmente antes de que corra un intento automático programado, dicho intento se cancela.
6. Sin tarjeta registrada → sin intentos automáticos, depende del pago manual.
7. Si los 3 intentos fallan y se llega a la Fecha de Vencimiento sin pago, la cuenta pasa a `MOROSA`.

### 7.8 No pago / Deuda / Bloqueo

**Flujo**
1. Se cumple la Fecha de Vencimiento de una orden sin que el cliente pague.
2. El cliente intenta realizar cualquier acción en el sistema.
3. El sistema muestra un modal con el detalle de la deuda + botón de pago, bloqueando la acción. La cuenta pasa a `MOROSA`.
4. Si pasan los días configurados (10 por defecto) sin pago, la cuenta pasa a `SUSPENDIDA` y el cliente pierde el acceso por completo.
5. El cliente solo puede ver la pantalla de opciones de pago.
6. Al pagar, el cliente reactiva su cuenta: **selecciona plan nuevamente desde cero**, con sus complementos anteriores precargados y con opción de eliminarlos.

**`MOROSA` (0–umbral configurable, 10 días por defecto)**
Ve todo, no ejecuta ninguna acción (solo pagar). No se generan órdenes nuevas.

**`SUSPENDIDA` (umbral configurado superado)**
No ve nada — solo pantalla de opciones de pago.

**Reactivación (`SUSPENDIDA` o `CANCELADA` — mismo flujo)**
Pasa por "Seleccionar Plan" de nuevo. Complementos previos aparecen precargados y removibles en el resumen.

### 7.9 Cancelación voluntaria

**Flujo**
1. El cliente hace clic en "Cancelar suscripción".
2. El sistema confirma la cancelación e informa hasta cuándo mantendrá el acceso (fin del periodo vigente).
3. El cliente sigue usando el sistema con normalidad hasta esa fecha (`CANCELACION_PROGRAMADA`).
4. Al llegar la fecha, pierde el acceso — la suscripción pasa a `CANCELADA`.
5. Si decide volver, reactiva su cuenta (mismo flujo que una cuenta `SUSPENDIDA`).

**Detalle**
Acceso se mantiene hasta fin del periodo vigente. No hay reembolsos automáticos (a lo mucho, crédito manual otorgado por Soporte).

### 7.10 Empresa Gratuita / GCG — vista del cliente

Mientras la cuenta está bajo régimen GCG:
- Acceso total, respetando los límites de recursos del plan base.
- No se generan órdenes.
- Complementos también son gratuitos.
- La cuenta **no genera ni conserva saldo a favor** mientras dure la condición.

> La activación/desactivación y la mecánica administrativa de este régimen se documentan en `aplicativo-administrativo-soporte-v1.md`.

### 7.11 UI — Modal "Ver pago" (variantes por estado)

**Flujo**
1. El cliente entra a la grilla de "Órdenes de pago".
2. Abre el menú de acciones de una fila y selecciona "Ver pago".
3. El sistema muestra el modal con la estructura fija más la variante correspondiente al estado de esa orden.

| # | Estado del Pago | Estado del Comprobante |
|---|---|---|
| 1 | Pagado | Aceptado |
| 2 | Pagado | Rechazado por SUNAT |
| 3 | Pagado | En Proceso |
| 4 | Deuda Pendiente | — (sin intento) |
| 5 | Deuda Pendiente | — (con intento rechazado) |
| 6 | Deuda Vencida | — (sin intento) |
| 7 | Deuda Vencida | — (con intento rechazado) |

**Estructura fija**: N° de orden + badge Estado + Fecha Emisión/Vencimiento + Plan/Complementos + Descuento global + Total + Periodo.
**Solo si Pagado**: bloque Comprobante + Forma de pago (datos reales) + tabla itemizada.
**Si no pagado**: Comprobante = "-"; Forma de pago = último intento (si hubo) con mensaje genérico de rechazo (catálogo de mensajes genéricos).

---

## 8. Reglas de Negocio

1. Cada periodo tiene fecha de corte. 5 días antes se genera la Orden + notificación.
2. El corte cierra a las 00:00 del día siguiente al Vencimiento.
3. Ciclo mensual = 30 días fijos; anual = 365 días fijos.
4. Pago adelantado evita duplicar orden; el nuevo periodo arranca en la fecha de corte, no antes.
5. Complementos ilimitados en cantidad; costo aparte del plan.
6. Las 4 reglas de compra de complementos (ver 7.3) amplían el modelo general de prorrateo.
7. Las órdenes pueden actualizarse automáticamente **solo mientras no han sido pagadas**; una vez pagadas, quedan inmutables.
8. Todo pago exitoso (sin excepción) dispara notificación + correo.
9. Baja de complementos: auto-deshabilitar el más nuevo + advertencia previa + reasignación manual libre después.
10. Deshabilitar ≠ Eliminar. Nunca se elimina un recurso, solo se deshabilita.
11. Precio y recursos siempre alineados entre planes (más caro = más de todo).
12. Cambio de plan o ciclo: si genera cobro, espera confirmación de la pasarela antes de aplicarse; si genera saldo a favor o diferencia cero, se aplica de inmediato.
13. Cambio de plan nunca toca complementos ya contratados.
14. Cambio de ciclo: mismo tratamiento que cambio de plan respecto a espera de confirmación de pago, en ambas direcciones.
15. Saldo a favor: se acumula en un único saldo de cuenta, se aplica automático contra la siguiente orden real; si sobra, se guarda para la próxima.
16. No puede coexistir saldo a favor sobrante + deuda/bloqueo simultáneamente.
17. Tarjeta guardada = activación automática del cobro automático.
18. 3 intentos automáticos, uno por día, en los últimos 3 días antes e incluyendo el Vencimiento; predeterminada primero. Un pago manual cancela el intento automático programado.
19. `MOROSA`: umbral configurable (10 días por defecto) tras Vencimiento, sin acciones, solo pagar.
20. `SUSPENDIDA`: al superar el umbral configurado, sin acceso al sistema.
21. `CANCELADA` es exclusivo de baja voluntaria; nunca del bloqueo automático (`SUSPENDIDA`).
22. Datos de la empresa se preservan intactos e indefinidamente en cualquier estado.
23. Reactivación (`SUSPENDIDA` o `CANCELADA`): mismo flujo, elegir plan de nuevo, complementos removibles.
24. Pago (dinero recibido) y Comprobante (documento fiscal) son estados independientes y secuenciales.
25. Motivo de rechazo: específico solo si la pasarela lo entrega; si no, catálogo de mensajes genéricos.
26. El saldo a favor pertenece a la empresa que lo generó y **no es transferible por autoservicio**.

---

## 9. Configuraciones del sistema

| Configuración | Valor |
|---|---|
| Días de anticipación para emisión de orden | 5 |
| Duración de ciclo mensual | 30 días |
| Duración de ciclo anual | 365 días |
| Ventana de estado `MOROSA` | Configurable — 10 días por defecto |
| Umbral para pasar a `SUSPENDIDA` | Configurable — 10 días por defecto |
| N° de intentos de pago automático | 3 |
| Ventana de intentos automáticos | Últimos 3 días antes e incluyendo el Vencimiento (uno por día) |
| Orden de cobro de tarjetas | Predeterminada → resto en orden de registro |
| Pasarela de pagos | Culqi |

---

## 10. Escenarios Gherkin

### Feature: Módulo de pagos — Cliente

### Background
```gherkin
Dado que existe una empresa con un plan activo
Y su ciclo de facturación es mensual (30 días)
```

### 10.1 Happy path

```gherkin
Escenario: Se genera la orden 5 días antes del corte
  Dado que faltan 5 días para la Fecha de Vencimiento del periodo actual
  Cuando el sistema ejecuta el proceso de emisión automática
  Entonces se genera una Orden de pago con Fecha de Emisión = hoy
  Y se envía una notificación de "debes pagar"

Escenario: El cliente paga dentro del plazo
  Dado que existe una Orden de pago en estado "Deuda Pendiente"
  Cuando el cliente paga el monto total antes de la Fecha de Vencimiento
  Entonces la Orden pasa a estado "Pagado"
  Y se envía notificación y correo de pago exitoso
  Y se renueva el periodo por la duración correspondiente
  Y se genera el comprobante fiscal después de la confirmación del pago
```

### 10.2 Complementos

```gherkin
Escenario: Comprar un complemento antes de que exista orden
  Dado que no existe ninguna orden generada para el siguiente periodo
  Y el plan no está pagado por adelantado
  Cuando el cliente compra un complemento
  Entonces se genera el cobro de inmediato por el prorrateo de los días restantes
  Y el complemento se incluye automáticamente en la próxima orden que se genere

Escenario: Comprar un complemento cuando la orden ya fue generada, eligiendo Opción A
  Dado que existe una Orden de pago en estado "Deuda Pendiente" para el siguiente periodo
  Cuando el cliente compra un complemento
  Entonces se cobra de inmediato el prorrateo del periodo actual, aparte
  Y el cliente elige la Opción A por defecto
  Y el costo completo del complemento se suma a la Orden ya existente
  Y no se genera ninguna orden nueva

Escenario: Dar de baja un complemento con exceso de recursos al llegar el corte
  Dado que el cliente tiene 4 cajas habilitadas y su nuevo límite tras la baja es 3
  Cuando llega la Fecha de Vencimiento del periodo actual
  Entonces el sistema deshabilita automáticamente la caja más reciente
  Y se envía notificación y correo indicando cuál caja fue deshabilitada
```

### 10.3 Cambio de plan y ciclo

```gherkin
Escenario: Upgrade en un periodo pagado sin orden generada
  Dado que el cliente está en un periodo pagado y no existe orden para el siguiente periodo
  Cuando el cliente cambia a un plan más caro
  Entonces se genera el cobro de la diferencia prorrateada por los días restantes
  Y el cambio de plan no se aplica hasta que la pasarela confirme el pago
  Y en la fecha automática se genera la orden normal con el precio del plan nuevo

Escenario: Downgrade genera saldo a favor
  Dado que el cliente cambia a un plan más económico
  Cuando se calcula la diferencia de precio prorrateada
  Y el resultado es negativo
  Entonces se genera un saldo a favor por ese monto
  Y el cambio se aplica de inmediato, sin esperar confirmación de pago

Escenario: Cambio de ciclo con cobro pendiente de confirmación
  Dado que el cliente decide cambiar de ciclo mensual a anual
  Y el cálculo neto resulta en un cobro
  Cuando el cliente confirma el cambio
  Entonces el sistema espera la confirmación de la pasarela antes de activar el nuevo ciclo

Escenario: Advertencia previa por impacto en recursos
  Dado que el plan nuevo tiene menos sucursales incluidas que las que el cliente tiene habilitadas
  Cuando el cliente confirma el cambio de plan
  Entonces se muestra una advertencia previa a cualquier pago indicando qué recursos se verán afectados
```

### 10.4 No pago

```gherkin
Escenario: Entrada en morosidad
  Dado que la Fecha de Vencimiento de una orden pasó sin que se pague
  Cuando pasan las 00:00 del día siguiente
  Entonces el estado de la cuenta cambia a "MOROSA"
  Y el cliente puede ver el sistema pero no ejecutar ninguna acción

Escenario: Suspensión tras superar el umbral configurado
  Dado que la cuenta está en estado "MOROSA" desde hace más días que el umbral configurado (10 por defecto)
  Cuando se cumple ese plazo sin pago
  Entonces el estado de la cuenta cambia a "SUSPENDIDA"
  Y el cliente solo puede ver la pantalla de opciones de pago

Escenario: Reactivación de una cuenta suspendida
  Dado que una cuenta está en estado "SUSPENDIDA"
  Cuando el cliente decide pagar y reactivar su cuenta
  Entonces debe seleccionar un plan nuevamente
  Y sus complementos anteriores aparecen precargados con opción de eliminarlos
```

### 10.5 Pago automático

```gherkin
Escenario: Primer intento automático exitoso
  Dado que faltan 3 días para el Vencimiento de una orden
  Y el cliente tiene una tarjeta predeterminada registrada
  Cuando el sistema ejecuta el primer intento de cobro automático
  Entonces se cobra con la tarjeta predeterminada
  Y la orden pasa a estado "Pagado"

Escenario: Intento automático rechazado
  Dado que el sistema intenta cobrar automáticamente una tarjeta
  Cuando la pasarela rechaza el cobro
  Entonces se notifica al cliente el rechazo
  Y se registra el intento en la Bitácora de la orden
  Y se prueba con la siguiente tarjeta registrada, si existe

Escenario: Se agotan los 3 intentos automáticos sin éxito
  Dado que el sistema ejecutó los 3 intentos automáticos (días -2, -1 y 0 respecto al Vencimiento)
  Y todos fueron rechazados
  Cuando se cumple la Fecha de Vencimiento
  Entonces la cuenta pasa a estado "MOROSA"

Escenario: Pago manual cancela el intento automático programado
  Dado que existe un intento de cobro automático programado para hoy
  Cuando el cliente paga manualmente antes de que se ejecute
  Entonces el intento automático programado se cancela
```

---

## 11. Dependencias

### 11.1 Dependencias funcionales

- Módulo de Planes y Complementos (catálogo de precios mensual/anual).
- Módulo de Recursos (Almacenes, Cajas, Usuarios, Sucursales, Tiendas virtuales) — para el auto-deshabilitar.
- Sistema de notificaciones in-app (aún en construcción).
- Aplicativo web de administración (para los casos que Soporte debe resolver — ver documento separado).

### 11.2 Dependencias técnicas

- Pasarela de pagos **Culqi** (cobro, motivo de rechazo cuando esté disponible).
- Integración con **SUNAT** para validación de comprobantes.
- Servicio de correo transaccional (mientras no exista notificación in-app).
- Este módulo comparte la misma API de Cobranza y la misma base de datos que el aplicativo administrativo (proyectos de interfaz separados).

---

## 12. Pendientes / Temas Abiertos

- [ ] Confirmar si la lógica A/B de la Regla 2 de complementos (7.3) también aplica al caso análogo de Upgrade con orden ya generada (7.5).
- [ ] Notificación in-app de comprobante rechazado por SUNAT, cuando el sistema de notificaciones esté listo.
- [ ] Flujo de baja de complementos con impacto en recursos en uso — mecánica v1/provisional, en revisión de UX (7.4).
- [ ] Manejo técnico de la falla cuando Culqi confirma el pago pero el sistema falla al persistir la confirmación (compensación/reconciliación) — pendiente de diseño técnico.
- [ ] Reversión/anulación de pagos confirmados: fase futura, fuera de alcance de esta iteración.
- [ ] Penalidades de reactivación: fase futura, fuera de alcance de esta iteración.

---

*Documento vivo — versión 1 (desglosado del documento unificado v4). Para las acciones de Soporte, ver `aplicativo-administrativo-soporte-v1.md`.*
