# Artefactos intermedios de Engram — change "cobranza-soporte-cliente"

Compilado de las 4 memorias persistentes generadas durante el proceso SDD. Cada una corresponde a una fase distinta.

---

## 1. `sdd/cobranza-soporte-cliente/explore` (obs #2838)

**What**: Exploración (sin propuesta) del change "cobranza-soporte-cliente": dos documentos de requerimientos hermanos —aplicativo-administrativo-soporte-v1.md (Soporte) y elicitacion-modulo-pagos-cliente-v1.md (Cliente/autoservicio)— que comparten la misma API de Cobranza y misma BD, pero son proyectos de interfaz separados.

**Why**: El usuario pidió documentar el estado actual antes de proponer solución (fase sdd-explore), para preparar sdd-propose sobre una mejora/expansión de un dominio de Cobranza YA implementado en backend (no greenfield).

**Where**:
- E:\2026-II\gentle-ai\aplicativo-administrativo-soporte-v1.md (leído completo)
- E:\2026-II\gentle-ai\elicitacion-modulo-pagos-cliente-v1.md (leído completo)
- Repos GitLab mencionados por el usuario (NO inspeccionados en esta sesión de agente por falta de acceso a mcp__gitlab__* / mcp__bookstack__*): gp-apps-cross/Pagos (.NET Clean Architecture), SmartClic/MSPagos (posible legado/paralelo, sin confirmar relación con Pagos), SmartClic/cliente-hub-front (Vue, frontend cliente ya desarrollado), gp-apps-cross/portal-sr-front (prácticamente vacío — candidato a ser el proyecto a construir para el aplicativo de Soporte).

### Dominio compartido (modelo unificado entre ambos docs)
- Estados de cuenta: PENDIENTE_ACTIVACION, PERIODO_GRATUITO, ACTIVA, MOROSA (0-10 días default), SUSPENDIDA (>10 días default), CANCELACION_PROGRAMADA, CANCELADA, GCG/Gratuita.
- Estados de orden de pago: Pagado (inmutable, incluso para Soporte), Deuda Pendiente, Deuda Vencida; sub-estado "Pago rechazado"; sub-estado de comprobante (Aceptado/Rechazado por SUNAT/En Proceso) solo si Pagado.
- Saldo a favor: único por empresa, se pierde en baja inmediata/suspensión/GCG, se aplica automático contra siguiente orden, NO transferible por autoservicio (solo Soporte).
- Culqi es la pasarela; motivo de rechazo específico solo si Culqi lo entrega, si no catálogo genérico.
- Bitácora/auditoría es requisito transversal para toda acción administrativa.

### Documento 1 — Aplicativo administrativo (Soporte), 12 acciones
1. Activar GCG (anula orden pendiente, respeta periodo pagado activo, empresa pierde saldo a favor)
2. Desactivar GCG (retoma facturación normal)
3. Reemitir comprobante (rechazado por SUNAT; dinero no se toca)
4. Modificar orden no pagada (Deuda Pendiente/Vencida únicamente; NUNCA orden Pagada)
5. Consultar histórico completo órdenes/pagos
6. Forzar cambio de estado de cuenta/suscripción
7. Consultar bitácora de cambios
8. Ejecutar cambios de plan/periodo en nombre del cliente
9. Transferir saldo a favor entre empresas
10. Consultar/gestionar saldo a favor
11. Aplicar precios especiales (con vigencia definida, inicio/fin)
12. Otorgar días adicionales de gratuidad

Reglas clave: todos los usuarios de Soporte mismo nivel de acceso (sin roles aún), acceso a todas las empresas sin restricción (sin segmentación aún), ninguna acción puede tocar un pago confirmado.

### Documento 2 — Módulo cliente (autoservicio)
- Ciclo de facturación: emisión 5 días antes del corte; cierre a las 00:00 del día siguiente al vencimiento; mensual=30 días fijos, anual=365 días fijos (no calendario real).
- Pago adelantado: evita orden duplicada, nuevo periodo arranca en fecha de corte real, no antes.
- Complementos (usuarios, almacenes, tiendas virtuales, cajas, sucursales) — ilimitados en cantidad, costo aparte; matriz de 4 condiciones de compra con fórmula de prorrateo = (precio/30 o 365) × días restantes; condiciones 2 y 3 ofrecen opción A (default, diferir) / B (pagar ya).
- Baja de complementos: v1/provisional en revisión de UX; auto-deshabilita instancia más nueva al corte si hay exceso; nunca se elimina, solo deshabilita; tiendas virtuales igual.
- Upgrade/downgrade: matriz 2x2 (orden generada/no generada × periodo pagado/gratuito); downgrade siempre genera saldo a favor aplicado de inmediato; cambio de plan nunca toca complementos.
- Cambio de ciclo mensual↔anual: crédito por no usado + costo completo del nuevo ciclo, espera confirmación de pasarela si hay cobro neto.
- Pago automático: 3 intentos, uno por día, en los últimos 3 días incluyendo el vencimiento; tarjeta predeterminada primero; pago manual cancela intento programado; sin tarjeta = sin intentos.
- Morosidad→Suspensión→reactivación: MOROSA ve todo pero no ejecuta nada; SUSPENDIDA sin acceso; reactivación = reelegir plan desde cero con complementos precargados removibles (mismo flujo para SUSPENDIDA y CANCELADA).
- Cancelación voluntaria: acceso hasta fin del periodo vigente, sin reembolsos automáticos.
- UI: modal "Ver pago" con 7 variantes según estado pago/comprobante.

### Preguntas abiertas de negocio (reportadas tal cual, NO respondidas)
Del doc Soporte:
- Expiración de precios especiales: ¿vuelve a catálogo estándar o a precio contratado previo?
- Días de gratuidad adicionales: ¿se suman (extienden corte) o reemplazan duración total?
- Nombre de categoría "Eliminación" en Bitácora del Contrato (transversal, no solo pagos)
- Segmentación de roles/permisos dentro de Soporte (diferida a iteración posterior)
- Segmentación de alcance por empresa/región dentro de Soporte (diferida)
- Restricciones de transición para forzar cambio de estado (¿matriz válida o cualquier estado→cualquier estado?)

Del doc Cliente:
- ¿Lógica A/B de compra de complementos (regla 2) también aplica al caso análogo de Upgrade con orden ya generada (7.5)? Hoy no se ofrece alternativa ahí.
- Notificación in-app de comprobante rechazado por SUNAT (pendiente de que exista sistema de notificaciones in-app — hoy solo correo)
- Baja de complementos: mecánica v1/provisional, en revisión de UX (posible cambio a elegir antes en vez de auto-seleccionar)
- Manejo técnico de compensación/reconciliación cuando Culqi confirma pago pero el sistema falla al persistir — sin diseño técnico aún (riesgo técnico crítico de integridad financiera)
- Reversión/anulación de pagos confirmados — fuera de alcance, fase futura
- Penalidades de reactivación — fuera de alcance, fase futura

### Incertidumbres de arquitectura (no resueltas en esa sesión)
- No se confirmó si SmartClic/MSPagos es el mismo dominio que gp-apps-cross/Pagos o un sistema legado paralelo.
- No se determinó qué endpoints/entidades de Pagos.Domain.Core ya cubren: GCG, saldo a favor, transferencia entre empresas, precios especiales con vigencia, bitácora administrativa, forzado de estado.
- No se inspeccionó cliente-hub-front para verificar cuánto del autoservicio descrito ya está implementado en UI vs pendiente.
- portal-sr-front está prácticamente vacío — fuerte indicio de que es el proyecto a construir desde cero para el aplicativo administrativo de Soporte.
- No se buscó documentación complementaria en Bookstack por no tener acceso a esas herramientas en esa sesión.

### Riesgos identificados
- Falta de diseño para compensación cuando Culqi confirma pago pero falla la persistencia.
- "Modificar orden no pagada" y "forzar cambio de estado" son acciones muy abiertas; sin matriz de transiciones válida, riesgo de introducir estados inconsistentes.
- Riesgo de doble-mantenimiento: si MSPagos y Pagos son sistemas distintos, cualquier nueva regla de negocio podría necesitar implementarse dos veces o migrar primero.
- Riesgo de auditoría: debe verificarse si existe ya una entidad de auditoría reutilizable en Pagos.Domain.Core antes de proponer una nueva.

**Learned**: Ambos documentos son "vivos" (v1, desglosados de un v4 unificado), están explícitamente marcados con checkboxes de pendientes — cualquier propuesta debe tratar esos pendientes como preguntas para el stakeholder de negocio, no como espacio para asumir una respuesta.

---

## 2. `sdd/cobranza-soporte-cliente/proposal` (obs #2843)

# Propuesta — change "cobranza-soporte-cliente"

Fuente de verdad: `aplicativo-administrativo-soporte-v1.md` y `elicitacion-modulo-pagos-cliente-v1.md` (ambos v1, 13/08/2026, desglosados de un v4 unificado). Nada en esta propuesta amplía el alcance más allá de lo escrito en esos documentos.

> ⚠️ **Nota de vigencia**: esta propuesta asumió "construir sobre gp-apps-cross/Pagos" como backend destino. Esa premisa fue **corregida posteriormente por el usuario** (ver memoria #2827 y #2854, sección "Contradicciones") — Pagos queda solo como referencia de dominio, no como destino confirmado. El resto del contenido (scope, riesgos, preguntas abiertas) sigue vigente.

### 1. Intent

**Problema (doc Cliente, §2, cita):** "El negocio necesita cobrar de forma recurrente y automática el uso de la plataforma (plan + complementos), gestionar periodos gratuitos, cambios de plan, compra/baja de recursos adicionales, y manejar de forma controlada los casos de no-pago — todo esto sin perder trazabilidad de cada orden, cada intento de cobro y cada comprobante emitido."

**Problema (doc Soporte, §2, cita):** "El ciclo de facturación (emisión, cobro, reintentos, bloqueo, reactivación) ocurre de forma automática y autoservicio para el cliente, pero existen casos excepcionales que el sistema no puede resolver por sí solo: comprobantes rechazados por SUNAT, órdenes que requieren corrección manual, empresas con condiciones comerciales especiales (GCG, precios especiales, extensiones de gratuidad), y transferencias de saldo entre empresas. El negocio necesita una vía manual, controlada y auditable para estos casos."

**Objetivo (doc Cliente, §3, cita):** "Permitir que el ciclo completo de facturación (emisión, cobro, reintentos, bloqueo, reactivación) ocurra de forma **automática y autoservicio** para el cliente, garantizando la integridad del dinero cobrado y la trazabilidad de cada intento de pago."

**Objetivo (doc Soporte, §3, cita):** "Dar al equipo de Soporte una herramienta que permita intervenir sobre el estado financiero y comercial de cualquier empresa cuando el autoservicio no cubre el caso, garantizando que toda intervención quede **trazada y auditada**, sin comprometer la integridad de los pagos ya confirmados."

**Éxito se ve como:** las 12 acciones de Soporte del §7 del doc administrativo son ejecutables desde un aplicativo web, cada una deja registro en la bitácora administrativa (regla 5, §8), ningún pago confirmado puede alterarse (regla 3, §8), y el ciclo de autoservicio del cliente cumple las 26 reglas de negocio del §8 del doc de cliente y los escenarios Gherkin de ambos §10.

### 2. Scope

**En alcance — Aplicativo administrativo de Soporte (las 12 acciones, §7 doc Soporte)**
1. Activar régimen GCG/Gratuita (§7.1)
2. Desactivar régimen GCG/Gratuita (§7.1)
3. Reemitir comprobante rechazado por SUNAT (§7.2)
4. Modificar una orden ya generada NO pagada (§7.3)
5. Consultar histórico completo de órdenes/pagos por empresa (§7.8, solo lectura)
6. Forzar cambio de estado de cuenta/suscripción (§7.7)
7. Consultar bitácora de cambios comerciales y financieros (§7.8)
8. Ejecutar cambios de plan y periodos en nombre del cliente (§7, acción 8)
9. Transferir saldo a favor entre empresas (§7.4)
10. Consultar y gestionar saldo a favor de una empresa y su historial (§7.8)
11. Aplicar precios especiales con vigencia definida inicio/fin (§7.5)
12. Otorgar días adicionales de periodo gratuito (§7.6)

Transversal (regla 5, §8 doc Soporte): bitácora de cambios administrativos con usuario, momento, empresa/entidad afectada y valores anterior/nuevo, para TODA acción del aplicativo.

**En alcance — Autoservicio del cliente (flujos §7 doc Cliente):** happy path (7.1), pago adelantado (7.2), compra de complementos (7.3), baja de complementos (7.4, v1/provisional), cambio de plan (7.5), cambio de ciclo (7.6), pago automático (7.7), no pago/deuda/bloqueo (7.8), cancelación voluntaria (7.9), vista GCG del cliente (7.10), modal "Ver pago" 7 variantes (7.11).

**Fuera de alcance (explícito en los propios documentos):**
- Reversión/anulación de pagos confirmados (doc Cliente §12: "fase futura")
- Penalidades de reactivación (doc Cliente §12: "fase futura")
- Segmentación de roles y permisos dentro de Soporte (doc Soporte §5 y §12: "iteración posterior")
- Segmentación de alcance por empresa/región dentro de Soporte (doc Soporte §8 regla 2 y §12)
- Modificación de cualquier pago ya confirmado, incluso por Soporte
- Nota de crédito como documento formal
- Eliminación de recursos (solo deshabilitar)

### 3. Enfoque técnico de alto nivel (ver nota de vigencia arriba — parcialmente superado)
1. Backend — (propuesto entonces) construir sobre `gp-apps-cross/Pagos` — **corregido después, destino real sin confirmar**.
2. Frontend Soporte — construir `gp-apps-cross/portal-sr-front` prácticamente desde cero (confirmado vacío, sigue vigente).
3. Frontend Cliente — (propuesto entonces) ajustar `SmartClic/cliente-hub-front` — **contradicho por evidencia posterior** (obs #2854): cliente-hub-front es un portal de referidos/revendedores, no de pagos.

### 4. Riesgos y preguntas abiertas (reportadas, NO resueltas)

**Riesgo técnico crítico:** reconciliación/compensación cuando Culqi confirma el pago pero el sistema falla al persistir la confirmación (doc Cliente §12).

**Preguntas de negocio (doc Soporte §12):** expiración de precios especiales; si los días de gratuidad se suman o reemplazan; nombre de categoría "Eliminación" en bitácora; segmentación de roles (diferida); segmentación por región (diferida); matriz de transiciones válidas para forzar estado.

**Preguntas de negocio (doc Cliente §12):** ¿lógica A/B también aplica a upgrade con orden generada?; notificación in-app de comprobante rechazado (bloqueada por sistema en construcción); baja de complementos en revisión de UX.

**Riesgos de ejecución:** alcance abierto en "modificar orden" y "forzar estado" sin matriz de transiciones; bitácora administrativa sin confirmar si ya existe entidad reutilizable; gap real del frente de cliente sin dimensionar; metadatos de los docs incompletos (Responsable, Stakeholder, Figma, mapas de flujo, manual de componentes todos "(pendiente)").

### 5. Dependencias
**Funcionales:** SUNAT, sistema de notificaciones in-app (aún en construcción), Módulo de Planes y Complementos, Módulo de Recursos, interdependencia mutua entre los dos aplicativos.
**Técnicas:** Culqi (ya integrada en Pagos.Infra.Culqi), misma API/BD compartida, servicio de correo transaccional.

### 6. Siguiente paso (según esta propuesta original)
`sdd-spec` y `sdd-design` en paralelo, abordando explícitamente reconciliación Culqi↔persistencia y modelo de bitácora administrativa. **Nota: este siguiente paso fue reemplazado por el flujo real seguido**, que en vez de eso corrigió el repo backend asumido y luego produjo el SDD Engineering Assessment completo (ver artefacto #4 abajo).

---

## 3. `architecture/cobranza-pagos-repos` (obs #2827 — versión vigente, 3 revisiones)

**Qué**: El usuario corrigió explícitamente que gp-apps-cross/Pagos NO es la solución ni el repositorio donde se va a trabajar para el change "cobranza-soporte-cliente", contradiciendo la conclusión previa guardada en esta misma topic_key (que decía que Pagos era la reescritura moderna de MSPagos y por tanto el destino correcto).

**Por qué**: La exploración anterior (basada en comparar árboles de archivos de Pagos vs MSPagos) asumió que Pagos era el backend activo correcto, pero el usuario indicó que es un error — había otra tarea/repo real pendiente de especificar.

**Dónde**: N/A en el momento de esta corrección — el repo/tarea correcto quedó por confirmar.

**Aprendido**: NO asumir que Pagos (gp-apps-cross) es el target solo por similitud de dominio/DTOs con MSPagos. Esperar la indicación explícita del usuario sobre cuál es el repo/tarea correcta antes de retomar sdd-propose/spec/design. Las conclusiones anteriores de este topic_key sobre "dónde construir" quedaron INVALIDADAS hasta nueva confirmación.

> Esta misma topic_key tiene 3 revisiones en total: (1) mapeo inicial de repos con Pagos como conclusión, (2) esta corrección, (3) el hallazgo posterior de que Pagos y MSPagos son el mismo dominio (Pagos = reescritura moderna) — dato que sigue siendo válido como *contexto de dominio*, aunque Pagos ya no se asuma como destino de la implementación.

---

## 4. `sdd/cobranza-soporte-cliente/engineering-assessment` (obs #2854) — resumen del assessment final

**What**: SDD Engineering Assessment completo (24 secciones) para el change "cobranza-soporte-cliente", en modo estrictamente análisis/planificación. Archivo: `E:\2026-II\gentle-ai\.claude\sdd-assessments\cobranza-soporte-cliente-assessment.md` (2166 líneas). NADA fue ejecutado en GitLab/BD/infra: solo lectura.

**Why**: El usuario pidió un assessment de ingeniería que clasificara el requerimiento, mapeara los repos reales, y expusiera decisiones humanas pendientes sin inventar arquitectura ni resolver silenciosamente contradicciones.

**Where**:
- Fuente de verdad: aplicativo-administrativo-soporte-v1.md (295 líneas) y elicitacion-modulo-pagos-cliente-v1.md (514 líneas), ambos leídos completos.
- Repos inspeccionados READ-ONLY: gp-apps-cross/Pagos, SmartClic/MSPagos, SmartClic/cliente-hub-front, gp-apps-cross/portal-sr-front, SmartClic/erp-mf-root-config, SmartClic/erp-mf-comun, SmartClic/erp-mf-configuraciones.

### Clasificación
Categoría C (Mixto). Greenfield: portal-sr-front. Extensión: backend de Cobranza. Frontend cliente: destino **DESCONOCIDO**.

### Hallazgos duros de la inspección
- **gp-apps-cross/Pagos**: .NET Clean Architecture real, 7 proyectos, CQRS por carpeta Features/, EF Core (SRPagosContext + ~50 POCOs + repos + UnitOfWork), Quartz, Kafka, SignalR, Redis. Culqi YA integrado (Pagos.Infra.Culqi/Services/CulqiService.cs: Card/Charge/Customer/Token/YapeToken/AntiFraud). 9 controladores. **SIN carpeta Migrations.**
- **SmartClic/MSPagos**: legado, DTOs ~1:1 con Pagos.Domain.Core, SIN Culqi (solo Niubiz + MercadoPago). Es el **ÚNICO repo con migración EF real**: SRPagosRepositorio/Migraciones/20220705211757_First.cs + SRPagosContextModelSnapshot.cs.
- **gp-apps-cross/portal-sr-front**: VACÍO. El árbol completo = 1 archivo README.md (template GitLab). Greenfield puro.
- **SmartClic/erp-mf-root-config**: single-spa REAL y productivo. Webpack crudo + TS + SystemJS + importmaps por entorno (local/dev/crt+4 variantes/prd+4). registerApplication para: erp-mf-security (/auth,/404,/error-permiso,/error-plan), erp-mf-header, erp-mf-home, erp-mf-tiendalink, erp-mf-punto-venta, erp-mf-logistica, erp-mf-configuracion, erp-mf-planillas. Carga global: erp-mf-styles, erp-mf-common. Deploy Jenkins a https://app.smartclic.pe/erp-mf-*/app.js.
- 14 repos erp-mf-* existen. VERIFICADO: erp-mf-configuracion (singular) Y erp-mf-configuraciones (plural) coexisten como proyectos distintos; el root-config registra el SINGULAR.
- El ecosistema tiene DOS variantes de build de microfrontend: webpack crudo + webpack-config-single-spa (root-config, comun) vs Vue CLI + vue-cli-plugin-single-spa (erp-mf-configuraciones).
- NINGÚN repo inspeccionado tiene AGENTS.md, CONTRIBUTING.md ni repo-profile. NINGUNO usa .gitlab-ci.yml — todos Jenkins Groovy en devops/.
- NINGÚN repo muestra infraestructura de testing.

### Contradicciones documentadas (no resueltas silenciosamente)
- **C1 (CRÍTICA)**: el usuario describió `SmartClic/cliente-hub-front` como "frontend cliente YA desarrollado, corresponde a elicitacion-modulo-pagos-cliente-v1.md". **LA EVIDENCIA LO CONTRADICE.** `src/views` completo = clientes-recomendados, contadores, contratos, recomendados, revendedores, simple-views, usuarios + Bienvenida/Registro/Resumen/Home/Login/DataTables/NotFound. CERO pantallas de planes, complementos, suscripción, órdenes, pagos, tarjetas, upgrade, ciclo, saldo o modal "Ver pago". API base = `https://erpperuapi-dev.smartclic.pe/ClientHub`, no Cobranza. Es un portal de referidos/revendedores. → **NEEDS_DECISION**.
- **C2**: Engram #2843 (proposal) decía "construir sobre gp-apps-cross/Pagos"; Engram #2827 (corrección posterior del usuario) dice que Pagos NO es el repo de trabajo. Resuelto a favor de #2827: Pagos = referencia de dominio, NO destino. Sustituto nunca nombrado → **NEEDS_DECISION**.
- **C4**: erp-mf-root-config package.json pinea single-spa ^5.9.3 pero importmap-prd.json carga single-spa@6.0.0 desde CDN.
- **C5 (relevante)**: erp-mf-header declara `activeWhen` con prefijo `/planes` pero NINGÚN registerApplication monta app en `/planes`. El shell reserva una ruta de planes sin microfrontend detrás — posible espacio previsto para facturación.
- **C6**: importmap-prd.json (PRODUCCIÓN) mapea erp-mf-planillas a `//localhost:8085/app.js`.

### Cobertura backend vs requerimiento
**EXISTE**: forzar cambio de estado (CambiarEstadoContrato), upgrade de plan, intentos de cobro recurrente (jobs Quartz), emisión de comprobantes, ObtenerContratosPaginadoSoporte (ya orientado a Soporte).
**NO EXISTE**: GCG, saldo a favor, transferencia de saldo, precios especiales con vigencia, bitácora administrativa de negocio (solo Logdemonios/Logrequests operativos), downgrade, prorrateo, cambio de ciclo, reconciliación Culqi↔persistencia, y NO se observó persistencia de tarjetas Culqi (solo Tarjetaniubiz) pese a que Culqi es la pasarela vigente.

### DATABASE SPECIALIST REVIEW: REQUIRED
Cinco motivos: (1) esquema compartido con ownership de migraciones ambiguo — Pagos sin Migrations, única migración EF en el legado MSPagos; (2) acceso mixto EF + stored procedures (StoreProcedures.cs); (3) transferencia de saldo = débito/crédito atómico entre agregados (dinero); (4) bitácora con valor anterior/nuevo (volumen, retención, append-only); (5) el gap de reconciliación SPEC-023 exige estructuras nuevas (outbox/idempotencia por charge_id).

### Insumo faltante BLOQUEANTE
Las 2 páginas de Notion ("Arquitectura Multiagente para Desarrollo SDD" y "Arquitectura multiagente — Proyectos existentes y proyectos nuevos") devolvieron `object_not_found` (404). La integración de Notion no tiene acceso. Bloquea el cierre con confianza plena de Gate 1 y Gate 2 — especialmente OD-04 (single-spa vs standalone). **El usuario debe compartir ambas páginas con la integración.**

### Decisiones humanas pendientes (top 6)
- OD-01: acceso a Notion
- OD-02: cuál es el repo backend (U1)
- OD-03: dónde vive el frontend de autoservicio del cliente (U2/C1)
- OD-04: portal-sr-front dentro del single-spa erp-mf-* o standalone (U3)
- OD-08: ownership de migraciones del esquema compartido
- OD-09: estrategia de reconciliación Culqi↔persistencia

Más 12 preguntas abiertas de negocio heredadas de los .md (OD-13..OD-24), 4 de las cuales bloquean specs completas (SPEC-006 matriz de transiciones, SPEC-007 expiración de precios especiales, SPEC-008 días de gratuidad suman vs reemplazan, SPEC-014 baja de complementos en revisión de UX).

### Contenido producido en el assessment
23 specs Gherkin (SPEC-001..023), 4 diagramas Mermaid (arquitectura actual, arquitectura propuesta, dependencias entre repos, ER de datos), Impact Map / Project Blueprint / database_impact / git_plan / Cross-Repo Manifest / Task Plan en YAML, 21 tareas T001-T021, matriz de verificación de 23 filas TODAS en NOT EXECUTED, 3 Human Gates todos NOT PASSED, 17 riesgos.

### Learned
- La premisa del usuario sobre cliente-hub-front NO se sostiene contra la evidencia del repo — invalida el dimensionamiento del frente de cliente que asumía "ya desarrollado, solo ajustar".
- El historial de migraciones EF del esquema compartido vive en el sistema que todos consideran obsoleto (MSPagos), mientras el sistema moderno (Pagos) escribe sin migraciones — riesgo de gobierno de datos no mencionado en ninguna fuente documental.
- La ruta /planes reservada en el shell single-spa sin microfrontend detrás es la pista más fuerte de que ya existe una intención arquitectónica para el área de facturación — merece confirmación humana antes de decidir dónde va portal-sr-front.
- Ausencia total de AGENTS.md/CONTRIBUTING.md/tests en los 7 repos: correr skill-registry + sdd-init sobre los repos objetivo es precondición, no opcional.

---

*Compilado el 14/08/2026 a partir de las memorias Engram #2838, #2843, #2827 y #2854 del proyecto gentle-ai.*
