# Especificación Viva: Cobertura de Integración Continua sobre el Binario Canónico

> **Dominio:** `axiom-binary-ci-coverage`  
> **Versión Canónica:** 1.0.0  
> **Estado:** Vigente  
> **Idioma:** Español (Castellano peninsular)

---

## 1. Capacidad: `axiom-binary-ci-coverage`

El pipeline de CI DEBE construir y ejercitar el binario canónico `cmd/axiom`, incluida su superficie exclusiva, asegurando que los comandos propios del fork no regresionen ni dependan exclusivamente del shim de compatibilidad.

### Requirement: Construcción y ejercicio bloqueante de la superficie exclusiva de axiom (REQ-20.15)

El flujo de trabajo de CI DEBE construir `cmd/axiom` y ejercitar de forma bloqueante su superficie exclusiva: `init`, `change`, `project`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive`, y el arranque/ayuda de `ui`.

#### Scenario: Humo bloqueante sobre la superficie exclusiva
- **DADO** el binario `cmd/axiom` recién construido en CI
- **CUANDO** el paso de cobertura se ejecuta
- **ENTONCES** ejercita los comandos de la superficie exclusiva de Axiom con salida esperada y stderr limpio de avisos de deprecación
- **Y** un fallo en cualquiera de ellos bloquea la fusión

---

### Requirement: Ventana informativa acotada y registrada para superficie roja (REQ-20.16)

Si el paso de cobertura de `cmd/axiom` destapa fallos preexistentes fuera de la superficie que bloquea el contrato de nombre, el sistema DEBE mantener esa parte como informativa únicamente durante una ventana acotada con fecha de apertura e incremento sucesor nombrado.

#### Scenario: Ventana informativa con inventario e incremento sucesor
- **DADO** que el paso de cobertura destapa fallos en una superficie de `cmd/axiom` no cubierta por el contrato de nombre
- **CUANDO** se registra esa parte como informativa
- **ENTONCES** el registro incluye el inventario escrito de la superficie roja y el nombre del incremento sucesor
