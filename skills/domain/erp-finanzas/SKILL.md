---
name: domain-erp-finanzas
description: "Contrato de lógica de negocio y fronteras para el dominio erp-finanzas. Trigger: operaciones vinculadas a cajas, cuentas o reglas financieras."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "2.0"
---

## 1. Execution Role
Cargar este skill para entender el flujo funcional cuando se diseñen, desarrollen o prueben flujos de **Finanzas** (ej. Cuentas por Cobrar, Caja Chica).

## 2. Business Language Contract
- Usar estrictamente los términos de negocio: `Caja Chica`, `Estado de Cuenta`, `Documento Financiero` y `Cuentas por Cobrar`. No inventar traducciones o sinónimos.

## 3. Business Invariants
1. **Veracidad Transaccional (Base de Datos):** La base de datos fuente es `BD Finanzas`. Existe sincronización asíncrona hacia Punto de Venta.
2. **Registro de Movimientos:** 
   - Permitido ingresar registros con fechas desfasadas (distintas a la actual) SOLO si se aplican reglas de auditoría de cierre de mes.
3. **Comunicación de Eventos:**
   - La regla dicta que toda transacción importante publica obligatoriamente en `KafkaTopics.Finanzas` (key: `erp_finanza`).

## 4. Implementation Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Cierre de Caja** | Verificación asíncrona de saldo contra `BD Finanzas` | Guardado directo en BD sin lanzar evento Kafka |
| **Sincronización** | Delegado a "Job de finanzas" (procesos batch) | Forzar sincronización bloqueante en la API |
| **Documentos Financieros**| Validar que la fecha no comprometa cierres contables | Permitir cualquier fecha sin rastro de auditoría |

## 5. Output / Delivery Contract
Al modificar flujos de negocio, retornar un resumen confirmando que se preservó la consistencia en Kafka y que no se rompieron los Jobs de regularización batch.
