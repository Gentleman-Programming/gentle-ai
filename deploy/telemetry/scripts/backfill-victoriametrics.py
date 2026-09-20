#!/usr/bin/env python3
"""Backfills VictoriaMetrics from the collector's raw runtime SQLite tables.

Reads runtime_deliveries and runtime_rows from the gentle-telemetry
collector's SQLite database (opened read-only, so this can safely run
against a live database) in received_at order, and reproduces exactly the
counter series the Go registry (internal/telemetrycollector/metrics.go)
would have produced had it observed every delivery in that same order,
then pushes them into VictoriaMetrics through POST /api/v1/import/prometheus
as Prometheus text lines carrying an explicit millisecond timestamp.

Metric names, labels, label order, and label sanitization all mirror
metrics.go exactly (see ROW_BASE_LABEL_NAMES, TOKEN_KINDS,
TOKEN_STATE_ORDER, ROWS_BY_EVIDENCE_LABEL_NAMES, METRIC_NAMES below, and
sanitize_label). backfill_victoriametrics_test.py asserts this table
matches metrics.go's text directly, so a rename on either side that is
not mirrored on the other fails that test.

Counters are cumulative, exactly like the Go registry: this script keeps
a running total per (metric, label set) while iterating deliveries in
received_at order, and emits one sample per series at the end of every
one-minute bucket that had at least one delivery (the bucket's last
cumulative value, timestamped at the bucket's end), plus one final sample
at the exact timestamp of the last delivery in range. This mirrors what a
real periodic /metrics scrape would have captured — the full current
state of every counter, not just the ones a given delivery touched — at
one-minute resolution instead of the real 15s scrape interval, which
keeps the backfill's data volume bounded for a multi-day window.

Re-running this script (same --db, --since, --until) is idempotent: it
always recomputes byte-identical samples for the same historical data, and
VictoriaMetrics deduplicates identical (metric, labels, timestamp) points
on import. Deliveries and lines are streamed end to end, so memory stays
bounded by the number of live series, not by the --since/--until window.

Usage:
    python3 backfill-victoriametrics.py --db /var/lib/gentle-telemetry/events.sqlite \\
        --since 2026-09-10T00:00:00Z --dry-run
    python3 backfill-victoriametrics.py --db /var/lib/gentle-telemetry/events.sqlite \\
        --vm-url http://127.0.0.1:8428 --verify
"""

import argparse
import itertools
import json
import math
import sqlite3
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone

# --- Label/metric tables mirroring internal/telemetrycollector/metrics.go ---
# Kept here as a single shared source of truth for this script; see the
# module docstring and backfill_victoriametrics_test.py's
# LabelOrderParityWithGoTests, which reads metrics.go's text and asserts
# these match it exactly.

ROW_BASE_LABEL_NAMES = [
    "host",
    "agent_kind",
    "agent_class",
    "provider",
    "model",
    "selected_effort",
]

TOKEN_KINDS = ["input", "output", "cache_read", "cache_creation", "reasoning", "total"]

# Maps each TOKEN_KINDS entry to the RuntimeRow JSON field it reads from,
# mirroring runtimeTokenRaw in metrics.go.
TOKEN_KIND_FIELD = {
    "input": "input_tokens",
    "output": "output_tokens",
    "cache_read": "cache_read_tokens",
    "cache_creation": "cache_creation_tokens",
    "reasoning": "reasoning_tokens",
    "total": "total_tokens",
}

TOKEN_STATE_ORDER = ["reported", "unavailable", "unsupported"]

ROWS_BY_EVIDENCE_LABEL_NAMES = ["host", "model_evidence", "effective_effort"]

METRIC_NAMES = [
    "gentle_runtime_deliveries_total",
    "gentle_runtime_rows_total",
    "gentle_runtime_responses_total",
    "gentle_runtime_launches_total",
    "gentle_runtime_tokens_total",
    "gentle_runtime_token_fields_total",
    "gentle_runtime_errors_total",
    "gentle_runtime_duration_ms_sum",
    "gentle_runtime_duration_measured_total",
    "gentle_runtime_rows_by_evidence_total",
]

BUCKET_NS = 60_000_000_000  # one minute, in nanoseconds


def sanitize_label(value):
    """Mirrors sanitizeRuntimeLabel in metrics.go exactly: empty becomes
    "unknown", then backslash, double quote, and newline are escaped in
    that order so escaping one never re-escapes another.
    """
    if value is None or value == "":
        return "unknown"
    value = str(value)
    value = value.replace("\\", "\\\\")
    value = value.replace('"', '\\"')
    value = value.replace("\n", "\\n")
    return value


def runtime_metric_number(raw):
    """Mirrors runtimeMetricNumber in metrics.go: a missing/null value or a
    quoted sentinel string contributes 0, matching "a row that could not
    observe a value still counts as one row, just with nothing to add."
    """
    if raw is None or isinstance(raw, str):
        return 0.0
    return float(raw)


def parse_iso_utc(value):
    """Parses a UTC ISO-8601 timestamp (e.g. "2026-09-10T00:00:00Z" or
    "2026-09-10") into Unix nanoseconds, matching receivedAtKey's encoding
    in internal/telemetrycollector/storage.go (t.UTC().UnixNano()).
    """
    text = value.strip()
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    dt = datetime.fromisoformat(text)
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    dt = dt.astimezone(timezone.utc)
    return int(dt.timestamp() * 1_000_000_000)


def _row_base_labels(host, row):
    model = row.get("model") or {}
    return [
        ("host", host),
        ("agent_kind", sanitize_label(row.get("agent_kind"))),
        ("agent_class", sanitize_label(row.get("agent_class"))),
        ("provider", sanitize_label(model.get("provider"))),
        ("model", sanitize_label(model.get("id"))),
        ("selected_effort", sanitize_label(row.get("selected_effort"))),
    ]


def _bump(totals, metric, labels, delta):
    key = (metric, tuple(labels))
    totals[key] = totals.get(key, 0.0) + delta


def apply_delivery(totals, host_raw, rows):
    """Applies one delivery's contribution to the running totals, exactly
    mirroring RuntimeMetrics.Observe in metrics.go: one increment of
    gentle_runtime_deliveries_total, plus every row-scoped series for each
    row.
    """
    host = sanitize_label(host_raw)
    _bump(totals, "gentle_runtime_deliveries_total", [("host", host)], 1.0)

    for row in rows:
        base = _row_base_labels(host, row)

        _bump(totals, "gentle_runtime_rows_total", base, 1.0)
        _bump(
            totals,
            "gentle_runtime_responses_total",
            base,
            runtime_metric_number(row.get("responses")),
        )
        _bump(
            totals,
            "gentle_runtime_launches_total",
            base,
            runtime_metric_number(row.get("launches")),
        )

        for kind in TOKEN_KINDS:
            token = row.get(TOKEN_KIND_FIELD[kind]) or {}
            with_kind = base + [("kind", kind)]
            _bump(
                totals,
                "gentle_runtime_tokens_total",
                with_kind,
                runtime_metric_number(token.get("sum")),
            )
            for state in TOKEN_STATE_ORDER:
                _bump(
                    totals,
                    "gentle_runtime_token_fields_total",
                    with_kind + [("state", state)],
                    runtime_metric_number(token.get(state)),
                )

        category = row.get("error_category") or ""
        if category and category != "none":
            _bump(
                totals,
                "gentle_runtime_errors_total",
                base + [("category", sanitize_label(category))],
                1.0,
            )

        duration = row.get("duration") or {}
        with_duration_kind = base + [
            ("duration_kind", sanitize_label(duration.get("kind")))
        ]
        _bump(
            totals,
            "gentle_runtime_duration_ms_sum",
            with_duration_kind,
            runtime_metric_number(duration.get("sum_ms")),
        )
        _bump(
            totals,
            "gentle_runtime_duration_measured_total",
            with_duration_kind,
            runtime_metric_number(duration.get("measured_count")),
        )

        _bump(
            totals,
            "gentle_runtime_rows_by_evidence_total",
            [
                ("host", host),
                ("model_evidence", sanitize_label(row.get("model_evidence"))),
                ("effective_effort", sanitize_label(row.get("effective_effort"))),
            ],
            1.0,
        )


def _format_value(value):
    if float(value).is_integer():
        return str(int(value))
    return repr(float(value))


def _format_line(metric, labels, value, timestamp_ms):
    label_text = ",".join(f'{name}="{val}"' for name, val in labels)
    return f"{metric}{{{label_text}}} {_format_value(value)} {timestamp_ms}"


def _emit_all(totals, timestamp_ns):
    timestamp_ms = timestamp_ns // 1_000_000
    for (metric, labels), value in totals.items():
        yield _format_line(metric, labels, value, timestamp_ms)


def stream_lines(deliveries, totals, stats):
    """Streams exposition lines for (received_at_ns, host, rows) deliveries in
    received_at order, mutating totals and stats in place."""
    current_bucket = None
    last_received_at_ns = None

    for received_at_ns, host, rows in deliveries:
        bucket = (received_at_ns // BUCKET_NS) * BUCKET_NS
        if current_bucket is None:
            current_bucket = bucket
        elif bucket > current_bucket:
            yield from _emit_all(totals, current_bucket + BUCKET_NS)
            current_bucket = bucket
        apply_delivery(totals, host, rows)
        stats["deliveries"] += 1
        last_received_at_ns = received_at_ns

    if last_received_at_ns is not None:
        stats["last_sample_ms"] = last_received_at_ns // 1_000_000
        yield from _emit_all(totals, last_received_at_ns)


def build_lines(deliveries):
    """List-returning wrapper over stream_lines: (lines, totals)."""
    totals = {}
    stats = {"deliveries": 0, "last_sample_ms": None}
    return list(stream_lines(deliveries, totals, stats)), totals


def iter_deliveries(conn, since_ns, until_ns):
    """Yields (received_at_ns, host, rows) from runtime_deliveries joined
    with runtime_rows, in received_at order (delivery_id as a stable
    tie-breaker). host comes from canonical_payload, the only place it is
    stored (RuntimeRow itself carries no host field); rows come from
    runtime_rows so this reads exactly the per-row canonical JSON the Go
    registry's Observe would have received. The outer cursor projects only
    delivery_id/received_at (there is no received_at index) and a second
    cursor runs one join query per delivery, so nothing is held in memory.
    """
    clauses = []
    params = []
    if since_ns is not None:
        clauses.append("received_at >= ?")
        params.append(since_ns)
    if until_ns is not None:
        clauses.append("received_at <= ?")
        params.append(until_ns)
    where = f" WHERE {' AND '.join(clauses)}" if clauses else ""
    outer_query = (
        "SELECT delivery_id, received_at FROM runtime_deliveries"
        f"{where} ORDER BY received_at ASC, delivery_id ASC"
    )
    outer_cursor = conn.cursor()
    row_cursor = conn.cursor()
    for delivery_id, received_at_ns in outer_cursor.execute(outer_query, params):
        rows = []
        payload_json = "{}"
        for payload_json, row_json in row_cursor.execute(
            "SELECT d.canonical_payload, r.row_json FROM runtime_deliveries d "
            "LEFT JOIN runtime_rows r ON r.delivery_id = d.delivery_id "
            "WHERE d.delivery_id = ? ORDER BY r.ordinal",
            (delivery_id,),
        ):
            if row_json is not None:
                rows.append(json.loads(row_json))
        yield received_at_ns, json.loads(payload_json).get("host", ""), rows


def _default_sender(url, body):
    request = urllib.request.Request(
        url, data=body, method="POST", headers={"Content-Type": "text/plain"}
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            response.read()
    except urllib.error.HTTPError as exc:
        # HTTPError is itself the (still-open) error response body; drain
        # and close it before re-raising so post_chunks's retry loop never
        # leaks a connection on a 4xx/5xx.
        exc.read()
        exc.close()
        raise


def post_chunks(
    vm_url,
    lines,
    chunk_size=5000,
    sender=None,
    max_retries=5,
    backoff_seconds=1.0,
):
    """POSTs lines to <vm_url>/api/v1/import/prometheus in chunks of
    chunk_size lines. Retries a chunk with exponential backoff on a 5xx or
    a network error, up to max_retries attempts; stops immediately (no
    retry, no further chunks) on a 4xx, since that means the request
    itself is malformed and retrying would just repeat the same rejection.
    `sender(url, body_bytes)` is injectable for tests; it must raise
    urllib.error.HTTPError on a non-2xx response. lines may be any
    iterable; returns the number of lines posted.
    """
    send = sender or _default_sender
    url = vm_url.rstrip("/") + "/api/v1/import/prometheus"
    source = iter(lines)
    posted = 0
    while True:
        chunk = list(itertools.islice(source, chunk_size))
        if not chunk:
            break
        posted += len(chunk)
        body = ("\n".join(chunk) + "\n").encode("utf-8")
        delay = backoff_seconds
        attempt = 0
        while True:
            try:
                send(url, body)
                break
            except urllib.error.HTTPError as exc:
                if 400 <= exc.code < 500:
                    raise RuntimeError(
                        f"VictoriaMetrics rejected the import (HTTP {exc.code}); stopping"
                    ) from exc
                attempt += 1
                if attempt > max_retries:
                    raise RuntimeError(
                        f"import failed after {max_retries} retries: HTTP {exc.code}"
                    ) from exc
                if delay:
                    time.sleep(delay)
                delay *= 2
            except urllib.error.URLError as exc:
                attempt += 1
                if attempt > max_retries:
                    raise RuntimeError(
                        f"import failed after {max_retries} retries: {exc}"
                    ) from exc
                if delay:
                    time.sleep(delay)
                delay *= 2

    return posted


VERIFY_SPECS = [
    (
        "deliveries_total",
        "sum(gentle_runtime_deliveries_total)",
        lambda totals: sum(
            v for (m, _), v in totals.items() if m == "gentle_runtime_deliveries_total"
        ),
    ),
    (
        "rows_total",
        "sum(gentle_runtime_rows_total)",
        lambda totals: sum(
            v for (m, _), v in totals.items() if m == "gentle_runtime_rows_total"
        ),
    ),
    (
        "responses_total",
        "sum(gentle_runtime_responses_total)",
        lambda totals: sum(
            v for (m, _), v in totals.items() if m == "gentle_runtime_responses_total"
        ),
    ),
    (
        "tokens_total{kind=total}",
        'sum(gentle_runtime_tokens_total{kind="total"})',
        lambda totals: sum(
            v
            for (m, labels), v in totals.items()
            if m == "gentle_runtime_tokens_total" and dict(labels).get("kind") == "total"
        ),
    ),
]


def vm_query(vm_url, promql, at_ms):
    url = vm_url.rstrip("/") + "/api/v1/query?" + urllib.parse.urlencode(
        {"query": promql, "time": f"{at_ms / 1000:.3f}"}
    )
    with urllib.request.urlopen(url, timeout=30) as response:
        data = json.loads(response.read())
    result = data.get("data", {}).get("result", [])
    if not result:
        return 0.0
    return float(result[0]["value"][1])


def run_verify(vm_url, totals, at_ms, attempts=5):
    """Compares sum(metric) from VictoriaMetrics at at_ms (Unix ms) against
    this run's own totals for VERIFY_SPECS. Freshly imported samples become
    searchable only once VictoriaMetrics flushes its ingest buffer, so each
    attempt forces a flush and a mismatch is retried one second later.
    """
    for attempt in range(attempts):
        try:
            urllib.request.urlopen(vm_url.rstrip("/") + "/internal/force_flush", timeout=30).close()
        except urllib.error.URLError:
            pass
        rows = [(name, fn(totals), vm_query(vm_url, promql, at_ms)) for name, promql, fn in VERIFY_SPECS]
        if all(math.isclose(e, a, rel_tol=1e-9, abs_tol=1e-6) for _, e, a in rows):
            break
        if attempt < attempts - 1:
            time.sleep(1)
    all_ok = True
    print(f"{'metric':28}{'expected':>16}{'actual':>16}  status")
    for name, expected, actual in rows:
        ok = math.isclose(expected, actual, rel_tol=1e-9, abs_tol=1e-6)
        all_ok = all_ok and ok
        print(f"{name:28}{expected:16.4f}{actual:16.4f}  {'PASS' if ok else 'FAIL'}")
    return all_ok


def parse_args(argv):
    parser = argparse.ArgumentParser(
        description="Backfill VictoriaMetrics from the collector's raw runtime SQLite tables."
    )
    parser.add_argument("--db", required=True, help="Path to the collector's SQLite database.")
    parser.add_argument(
        "--vm-url", default="http://127.0.0.1:8428", help="VictoriaMetrics base URL."
    )
    parser.add_argument("--since", help="UTC ISO-8601 lower bound on received_at (inclusive).")
    parser.add_argument("--until", help="UTC ISO-8601 upper bound on received_at (inclusive).")
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print line counts and the first lines; make no network request.",
    )
    parser.add_argument(
        "--verify",
        action="store_true",
        help="After import, compare sum(metric) at --until against this run's own totals.",
    )
    parser.add_argument(
        "--chunk-size",
        type=int,
        default=5000,
        help="Exposition lines per import POST (default: 5000).",
    )
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(sys.argv[1:] if argv is None else argv)

    since_ns = parse_iso_utc(args.since) if args.since else None
    until_ns = parse_iso_utc(args.until) if args.until else None

    conn = sqlite3.connect(f"file:{args.db}?mode=ro", uri=True)
    try:
        totals = {}
        stats = {"deliveries": 0, "last_sample_ms": None}
        lines = stream_lines(iter_deliveries(conn, since_ns, until_ns), totals, stats)
        if args.dry_run:
            count = 0
            for count, line in enumerate(lines, 1):
                if count <= 10:
                    print(line)
            print(f"{count} lines across {stats['deliveries']} deliveries")
            return 0
        count = post_chunks(args.vm_url, lines, chunk_size=args.chunk_size)
        if count == 0:
            print("no deliveries in the given range; nothing to import")
        else:
            print(f"imported {count} lines across {stats['deliveries']} deliveries")
        if args.verify:
            at_ms = stats["last_sample_ms"]
            if at_ms is None:
                at_ms = int(time.time() * 1000)
            if not run_verify(args.vm_url, totals, at_ms):
                return 1
    finally:
        conn.close()

    return 0


if __name__ == "__main__":
    sys.exit(main())
