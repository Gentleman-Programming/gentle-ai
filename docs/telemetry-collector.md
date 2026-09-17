# Telemetry Collector

← [Back to README](../README.md)

`cmd/gentle-telemetry` is the self-hosted collector for gentle-ai's anonymous
telemetry (issue [#4310](https://github.com/Gentleman-Programming/gentle-ai/issues/4310)).
It answers one question — "how many people use this, and how" — from events
the client sends opportunistically, without ever learning who they are or
where they run.

The client side (when to send, `DO_NOT_TRACK`/`GENTLE_AI_TELEMETRY`/`CI`
opt-out, `gentle-ai telemetry status|enable|disable|preview`) is implemented
on a sibling branch and is out of scope here. This document covers the
collector: the wire contract it accepts, storage and retention, the deploy
kit under `deploy/telemetry/`, and how to read `/v1/summary`.

## Wire contract: `gentle-ai.telemetry-event/v1`

```json
{
  "schema": "gentle-ai.telemetry-event/v1",
  "event": "install | heartbeat",
  "install_id": "uuid-v4, generated once by the client",
  "sent_at": "RFC3339 UTC",
  "version": "semver, e.g. 2.3.0 or 2.3.0-rc.1",
  "os": "darwin | linux | windows",
  "arch": "one of Go's GOARCH values, e.g. arm64 | amd64 | ...",
  "agents": ["claude-code", "opencode", "... (enum of known agent ids)"],
  "components": ["sdd", "engram", "... (enum of known component ids)"],
  "rdd_enabled": true,
  "counters": {
    "syncs": 0,
    "sdd_phase_runs": 0,
    "reviews_approved": 0,
    "reviews_correction": 0,
    "reviews_escalated": 0
  }
}
```

`counters` is present only on `heartbeat` events; the collector rejects it
on `install` events. The maximum body size is 4 KiB.

The canonical schema will live at
`contracts/telemetry/v1/schemas/event.schema.json`, published by the client
branch. Until that lands, this collector validates against a local,
byte-identical copy at
[`internal/telemetrycollector/schema/event.schema.json`](../internal/telemetrycollector/schema/event.schema.json).
Once the canonical copy is published, point the collector at it instead of
keeping two copies of the same schema.

**What the collector never receives or stores**: the payload above carries
no paths, repository names, usernames, hostnames, prompts, or diffs — that
is a client-side guarantee. The one thing the *collector* independently
guarantees on its own side is that it never persists or logs the caller's
IP address; see [No IP addresses, anywhere](#no-ip-addresses-anywhere).

**Free text is impossible, not just discouraged**: every string field in
the schema is closed with either an `enum` (`event`, `os`, `arch`,
`agents[]`, `components[]`, and `schema` itself) or a `pattern`
(`install_id`, `version`, `sent_at`). `agents[]` and `components[]` accept
only the fixed sets of known agent and component ids gentle-ai ships —
never an arbitrary string — and `additionalProperties: false` at every
object level rejects any field this document doesn't name.
`TestEventSchema_EveryStringPropertyIsClosed` walks the schema and fails
the build if a future string property is ever added without one of these
constraints. The result is a dataset that is statistical only: no field in
it can carry user-authored text, which is what makes it safe to aggregate
and report on in the first place.

## Runtime observations

`POST /v1/runtime-events` accepts the separate
[runtime event schema](../contracts/telemetry/runtime/v1/schemas/event.schema.json):
exactly `schema`, `registry`, a fresh independent `delivery_id`, `host`, and public
`rows`, bounded to 16 KiB and 32 rows. No batch/session/task/install/user identity,
activity timestamp, private name, prompt, code, path, or raw error is accepted.
See [runtime fields and native command](telemetry.md#runtime-metrics-one-attempt-no-client-storage).

Native clients send once over HTTPS, without redirects, with a 3-second network
budget and bounded acknowledgement. Failure discards metrics; no client queue,
outbox, retry, cooldown, daemon, or migration cleanup exists. Server deduplication
remains defensive: HTTP 200 returns exactly
`{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}` or
`duplicate`. A conflicting identity returns 409; invalid, oversized, rate-limited,
and unavailable requests return 400, 413, 429, and 500 respectively. Clients do
not retry any of them or retain the event after a lost response.

Existing SQLite `runtime_deliveries`/`runtime_rows`, `user_version=1`, transactional
storage, and startup/daily retention remain unchanged. Rate limiting for
`/v1/runtime-events` has its own budget, separate from `/v1/events` — see
[Rate limiting](#rate-limiting).
`received_at` is delivery time, not activity time. Existing `--retention-days`
purges whole deliveries strictly before the UTC cutoff; deduplication ends when
the corresponding delivery is purged. No runtime daily rollup, new scheduler,
production configuration change, or deployment is included.

## Runtime metrics for VictoriaMetrics

`--runtime-store` selects how a newly stored `/v1/runtime-events` delivery is
persisted:

- `sqlite` (default): today's behavior, unchanged — `runtime_deliveries` and
  `runtime_rows` as described above, no counters, `GET /metrics` serves an
  empty body.
- `metrics`: no raw rows at all. The delivery is deduplicated by id alone in
  `runtime_delivery_ids(delivery_id TEXT PRIMARY KEY, received_at INTEGER NOT
  NULL)`, and its rows are folded into an in-memory Prometheus counters
  registry instead. This table stores no payload, because in this mode there
  is no canonical payload to compare a repeat against — a repeated id is
  always treated as `duplicate`, the same trust model as any bare idempotency
  key (contrast `runtime_deliveries`, which detects a same-id-different-payload
  conflict via its stored `canonical_payload`). It shares `--retention-days`
  and the daily purge with `runtime_deliveries`/`runtime_rows`; an id expires
  the same way and a retry past that point is `stored` again, not tombstoned.
- `both`: writes raw rows and observes into the registry, for a transition
  window before cutover.

A delivery is only ever observed into the registry once, on `stored` — never
on `duplicate`, and never at all under `sqlite`.

`GET /metrics` renders that registry as Prometheus text exposition
(`Content-Type: text/plain; version=0.0.4`) for VictoriaMetrics to scrape. It
has no auth: the collector's listener is loopback-only and VictoriaMetrics
scrapes it from the same host, the same trust boundary every other
unauthenticated route on this listener already relies on. Every metric is a
monotonically increasing counter (reset only by process restart, handled
downstream by `increase()`/`rate()`), all carrying `host`:

| Metric | Labels | Meaning |
|---|---|---|
| `gentle_runtime_deliveries_total` | `host` | one per stored delivery |
| `gentle_runtime_rows_total` | `host,agent_kind,agent_class,provider,model,selected_effort` | one per row |
| `gentle_runtime_responses_total` | same as rows | sum of `responses` |
| `gentle_runtime_launches_total` | same as rows | sum of `launches` (a `null` observation adds 0) |
| `gentle_runtime_tokens_total` | same as rows, `+kind` (`input\|output\|cache_read\|cache_creation\|reasoning\|total`) | sum of each token object's `sum` |
| `gentle_runtime_token_fields_total` | same as rows, `+kind,state` (`reported\|unavailable\|unsupported`) | sum of each token object's coverage counts |
| `gentle_runtime_errors_total` | same as rows, `+category` | one per row, skipped entirely when `error_category` is `none` |
| `gentle_runtime_duration_ms_sum` | same as rows, `+duration_kind` | sum of `duration.sum_ms` |
| `gentle_runtime_duration_measured_total` | same as rows, `+duration_kind` | sum of `duration.measured_count` |
| `gentle_runtime_rows_by_evidence_total` | `host,model_evidence,effective_effort` | one per row, kept low-cardinality by leaving out agent/provider/model |

A label value is sanitized for the exposition format (`\`, `"`, and newline
escaped) and an empty value renders as `unknown`; in practice every label
already comes from a closed wire-contract vocabulary (see
[Runtime observations](#runtime-observations)), so this only matters if
`RuntimeMetrics.Observe` is ever called from something other than a parsed,
validated `telemetry.RuntimeEvent`.

**Cutover**: the default stays `sqlite` until the VictoriaMetrics deploy
(`deploy/telemetry/`, not yet built — see the feature's task list) is
installed, backfilled, and verified. Flipping `--runtime-store` before that
exists means either losing runtime history (`metrics` with nothing scraping
`/metrics` yet) or, in `both`, doubling work for no benefit.

## HTTP API

| Endpoint | Method | Auth | Notes |
|---|---|---|---|
| `/v1/events` | POST | none | Body ≤ 4 KiB, strict schema validation, own per-address rate limit (`--rate-limit-per-minute`). `202` on accept, `400` invalid, `413` oversize, `429` rate-limited. |
| `/v1/runtime-events` | POST | none | Body ≤ 16 KiB; strict public observations; own per-address rate limit (`--runtime-rate-limit-per-minute`). `200` with `stored`/`duplicate`; one client attempt only. |
| `/v1/summary` | GET | `Authorization: Bearer <token>` | Returns the JSON described below. `401` without a valid token. |
| `/healthz` | GET | none | Liveness check for the reverse proxy / process supervisor. |
| `/metrics` | GET | none | Prometheus text exposition of the runtime counters registry — see [Runtime metrics for VictoriaMetrics](#runtime-metrics-for-victoriametrics). Empty body under `--runtime-store=sqlite` (the default). |

### No IP addresses, anywhere

- The per-address rate limiters (one for `/v1/events`, one for
  `/v1/runtime-events`) key an in-memory token bucket by remote address,
  and never write that address to disk, to a log line, or anywhere else.
  See `internal/telemetrycollector/limiter.go`.
- The `events` table has no column that could hold a remote address (see
  the schema below); `Event`, decoded from the request body, structurally
  cannot carry one either, since the wire contract has no such field.
- HTTP handler logs record the event kind and outcome only — never
  `r.RemoteAddr`. `TestHandleEvents_NeverStoresOrLogsRemoteAddress` in
  `internal/telemetrycollector/handlers_test.go` asserts this against both
  the database row and the log output for a request from a known address.

## Storage

SQLite via `modernc.org/sqlite` (cgo-free), opened with `PRAGMA
journal_mode=WAL` and a single connection (`SetMaxOpenConns(1)`): this
collector's expected throughput does not justify concurrent SQLite
writers, and a single connection avoids `SQLITE_BUSY` entirely rather than
tuning around it. WAL, not DELETE, because this process is not the only
reader of the database: Grafana's SQLite datasource and the open-data
export both hold read-only connections against the same file from outside
this process. Under DELETE mode, an external read that outlasted
`busy_timeout` made `InsertRuntimeEvent` fail outright, since DELETE takes
an exclusive lock to commit a write. WAL lets writes commit without
waiting on those external readers, at the cost of two extra sidecar files
next to the database, `events.sqlite-wal` and `events.sqlite-shm`; the
read-only Grafana deployment (below) is granted read access to both via
POSIX ACLs, including a default ACL so a sidecar recreated later stays
readable without rerunning the installer.

```sql
CREATE TABLE events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  received_at INTEGER NOT NULL,    -- Unix nanoseconds, UTC (not RFC3339 text —
                                    -- see the comment on receivedAtKey in storage.go
                                    -- for why a text encoding broke day-boundary comparisons)
  event TEXT NOT NULL,             -- "install" | "heartbeat"
  install_id TEXT NOT NULL,
  version TEXT NOT NULL,
  os TEXT NOT NULL,
  arch TEXT NOT NULL,
  agents_json TEXT NOT NULL,
  components_json TEXT NOT NULL,
  rdd_enabled INTEGER NOT NULL,
  counters_json TEXT               -- NULL for "install"
);

CREATE TABLE rollups_daily (
  day TEXT NOT NULL,               -- "YYYY-MM-DD", UTC
  metric TEXT NOT NULL,            -- active_install | agent | component | version | rdd_enabled
  key TEXT NOT NULL,
  value INTEGER NOT NULL,
  PRIMARY KEY (day, metric, key)
);
```

An in-process daily job (`telemetrycollector.RunMaintenance`, driven by
`cmd/gentle-telemetry/main.go`) runs once at startup (catching up any
rollup missed while the process was down) and then once a day, anchored to
00:05 UTC rather than 24 hours after startup, so the previous day's rollup
is never more than a few minutes stale on the dashboard regardless of when
the process last restarted:

1. Rolls up **yesterday**'s events into `rollups_daily` (idempotent — safe
   to re-run after a crash or restart).
2. Purges raw `events` rows older than `--retention-days` (default 90).

`rollups_daily` itself is never purged: it is the durable historical record
once the raw rows behind it age out.

### Rollup metrics

For each UTC calendar day, one install's *latest* event of that day is used
as its attribute snapshot (so an install that updates mid-day is counted
once, under its newest version/agents/RDD state); *any* event makes it
"active" for the day:

- `active_install`: one row per active install id (`value=1`). This is what
  lets the summary endpoint count *distinct* installs across a date range
  without re-reading raw events.
- `agent`, `component`: one row per attribute value, `value` = number of
  distinct installs whose latest event that day reported it.
- `version`: same shape, keyed by version string.
- `rdd_enabled`: two rows, keyed `"true"`/`"false"`.

## `GET /v1/summary`

```json
{
  "generated_at": "2026-06-15T00:00:00Z",
  "installs_per_month": [{"month": "2026-06", "unique_installs": 42}, "... 12 entries"],
  "weekly_active_installs": [{"week": "2026-W24", "active_installs": 17}, "... 12 entries"],
  "agent_distribution": {"claude-code": 30, "opencode": 12},
  "component_distribution": {"sdd": 28, "engram": 19},
  "version_distribution": {"2.3.0": 25, "2.2.1": 5},
  "rdd_enabled_ratio": 0.63,
  "downloads": {
    "npm": {"gentle-pi": {"last_day": 120, "last_30_days": 3400}},
    "github": {"v2.3.0": 890}
  }
}
```

- **`installs_per_month`**: distinct install ids active in each of the last
  12 calendar months (UTC), including the current, partial month.
- **`weekly_active_installs`**: distinct install ids active in each of the
  last 12 ISO (Monday-start) weeks, including the current, partial week.
- **`agent_distribution` / `component_distribution` / `version_distribution`
  / `rdd_enabled_ratio`**: aggregated over the trailing 30 days. This window
  is a design choice, not part of the wire contract — the issue asks for
  these distributions without specifying a period, so 30 days was chosen to
  answer "who's using it now" without a second rollups table. **These are
  install-days, not distinct installs**: an install active on 10 different
  days in the window contributes 10 to whichever agent/component/version it
  reported, once per day. Read them as "how usage breaks down day to day",
  not "how many distinct installs use X".
- Both computations read `rollups_daily` for every day except today, and
  read today's not-yet-rolled-up events directly from the `events` table,
  merging the two — matching the design's "computed from `rollups_daily`
  plus today's raw events."
- **`downloads`**: public download counts, fetched daily from the npm
  registry (`--npm-package`, repeatable, default `gentle-pi`,
  `gentle-engram`) and the GitHub API (`--github-repo`, repeatable, default
  `Gentleman-Programming/gentle-ai`) — see
  [External download counts](#external-download-counts). Nothing here
  comes from a gentle-ai install; it is a public count of who downloaded
  the tools, not telemetry about how they are used.

All of this is computed on demand in `BuildSummary`
(`internal/telemetrycollector/summary.go`); there is no caching layer, since
querying a handful of small SQLite tables is fast enough at this scale.

## Rate limiting

`POST /v1/events` and `POST /v1/runtime-events` each have their own
in-memory token bucket per remote address, keyed by the same client
address (see "Deriving the client address" below): `--rate-limit-per-minute`
(default 60) for `/v1/events`, and `--runtime-rate-limit-per-minute`
(default 600) for `/v1/runtime-events`. They used to share one bucket,
which meant frequent runtime heartbeats and infrequent stored events
competed for the same budget — stored deliveries plateaued at exactly the
configured limit while heartbeats were rejected once it was exhausted, and
"active machines" counts collapsed as a result. Burst capacity equals each
limit, refilled continuously at `limit/60` tokens per second. Buckets are
swept every 10 minutes and evicted after 30 minutes of inactivity, so
neither limiter's memory grows unbounded across many distinct callers.
None of this state is ever persisted. A 429 on either endpoint is logged
(`"telemetry event rejected"` / `"runtime telemetry rejected"`,
`"reason", "rate_limited"`) with no client address attached.

### Deriving the client address

The rate limiter's key is the TCP peer address, unless the peer is a
trusted proxy (`--trusted-proxy-cidr`, default `127.0.0.0/8,::1/128`), in
which case it is the first `X-Forwarded-For` hop that parses as an IP
address (falling back to `X-Real-IP`, then the peer). A forwarded value
that does not parse as an IP is skipped rather than trusted verbatim: a
misconfigured reverse proxy has been observed sending
`X-Forwarded-For: (null), <client>`, which — without this validation —
keyed every client on the literal string `"(null)"`, collapsing the rate
limiter (and "active machines" counts derived from it) to a single shared
bucket. When a trusted proxy's forwarded header has no element that
parses, this is logged once per minute as `"ignored an invalid forwarded
address"`, with the header's value itself never logged.

## Running it

```
--listen 127.0.0.1:18181                                 # bind address
--db /var/lib/gentle-telemetry/events.sqlite              # SQLite file
--summary-token-file <path>                               # bearer token for /v1/summary (local runs; systemd uses LoadCredential, see Token rotation)
--retention-days 90                                        # raw event retention
--rate-limit-per-minute 60                                 # per-address budget on /v1/events
--runtime-rate-limit-per-minute 600                         # per-address budget on /v1/runtime-events
--runtime-store sqlite                                      # sqlite (default) | metrics | both — see Runtime metrics for VictoriaMetrics
--trusted-proxy-cidr 127.0.0.0/8 --trusted-proxy-cidr ::1/128  # peers allowed to set X-Forwarded-For (repeatable; this is the default)
--npm-package gentle-pi --npm-package gentle-engram        # npm packages to fetch daily downloads for (repeatable; this is the default)
--github-repo Gentleman-Programming/gentle-ai              # GitHub repo to fetch release downloads for (repeatable; this is the default)
--github-token-file <path>                                  # optional: raises the GitHub API rate limit
```

The process listens on loopback by default (port 18181, chosen simply
because it was free on the reference VPS — 8080/8787-style defaults were
already taken by other services there) and expects a reverse proxy — this
deploy kit targets a cPanel/WHM box, so that means Apache, not Caddy — in
front of it for TLS. It shuts down gracefully on `SIGINT`/`SIGTERM`,
draining in-flight requests before exiting, and waits for any in-flight
daily maintenance run (see [Retention](#retention)) to finish before
closing the database, so a shutdown mid-rollup never races a closed
connection.

**Building it**: `cmd/gentle-telemetry` is not currently wired into
`.goreleaser.yaml`. The existing config builds a single binary
(`cmd/gentle-ai`) with release hooks specific to that binary (provider
contract bundling, release provenance, minisign signing); bolting a second,
unrelated binary onto the same `archives:`/`builds:` block would either
duplicate those hooks unnecessarily or silently bundle `gentle-telemetry`
into the `gentle-ai` release archive. Until there is a real need for signed
releases of the collector, build it directly:

```
go build ./cmd/gentle-telemetry
```

`deploy/telemetry/install.sh` supports both this local-build path
(`--local-source`) and, for when a release pipeline exists,
`--release-tag`.

## Deploying (`deploy/telemetry/`)

This kit targets a real reference VPS shape: **AlmaLinux 9 with cPanel/WHM**,
Apache (`httpd`) already bound to 80/443, systemd, and Docker — 3 vCPU,
3.6 GiB RAM. There is no Caddy here, and none is installed by this kit.

Subdomains on this server are **not** cPanel accounts: they are explicit
`<VirtualHost>` blocks appended directly to
`/etc/apache2/conf.d/includes/post_virtualhost_global.conf` (confirmed by
inspecting the existing `engram.condetuti.com` blocks there — a `:80` block
that lets the ACME challenge through and redirects everything else to
HTTPS, and a `:443` block with the Let's Encrypt certificate paths and
`ProxyPass`). An EasyApache "userdata" include is **never loaded** for a
domain configured this way, so this kit does not use one.

| File | Purpose |
|---|---|
| `apache/telemetry-vhost.conf.tmpl` | Template for the two `<VirtualHost>` blocks (`:80` and `:443`), mirroring the existing pattern: proxies `/v1/`, `/healthz`, and (with `--with-grafana`) `/grafana/` to loopback, asserts `X-Forwarded-For` from Apache itself, force-HTTPS except for the ACME challenge path, and a supplementary access log that omits the client address for `/v1/`. `__DOMAIN__` is substituted by `install.sh --domain`. Not applied automatically — see below. |
| `axiom-telemetry.service` | systemd unit: runs as the static `gentle-telemetry` system user (created by `install.sh`), `StateDirectory=gentle-telemetry`, and a hardened sandbox (no new privileges, restricted syscalls/namespaces/capabilities, private `/tmp` and devices). See [Why a static user, not `DynamicUser`](#why-a-static-user-not-dynamicuser). |
| `axiom-telemetry-backup` + `.service` + `.timer` | Nightly `sqlite3 VACUUM INTO` snapshot uploaded via `rclone copy` to a configurable remote, then deleted locally; also backs up VictoriaMetrics when it is installed. The logic lives in the standalone `axiom-telemetry-backup` script (installed to `/usr/local/bin`), not inline in the unit's `ExecStart` — systemd expands `$VAR`/`${VAR}` there using its own environment before the shell runs, which would mangle a script's local variables. The unit runs as root for simplicity: it just needs read access to the collector's state directory. See [VictoriaMetrics](#victoriametrics). |
| `victoria-metrics.service` | systemd unit for single-node VictoriaMetrics, installed with `--with-victoria-metrics`; runs as the static `victoria-metrics` system user with the same hardening approach as `axiom-telemetry.service`. See [VictoriaMetrics](#victoriametrics). |
| `victoria-metrics-scrape.yaml` | The one `promscrape.config` job (`gentle-telemetry`, 15s interval) scraping the collector's `/metrics` on `127.0.0.1:18181`. Installed to `/etc/victoria-metrics/scrape.yaml`. |
| `grafana/` | Datasource and dashboard provisioning for an optional on-box Grafana; see [Grafana dashboards](#grafana-dashboards). With both `--with-grafana` and `--with-victoria-metrics`, also provisions the `gentle-runtime-vm` Prometheus datasource. |
| `install.sh` | Creates the static `gentle-telemetry` system user (migrating an older `DynamicUser`-layout install in place if found), installs the binary, `sqlite3` and `rclone` (via `dnf`), the systemd units, and a generated summary token; with `--domain`, renders the vhost template to `/root/telemetry-vhost.conf.rendered`; with `--with-grafana`, installs Grafana OSS from its official rpm repo; with `--with-victoria-metrics`, installs single-node VictoriaMetrics (see [VictoriaMetrics](#victoriametrics)). It never edits `post_virtualhost_global.conf`, runs `apachectl configtest`, or reloads `httpd` — those, plus DNS and the certificate, are printed at the end as operator steps, in the order they must run. |

```
sudo ./deploy/telemetry/install.sh --local-source /path/to/gentle-ai/checkout \
  --domain telemetry.example.com --with-grafana
```

(Omit `--domain` to install just the service and have the script print
where to render `apache/telemetry-vhost.conf.tmpl` yourself; omit
`--with-grafana` to skip Grafana entirely.)

### Why a static user, not DynamicUser

`axiom-telemetry.service` used to run with `DynamicUser=yes`. With
`DynamicUser`, systemd materializes `StateDirectory=gentle-telemetry` under
`/var/lib/private/gentle-telemetry` (a directory it keeps at exactly mode
`0700`) and leaves a symlink at `/var/lib/gentle-telemetry`. To let Grafana
read the SQLite file, `install.sh --with-grafana` had to grant the
`grafana` user search access on every ancestor of that private directory
that was not already world-searchable, including `/var/lib/private`
itself. That ACL raised `/var/lib/private`'s effective mode above `0700`,
and on the next restart systemd refused to start the unit at all:

```
Directory "/var/lib/private" already exists, but has mode 0710 that is
too permissive (0700 was requested), refusing.
```

That is a hard requirement of `DynamicUser`, not a bug to work around with
a looser ACL: `/var/lib/private` is shared by every `DynamicUser` unit on
the box, so anything that widens it risks every one of them. The fix is to
not need a path under `/var/lib/private` at all. `install.sh` now creates
a static system user/group (`useradd --system --home-dir
/var/lib/gentle-telemetry --shell /sbin/nologin --user-group
gentle-telemetry`), and the unit runs with `DynamicUser=no`,
`User=gentle-telemetry`, `Group=gentle-telemetry`. `StateDirectory` then
resolves to the real directory `/var/lib/gentle-telemetry` — no symlink,
nothing under `/var/lib/private` — owned by that user, with
`StateDirectoryMode=0750`. Grafana gets access via a POSIX ACL scoped to
exactly two paths: the state directory itself (`u:grafana:rx`) and
`events.sqlite` (`u:grafana:r`) — never an ancestor.

**Migrating an existing install**: run `install.sh` again. It detects the
old layout (`/var/lib/gentle-telemetry` is a symlink into
`/var/lib/private`) and migrates it in place before installing the unit:
stops `gentle-telemetry.service`, removes the symlink, moves the private
directory to `/var/lib/gentle-telemetry`, `chown -R`s it to the new
`gentle-telemetry` user, and strips any ACL this script previously granted
on `/var/lib/private` (`setfacl -b /var/lib/private; chmod 0700
/var/lib/private`). It prints one line per step actually taken; nothing
prints on a fresh install or one already migrated.

### Going live: DNS, the vhost blocks, and the certificate

`install.sh` prints these steps; it does not run them. The order matters:
the `:80` block must be live before Certbot's webroot check can pass, and
the `:443` block must **not** be appended until the certificate it
references actually exists, or `apachectl configtest` fails.

On a cPanel/WHM box, Apache selects a name-based vhost only among the
vhosts already bound to the address a request arrived on, so a
`*:80`/`*:443` block is silently skipped once any other vhost (cPanel's
own defaults included) is bound to the server's IPv4 address instead of
`*` — requests for the subdomain fall through to cPanel's default vhost
(404), and Certbot's HTTP-01 challenge fails the same way. `install.sh`
renders both blocks bound to that same IPv4 address automatically,
detected from an existing `:443` vhost in `${APACHE_INCLUDE_FILE}`; pass
`--address <ipv4|*>` to override the detected value (`*` is only correct
when every other vhost on the box is also `*`).

1. **DNS**: at your registrar, create an A record for the subdomain
   (`telemetry`, in this doc) pointing at the VPS's IP. Confirm with
   `dig +short telemetry.example.com`.
2. **Issue the certificate**, in this exact order:
   ```
   cp -a /etc/apache2/conf.d/includes/post_virtualhost_global.conf \
         /etc/apache2/conf.d/includes/post_virtualhost_global.conf.bak-$(date +%Y%m%dT%H%M%SZ)
   sed -n '/# --- BEGIN :80 VHOST ---/,/# --- END :80 VHOST ---/p' \
       /root/telemetry-vhost.conf.rendered >> /etc/apache2/conf.d/includes/post_virtualhost_global.conf
   apachectl configtest
   systemctl reload httpd

   certbot certonly --webroot -w /var/www/gentle-telemetry-acme -d telemetry.example.com

   cp -a /etc/apache2/conf.d/includes/post_virtualhost_global.conf \
         /etc/apache2/conf.d/includes/post_virtualhost_global.conf.bak-$(date +%Y%m%dT%H%M%SZ)
   sed -n '/# --- BEGIN :443 VHOST ---/,/# --- END :443 VHOST ---/p' \
       /root/telemetry-vhost.conf.rendered >> /etc/apache2/conf.d/includes/post_virtualhost_global.conf
   ```
   The template's `:80` block serves the ACME challenge from
   `/var/www/gentle-telemetry-acme` (confirm this matches how the existing
   vhosts on this box serve `/.well-known/acme-challenge/` — this kit
   could not read the live `post_virtualhost_global.conf` to verify that
   detail directly, only mirror the pattern as described). Add a renewal
   hook so Apache picks up the renewed certificate, matching this server's
   existing pattern:
   ```
   cat > /etc/letsencrypt/renewal-hooks/deploy/reload-httpd.sh <<'EOF'
   #!/bin/sh
   apachectl configtest && systemctl reload httpd
   EOF
   chmod +x /etc/letsencrypt/renewal-hooks/deploy/reload-httpd.sh
   ```
3. **Apply and verify the full config**:
   ```
   apachectl configtest
   systemctl reload httpd
   ```
4. **Verify**:
   ```
   curl -I https://telemetry.example.com/healthz
   ```
   expect `200`.
5. Set `GENTLE_TELEMETRY_BACKUP_REMOTE` in `/etc/gentle-telemetry/backup.env`
   to an `rclone` remote:path, then
   `systemctl enable --now axiom-telemetry-backup.timer`.

### Capacity

On the reference shape (3 vCPU, 3.6 GiB RAM, already running cPanel and
Docker): the collector itself is a single lightweight Go process, roughly
**~30 MB RSS** at this scale. Grafana OSS is heavier, roughly **~200 MB
RSS** once warmed up. Both fit comfortably alongside the existing cPanel
and Docker workloads on this host, but Grafana is optional for a reason —
pass `--with-grafana` only when you actually want the on-box dashboard;
skip it if `/v1/summary` (see below) is enough.

### Token rotation

`install.sh` generates `/etc/gentle-telemetry/summary.token` (root-owned
0600) if missing; `LoadCredential` in the unit hands it to the service
(running as the static `gentle-telemetry` user) without loosening
ownership. To rotate, edit the file and restart:

```
sudo install -m 0600 <(openssl rand -hex 32) /etc/gentle-telemetry/summary.token
sudo systemctl restart axiom-telemetry.service
```

The old token stops working the moment the service restarts and picks up
the new file; there is no overlap window, so coordinate with whatever reads
`/v1/summary`.

### Retention

Raw events older than `--retention-days` (default 90) are purged by the
daily job; `rollups_daily` — and therefore the historical monthly/weekly
counts in `/v1/summary` — is retained indefinitely. To change the retention
window, edit the `--retention-days` flag in
`/etc/systemd/system/axiom-telemetry.service` and
`systemctl daemon-reload && systemctl restart axiom-telemetry.service`.

The daily job catches up: if the process was down for a while, the next
run rolls up every UTC day between the last one it committed (or the
oldest raw event, on a fresh database) and yesterday, not just yesterday.
Each day's rollup is one all-or-nothing transaction, and a shutdown only
ever stops the catch-up loop *between* days — the in-progress day always
either finishes and commits, or never starts — so a restart mid-catch-up
never leaves a half-written day behind.

### VictoriaMetrics

`install.sh --with-victoria-metrics [--victoria-metrics-version <tag>]`
(default `v1.152.0`) installs single-node VictoriaMetrics next to the
collector, so the counters exposed at `/metrics` (see
[Runtime metrics for VictoriaMetrics](#runtime-metrics-for-victoriametrics))
get scraped and kept with effectively unlimited retention.

| What | Where |
|---|---|
| Binary | `/usr/local/bin/victoria-metrics` (the release's `victoria-metrics-prod` asset, checksum-verified against that release's own `_checksums.txt` before install) |
| Unit | `/etc/systemd/system/victoria-metrics.service`, static `victoria-metrics` system user, same hardening approach as `gentle-telemetry.service` |
| Data | `/var/lib/victoria-metrics`, `-retentionPeriod=100y` |
| Scrape config | `/etc/victoria-metrics/scrape.yaml` — one job, `gentle-telemetry`, `scrape_interval: 15s`, `metrics_path: /metrics`, target `127.0.0.1:18181` |
| HTTP API | loopback only, `127.0.0.1:8428` (never proxied publicly by this kit) |

Install is idempotent: re-running `install.sh --with-victoria-metrics`
skips the download once the installed binary already reports the pinned
version via `victoria-metrics --version`, but always reinstalls the unit
and scrape config so a version bump or a scrape config change still takes
effect without a redundant download. After installing the unit, `install.sh`
waits (bounded retries against `http://127.0.0.1:8428/health`) for it to
come up before returning.

**How the scrape works**: `-promscrape.config=/etc/victoria-metrics/scrape.yaml`
tells VictoriaMetrics to pull the collector's `/metrics` endpoint every 15s
over loopback. The collector's own counters reset to 0 on every process
restart (an in-memory registry — see
[Runtime metrics for VictoriaMetrics](#runtime-metrics-for-victoriametrics)),
so every PromQL query against this data uses `increase()`/`rate()`, never
the raw counter value, and a restart never shows up as a drop.

**Backup**: `gentle-telemetry-backup` skips the VictoriaMetrics step
silently when `victoria-metrics.service` is not installed/active. When it
is, the script takes a consistent snapshot via `POST /snapshot/create`,
tars `/var/lib/victoria-metrics/snapshots/<name>` to `vm-<timestamp>.tar.gz`,
uploads it via the same `rclone copy` used for the SQLite backup, then
deletes the snapshot via `POST /snapshot/delete?snapshot=<name>` (this
only removes VictoriaMetrics' own on-disk snapshot hardlinks; the
already-uploaded archive and the live series data are unaffected). The
SQLite half of the same script now takes its snapshot with `VACUUM INTO`
instead of `sqlite3 .backup`: `.backup` restarts its copy loop every time
it notices the source changed mid-copy, and under this collector's real
write rate (~1,150 deliveries/min once #4723 shipped) that restart never
stopped recurring, so `.backup` never finished under load. `VACUUM INTO`
reads one consistent snapshot in a single pass regardless of concurrent
writes.

**Grafana**: with both `--with-victoria-metrics` and `--with-grafana`,
`install.sh` also provisions a Prometheus datasource named
`gentle-runtime-vm` (fixed `uid: gentle-runtime-vm`, `url:
http://127.0.0.1:8428`, not default) alongside the existing SQLite
datasource, so dashboard panels can reference it directly without a
manual re-link after install.

**Dashboard**: in `deploy/telemetry/grafana/dashboards/gentle-ai-usage.json`,
the 25 runtime panels under the "Live activity" and "Subagents" rows (live
deliveries/responses/tokens/hosts, subagent coverage and breakdowns, host
and model and effort usage, token coverage, error and duration observations)
now query VictoriaMetrics through the `gentle-runtime-vm` datasource with
PromQL `increase()`/`rate()` expressions instead of raw SQL against the
retired `runtime_rows`/`runtime_deliveries` SQLite tables; each rewritten
panel's `description` states its exact windowing choice (a fixed window for
the "last 15 min"/"last 3h"/"last 24h" panels, `increase(metric[$__range])`
for the panels driven by the dashboard's time picker). The adoption panels
(install/heartbeat events, rollups, npm and GitHub download counts) are
unaffected and still read the SQLite datasource. The dashboard's default
time range (`time.from`) now starts at **2026-09-10**, the start of the
backfilled VictoriaMetrics history, so the "in range" runtime stat and table
panels show a meaningful total by default instead of only the last 7 days.

### Answering "how many people use it"

```
curl -sH "Authorization: Bearer $(sudo cat /etc/gentle-telemetry/summary.token)" \
  https://telemetry.example.com/v1/summary | jq .
```

- **Monthly/weekly reach**: `installs_per_month[-1].unique_installs` and
  `weekly_active_installs[-1].active_installs` are the current, still-filling
  month/week; use the second-to-last entry for the most recent *complete*
  period.
- **What people run it with**: `agent_distribution` and
  `component_distribution` (remember: install-days over the trailing 30
  days, not distinct installs — see [above](#get-v1summary)).
- **RDD adoption**: `rdd_enabled_ratio`.
- **Upgrade lag**: `version_distribution`.

### Runtime store selection

`gentle-telemetry.service` reads `/etc/gentle-telemetry/runtime.env`. `install.sh --with-victoria-metrics` writes `GENTLE_TELEMETRY_RUNTIME_STORE_FLAG=--runtime-store=metrics` there, so the next collector restart stops writing raw runtime rows and serves counters on `/metrics`; without VictoriaMetrics the file stays commented and the collector keeps the `sqlite` default. Change the mode by editing that file and restarting the unit.

## Grafana dashboards

### Runtime received observations

Four runtime tables read retained `runtime_rows` joined to `runtime_deliveries`:

| Panel | Values grouped by UTC receipt day |
|---|---|
| Runtime received observations — responses | Response occurrences by public provider/model, model evidence, and tool host |
| Runtime received observations — launches | Launch occurrences by agent class, selected effort, and separately effective effort |
| Runtime received observations — tokens and coverage | Six independent token sums, each with reported/unavailable/unsupported counts |
| Runtime received observations — duration and errors | Measured count and millisecond sum by request/message/unavailable kind and error category |

These are received observations, not complete consumption or reconstructed
sessions. A null token sum means no reported values; a reported zero remains zero.
`total_tokens` is never derived from other fields. Duration sums with no measured
observations are null; timing kinds are never combined into a latency estimate.
Occurrence sums include only reported integers, never coercing `unsupported` to
zero. SQLite uses floating-point arithmetic for fractional duration sums; these
are not arbitrary-precision decimal totals. Integer sums retain SQLite's signed
64-bit limit, and Grafana numeric display may round very large values.

The dashboard range applies inclusive bounds to server receipt time:
`d.received_at >= ${__from} * 1000000 AND d.received_at <= ${__to} * 1000000`.
Grafana supplies milliseconds; storage uses signed Unix nanoseconds. Whole
millisecond bounds from 0 through 9223372036854 (2262-04-11 UTC) multiply exactly
as SQLite integers. Use ranges within that supported epoch, not later dates.
UTC day grouping converts nanoseconds to seconds, as the existing dashboard does.
Each table returns at most 1000 groups in receipt-day order; narrow the range if
that limit is reached. Empty ranges show no observations, not synthetic zeros.

The existing service retains raw observations for 90 days and purges them through
the existing retention job. These panels add no durable daily aggregates or
retention setting. SQL and JSON1 are tested against the actual dashboard queries
with the collector's SQLite driver; no production Grafana deployment is required.

### Grafana setup

`--with-grafana` installs Grafana OSS (free, self-hosted) on the same VPS
with a read-only view over the collector's own SQLite database — no second
copy of the data, no separate store to keep in sync. It is entirely
optional; `/v1/summary` above already answers the headline questions.

**Datasource**: `deploy/telemetry/grafana/provisioning/datasources/telemetry.yaml`
uses the [`frser-sqlite-datasource`](https://grafana.com/grafana/plugins/frser-sqlite-datasource/)
plugin pointed read-only at `/var/lib/gentle-telemetry/events.sqlite`.
Grafana's own process needs read access to that database; since
`axiom-telemetry.service` runs as the static `gentle-telemetry` system
user (see [Why a static user, not DynamicUser](#why-a-static-user-not-dynamicuser)),
which `grafana` does not belong to, `install.sh --with-grafana` grants
access via POSIX ACLs (`setfacl -m u:grafana:r ...`) instead of group
membership. Because the collector opens SQLite with `journal_mode=WAL`
(see [Storage](#storage)), this now covers three files, not one:
`events.sqlite` itself plus its `-wal`/`-shm` sidecars, each granted
individually when present, plus a default ACL on the directory so a
sidecar created on a later collector restart inherits read access without
rerunning the installer. A WAL checkpoint does not remove the `-wal`/`-shm`
files; SQLite only removes them when the last connection to the database
closes cleanly, and recreates them on the next open.

A few things follow from that setup, worth knowing before relying on it:

- The default ACL on `STATE_DIR` grants `grafana` read on every file
  created there from that point on, not only the three SQLite files above.
  Do not treat the directory as a general scratch space (a manual backup
  copy, a debug dump) without accounting for that: anything dropped there
  becomes Grafana-readable too.
- A read-only reader can only open a WAL database while its sidecars
  exist. They exist for the lifetime of the collector's own connection and
  are only removed on a clean close (a graceful `systemctl stop
  gentle-telemetry`), so Grafana has nothing consistent to read from in
  the window between that stop and the collector's next start, which
  recreates them.
- SQLite creates a `-wal`/`-shm` sidecar with the same file mode as
  `events.sqlite` at that moment. Once `events.sqlite` itself carries the
  `grafana` ACL entry (from the per-file grant above, or inherited from
  the directory's default ACL if it did not exist yet when the ACL was
  set), a freshly created sidecar mirrors that mode and inherits the entry
  too. This is why the default ACL keeps working across restarts even
  though it only directly targets `STATE_DIR` itself, not the database
  file.

**Dashboard**: `deploy/telemetry/grafana/dashboards/gentle-ai-usage.json`,
provisioned via `deploy/telemetry/grafana/provisioning/dashboards/telemetry.yaml`
into the "Axiom" folder. Its panels:

| Panel | What it shows |
|---|---|
| Unique installs per month | `SELECT substr(day,1,7) AS month, COUNT(DISTINCT key) ... GROUP BY month` over `rollups_daily.active_install` — matches `/v1/summary`'s `installs_per_month`. |
| Weekly active installs | Same idea, bucketed by SQLite's `strftime('%W')` (Monday-based week-of-year). This can disagree with the API's ISO week numbering right at a year boundary — read it as a trend, not a byte-for-byte match to `/v1/summary`. |
| Agent / component distribution | Install-days over the trailing 30 rolled-up days, from `rollups_daily.agent`/`.component` — same install-days semantics as `/v1/summary` (see [above](#get-v1summary)), not distinct installs. |
| RDD enabled ratio | Share of install-days over the trailing 30 days reporting `rdd_enabled=true`. |
| Version distribution | Install-days per version over the trailing 30 days — a proxy for upgrade lag. |
| Events per day | Raw event volume from the `events` table directly, so — unlike every other panel here — it is bounded by `--retention-days`: older days are gone once purged, since `rollups_daily` does not keep a raw event count. |
| npm downloads per day | Daily download counts for `--npm-package` (gentle-pi, gentle-engram by default), from `rollups_daily.npm_downloads_day`. See [External download counts](#external-download-counts) — this is a public count, not telemetry. |
| gentle-ai release downloads | Cumulative GitHub release asset download counts per tag, from `rollups_daily.github_release_downloads_total` — the latest known total per tag, not a per-day delta. See [External download counts](#external-download-counts) — also a public count, not telemetry. |

**Reverse proxy**: the `:443` block in `apache/telemetry-vhost.conf.tmpl`
proxies `/grafana/` to `127.0.0.1:3000`; `install.sh --with-grafana` sets
`root_url` and `serve_from_sub_path = true` in `/etc/grafana/grafana.ini`
to match. It also turns off Grafana's background reporting and update
checks (`[analytics] reporting_enabled`/`check_for_updates`), which are
pure overhead on a small box that only needs one dashboard.

**First login**: `install.sh --with-grafana` never leaves the default
`admin`/`admin` login reachable. Before Grafana's first start, it
generates a random password (`openssl rand -base64 24`), writes it
root-only 0600 to `/etc/grafana/admin-password`, and sets
`admin_user = axiom` plus `admin_password` in `grafana.ini`, alongside
`[auth.anonymous] enabled = false` and `[users] allow_sign_up = false`.
The script prints that file's path once. Read it with
`sudo cat /etc/grafana/admin-password` and sign in at
`https://telemetry.example.com/grafana/` as `axiom`. Grafana only seeds
`admin_user`/`admin_password` into its own database on that very first
startup — on a re-run against an already-initialized Grafana, rotate the
live password instead with `grafana cli --homepath /usr/share/grafana admin reset-admin-password
<new-password>` on the VPS (and update `/etc/grafana/admin-password` to
match, so the two stay in sync). To change it via the UI later:
**Administration → Users → axiom**.

**The access log**: the `:443` block's `CustomLog` directive logs every
request through this vhost — `/v1/`, `/healthz`, and `/grafana/` alike —
using a format (`%t "%r" %>s %b`) that never includes the client address,
matching the collector's own guarantee that it never stores or logs the
caller's IP (see [No IP addresses, anywhere](#no-ip-addresses-anywhere)
and its test). Requests ARE logged; the address is simply never part of
what gets written. Since `telemetry.example.com` is its own explicit
`<VirtualHost>` rather than a cPanel-managed per-domain vhost, this
`CustomLog` directive fully replaces the main server's default access log
for this vhost's requests — Apache does not additionally write a second,
IP-bearing log entry alongside it. Grafana and the dashboards above never
read this log anyway; they only ever see the SQLite database.

## External download counts

Alongside the collector's own telemetry, the daily job also fetches two
**public** counts that have nothing to do with any gentle-ai install:

- **npm downloads** (`--npm-package`, repeatable, default `gentle-pi`,
  `gentle-engram`): `GET https://api.npmjs.org/downloads/point/last-day/<pkg>`,
  the npm registry's own public download-count API. Stored under the day
  it reports (npm's "last-day" is always the day *before* the request,
  since today's count is still incomplete).
- **GitHub release downloads** (`--github-repo`, repeatable, default
  `Gentleman-Programming/gentle-ai`; optional `--github-token-file` to
  raise the rate limit — the token is never logged): `GET
  https://api.github.com/repos/<owner>/<repo>/releases?per_page=100`,
  following the response's `Link: rel="next"` header for up to 10 pages
  (1,000 releases) so older releases are not silently dropped, and summing
  `assets[].download_count` per release across every page fetched. A
  failure on a later page (or too little of the collector's own fetch
  budget left to be worth starting another request) stops pagination but
  keeps whatever earlier pages already returned, rather than discarding
  the whole fetch. GitHub's own counts are already cumulative-since-release,
  so each day's fetch stores a snapshot of that running total under today,
  not a daily delta.

Both are stored in `rollups_daily` (`npm_downloads_day` / `key=<pkg>`,
`github_release_downloads_total` / `key=<tag>`) alongside the collector's
own metrics, purely so `/v1/summary` and the dashboard can read them from
the same store — they are never derived from, or joined against, any
install's data. A failure fetching one package or repo is logged once and
simply retried the next day; it never touches ingest or the other sources.
