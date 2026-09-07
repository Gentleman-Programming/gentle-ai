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

## HTTP API

| Endpoint | Method | Auth | Notes |
|---|---|---|---|
| `/v1/events` | POST | none | Body ≤ 4 KiB, strict schema validation, per-IP rate limit. `202` on accept, `400` invalid, `413` oversize, `429` rate-limited. |
| `/v1/summary` | GET | `Authorization: Bearer <token>` | Returns the JSON described below. `401` without a valid token. |
| `/healthz` | GET | none | Liveness check for the reverse proxy / process supervisor. |

### No IP addresses, anywhere

- The per-IP rate limiter keys an in-memory token bucket by remote address,
  and never writes that address to disk, to a log line, or anywhere else.
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
journal_mode=DELETE` and a single connection (`SetMaxOpenConns(1)`): this
collector's expected throughput does not justify concurrent SQLite
writers, and a single connection avoids `SQLITE_BUSY` entirely rather than
tuning around it. DELETE, not WAL, because WAL's main benefit — readers
and a writer proceeding concurrently — is moot when every read and write
of the collector's own is already serialized onto one connection; skipping
it also means no `-wal`/`-shm` sidecar files ever exist, which keeps the
read-only Grafana deployment (below) down to one file instead of three.

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

An in-process daily job (`telemetrycollector.RunMaintenance`, driven by a
24-hour ticker in `cmd/gentle-telemetry/main.go`) runs once at startup and
once a day thereafter:

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

`POST /v1/events` is limited per remote address by an in-memory token
bucket (`--rate-limit-per-minute`, default 60): burst capacity equals the
configured limit, refilled continuously at `limit/60` tokens per second.
Buckets are swept every 10 minutes and evicted after 30 minutes of
inactivity, so the limiter's memory does not grow unbounded across many
distinct callers. None of this state is ever persisted.

## Running it

```
--listen 127.0.0.1:18181                                 # bind address
--db /var/lib/gentle-telemetry/events.sqlite              # SQLite file
--summary-token-file <path>                               # bearer token for /v1/summary (local runs; systemd uses LoadCredential, see Token rotation)
--retention-days 90                                        # raw event retention
--rate-limit-per-minute 60                                 # per-IP budget on /v1/events
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
| `gentle-telemetry.service` | systemd unit: `DynamicUser=yes`, `StateDirectory=gentle-telemetry`, and a hardened sandbox (no new privileges, restricted syscalls/namespaces/capabilities, private `/tmp` and devices). |
| `gentle-telemetry-backup` + `.service` + `.timer` | Nightly `sqlite3 .backup` snapshot uploaded via `rclone copy` to a configurable remote, then deleted locally. The logic lives in the standalone `gentle-telemetry-backup` script (installed to `/usr/local/bin`), not inline in the unit's `ExecStart` — systemd expands `$VAR`/`${VAR}` there using its own environment before the shell runs, which would mangle a script's local variables. The unit runs as root (not `DynamicUser`) because it needs to read the collector's `DynamicUser`-owned state directory, which a second, independently allocated dynamic user could not. |
| `grafana/` | Datasource and dashboard provisioning for an optional on-box Grafana; see [Grafana dashboards](#grafana-dashboards). |
| `install.sh` | Installs the binary, `sqlite3` and `rclone` (via `dnf`), the systemd units, and a generated summary token; with `--domain`, renders the vhost template to `/root/telemetry-vhost.conf.rendered`; with `--with-grafana`, installs Grafana OSS from its official rpm repo. It never edits `post_virtualhost_global.conf`, runs `apachectl configtest`, or reloads `httpd` — those, plus DNS and the certificate, are printed at the end as operator steps, in the order they must run. |

```
sudo ./deploy/telemetry/install.sh --local-source /path/to/gentle-ai/checkout \
  --domain telemetry.example.com --with-grafana
```

(Omit `--domain` to install just the service and have the script print
where to render `apache/telemetry-vhost.conf.tmpl` yourself; omit
`--with-grafana` to skip Grafana entirely.)

### Going live: DNS, the vhost blocks, and the certificate

`install.sh` prints these steps; it does not run them. The order matters:
the `:80` block must be live before Certbot's webroot check can pass, and
the `:443` block must **not** be appended until the certificate it
references actually exists, or `apachectl configtest` fails.

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
   `systemctl enable --now gentle-telemetry-backup.timer`.

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
0600) if missing; `LoadCredential` in the unit hands it to the DynamicUser
service without loosening ownership. To rotate, edit the file and restart:

```
sudo install -m 0600 <(openssl rand -hex 32) /etc/gentle-telemetry/summary.token
sudo systemctl restart gentle-telemetry.service
```

The old token stops working the moment the service restarts and picks up
the new file; there is no overlap window, so coordinate with whatever reads
`/v1/summary`.

### Retention

Raw events older than `--retention-days` (default 90) are purged by the
daily job; `rollups_daily` — and therefore the historical monthly/weekly
counts in `/v1/summary` — is retained indefinitely. To change the retention
window, edit the `--retention-days` flag in
`/etc/systemd/system/gentle-telemetry.service` and
`systemctl daemon-reload && systemctl restart gentle-telemetry.service`.

The daily job catches up: if the process was down for a while, the next
run rolls up every UTC day between the last one it committed (or the
oldest raw event, on a fresh database) and yesterday, not just yesterday.
Each day's rollup is one all-or-nothing transaction, and a shutdown only
ever stops the catch-up loop *between* days — the in-progress day always
either finishes and commits, or never starts — so a restart mid-catch-up
never leaves a half-written day behind.

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

## Grafana dashboards

`--with-grafana` installs Grafana OSS (free, self-hosted) on the same VPS
with a read-only view over the collector's own SQLite database — no second
copy of the data, no separate store to keep in sync. It is entirely
optional; `/v1/summary` above already answers the headline questions.

**Datasource**: `deploy/telemetry/grafana/provisioning/datasources/telemetry.yaml`
uses the [`frser-sqlite-datasource`](https://grafana.com/grafana/plugins/frser-sqlite-datasource/)
plugin pointed read-only at `/var/lib/gentle-telemetry/events.sqlite`.
Grafana's own process needs read access to that one file; since
`gentle-telemetry.service` runs under systemd's `DynamicUser` (a group
allocated per-unit, with no stable name to add `grafana` to),
`install.sh --with-grafana` grants access via a POSIX ACL
(`setfacl -m u:grafana:r ...`) instead of group membership. This stays a
single file to grant because the collector opens SQLite with
`journal_mode=DELETE`, not WAL (see [Storage](#storage)) — no `-wal`/`-shm`
sidecar files are ever created for a second ACL entry to chase.

**Dashboard**: `deploy/telemetry/grafana/dashboards/gentle-ai-usage.json`,
provisioned via `deploy/telemetry/grafana/provisioning/dashboards/telemetry.yaml`
into the "Gentle AI" folder. Its panels:

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
`admin_user = gentle` plus `admin_password` in `grafana.ini`, alongside
`[auth.anonymous] enabled = false` and `[users] allow_sign_up = false`.
The script prints that file's path once. Read it with
`sudo cat /etc/grafana/admin-password` and sign in at
`https://telemetry.example.com/grafana/` as `gentle`. Grafana only seeds
`admin_user`/`admin_password` into its own database on that very first
startup — on a re-run against an already-initialized Grafana, rotate the
live password instead with `grafana cli --homepath /usr/share/grafana admin reset-admin-password
<new-password>` on the VPS (and update `/etc/grafana/admin-password` to
match, so the two stay in sync). To change it via the UI later:
**Administration → Users → gentle**.

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
  https://api.github.com/repos/<owner>/<repo>/releases?per_page=20`,
  summing `assets[].download_count` per release. GitHub's own counts are
  already cumulative-since-release, so each day's fetch stores a snapshot
  of that running total under today, not a daily delta.

Both are stored in `rollups_daily` (`npm_downloads_day` / `key=<pkg>`,
`github_release_downloads_total` / `key=<tag>`) alongside the collector's
own metrics, purely so `/v1/summary` and the dashboard can read them from
the same store — they are never derived from, or joined against, any
install's data. A failure fetching one package or repo is logged once and
simply retried the next day; it never touches ingest or the other sources.
