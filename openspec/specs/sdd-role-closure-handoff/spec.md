<!-- Especificación Viva generada a partir de '2026-09-22-inc-21-upfront-flow-governance' -->

# Especificación de Requerimientos: Cierre de Último Rol, Aviso Formal y Relevo de Integración (INC-21)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y escenarios BDD para el cierre determinista del último rol activo en un cambio SDD, incluyendo el aviso formal al usuario, la generación del artefacto `handoff.md` de integración y la transición hacia la fase `verify` global de toda la solución.

---

## 1. Capacidad: `sdd-role-closure-handoff`

Al concluir el último rol activo del cambio, el orquestador cierra la fase de implementación con un aviso formal, un relevo de integración y la apertura de la verificación global de toda la solución.

### Requirement: Aviso Formal de Conclusión del Último Rol Activo (REQ-21.13)

Al aprobarse la compuerta de `apply` (REQ-21.11) del último rol pendiente entre los roles activos del cambio —incluido el caso de un único rol `fullstack`— el orquestador DEBE notificar formalmente al usuario que ha concluido la implementación del último rol activo y que se genera el relevo de integración para la verificación global de la solución.

#### Scenario: Último de varios roles activa el aviso y el relevo

- **DADO** un cambio con los roles `core`, `web` y `qa`, donde `core` y `web` ya están concluidos y `qa` es el único rol pendiente
- **CUANDO** se aprueba la compuerta de `apply` del rol `qa`
- **ENTONCES** el orquestador emite el aviso formal de conclusión del último rol
- **Y** genera el relevo de integración descrito en REQ-21.14

#### Scenario: Rol único fullstack satisface la condición de último rol de inmediato

- **DADO** un cambio con el rol único `fullstack`
- **CUANDO** se aprueba la compuerta de `apply` de ese único rol
- **ENTONCES** esa misma aprobación satisface la condición de "último rol activo"
- **Y** el orquestador emite el aviso formal y genera el relevo de integración inmediatamente, sin esperar ningún otro rol

#### Scenario: Compuerta rechazada no dispara el cierre aunque sea el único rol pendiente

- **DADO** un cambio en el que el único rol pendiente tiene su compuerta de `apply` rechazada (REQ-21.12)
- **CUANDO** se registra el rechazo
- **ENTONCES** el orquestador no emite el aviso de último rol ni genera el relevo de integración
- **Y** espera a que esa compuerta se apruebe explícitamente antes de reevaluar la condición de cierre

---

### Requirement: Generación del `handoff.md` de Integración (REQ-21.14)

Junto con el aviso de REQ-21.13, el orquestador DEBE generar un artefacto `handoff.md` canónico, conforme al esquema ya definido para relevos estructurados (frontmatter YAML con `from_phase`, `to_phase`, `from_role`, `to_role`, `timestamp` y `status`, seguido de las cinco secciones descriptivas), con `status: ready`, consolidando los cambios de todos los roles participantes y especificando las instrucciones de prueba para la fase `verify`.

#### Scenario: Handoff de integración consolida todos los roles participantes

- **DADO** el aviso de conclusión del último rol activo de un cambio con varios roles
- **CUANDO** el orquestador genera el relevo de integración
- **ENTONCES** el `handoff.md` resultante declara `from_phase: apply`, `to_phase: verify` y `status: ready`
- **Y** su contenido consolida los artefactos y decisiones de todos los roles participantes, no solo del último rol concluido

---

### Requirement: Ejecución de la Fase `verify` Global tras el Handoff (REQ-21.15)

Recibido el relevo de integración en estado `ready` (REQ-21.14), la fase `verify` DEBE ejecutar la suite completa de pruebas de la solución (integración, regresión y compuertas de seguridad aplicables) considerando el trabajo consolidado de todos los roles, y DEBE emitir un informe consolidado `verify-report.md`, distinto de los informes individuales `verify-report.<rol>.md` de cada rol. La fase `verify` global NO DEBE iniciarse si el `handoff.md` de integración no tiene `status: ready`.

#### Scenario: Verify global emite un informe consolidado distinto de los informes por rol

- **DADO** un `handoff.md` de integración con `status: ready` para un cambio con varios roles ya verificados individualmente
- **CUANDO** se ejecuta la fase `verify` global
- **ENTONCES** se genera `verify-report.md` consolidado, distinto de los `verify-report.<rol>.md` de cada rol
- **Y** el informe refleja la suite completa de pruebas de la solución, no solo la de un rol

#### Scenario: Handoff bloqueado impide iniciar el verify global

- **DADO** un `handoff.md` de integración con `status: blocked` o `needs_clarification`
- **CUANDO** se evalúa si procede iniciar la fase `verify` global
- **ENTONCES** el orquestador no la inicia
- **Y** exige resolver el bloqueo del relevo antes de reintentar
