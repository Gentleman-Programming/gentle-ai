# Repository Registry

This file acts as the single source of truth for the ecosystem's repositories to assist in Impact Discovery.

The `repo-slug` column is the output of `Slugify(gitlab_path)` (see `internal/repository/slug.go`): trim, lowercase,
collapse `/` and any run of non-`[a-z0-9]` characters to a single `-`, trim leading/trailing `-`. It is the stable key
used to scope per-repo artifacts (e.g. `sdd/{change}/apply-progress/{repo-slug}`).

The `Profile` column points at `skills/repo-profiles/{name}/SKILL.md` when a profile exists. This registry is the
single source of truth; where a profile's `metadata.gitlab_path` disagrees with a row here, this file wins. Profiles
that do not yet declare `metadata.gitlab_path` are a deferred follow-up and do not block this registry from being
authoritative.

| Repository (gitlab_path) | repo-slug | Owner | Type | Purpose | Profile |
|---|---|---|---|---|---|
| `gp-apps-cross/Pagos` | `gp-apps-cross-pagos` | gp-apps-cross | backend (.NET, Clean Architecture) | Payments backend, Clean Architecture + CQRS reference implementation | `skills/repo-profiles/payments-api/SKILL.md` |
| `SmartClic/MSPagos` | `smartclic-mspagos` | SmartClic | backend (legacy) | Legacy payments backend (predates the Clean Architecture standard) | none |
| `SmartClic/cliente-hub-front` | `smartclic-cliente-hub-front` | SmartClic | frontend (Vue) | Referrals / resellers portal. NOT the payments frontend. | none |
| `gp-apps-cross/portal-sr-front` | `gp-apps-cross-portal-sr-front` | gp-apps-cross | frontend (greenfield, empty) | New portal, no code yet; pattern selection pending (see Architecture Catalog, Greenfield Guidance) | none |
| `SmartClic/erp-mf-root-config` | `smartclic-erp-mf-root-config` | SmartClic | single-spa root orchestrator | Owns the import map and microfrontend registration for the ERP single-spa ecosystem | `skills/repo-profiles/erp-mf-root-config/SKILL.md` |
| `SmartClic/erp-mf-comun` | `smartclic-erp-mf-comun` | SmartClic | single-spa MFE (shared utilities) | Shared utilities consumed by other `erp-mf-*` microfrontends | `skills/repo-profiles/erp-mf-comun/SKILL.md` |
| `SmartClic/erp-mf-configuracion` | `smartclic-erp-mf-configuracion` | SmartClic | single-spa MFE | Configuration microfrontend | `skills/repo-profiles/erp-mf-configuracion/SKILL.md` |
| `SmartClic/erp-mf-configuraciones` | `smartclic-erp-mf-configuraciones` | SmartClic | single-spa MFE | Configurations microfrontend | `skills/repo-profiles/erp-mf-configuraciones/SKILL.md` |
| `SmartClic/erp-mf-estilos` | `smartclic-erp-mf-estilos` | SmartClic | single-spa MFE (shared styles) | Shared styles consumed by other `erp-mf-*` microfrontends | `skills/repo-profiles/erp-mf-estilos/SKILL.md` |
| `SmartClic/erp-mf-header` | `smartclic-erp-mf-header` | SmartClic | single-spa MFE | Header microfrontend | `skills/repo-profiles/erp-mf-header/SKILL.md` |
| `SmartClic/erp-mf-home` | `smartclic-erp-mf-home` | SmartClic | single-spa MFE | Home microfrontend | `skills/repo-profiles/erp-mf-home/SKILL.md` |
| `SmartClic/erp-mf-logistica` | `smartclic-erp-mf-logistica` | SmartClic | single-spa MFE | Logistics microfrontend | `skills/repo-profiles/erp-mf-logistica/SKILL.md` |
| `SmartClic/erp-mf-menu` | `smartclic-erp-mf-menu` | SmartClic | single-spa MFE | Menu microfrontend | `skills/repo-profiles/erp-mf-menu/SKILL.md` |
| `SmartClic/erp-mf-punto-venta` | `smartclic-erp-mf-punto-venta` | SmartClic | single-spa MFE | Point-of-sale microfrontend | `skills/repo-profiles/erp-mf-punto-venta/SKILL.md` |
| `SmartClic/erp-mf-punto-venta-menu` | `smartclic-erp-mf-punto-venta-menu` | SmartClic | single-spa MFE | Point-of-sale menu microfrontend | `skills/repo-profiles/erp-mf-punto-venta-menu/SKILL.md` |
| `SmartClic/erp-mf-resources` | `smartclic-erp-mf-resources` | SmartClic | single-spa MFE (shared resources) | Shared resources consumed by other `erp-mf-*` microfrontends | `skills/repo-profiles/erp-mf-resources/SKILL.md` |
| `SmartClic/erp-mf-seguridad` | `smartclic-erp-mf-seguridad` | SmartClic | single-spa MFE | Security microfrontend | `skills/repo-profiles/erp-mf-seguridad/SKILL.md` |
| `SmartClic/erp-mf-tiendalink` | `smartclic-erp-mf-tiendalink` | SmartClic | single-spa MFE | TiendaLink microfrontend | `skills/repo-profiles/erp-mf-tiendalink/SKILL.md` |
| `SReasonsERP/ERPLogistica` | `sreasonserp-erplogistica` | SReasonsERP | backend (.NET, Clean Architecture) | Logistics backend, Clean Architecture + CQRS reference implementation | `skills/repo-profiles/ERPLogistica/SKILL.md` |
| `SReasonsERP/ERPPlanillas` | `sreasonserp-erpplanillas` | SReasonsERP | backend (.NET, Clean Architecture) | Payroll backend | `skills/repo-profiles/ERPPlanillas/SKILL.md` |
| `SmartClic/ERPBalanceContable` | `smartclic-erpbalancecontable` | SmartClic | backend (.NET, Clean Architecture) | Accounting balance backend | `skills/repo-profiles/ERPBalanceContable/SKILL.md` |
| `GP-GCG/erptalleres` | `gp-gcg-erptalleres` | GP-GCG | backend (.NET, Clean Architecture) | Workshops backend | `skills/repo-profiles/ERPTalleres/SKILL.md` |
| `SmartClic/ERPIntegracion` | `smartclic-erpintegracion` | SmartClic | backend (.NET, Clean Architecture) | Integration backend | `skills/repo-profiles/ERPIntegracion/SKILL.md` |
| `SmartClic/ERPFinanzasCore` | `smartclic-erpfinanzascore` | SmartClic | backend (.NET, Clean Architecture) | Core finance backend | `skills/repo-profiles/ERPFinanzasCore/SKILL.md` |
| `SmartClic/erpintegracionsunat` | `smartclic-erpintegracionsunat` | SmartClic | backend (.NET, Clean Architecture) | SUNAT (Peru tax authority) integration backend | `skills/repo-profiles/ERPIntegracionSunat/SKILL.md` |
| `Gentleman-Programming/gentle-ai` | `gentleman-programming-gentle-ai` | Gentleman-Programming | platform | Agent Ecosystem Platform (this repository) | none |

## Notes

- `SmartClic/cliente-hub-front` was confirmed during discovery to be the referrals/resellers portal, distinct from
  the payments frontend. There is currently no dedicated payments frontend repository tracked in this registry.
- `gp-apps-cross/portal-sr-front` is greenfield/empty; see `docs/architecture-catalog.md` (Greenfield Guidance) for
  the pattern-selection decision that is still pending for it.
- The `SmartClic/erp-mf-*` rows (14 repositories including `root-config`) form the single-spa microfrontend
  ecosystem; see `docs/architecture-catalog.md` (Pattern: Single-SPA Microfrontends).
- ~24 repo-profile `SKILL.md` files do not yet declare `metadata.gitlab_path`. This is a deferred follow-up (see
  `sdd/dev-orchestrator-p1-foundations` design, Open Questions); this registry stands alone as the source of truth
  in the meantime.
