#!/usr/bin/env python3
"""Tests for backfill-victoriametrics.py.

Run with:
    python3 -m unittest deploy/telemetry/scripts/backfill_victoriametrics_test.py

The target module's filename has a hyphen (matching every other file in
deploy/telemetry/scripts/), so it cannot be `import`-ed by name; it is
loaded dynamically by file path below, the standard way to unit-test a
hyphenated CLI script. Nothing here depends on this module's own name.
"""

import http.server
import json
import re
import sqlite3
import sys
import threading
import unittest
import urllib.error
import urllib.parse
import urllib.request
from importlib import util as importlib_util
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent.parent.parent
METRICS_GO = REPO_ROOT / "internal" / "telemetrycollector" / "metrics.go"

_spec = importlib_util.spec_from_file_location(
    "backfill_victoriametrics", SCRIPT_DIR / "backfill-victoriametrics.py"
)
backfill = importlib_util.module_from_spec(_spec)
_spec.loader.exec_module(backfill)


def make_row(
    agent_kind="sdd-apply",
    agent_class="built_in",
    provider="openai",
    model_id="gpt-5.4",
    selected_effort="minimal",
    effective_effort="minimal",
    model_evidence="declared",
    responses=1,
    launches=1,
    error_category="none",
    duration_kind="wall",
    duration_measured_count=1,
    duration_sum_ms=250,
    input_sum=100,
):
    def token(sum_value):
        return {"reported": 1, "unavailable": 0, "unsupported": 0, "sum": sum_value}

    return {
        "model": {"provider": provider, "id": model_id},
        "model_evidence": model_evidence,
        "agent_kind": agent_kind,
        "agent_class": agent_class,
        "selected_effort": selected_effort,
        "effective_effort": effective_effort,
        "launches": launches,
        "responses": responses,
        "input_tokens": token(input_sum),
        "output_tokens": token(0),
        "cache_read_tokens": token(0),
        "cache_creation_tokens": token(0),
        "reasoning_tokens": token(0),
        "total_tokens": token(input_sum),
        "error_category": error_category,
        "duration": {
            "kind": duration_kind,
            "measured_count": duration_measured_count,
            "sum_ms": duration_sum_ms,
        },
    }


def make_db(path, deliveries):
    """deliveries: list of (delivery_id, received_at_ns, host, [row, ...])."""
    conn = sqlite3.connect(path)
    conn.execute(
        "CREATE TABLE runtime_deliveries (delivery_id TEXT PRIMARY KEY, "
        "received_at INTEGER NOT NULL, canonical_payload TEXT NOT NULL)"
    )
    conn.execute(
        "CREATE TABLE runtime_rows (delivery_id TEXT NOT NULL, "
        "ordinal INTEGER NOT NULL, row_json TEXT NOT NULL, "
        "PRIMARY KEY(delivery_id, ordinal))"
    )
    for delivery_id, received_at_ns, host, rows in deliveries:
        payload = json.dumps(
            {
                "schema": "gentle-ai.telemetry-runtime-event/v1",
                "registry": 1,
                "delivery_id": delivery_id,
                "host": host,
                "rows": rows,
            }
        )
        conn.execute(
            "INSERT INTO runtime_deliveries(delivery_id, received_at, canonical_payload) "
            "VALUES (?, ?, ?)",
            (delivery_id, received_at_ns, payload),
        )
        for ordinal, row in enumerate(rows):
            conn.execute(
                "INSERT INTO runtime_rows(delivery_id, ordinal, row_json) VALUES (?, ?, ?)",
                (delivery_id, ordinal, json.dumps(row)),
            )
    conn.commit()
    conn.close()


def iso_ns(s):
    return backfill.parse_iso_utc(s)


class SanitizeLabelTests(unittest.TestCase):
    def test_empty_becomes_unknown(self):
        self.assertEqual(backfill.sanitize_label(""), "unknown")
        self.assertEqual(backfill.sanitize_label(None), "unknown")

    def test_escapes_backslash_quote_newline_in_order(self):
        self.assertEqual(backfill.sanitize_label("a\\b"), "a\\\\b")
        self.assertEqual(backfill.sanitize_label('a"b'), 'a\\"b')
        self.assertEqual(backfill.sanitize_label("a\nb"), "a\\nb")
        # A backslash introduced by escaping the quote/newline must never
        # itself be re-escaped by a later step.
        self.assertEqual(backfill.sanitize_label('"\n'), '\\"\\n')

    def test_ordinary_value_untouched(self):
        self.assertEqual(backfill.sanitize_label("claude-code"), "claude-code")


class RuntimeMetricNumberTests(unittest.TestCase):
    def test_none_is_zero(self):
        self.assertEqual(backfill.runtime_metric_number(None), 0.0)

    def test_quoted_sentinel_is_zero(self):
        self.assertEqual(backfill.runtime_metric_number("unsupported"), 0.0)

    def test_number_passthrough(self):
        self.assertEqual(backfill.runtime_metric_number(42), 42.0)
        self.assertEqual(backfill.runtime_metric_number(0), 0.0)


class LabelOrderParityWithGoTests(unittest.TestCase):
    """Reads internal/telemetrycollector/metrics.go's text and asserts the
    Python script's label tables describe the exact same names and order,
    so a future change to one side that is not mirrored on the other
    fails this test rather than silently producing divergent series.
    """

    @classmethod
    def setUpClass(cls):
        cls.go_source = METRICS_GO.read_text(encoding="utf-8")

    def test_row_base_labels_match_go_order(self):
        match = re.search(
            r"func runtimeBaseLabels\(.*?\{(.*?)\n\}", self.go_source, re.DOTALL
        )
        self.assertIsNotNone(match, "runtimeBaseLabels function not found in metrics.go")
        names = re.findall(r'\{"([a-z_]+)",', match.group(1))
        self.assertEqual(names, backfill.ROW_BASE_LABEL_NAMES)

    def test_token_kinds_match_go_order(self):
        match = re.search(
            r"var runtimeTokenFields = \[\]string\{(.*?)\}", self.go_source
        )
        self.assertIsNotNone(match, "runtimeTokenFields not found in metrics.go")
        kinds = re.findall(r'"([a-z_]+)"', match.group(1))
        self.assertEqual(kinds, backfill.TOKEN_KINDS)

    def test_token_state_order_matches_go(self):
        states = re.findall(r'withLabel\(withKind, "state", "([a-z]+)"\)', self.go_source)
        self.assertTrue(states, "no state withLabel calls found in metrics.go")
        self.assertEqual(states, backfill.TOKEN_STATE_ORDER)

    def test_rows_by_evidence_labels_match_go_order(self):
        match = re.search(
            r'gentle_runtime_rows_by_evidence_total",\s*1,(.*?)\)\n', self.go_source, re.DOTALL
        )
        self.assertIsNotNone(match, "rows_by_evidence_total call not found in metrics.go")
        names = re.findall(r'runtimeLabel\{"([a-z_]+)"', match.group(1))
        self.assertEqual(names, backfill.ROWS_BY_EVIDENCE_LABEL_NAMES)

    def test_metric_family_order_matches_go(self):
        match = re.search(
            r"var runtimeMetricNames = \[\]string\{(.*?)\}", self.go_source, re.DOTALL
        )
        self.assertIsNotNone(match, "runtimeMetricNames not found in metrics.go")
        names = re.findall(r'"(gentle_runtime_[a-z_]+)"', match.group(1))
        self.assertEqual(names, backfill.METRIC_NAMES)


class BuildLinesTests(unittest.TestCase):
    """Two deliveries a minute apart: asserts the exact exposition lines
    (names, labels, cumulative values, timestamps) for the metrics this
    test exercises.
    """

    def setUp(self):
        self.t1 = iso_ns("2026-09-10T00:00:10Z")
        self.t2 = iso_ns("2026-09-10T00:01:20Z")
        self.bucket_end_ms = iso_ns("2026-09-10T00:01:00Z") // 1_000_000
        self.final_ms = self.t2 // 1_000_000
        row1 = make_row(input_sum=100, responses=1, launches=1)
        row2 = make_row(input_sum=50, responses=2, launches=1)
        # (received_at_ns, host, rows), the shape backfill.iter_deliveries
        # yields. Built directly here for this test's tight, deterministic
        # focus; the sqlite-reading path is covered separately in
        # EndToEndImportAndVerifyTests.
        self.deliveries = [
            (self.t1, "pi", [row1]),
            (self.t2, "pi", [row2]),
        ]

    def test_deliveries_total_and_rows_total_lines(self):
        lines, totals = backfill.build_lines(self.deliveries)

        want_bucket_deliveries = (
            f'gentle_runtime_deliveries_total{{host="pi"}} 1 {self.bucket_end_ms}'
        )
        want_bucket_rows = (
            "gentle_runtime_rows_total{host=\"pi\",agent_kind=\"sdd-apply\","
            "agent_class=\"built_in\",provider=\"openai\",model=\"gpt-5.4\","
            f'selected_effort="minimal"}} 1 {self.bucket_end_ms}'
        )
        want_final_deliveries = (
            f'gentle_runtime_deliveries_total{{host="pi"}} 2 {self.final_ms}'
        )
        want_final_rows = (
            "gentle_runtime_rows_total{host=\"pi\",agent_kind=\"sdd-apply\","
            "agent_class=\"built_in\",provider=\"openai\",model=\"gpt-5.4\","
            f'selected_effort="minimal"}} 2 {self.final_ms}'
        )

        self.assertIn(want_bucket_deliveries, lines)
        self.assertIn(want_bucket_rows, lines)
        self.assertIn(want_final_deliveries, lines)
        self.assertIn(want_final_rows, lines)

        # The bucket-end sample must come before the final sample in the
        # emitted stream (import order does not matter to VictoriaMetrics,
        # but this pins the algorithm's intended emission order).
        self.assertLess(lines.index(want_bucket_deliveries), lines.index(want_final_deliveries))

        want_bucket_tokens = (
            'gentle_runtime_tokens_total{host="pi",agent_kind="sdd-apply",'
            'agent_class="built_in",provider="openai",model="gpt-5.4",'
            f'selected_effort="minimal",kind="input"}} 100 {self.bucket_end_ms}'
        )
        want_final_tokens = (
            'gentle_runtime_tokens_total{host="pi",agent_kind="sdd-apply",'
            'agent_class="built_in",provider="openai",model="gpt-5.4",'
            f'selected_effort="minimal",kind="input"}} 150 {self.final_ms}'
        )
        self.assertIn(want_bucket_tokens, lines)
        self.assertIn(want_final_tokens, lines)

        self.assertEqual(
            totals[("gentle_runtime_deliveries_total", (("host", "pi"),))], 2.0
        )

    def test_only_two_timestamps_are_emitted(self):
        lines, _ = backfill.build_lines(self.deliveries)
        timestamps = {int(line.rsplit(" ", 1)[1]) for line in lines}
        self.assertEqual(timestamps, {self.bucket_end_ms, self.final_ms})

    def test_no_deliveries_emits_nothing(self):
        lines, totals = backfill.build_lines([])
        self.assertEqual(lines, [])
        self.assertEqual(totals, {})

    def test_stream_lines_reports_stats_and_matches_build_lines(self):
        totals = {}
        stats = {"deliveries": 0, "last_sample_ms": None}
        streamed = list(backfill.stream_lines(self.deliveries, totals, stats))
        self.assertEqual(stats["deliveries"], 2)
        self.assertEqual(stats["last_sample_ms"], self.final_ms)
        self.assertEqual(streamed, backfill.build_lines(self.deliveries)[0])


class ChunkingTests(unittest.TestCase):
    def test_post_chunks_splits_by_chunk_size(self):
        lines = [f"metric{{host=\"h\"}} {i} 1000" for i in range(12)]
        seen_bodies = []

        def fake_post(url, body):
            seen_bodies.append(body.decode("utf-8"))

        backfill.post_chunks(
            "http://127.0.0.1:8428", lines, chunk_size=5, sender=fake_post
        )

        self.assertEqual(len(seen_bodies), 3)
        self.assertEqual(seen_bodies[0].count("\n"), 5)
        self.assertEqual(seen_bodies[1].count("\n"), 5)
        self.assertEqual(seen_bodies[2].count("\n"), 2)

    def test_post_chunks_accepts_a_generator(self):
        lines = [f"metric{{host=\"h\"}} {i} 1000" for i in range(12)]
        seen_bodies = []

        def fake_post(url, body):
            seen_bodies.append(body.decode("utf-8"))

        count = backfill.post_chunks(
            "http://127.0.0.1:8428", (x for x in lines), chunk_size=5, sender=fake_post
        )
        self.assertEqual(count, 12)
        self.assertEqual(len(seen_bodies), 3)
        self.assertEqual(seen_bodies[0].count("\n"), 5)
        self.assertEqual(seen_bodies[2].count("\n"), 2)

    def test_stops_on_4xx_without_further_chunks(self):
        calls = []

        def fake_post(url, body):
            calls.append(body)
            if len(calls) == 1:
                raise urllib.error.HTTPError(url, 400, "bad request", {}, None)

        with self.assertRaises(RuntimeError):
            backfill.post_chunks(
                "http://127.0.0.1:8428",
                ["a 1 1000", "b 1 1000"],
                chunk_size=1,
                sender=fake_post,
            )
        self.assertEqual(len(calls), 1)

    def test_retries_on_5xx_then_succeeds(self):
        calls = []

        def flaky_post(url, body):
            calls.append(body)
            if len(calls) < 3:
                raise urllib.error.HTTPError(url, 503, "busy", {}, None)

        backfill.post_chunks(
            "http://127.0.0.1:8428",
            ["a 1 1000"],
            chunk_size=1,
            sender=flaky_post,
            max_retries=5,
            backoff_seconds=0,
        )
        self.assertEqual(len(calls), 3)


class FakeVictoriaMetrics(http.server.BaseHTTPRequestHandler):
    imports = []
    query_sums = {}
    query_times = []

    def log_message(self, *args):
        pass

    def do_POST(self):
        if self.path == "/api/v1/import/prometheus":
            length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(length).decode("utf-8")
            FakeVictoriaMetrics.imports.append(body)
            self.send_response(204)
            self.end_headers()
            return
        self.send_response(404)
        self.end_headers()

    def do_GET(self):
        if self.path.startswith("/api/v1/query?"):
            query = urllib.parse.parse_qs(self.path.split("?", 1)[1])
            promql = query.get("query", [""])[0]
            FakeVictoriaMetrics.query_times.append(query.get("time", [""])[0])
            value = FakeVictoriaMetrics.query_sums.get(promql)
            if value is None:
                result = []
            else:
                result = [{"metric": {}, "value": [0, str(value)]}]
            body = json.dumps(
                {"status": "success", "data": {"resultType": "vector", "result": result}}
            ).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        self.send_response(404)
        self.end_headers()


class EndToEndImportAndVerifyTests(unittest.TestCase):
    """A real temp SQLite DB plus a real fake VictoriaMetrics HTTP server
    (http.server in a background thread): exercises main()'s import path
    and its --verify comparison end to end.
    """

    @classmethod
    def setUpClass(cls):
        FakeVictoriaMetrics.imports = []
        FakeVictoriaMetrics.query_sums = {}
        FakeVictoriaMetrics.query_times = []
        cls.server = http.server.HTTPServer(("127.0.0.1", 0), FakeVictoriaMetrics)
        cls.port = cls.server.server_address[1]
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.thread.join(timeout=5)
        cls.server.server_close()

    def setUp(self):
        FakeVictoriaMetrics.imports = []
        FakeVictoriaMetrics.query_sums = {}
        FakeVictoriaMetrics.query_times = []
        self.tmpdir = self._make_tmpdir()
        self.db_path = str(self.tmpdir / "events.sqlite")
        t1 = iso_ns("2026-09-10T00:00:10Z")
        self.t2 = iso_ns("2026-09-10T00:01:20.250Z")
        make_db(
            self.db_path,
            [
                ("d1", t1, "pi", [make_row(responses=1, launches=1, input_sum=10)]),
                ("d2", self.t2, "pi", [make_row(responses=2, launches=1, input_sum=20)]),
            ],
        )
        self.vm_url = f"http://127.0.0.1:{self.port}"

    def _make_tmpdir(self):
        import tempfile

        d = tempfile.mkdtemp()
        self.addCleanup(__import__("shutil").rmtree, d, True)
        return Path(d)

    def test_dry_run_prints_counts_and_makes_no_request(self):
        rc = backfill.main(["--db", self.db_path, "--dry-run"])
        self.assertEqual(rc, 0)
        self.assertEqual(FakeVictoriaMetrics.imports, [])

    def test_import_reaches_fake_server(self):
        rc = backfill.main(["--db", self.db_path, "--vm-url", self.vm_url, "--chunk-size", "1"])
        self.assertEqual(rc, 0)
        self.assertGreater(len(FakeVictoriaMetrics.imports), 0)
        combined = "\n".join(FakeVictoriaMetrics.imports)
        self.assertIn("gentle_runtime_deliveries_total", combined)
        self.assertIn('host="pi"', combined)

    def test_verify_passes_when_server_reports_matching_sums(self):
        FakeVictoriaMetrics.query_sums = {
            "sum(gentle_runtime_deliveries_total)": "2",
            "sum(gentle_runtime_rows_total)": "2",
            "sum(gentle_runtime_responses_total)": "3",
            'sum(gentle_runtime_tokens_total{kind="total"})': "30",
        }
        rc = backfill.main(["--db", self.db_path, "--vm-url", self.vm_url, "--verify"])
        self.assertEqual(rc, 0)
        want_time = f"{self.t2 // 1_000_000 / 1000:.3f}"
        self.assertTrue(FakeVictoriaMetrics.query_times)
        for got_time in FakeVictoriaMetrics.query_times:
            self.assertEqual(got_time, want_time)

    def test_verify_fails_when_server_reports_mismatching_sums(self):
        FakeVictoriaMetrics.query_sums = {
            "sum(gentle_runtime_deliveries_total)": "999",
            "sum(gentle_runtime_rows_total)": "2",
            "sum(gentle_runtime_responses_total)": "3",
            'sum(gentle_runtime_tokens_total{kind="total"})': "30",
        }
        rc = backfill.main(["--db", self.db_path, "--vm-url", self.vm_url, "--verify"])
        self.assertEqual(rc, 1)


if __name__ == "__main__":
    unittest.main()
