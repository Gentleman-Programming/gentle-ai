---
name: qa-locator-hunting
description: "Caza locators de UI en microfronts erp-mf-*: POM, DOM en vivo (Playwright MCP), GitLab, nunca inventa. Trigger: necesitas un locator/selector."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "2.1"
---

## Activation Contract

Carga esta skill cuando necesites un locator/selector para un test E2E sobre un
microfront SmartClic/`erp-mf-*` y no exista ya en el POM. `qa-explore` la invoca
automáticamente al detectar (G2) un Target sin resolver, antes de `qa-spec`.

## Hard Rules

- Orden estricto **NIVEL 0 → NIVEL 1 → NIVEL 2**: nunca saltees un nivel sin
  agotar el anterior.
- Prioridad de atributos siempre igual: `data-testid > id > name >
  formControlName > aria-label > clases CSS`.
- **PROHIBIDO** inventar selectores, `data-testid` inexistentes o atributos
  adivinados — produce tests flaky o falsos positivos.
- Nunca modifiques el microfront; NIVEL 1 nunca navega a producción sin
  confirmación explícita del humano.
- Si el locator no aparece tras los 3 niveles: seguí
  `references/locator-fallback.md` — nunca marques el Target como "pendiente".

## Decision Gates

| Nivel | Cuándo | Fuente | Salida |
|---|---|---|---|
| 0 Reutilizar | Siempre primero | POM `src/pages/**`, Screenplay | Locator existente; si está roto, corregilo con evidencia de NIVEL 1 |
| 1 DOM en vivo | Falta en el POM | Playwright MCP en dev/staging | Selector real del DOM renderizado |
| 2 Código fuente | Playwright no alcanza | GitLab MCP (ver `references/erp-mf-catalog.md`) | Selector del template (`.html`/`.tsx`) |

## Execution Steps

1. **NIVEL 0**: buscá en el POM/Screenplay del mismo flujo; si funciona,
   reutilizalo sin reescribirlo.
2. **NIVEL 1**: confirmá entorno/credenciales con el humano, navegá con
   Playwright MCP, localizá el elemento por texto visible y aplicá la
   prioridad de atributos. No extraigas más DOM del necesario.
3. **NIVEL 2**: resolvé el proyecto `erp-mf-*` (catálogo primero, GitLab en
   vivo gana — D1/D2/D3 en `references/erp-mf-catalog.md`), confirmá con
   `search_projects`, localizá el componente y extraé el atributo con la
   misma prioridad.
4. Devolvé el selector con su estrategia Playwright (`getByTestId`,
   `getByRole`, `getByLabel`, `getByText`, `locator(...)`).
5. Si los 3 niveles se agotan: seguí `references/locator-fallback.md` antes
   de continuar hacia `qa-spec`.

## Output Contract

Devolvé el selector, su estrategia Playwright y el nivel que lo produjo. Si no
se pudo cazar con certeza, devolvé el reporte exacto de
`references/locator-fallback.md` — nunca un Target inventado.

## References

- `references/erp-mf-catalog.md` — catálogo `erp-mf-*` y reglas D1-D3.
- `references/locator-fallback.md` — preguntas y reportes del fallback honesto.
- Reglas G1-G6: lee `skills/_shared/qa-gate-policy.md` (fuente única in-repo).
- Contrato de herramienta standalone (uso sin `{change}` activo, nunca avanza
  el ledger): sección "Contrato de herramientas QA standalone" del mismo
  archivo.
