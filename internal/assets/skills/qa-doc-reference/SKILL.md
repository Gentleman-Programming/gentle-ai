---
name: qa-doc-reference
description: "Trigger: citar documentación PRD de BookStack en exploración o búsqueda. Emite la ficha documental: 13 campos + Qué contiene, URL exacta, STOP si diverge."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "1.0"
---

## Activation Contract

Carga esta skill cuando traigas una página de BookStack como referencia de
documentación PRD: en la fase de exploración (sdd-explore / qa-explore) o en
búsqueda de documentación bajo demanda. Su objetivo es presentar cada página
consultada como una **ficha de documentación PRD**: tabla de metadatos
(13 campos) arriba + resumen "Qué contiene" debajo.

## Output Contract — La Ficha

Cada página de BookStack citada se renderiza SIEMPRE con esta estructura.

### 0. Resolución de Libro/Capítulo (obligatorio antes de armar la ficha)

`bookstack_search`/`bookstack_get_page` devuelven únicamente `book_id`/
`chapter_id` numéricos — nunca el nombre del libro o capítulo. Para llenar
"Libro"/"Capítulo" de la tabla, resuelve el nombre real con una llamada
adicional por cita: `bookstack_get_book(book_id)` y, si `chapter_id` no es 0,
`bookstack_get_chapter(chapter_id)`. No inventes ni abrevies el nombre a
partir del slug de la página.

### 1. Tabla de metadatos (13 campos, arriba)

| Campo | Valor |
|---|---|
| Página PRD | {título de la página} |
| Nombre | {nombre corto del documento} |
| BookStack ID | {id numérico} |
| Libro | {libro al que pertenece} |
| Capítulo | {capítulo / sección} |
| Slug / URL | {slug o ruta corta} |
| Título PRD | {título del PRD} |
| Versión del documento | {versión} |
| Creada | {fecha de creación} |
| Actualizada | {fecha de última actualización} |
| Responsables | {equipo / personas responsables} |
| Ticket Redmine | {ticket asociado, si existe} |
| URL directa de BookStack | {URL exacta de la página} |

### 2. "Qué contiene" (texto plano, debajo de la tabla)

Resume en 3–5 líneas de texto plano qué define la página: alcance del módulo,
reglas de negocio, criterios de aceptación, datos o flujos. Sin tablas ni
listas anidadas: un párrafo legible para el humano y el orquestador.

### 3. Ejemplo trabajado — PV_PRD_13349

Valores ilustrativos de un PRD de Punto de Venta (NO es una cita real):

| Campo | Valor |
|---|---|
| Página PRD | 01. PRD Punto de Venta — Descripción General |
| Nombre | PRD Punto de Venta |
| BookStack ID | 13349 |
| Libro | ERP_PV_Punto Venta |
| Capítulo | Descripción General |
| Slug / URL | 01-prd-punto-de-venta-descripcion-general |
| Título PRD | PRD Punto de Venta (PV) |
| Versión del documento | 1.3 |
| Creada | 2024-03-12 |
| Actualizada | 2025-11-20 |
| Responsables | QA Punto de Venta |
| Ticket Redmine | #4821 |
| URL directa de BookStack | https://bookstack.smartclic.pe/books/erp-pv-punto-venta/page/01-prd-punto-de-venta-descripcion-general |

## Hard Rules

- **URL exacta siempre**: "URL directa de BookStack" es la URL de la página
  citada, nunca una URL de búsqueda.
- **G1 — STOP en divergencia**: si la documentación difiere del código actual,
  DETENTE y presenta la contradicción al humano; nunca decidas por tu cuenta.
- **Fallback honesto**: si un campo no está disponible en la página consultada,
  emite los campos disponibles y marca los faltantes como "no disponible".
  PROHIBIDO inventar valores. La API real de BookStack no expone
  "Responsables"/"Ticket Redmine"/"Versión del documento" como campos
  estructurados (solo `tags` libres y `revision_count`) — repórtalos como
  "no disponible" salvo que la página misma los declare explícitamente en su
  texto (p. ej. una línea "Responsable: ..."); nunca los infieras de otro
  lugar.

## Comandos de referencia

- Reglas G1-G6: lee `skills/_shared/qa-gate-policy.md` (fuente única in-repo).
- Contrato de herramienta standalone (uso sin `{change}` activo, nunca avanza
  el ledger): sección "Contrato de herramientas QA standalone" del mismo
  archivo.