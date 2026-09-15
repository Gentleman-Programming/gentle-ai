---
name: qa-locator-hunting
description: "Caza locators de UI en microfronts erp-mf-*: POM, DOM en vivo (Playwright MCP), GitLab, nunca inventa. Trigger: necesitas un locator/selector."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "2.0"
---

## Activation Contract

Carga esta skill cuando necesites un locator/selector para un test E2E de Playwright
sobre un microfront SmartClic/`erp-mf-*` (Punto de Venta, Facturación, Logística,
Común/Shared) y el selector no esté disponible de forma inmediata en el proyecto de
automatización. Su objetivo es **reutilizar** locators existentes y, solo si faltan,
**cazarlos** en el entorno real o el código del microfront — nunca inventarlos.

`qa-explore` invoca esta skill automáticamente en cuanto detecta, durante el análisis
previo (G2), un Target que el spec va a necesitar y que no existe todavía en el POM.
Cazar el locator en ese momento — antes de `qa-spec` — es lo que permite que el spec y
el diseño Screenplay+POM salgan exactos desde la primera pasada, sin locators
pendientes de resolver durante la implementación.

## Rol

Eres el cazador de locators del ecosistema QA. Trabajas en 3 niveles, siempre en
este orden — nunca saltes a NIVEL 2 sin agotar NIVEL 1, ni a NIVEL 1 sin agotar
NIVEL 0:

- **NIVEL 0 (siempre primero)**: reutilizar los locators que ya existen en el proyecto
  de automatización (POM `src/pages/**`, tareas/questions del patrón Screenplay).
  No reinventes selectores que ya están resueltos y verdes.
- **NIVEL 1 (si falta en el POM)**: inspeccionar el DOM en vivo del entorno real
  (dev/staging) vía **MCP Playwright**, con credenciales/sesión del proyecto, navegando
  la URL que el propio proyecto resuelve por entorno. Es la fuente más confiable: el
  locator que devuelve es el que realmente renderiza la pantalla, incluidos elementos
  generados en runtime (`*ngFor`/`.map()`, componentes de librería UI, contenido cargado
  por API) que no siempre aparecen tal cual en el template fuente.
- **NIVEL 2 (si Playwright no alcanza)**: cazar el locator/id en el código fuente del
  microfront vía GitLab MCP, para el componente/comprobante específico que se necesita —
  útil cuando no hay acceso al entorno en vivo o el elemento no es reproducible ahí
  (feature flag apagado, dato de prueba no disponible, etc.).

## NIVEL 0 — Reutilizar el POM (obligatorio primero)

1. Busca en `src/pages/**` del proyecto de automatización si el elemento ya tiene un
   locator definido (mismo flujo/módulo: emisión, pedido, cotización, guía, etc.).
2. Busca en las tareas (`tasks/`), preguntas (`questions/`) e interacciones del patrón
   Screenplay que ya orquestan ese elemento.
3. Si existe y funciona: **reutilízalo**. No lo reescribas.
4. Si existe pero está roto (flaky o desactualizado): corrígelo SOLO con evidencia del
   microfront (ver NIVEL 1) y documenta el cambio.
5. Si no existe: pasa al NIVEL 1.

## NIVEL 1 — Inspeccionar el DOM en vivo vía Playwright MCP (primera opción de caza)

1. Confirma con el humano el entorno (dev/staging — **nunca asumas producción** sin
   confirmación explícita) y que hay credenciales/sesión válidas para llegar a la
   pantalla del elemento. Si el proyecto ya resuelve la URL por entorno (config/env del
   propio repo de automatización), úsala como está — no inventes ni adivines una URL.
2. Con el MCP Playwright, navega hasta la pantalla real del flujo (mismo camino que
   seguiría el test) usando esas credenciales.
3. Localiza el elemento en el DOM renderizado — por texto visible (botón, label,
   placeholder, título de columna) o por su posición en el flujo — y extrae su atributo
   real. **Prioridad estricta**:

   ```
   data-testid  >  id  >  name  >  formControlName  >  aria-label  >  clases CSS
   ```

4. Prefiere atributos estables (testing hooks, atributos de formulario Angular,
   `aria-label`) antes que clases CSS de estilos, que cambian con el diseño.
5. Devuelve el selector con la estrategia de Playwright correspondiente
   (`getByTestId`, `getByRole`, `getByLabel`, `getByText`, `locator(...)`).
6. **No navegues ni extraigas más DOM del que hace falta para identificar ese elemento
   puntual** — no es una skill de scraping general, es puntual para cazar un selector.
7. Si no hay acceso al entorno (sin credenciales, sin MCP Playwright disponible, o el
   elemento no es reproducible ahí): pasa al NIVEL 2.

## NIVEL 2 — Cazar en el código fuente vía GitLab MCP (solo si Playwright no alcanza)

### 1. Resolver el proyecto `erp-mf-*` (catálogo primero)

1. **Leé el catálogo primero**: `references/erp-mf-catalog.md`, junto a esta skill.
   Buscá el vocabulario de la consulta en **Términos de dominio** y **Flujo de negocio**.
   Es un atajo de direcciones, **no** una fuente de verdad.
2. **Si hay una fila clara**: usá su `slug` como candidato y confirmalo en vivo con el MCP
   de GitLab (`search_projects`) antes de leer archivos — el nombre real puede llevar
   sufijos (`-web`, `-app`, `-frontend`).
3. **Si no hay fila, hay dos o más filas plausibles, o `search_projects` no encuentra ese
   slug (renombrado/404)**: resolvé desde cero con `search_projects` usando el término de
   negocio. El catálogo nunca bloquea la caza.
4. **Si no hay MCP de GitLab disponible**: el catálogo queda como pista de lectura; seguí
   al fallback honesto. Nunca inventes el proyecto ni el selector.

**Reglas vinculantes del catálogo**

- **D1 — GitLab en vivo siempre gana.** Ante cualquier conflicto entre el catálogo y
  `search_projects`, el resultado en vivo es el autoritativo.
- **D2 — El catálogo es pista, nunca compuerta.** Fila faltante, ambigua o slug 404 ⇒
  fallback obligatorio a `search_projects`; nunca abortes la caza por el catálogo.
- **D3 — El drift se reporta, nunca se absorbe en silencio.** Cuando GitLab contradiga una
  fila, hacé **las dos cosas**:
  1. **Nota en la respuesta** (obligatoria, aunque Engram falle), con este formato:
     `Drift de catálogo: la fila `{slug}` dice `{valor_catalogo}`, GitLab en vivo dice
     `{valor_vivo}`. Usé el valor en vivo (D1). Corregir la fila en el repo gentle-ai.`
  2. **Registro durable en Engram** con `mem_save`, `topic_key`
     `qa/erp-mf-catalog/drift/{slug}`, `type: "discovery"`, `scope: "personal"`,
     `capture_prompt: false`, y el contenido **What/Why/Where/Learned** descrito en el
     encabezado del catálogo.
  No edites el catálogo vos mismo: la copia instalada vive fuera del repo y hay dos copias
  que deben cambiar juntas. Reportá y registrá; la corrección la hace un mantenedor.
- Las filas marcadas `Verificado: unverified` nunca fueron confirmadas en vivo: tratá su
  slug como hipótesis y confirmalo siempre con `search_projects` cuando el MCP esté.

### 2. Localizar el componente

5. Busca en el microfront por **texto visible** del elemento (botón, label, placeholder,
   título de columna) o por fragmentos del flujo (componente, ruta, feature flag).
6. Navega al template real del componente:
   - Angular → archivo `.html` del componente (busca el `.ts` que lo referencia).
   - React → archivo `.tsx` donde se renderiza el elemento.

### 3. Extraer el selector auténtico

7. Extrae el atributo del elemento en el template, con la misma prioridad estricta del
   NIVEL 1 (`data-testid > id > name > formControlName > aria-label > clases CSS`).
8. Devuelve el selector con la estrategia de Playwright correspondiente
   (`getByTestId`, `getByRole`, `getByLabel`, `getByText`, `locator(...)`).

## Fallback honesto — NUNCA inventar

Si tras agotar los 3 niveles el locator sigue sin aparecer, **detente y hacé estas 3
preguntas al humano** — no inventes, no adivines, no marques el Target como "pendiente"
en el spec:

1. **¿En qué microfront (`erp-mf-*`) y pantalla/ruta exacta vive el elemento?** — para
   ubicar el proyecto si el catálogo no tiene la fila o GitLab no lo encuentra.
2. **¿Cuál es el texto visible exacto del elemento (label del botón, columna,
   placeholder) o podés mandar un screenshot?** — para cazar por texto visible cuando
   no hay `data-testid` documentado.
3. **¿Tenés acceso a un entorno donde el ERP2 corra en vivo (dev/staging) y
   credenciales para llegar a esa pantalla?** — si el NIVEL 1 falló por falta de acceso,
   esto lo desbloquea; si ya se agotó también, confirma que no hay otro entorno posible.

- Sin acceso a Playwright/entorno en vivo NI a GitLab: reporta `"locator no
  encontrado — sin acceso al entorno ni a GitLab"`.
- Locator no encontrado tras agotar los 3 niveles y las 3 preguntas: reporta
  `"locator no encontrado"` explícitamente en el reporte de `qa-explore`, como
  información pendiente — nunca como un Target inventado.
- **PROHIBIDO** inventar selectores, `data-testid` que no existen o atributos
  adivinados: un selector inventado produce tests flaky o falsos positivos.

## Guardrails

- Orden estricto: NIVEL 0 → NIVEL 1 → NIVEL 2, nunca salteado.
- Nunca inventes un locator ni un `data-testid`.
- Nunca modifiques el microfront para "facilitar" el test (no es tu repo).
- NIVEL 1 nunca navega a producción sin confirmación explícita del humano.
- Si el elemento no se puede cazar con certeza tras los 3 niveles, detente y hacé las
  3 preguntas de fallback — no sigas a `qa-spec` con Targets sin resolver.
