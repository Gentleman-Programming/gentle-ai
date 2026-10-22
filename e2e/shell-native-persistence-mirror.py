#!/usr/bin/python3
"""Guest-only replay of held original responses; no installer or upstream."""
import hashlib
import importlib.util
import json
from pathlib import Path
import ssl
import sys
import threading
import urllib.parse

fixture, ready, report = map(Path, sys.argv[1:])

def require(condition, message):
    if not condition:
        raise RuntimeError(message)

legacy_path = fixture / 'legacy.py'
require(hashlib.sha256(legacy_path.read_bytes()).hexdigest() ==
        '09b12ad817316a3cf30d709a13c722e6048682d5191ba72d258bc6c2b96b4659', 'qualified import guard differs')
spec = importlib.util.spec_from_file_location('held_legacy', legacy_path)
legacy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(legacy)  # Qualified guarded main: import does not install.
rows = json.loads((fixture / 'requests.json').read_bytes())
require(len(rows) == 7, 'exact six acquired responses and held PNPM required')
by_url = {row['url']: row for row in rows}
require(len(by_url) == len(rows), 'duplicate response URL')
requests, refused, tls_errors = [], [], []

def record(bucket, item):
    require(len(bucket) < 32 and len(item) <= 256, 'observation bound')
    bucket.append(item)

def frozen_bytes(host, resource):
    url = 'https://' + host + resource
    row = by_url[url]
    name = row['file']
    require(name not in ('', '.', '..') and '/' not in name, 'fixture basename')
    data = (fixture / name).read_bytes()
    require(0 < len(data) <= 64*1024*1024 and len(data) == row['size'] and
            hashlib.sha256(data).hexdigest() == row['sha256'], 'retained response changed')
    record(requests, url)
    return 200, {'content-type': 'application/json' if name.endswith('.json') else 'application/octet-stream'}, data

def no_upstream(*args, **kwargs):
    raise RuntimeError('upstream forbidden')

class ClosedMirror(legacy.Mirror):
    def do_GET(self):
        host = self.headers.get('Host', '').split(':')[0].lower()
        url = 'https://' + host + self.path
        if url not in by_url:
            record(refused, url)
            self.send_error(404, 'non-inventoried request')
            return  # Refuse before public_bytes/DirectTLS/upstream_ip.
        try:
            status, headers, data = frozen_bytes(host, self.path)
            self.send_response(status)
            for name, value in headers.items():
                self.send_header(name, value)
            self.send_header('Content-Length', str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        except Exception:
            self.send_error(502, 'held response refused')

legacy.FIXTURE = fixture
legacy.ALLOW = {urllib.parse.urlsplit(url).hostname for url in by_url}
legacy.public_bytes, legacy.upstream_ip, legacy.DirectTLS = frozen_bytes, no_upstream, no_upstream
class ObservedServer(legacy.BoundedServer):
    def get_request(self):
        try:
            return super().get_request()
        except ssl.SSLError as error:
            record(tls_errors, str(error)[:256])
            raise

server = ObservedServer(('127.0.0.1', 443), ClosedMirror)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.minimum_version = ssl.TLSVersion.TLSv1_2
context.load_cert_chain('/fixture/server.crt', '/fixture/server.key')
server.socket = context.wrap_socket(server.socket, server_side=True)

def stop_from_owner():
    if sys.stdin.readline() == 'stop\n':
        server.shutdown()

with ready.open('x') as stream:
    stream.write('ready')
threading.Thread(target=stop_from_owner, daemon=True).start()
try:
    server.serve_forever()
finally:
    server.server_close()
    data = json.dumps({'requests': requests, 'refused': refused, 'tlsErrors': tls_errors, 'upstream': False}).encode()
    require(len(data) <= 32*1024, 'report byte bound')
    with report.open('xb') as stream:
        stream.write(data)
