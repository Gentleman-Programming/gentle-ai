# ODD: Contratos de Protocolo y Schemas Duales Axiom

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/contratos-y-schemas-dual-axiom.md`.
> Espejo de recuperación en Engram: topic `odd/contratos-y-schemas-dual-axiom/tasks`, proyecto `axiom`.

## Objetivo

Introducir los identificadores de esquema y contrato de protocolo canónicos de Axiom (`axiom.review-integration/v2`, `axiom.sdd-status/v1`, `axiom.telemetry-event/v1`, etc.) en los despachadores de revisión RDD y de estado SDD, implementando un mecanismo de negociación dual retrocompatible que permita la adopción del nuevo protocolo canónico sin romper clientes o herramientas existentes que aún soliciten esquemas bajo el prefijo legado `gentle-ai.*`.

## Problema

1. Las invocaciones de RDD (`axiom review status --contract gentle-ai.review-integration/v2`) y las aserciones de compuerta SDD (`gentle-ai.sdd-status/v1`, `gentle-ai.review-integration.consent/v3`) contienen la denominación `gentle-ai.` en constantes de Go y validadores de schemas JSON.
2. Varios paquetes críticos (`internal/reviewtransaction`, `internal/sddstatus`, `internal/telemetrycollector`) aplican validación estricta (*fail-closed*): si un cliente o subagente envía un sobre que no coincide exactamente con el schema esperado, la transacción se aborta.
3. Las pruebas de congelación (`contracts/review-integration/v1/FREEZE.md`, `internal/releasepolicy/policy.go`) auditan que no se alteren los esquemas versionados sin un incremento formal de versión.
4. Un renombramiento simple y unilateral provocaría rechazos inmediatos en clientes, subagentes de revisión o herramientas de telemetría.

## Alcance Autorizado

- `internal/reviewtransaction/`: Definir las constantes canónicas de protocolo `AxiomReviewIntegrationV2Contract = "axiom.review-integration/v2"` y mantener mapa de equivalencias bidireccional con `gentle-ai.review-integration/v2`.
- `internal/reviewtransaction/dispatcher.go` (o equivalente): Aceptar ambos contratos en el flag `--contract`, emitiendo la respuesta en el schema negociado (o canónico Axiom por defecto).
- `internal/sddstatus/`: Admitir `axiom.sdd-status/v1` y `axiom.sdd-integration.consent/v1` junto a sus variantes legadas en la verificación de sobres y transiciones de estado.
- `contracts/`: Incorporar las definiciones de schema correspondientes bajo el espacio de nombres de Axiom o actualizar los enums de `$id` y `schema` para admitir ambas identidades.
- `internal/telemetrycollector/`: Admitir eventos `axiom.telemetry-event/v1` manteniendo compatibilidad con `gentle-ai.telemetry-event/v1`.
- Pruebas de integración de negociación cruzada: cliente Axiom pide contrato Axiom (éxito), cliente legado pide contrato Gentle AI (éxito con envelope compatible), contrato inválido (rechazo tipado).

## Restricciones

- **Negociación no destructiva:** Toda petición con `--contract gentle-ai.review-integration/v2` debe seguir funcionando y devolviendo sobres válidos.
- **Inmutabilidad de contratos congelados:** Las carpetas `contracts/review-integration/v1/` marcadas con `FREEZE.md` no deben alterarse; los nuevos esquemas se añaden en su versión correspondiente o como ampliación no destructiva de v2.
- **Idioma Obligatorio:** Español (castellano peninsular con tuteo profesional) para reportes, especificaciones y documentación de contratos.

---

## Tareas

- [ ] **T1 · Mapeo canónico de contratos y alias de compatibilidad en RDD**
  - Definir identificadores canónicos: `axiom.review-integration/v2`, `axiom.review-integration.consent/v3`, `axiom.review-assessment/v1`, `axiom.review-acknowledged/v1`.
  - Crear resolver centralizado de contratos (`ResolveReviewContract(requested string) (canonical string, isLegacy bool, err error)`).
  - Permitir que el despachador responda con el formato solicitado preservando los campos semánticos.

- [ ] **T2 · Soporte dual en compuertas y estado SDD**
  - Implementar soporte para `axiom.sdd-status/v1` y `axiom.sdd-integration.consent/v1` en `internal/sddstatus/`.
  - Validar que `axiom sdd-status --json` y las comprobaciones de transición acepten ambos contratos sin rechazo.

- [ ] **T3 · Actualización y ampliación de Schemas JSON**
  - Actualizar los schemas JSON de validación (`contracts/review-integration/v2/schemas/`) para admitir tanto `axiom.*` como `gentle-ai.*` en las propiedades `"enum"` de identificación de schema.
  - Asegurar que los validadores de esquema de tests pasen sin infracciones de política de release.

- [ ] **T4 · Contratos de telemetría y eventos de colector**
  - Soportar `axiom.telemetry-event/v1` y `axiom.telemetry-runtime-event/v1` en `internal/telemetrycollector/`.
  - Actualizar el emisor de telemetría en `internal/telemetry/` para usar la cabecera canónica de Axiom.

- [ ] **T5 · Batería de pruebas de negociación de contratos**
  - Tests unitarios y de tabla que comprueben:
    1. Negociación canónica directa con `axiom.review-integration/v2`.
    2. Negociación retrocompatible con `gentle-ai.review-integration/v2`.
    3. Validación de contratos desconocidos (debe fallar con lista de contratos admitidos).
    4. Serialización y deserialización de sobres de estado SDD y RDD.
