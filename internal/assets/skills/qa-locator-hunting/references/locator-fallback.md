# Fallback honesto de `qa-locator-hunting` — NUNCA inventar

Si tras agotar los 3 niveles (NIVEL 0 → NIVEL 1 → NIVEL 2) el locator sigue sin
aparecer, **detente y hacé estas 3 preguntas al humano** — no inventes, no
adivines, no marques el Target como "pendiente" en el spec:

1. **¿En qué microfront (`erp-mf-*`) y pantalla/ruta exacta vive el elemento?**
   — para ubicar el proyecto si el catálogo no tiene la fila o GitLab no lo
   encuentra.
2. **¿Cuál es el texto visible exacto del elemento (label del botón, columna,
   placeholder) o podés mandar un screenshot?** — para cazar por texto visible
   cuando no hay `data-testid` documentado.
3. **¿Tenés acceso a un entorno donde el ERP2 corra en vivo (dev/staging) y
   credenciales para llegar a esa pantalla?** — si el NIVEL 1 falló por falta
   de acceso, esto lo desbloquea; si ya se agotó también, confirma que no hay
   otro entorno posible.

## Reportes exactos

- Sin acceso a Playwright/entorno en vivo NI a GitLab: reporta `"locator no
  encontrado — sin acceso al entorno ni a GitLab"`.
- Locator no encontrado tras agotar los 3 niveles y las 3 preguntas: reporta
  `"locator no encontrado"` explícitamente en el reporte de `qa-explore`, como
  información pendiente — nunca como un Target inventado.
- **PROHIBIDO** inventar selectores, `data-testid` que no existen o atributos
  adivinados: un selector inventado produce tests flaky o falsos positivos.
