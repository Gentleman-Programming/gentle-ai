# Architecture Catalog

This catalog defines the enterprise standards for the `solution-architect` agent to use when selecting a pattern for
new or extended work. Every "Reference repos" entry below must match a row in `docs/repository-registry.md`.

## Pattern: Clean Architecture + CQRS (.NET)

**When to use**: new or extended backend services in the .NET ecosystem — payments, logistics, finance, integration,
payroll, and similar domain backends.

**Invariants** (lifted from the repo profiles that already state them):
- `Controllers/Query` handles reads; `Controllers/Command` handles mutations. Never mix read and write logic in the
  same controller action.
- `Entidades/` (Domain.Core) owns domain types. The API project never defines domain entities directly.
- Persistence access goes through a Unit-of-Work (`{Domain}Unit.cs`, in `Domain.Application`/`Infra.*`). Controllers
  and application services never instantiate or query `DbContext` directly.
- Configuration comes from Steeltoe Config Server. Connection strings and secrets are never hardcoded in source.
- Inter-service communication uses Kafka events. Services never call each other via direct synchronous HTTP.

**Layering** (reference: `gp-apps-cross/Pagos`):
```
Domain.Core        -- entities, value objects, domain contracts
Domain.Application -- use cases / CQRS handlers, Unit-of-Work interfaces
Infra.*             -- persistence, external integrations, Kafka producers/consumers
WebApi              -- Controllers/Query, Controllers/Command, composition root
```

**Reference repos**: `gp-apps-cross/Pagos`, `SReasonsERP/ERPLogistica`, `SReasonsERP/ERPPlanillas`,
`SmartClic/ERPBalanceContable`, `GP-GCG/erptalleres`, `SmartClic/ERPIntegracion`, `SmartClic/ERPFinanzasCore`,
`SmartClic/erpintegracionsunat`.

**Legacy note**: `SmartClic/MSPagos` predates this standard and does not follow the layering above. It is tracked in
the registry as `backend (legacy)` and is not a reference implementation for new work.

**Anti-patterns**:
- Injecting `DbContext` into a controller or query handler directly.
- Hardcoding connection strings or API keys instead of resolving them from Steeltoe Config Server.
- Synchronous service-to-service HTTP calls for events that should be published to Kafka.
- Placing domain logic in the `WebApi` project instead of `Domain.Core`/`Domain.Application`.

---

## Pattern: Single-SPA Microfrontends

**When to use**: new or extended user-facing modules within the ERP frontend ecosystem.

**Invariants**:
- `erp-mf-root-config` owns the import map and microfrontend registration. No other microfrontend registers routes
  or applications on its own.
- Microfrontends never import each other directly; cross-cutting code goes through the shared packages below.
- Shared styles are consumed via `erp-mf-estilos`; shared utilities via `erp-mf-comun` and shared static assets via
  `erp-mf-resources`. A new microfrontend does not duplicate style or utility code that already exists in these.
- Build contract: Webpack + EJS, matching the existing `erp-mf-*` repositories.

**Reference repos**: `SmartClic/erp-mf-root-config` (orchestrator) plus its 13 children —
`erp-mf-comun`, `erp-mf-configuracion`, `erp-mf-configuraciones`, `erp-mf-estilos`, `erp-mf-header`, `erp-mf-home`,
`erp-mf-logistica`, `erp-mf-menu`, `erp-mf-punto-venta`, `erp-mf-punto-venta-menu`, `erp-mf-resources`,
`erp-mf-seguridad`, `erp-mf-tiendalink`.

**Anti-patterns**:
- A new microfrontend registering itself outside `erp-mf-root-config`'s import map.
- Direct imports between sibling microfrontends (e.g. `erp-mf-menu` importing from `erp-mf-home` directly).
- Re-implementing shared styles or utilities instead of extending `erp-mf-estilos`/`erp-mf-comun`/`erp-mf-resources`.

---

## Greenfield Guidance

For a genuinely new frontend with no existing owner ecosystem to extend, two options exist. This catalog does not
resolve which one applies to a specific greenfield repo — that decision belongs to the change that creates it.

**Reference repo (pending decision)**: `gp-apps-cross/portal-sr-front` — currently empty/greenfield. This is a
`NEEDS_DECISION` carried over from a prior audit; it is documented here, not resolved here.

| Option | Description | Tradeoff |
|---|---|---|
| Standalone Vue CLI app | Independent Vue application, its own build/deploy pipeline, no coupling to `erp-mf-root-config` | Faster to bootstrap in isolation; duplicates shared-style/shared-utility concerns already solved by `erp-mf-estilos`/`erp-mf-comun`; no automatic integration with the ERP single-spa shell |
| Integrate into existing single-spa ecosystem | Register as a new `erp-mf-*` child under `erp-mf-root-config`, reuse `erp-mf-estilos`/`erp-mf-comun`/`erp-mf-resources` | Consistent with the rest of the ERP frontend estate and the Single-SPA Microfrontends pattern above; requires onboarding into the root-config import map and following the shared build contract |

Before creating a new frontend repository, the deciding change must:
1. Confirm the owner and GitLab namespace.
2. Add a row to `docs/repository-registry.md` (gitlab_path, repo-slug, owner, type).
3. Create or select a `skills/repo-profiles/{name}/SKILL.md`.
4. Record the pattern choice (standalone vs. single-spa integration) in that change's own artifacts.

---

## Choosing a Pattern (decision table)

| Situation | Pattern |
|---|---|
| New or extended .NET backend service in this ecosystem | Clean Architecture + CQRS (.NET) |
| New or extended user-facing module in the ERP frontend estate | Single-SPA Microfrontends |
| Genuinely new frontend with no existing owner ecosystem to extend | Greenfield Guidance — decide standalone vs. single-spa integration explicitly |
| Legacy backend already deviating from Clean Architecture (e.g. `SmartClic/MSPagos`) | Do not extend the legacy shape for new features; propose migration or isolate new work behind the Clean Architecture pattern |

---

## Do NOT Default To New Repository

Creating a new repository is the highest-cost, hardest-to-reverse decision this catalog covers. Before proposing one:

1. Check `docs/repository-registry.md` for an existing repository owned by the same team that could be extended
   instead.
2. Check whether the work fits inside an existing `erp-mf-*` microfrontend or an existing `.NET` backend service
   following the Clean Architecture + CQRS pattern.
3. Only propose a new repository when neither an existing owner nor an existing pattern boundary can absorb the
   work, and justify that explicitly in the proposing change's artifacts (per `solution-architect`'s standing rule to
   justify against extending an existing owner before proposing a new repository).
4. If a new repository is justified, follow the Greenfield Guidance section above and register it immediately in
   `docs/repository-registry.md`.
