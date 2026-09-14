# Catálogo de proyectos GitLab `erp-mf-*`

Caché de direcciones para el NIVEL 1 de `qa-locator-hunting`. **No es fuente de verdad**:
GitLab en vivo (`search_projects`) siempre gana (D1); una fila faltante, ambigua o
incorrecta nunca bloquea la caza (D2); todo conflicto se reporta y se registra (D3).

`Verificado: unverified` = la fila nunca fue confirmada contra GitLab en vivo. Al
confirmarla, reemplazá `unverified` por la fecha `YYYY-MM-DD` de esa confirmación.

Mantenimiento (repo `gentle-ai`): este archivo existe en dos copias que deben editarse
en el mismo commit — no hay chequeo automático de paridad entre ellas:

- `skills/qa-locator-hunting/references/erp-mf-catalog.md`
- `internal/assets/skills/qa-locator-hunting/references/erp-mf-catalog.md`

## Índice rápido

| Slug | En una línea |
|---|---|
| `erp-mf-root-config` | shell/host del microfrontend, remotes y rutas raíz |
| `erp-mf-comun` | controles compartidos: tablas, grillas, modales, toasts, buscadores |
| `erp-mf-configuracion` | parámetros de empresa: series, correlativos, impuestos, monedas, sucursales |
| `erp-mf-configuraciones` | catálogos maestros y ajustes por módulo |
| `erp-mf-estilos` | tema, tokens de diseño, SCSS, variables, CSS global |
| `erp-mf-header` | barra superior global: usuario, empresa/sucursal, notificaciones |
| `erp-mf-home` | dashboard de inicio tras el login |
| `erp-mf-logistica` | almacén, kardex, stock, inventario, guía de remisión |
| `erp-mf-menu` | navegación estructural entre módulos |
| `erp-mf-punto-venta` | emisión y cobro de comprobantes en caja (POS) |
| `erp-mf-punto-venta-menu` | carta/productos del punto de venta |
| `erp-mf-resources` | activos estáticos, íconos, i18n |
| `erp-mf-seguridad` | autenticación y autorización |
| `erp-mf-tiendalink` | integración ERP ↔ tienda online |

## Filas

### `erp-mf-root-config`

- **gitlab_path**: `SmartClic/erp-mf-root-config`
- **slug**: `erp-mf-root-config`
- **Términos de dominio**: shell, host, module federation, remotes, bootstrap, ruteo raíz
- **Flujo de negocio**: arranque y composición de la app; casi nunca tiene locators de negocio.
- **No confundir con**: `erp-mf-menu` (navegación entre módulos ya arrancados).
- **Verificado**: unverified

### `erp-mf-comun`

- **gitlab_path**: `SmartClic/erp-mf-comun`
- **slug**: `erp-mf-comun`
- **Términos de dominio**: compartido, común, tabla, grilla, modal, toast, paginador,
  datepicker, buscador, combo, directiva, pipe
- **Flujo de negocio**: controles reutilizados por todos los microfronts; primer lugar
  a mirar si el elemento aparece igual en varias pantallas.
- **No confundir con**: `erp-mf-estilos` (tema visual, no controles funcionales).
- **Verificado**: unverified

### `erp-mf-configuracion`

- **gitlab_path**: `SmartClic/erp-mf-configuracion`
- **slug**: `erp-mf-configuracion`
- **Términos de dominio**: configuración, parámetros de empresa, series, correlativos,
  impuestos, monedas, sucursales
- **Flujo de negocio**: ajustes transversales que condicionan la emisión y los catálogos.
- **No confundir con**: `erp-mf-configuraciones` (catálogos maestros por módulo — si no
  está en uno, buscar en el otro).
- **Verificado**: unverified

### `erp-mf-configuraciones`

- **gitlab_path**: `SmartClic/erp-mf-configuraciones`
- **slug**: `erp-mf-configuraciones`
- **Términos de dominio**: configuraciones, catálogos maestros, ajustes por módulo,
  listas de valores
- **Flujo de negocio**: mantenimiento de maestros por módulo.
- **No confundir con**: `erp-mf-configuracion` (parámetros transversales de empresa —
  ambigüedad conocida, si no está en uno, buscar en el otro).
- **Verificado**: unverified

### `erp-mf-estilos`

- **gitlab_path**: `SmartClic/erp-mf-estilos`
- **slug**: `erp-mf-estilos`
- **Términos de dominio**: estilos, tema, tokens de diseño, SCSS, variables, CSS global
- **Flujo de negocio**: capa visual; rara vez expone locators propios.
- **No confundir con**: `erp-mf-comun` (controles funcionales compartidos).
- **Verificado**: unverified

### `erp-mf-header`

- **gitlab_path**: `SmartClic/erp-mf-header`
- **slug**: `erp-mf-header`
- **Términos de dominio**: header, barra superior, usuario logueado, selector de
  empresa/sucursal, notificaciones, cerrar sesión
- **Flujo de negocio**: cabecera global presente en toda la app.
- **No confundir con**: `erp-mf-menu` (navegación lateral, no cabecera).
- **Verificado**: unverified

### `erp-mf-home`

- **gitlab_path**: `SmartClic/erp-mf-home`
- **slug**: `erp-mf-home`
- **Términos de dominio**: home, inicio, dashboard, accesos directos, widgets,
  tarjetas resumen
- **Flujo de negocio**: pantalla de aterrizaje tras el login.
- **No confundir con**: `erp-mf-menu` (navegación estructural, no la pantalla inicial).
- **Verificado**: unverified

### `erp-mf-logistica`

- **gitlab_path**: `SmartClic/erp-mf-logistica`
- **slug**: `erp-mf-logistica`
- **Términos de dominio**: logística, almacén, kardex, stock, inventario, ingreso,
  salida, transferencia, guía de remisión
- **Flujo de negocio**: movimiento y control de existencias entre almacenes.
- **No confundir con**: `erp-mf-punto-venta` (emisión/cobro, no stock).
- **Verificado**: unverified

### `erp-mf-menu`

- **gitlab_path**: `SmartClic/erp-mf-menu`
- **slug**: `erp-mf-menu`
- **Términos de dominio**: menú lateral, navegación, árbol de módulos, permisos de
  menú, breadcrumb
- **Flujo de negocio**: navegación estructural entre módulos.
- **No confundir con**: `erp-mf-header` (cabecera, no navegación lateral).
- **Verificado**: unverified

### `erp-mf-punto-venta`

- **gitlab_path**: `SmartClic/erp-mf-punto-venta`
- **slug**: `erp-mf-punto-venta`
- **Términos de dominio**: venta, punto de venta, POS, emisión, comprobante, boleta,
  factura, nota de venta, caja, cobro, medio de pago, cliente, anulación
- **Flujo de negocio**: emisión y cobro de comprobantes en caja — selección de cliente
  y productos, cálculo de totales e impuestos, medio de pago, impresión y anulación.
- **No confundir con**: `erp-mf-punto-venta-menu` (carta/productos del POS),
  `erp-mf-configuracion` (series y correlativos del comprobante).
- **Verificado**: unverified

### `erp-mf-punto-venta-menu`

- **gitlab_path**: `SmartClic/erp-mf-punto-venta-menu`
- **slug**: `erp-mf-punto-venta-menu`
- **Términos de dominio**: carta, menú de productos del POS, categorías, precios,
  mesas, pedido rápido
- **Flujo de negocio**: selección de productos/carta dentro del punto de venta.
- **No confundir con**: `erp-mf-punto-venta` (emisión y cobro del comprobante).
- **Verificado**: unverified

### `erp-mf-resources`

- **gitlab_path**: `SmartClic/erp-mf-resources`
- **slug**: `erp-mf-resources`
- **Términos de dominio**: recursos, assets, íconos, imágenes, i18n, traducciones
- **Flujo de negocio**: activos estáticos; sin UI de negocio propia.
- **No confundir con**: `erp-mf-estilos` (tema y variables visuales, no assets).
- **Verificado**: unverified

### `erp-mf-seguridad`

- **gitlab_path**: `SmartClic/erp-mf-seguridad`
- **slug**: `erp-mf-seguridad`
- **Términos de dominio**: seguridad, login, usuarios, roles, permisos, cambio de
  contraseña, auditoría de acceso
- **Flujo de negocio**: autenticación y autorización.
- **No confundir con**: `erp-mf-header` (usuario logueado en la barra superior, no el
  flujo de login/roles).
- **Verificado**: unverified

### `erp-mf-tiendalink`

- **gitlab_path**: `SmartClic/erp-mf-tiendalink`
- **slug**: `erp-mf-tiendalink`
- **Términos de dominio**: tiendalink, tienda online, e-commerce, catálogo web,
  pedidos web, sincronización
- **Flujo de negocio**: integración entre el ERP y la tienda online.
- **No confundir con**: `erp-mf-punto-venta` (caja física, no tienda online).
- **Verificado**: unverified

## Ejemplo de desambiguación

`"reporte de venta"` → `erp-mf-punto-venta` si es la pantalla de emisión/caja;
`erp-mf-configuracion` si es sobre series/correlativos del comprobante; si sigue
ambiguo, no elijas — andá a `search_projects` (D2).
