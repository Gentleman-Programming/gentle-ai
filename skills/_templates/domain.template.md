<!--
PLANTILLA: skills/domain/<bounded-context-id>/SKILL.md
Categoría cognitiva: "El Negocio" — qué significa funcionalmente lo que el
agente está construyendo (Domain-Driven Design). Responde: "¿qué significa
esto para el negocio?", no "¿cómo se implementa?" (eso es technology/).

GRANULARIDAD: esto es POR MÓDULO DE NEGOCIO, no por repositorio ni por
entidad individual. Un módulo = un contexto acotado con límites claros, del
tamaño que se puede explicar en una sola conversación con negocio. Ejemplo
de separación correcta en este ecosistema:

  domain/
  ├── facturacion-sunat/        (no "facturacion-y-logistica-y-planillas" junto)
  ├── logistica-inventarios/
  ├── planillas/
  ├── punto-de-venta/
  ├── finanzas-contabilidad/
  └── pagos/

Si un módulo crece demasiado para leerse de una sola vez (ej.
"facturacion-sunat" termina con reglas de retención + crédito fiscal +
comprobantes electrónicos, cada una compleja por sí sola), ahí SÍ se
sub-divide en carpetas dentro del mismo módulo
(`domain/facturacion-sunat/retenciones/`,
`domain/facturacion-sunat/comprobantes/`) — pero solo cuando de verdad
se vuelve difícil de leer junto, nunca por defecto.

Objetivo explícito: que el código use el MISMO lenguaje que usa el negocio.
Si el negocio dice "monto imponible", el código y este documento deben decir
"monto imponible", nunca "taxableAmount" ni "money" a secas.

Reemplaza cada bloque [ENTRE CORCHETES]. Si una regla de negocio no está
confirmada, escribe "PENDIENTE DE VALIDAR CON NEGOCIO: <pregunta exacta>" en
vez de asumir — nunca conviertas una suposición en regla de negocio.
-->
---
name: [bounded-context-id]              # ej. reglas-facturacion-sunat, glosario-contable, autenticacion-corporativa
description: "Trigger: [bounded-context-id]. Domain rules for [una frase: qué parte del negocio cubre]."
license: Apache-2.0
metadata:
  author: [tu nombre]
  version: "1.0"
---

# Domain: [Nombre legible del contexto acotado]

## 1. Propósito del contexto
[Qué parte del negocio cubre este bounded context y, tan importante como eso,
qué NO cubre — dónde empieza otro dominio. 2-4 frases.]

## 2. Lenguaje ubicuo (glosario obligatorio)
| Término del negocio | Significa | NUNCA usar en su lugar |
|---|---|---|
| [ej. monto imponible] | [definición exacta, con la fórmula o regla si aplica] | [ej. "taxable amount", "money"] |
| [término 2] | [definición] | [sinónimo prohibido] |

## 3. Entidades y agregados principales
- **[Nombre de la entidad]**: [qué representa, invariantes que siempre debe
  cumplir — ej. "un Comprobante nunca existe sin al menos un ítem"]
- **[Agregado 2]**: [...]

## 4. Reglas de negocio obligatorias
1. [Regla numerada, verificable y sin ambigüedad — evitar "debería", usar
   "DEBE"/"NUNCA". Ejemplo: "Una factura con retención DEBE calcular el
   monto retenido antes de emitir el comprobante ante SUNAT."]
2. [Regla 2]

## 5. Procesos / flujos clave
### [Nombre del caso de uso real, ej. "Emisión de factura con retención"]
1. [Paso del flujo de negocio, no técnico — qué decide el negocio en cada paso]
2. [Paso 2]

## 6. Relaciones con otros dominios
| Este dominio... | ...con el dominio | Vía |
|---|---|---|
| [consume / expone] | `domain/[otro-contexto]` | [evento, API, tabla compartida] |

## 7. Regulación / compliance aplicable
[Normativa externa que este dominio debe respetar (ej. SUNAT, protección de
datos), con la fuente oficial si existe. Si no aplica, escribe "No aplica
regulación externa conocida" explícitamente — no lo omitas en silencio.]

## 8. Anti-patrones conocidos
- ❌ [Ejemplo concreto de algo que YA pasó o podría pasar y por qué viola el
  dominio] → ✅ [la forma correcta]

## 9. Preguntas abiertas
- [ ] PENDIENTE DE VALIDAR CON NEGOCIO: [pregunta exacta, y quién la puede responder]
