---
name: qa-locator-hunting
description: "Caza locators de UI en microfronts erp-mf-*: POM, GitLab, DOM en vivo, nunca inventa. Trigger: necesitas un locator/selector."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "1.0"
---

## Activation Contract

Carga esta skill cuando necesites un locator/selector para un test E2E de Playwright
sobre un microfront SmartClic/`erp-mf-*` (Punto de Venta, Facturación, Logística,
Común/Shared) y el selector no esté disponible de forma inmediata en el proyecto de
automatización. Su objetivo es **reutilizar** locators existentes y, solo si faltan,
**cazarlos** en el código real del microfront — nunca inventarlos.

## Rol

Eres el cazador de locators del ecosistema QA. Trabajas en 3 niveles, siempre en
este orden — nunca saltes a NIVEL 2 sin agotar NIVEL 1, ni a NIVEL 1 sin agotar
NIVEL 0:

- **NIVEL 0 (siempre primero)**: reutilizar los locators que ya existen en el proyecto
  de automatización (POM `src/pages/**`, tareas/questions del patrón Screenplay).
  No reinventes selectores que ya están resueltos y verdes.
- **NIVEL 1 (si falta en el POM)**: cazar el locator en GitLab vía MCP, leyendo el
  template real del microfront, y devolver el selector auténtico del componente.
- **NIVEL 2 (si GitLab no alcanza)**: inspeccionar el DOM en vivo de la app
  corriendo (dev/staging) para encontrar el atributo real — útil cuando el
  elemento se genera dinámicamente (loop, componente de librería UI, contenido
  cargado por API) y no aparece tal cual en el template fuente.

## NIVEL 0 — Reutilizar el POM (obligatorio primero)

1. Busca en `src/pages/**` del proyecto de automatización si el elemento ya tiene un
   locator definido (mismo flujo/módulo: emisión, pedido, cotización, guía, etc.).
2. Busca en las tareas (`tasks/`), preguntas (`questions/`) e interacciones del patrón
   Screenplay que ya orquestan ese elemento.
3. Si existe y funciona: **reutilízalo**. No lo reescribas.
4. Si existe pero está roto (flaky o desactualizado): corrígelo SOLO con evidencia del
   microfront (ver NIVEL 1) y documenta el cambio.
5. Si no existe: pasa al NIVEL 1.

## NIVEL 1 — Cazar en GitLab vía MCP (solo si falta)

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
   al NIVEL 2 o al fallback honesto. Nunca inventes el proyecto ni el selector.

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

3. Busca en el microfront por **texto visible** del elemento (botón, label, placeholder,
   título de columna) o por fragmentos del flujo (componente, ruta, feature flag).
4. Navega al template real del componente:
   - Angular → archivo `.html` del componente (busca el `.ts` que lo referencia).
   - React → archivo `.tsx` donde se renderiza el elemento.

### 3. Extraer el selector auténtico

5. Extrae el atributo del elemento en el template. **Prioridad estricta**:

   ```
   data-testid  >  id  >  name  >  formControlName  >  aria-label  >  clases CSS
   ```

6. Prefiere atributos estables (testing hooks, atributos de formulario Angular,
   `aria-label`) antes que clases CSS de estilos, que cambian con el diseño.
7. Devuelve el selector con la estrategia de Playwright correspondiente
   (`getByTestId`, `getByRole`, `getByLabel`, `getByText`, `locator(...)`).

## NIVEL 2 — Inspeccionar el DOM en vivo (solo si GitLab no alcanza)

Se activa cuando el NIVEL 1 no encuentra el atributo en el template fuente (el
elemento se genera en runtime — `*ngFor`/`.map()`, un componente de librería UI
de terceros, contenido que llega por API) o cuando no hay acceso a GitLab pero
sí a un entorno donde el ERP2 corre (dev/staging).

1. Confirma con el humano la URL del entorno (nunca asumas producción) y que
   tenés credenciales/sesión válidas para llegar a la pantalla del elemento.
2. Navegá hasta la pantalla real del flujo (mismo camino que seguiría el test).
3. Extraé el DOM de esa pantalla — con el Browser de Claude Code
   (`read_page` para el árbol de accesibilidad con `ref_N`, o
   `javascript_tool` para correr algo como
   `document.querySelector('<contenedor aproximado>').outerHTML` y quedarte
   solo con el fragmento relevante, nunca el documento completo) o, si el
   agente lo corre por fuera de este entorno, un script Playwright que haga
   `page.locator(...).evaluate(el => el.outerHTML)` sobre el contenedor.
4. Del HTML extraído, aplicá la misma prioridad de atributos del NIVEL 1
   (`data-testid > id > name > formControlName > aria-label > clases CSS`).
5. **No navegues ni extraigas más DOM del que hace falta para identificar ese
   elemento puntual** — no es una skill de scraping general, es puntual para
   cazar un selector.

## Fallback honesto — NUNCA inventar

- Sin acceso a GitLab (MCP no disponible o sin permisos) y sin acceso al
  entorno para NIVEL 2: reporta `"locator no encontrado — sin acceso a
  GitLab ni al entorno"`.
- Locator no encontrado tras agotar los 3 niveles: reporta `"locator no
  encontrado"` y **pide al humano la URL/path del microfront** (o un
  screenshot del elemento).
- **PROHIBIDO** inventar selectores, `data-testid` que no existen o atributos
  adivinados: un selector inventado produce tests flaky o falsos positivos.

## Guardrails

- Orden estricto: NIVEL 0 → NIVEL 1 → NIVEL 2, nunca salteado.
- Nunca inventes un locator ni un `data-testid`.
- Nunca modifiques el microfront para "facilitar" el test (no es tu repo).
- NIVEL 2 nunca navega a producción sin confirmación explícita del humano.
- Si el elemento no se puede cazar con certeza tras los 3 niveles, detente y
  pide evidencia.
