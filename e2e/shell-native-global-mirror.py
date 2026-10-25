#!/usr/bin/python3
"""Guest-only authenticated response replay; no registry synthesis or upstream."""
import hashlib
import http.server
import json
import os
from pathlib import Path
import ssl
import socket
import stat
import sys
import threading

if len(sys.argv) not in (4, 5):
    raise ValueError('expected global fixture, ready, report and optional shared fixture')
fixture, ready, report = map(Path, sys.argv[1:4])
secondary = Path(sys.argv[4]) if len(sys.argv) == 5 else None
lock = threading.Lock()
samples = {'requests': [], 'refused': [], 'errors': []}
counts = {name: 0 for name in samples}
served = set()
truncated = False
active = set()
stopping = threading.Event()
interrupted = 0


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, 'duplicate JSON key')
        result[key] = value
    return result


def canonical(url):
    # Registry scoped names use either case of percent-encoded slash.
    return url.replace('%2F', '/').replace('%2f', '/')


def record(bucket, item):
    global truncated
    with lock:
        counts[bucket] += 1
        safe = ''.join(c if ' ' <= c <= '~' and c not in '\\"' else '?' for c in item)
        truncated |= safe != item or len(safe) > 256
        if len(samples[bucket]) < 32:
            samples[bucket].append(safe[:256])


index = fixture / 'requests.json'
require(index.stat().st_size <= 256 * 1024, 'inventory byte bound')
rows = json.loads(index.read_bytes(), object_pairs_hook=unique)
require(7 < len(rows) <= 512, 'global inventory count bound')
by_url = {canonical(row['url']): row for row in rows}
require(len(by_url) == len(rows), 'duplicate response URL')
body_roots = {}
if secondary is not None:
    try:
        require(secondary.is_absolute() and secondary.resolve() == secondary
                and secondary.is_dir(), 'shared inventory physical')
        fd = os.open(secondary / 'requests.json', os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, 'rb') as stream:
            info = os.fstat(stream.fileno())
            require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1
                    and 0 < info.st_size <= 256 * 1024, 'shared inventory physical')
            data = stream.read(info.st_size + 1)
            require(len(data) == info.st_size, 'shared inventory physical')
        shared = json.loads(data, object_pairs_hook=unique)
    except OSError as error:
        raise RuntimeError('shared inventory physical') from error
    require(len(shared) == 7, 'shared inventory count')
    for row in shared:
        key = canonical(row['url'])
        require(key in by_url and key not in body_roots
                and all(row[field] == by_url[key][field] for field in ('file', 'size', 'sha256')),
                'shared inventory conflict')
        body_roots[key] = secondary


class ClosedMirror(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        if stopping.is_set():
            return
        host = self.headers.get('Host', '').split(':')[0].lower()
        url = canonical('https://' + host + self.path)
        if url not in by_url:
            record('refused', url)
            self.send_error(404, 'non-inventoried request')
            return
        row = by_url[url]
        try:
            name = row['file']
            require(name not in ('', '.', '..') and '/' not in name and '\\' not in name, 'fixture basename')
            fd = os.open(body_roots.get(url, fixture) / name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
            with os.fdopen(fd, 'rb') as stream:
                before = os.fstat(stream.fileno())
                require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1
                        and 0 < before.st_size == row['size'] <= 64 * 1024 * 1024, 'response physical/size bound')
                body = stream.read(before.st_size + 1)
                require(len(body) == row['size'] and hashlib.sha256(body).hexdigest() == row['sha256'], 'response digest')
            with lock:
                served.add(url)
            record('requests', url)
            self.send_response(200)
            self.send_header('Content-Type', 'application/json' if name.endswith('.json') else 'application/octet-stream')
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        except Exception as error:
            record('errors', type(error).__name__ + ':' + url)
            self.send_error(502, 'held response refused')


class BoundedServer(http.server.ThreadingHTTPServer):
    slots = threading.BoundedSemaphore(4)
    daemon_threads = False

    def process_request(self, request, address):
        if not self.slots.acquire(timeout=3):
            record('errors', 'worker ceiling')
            self.shutdown_request(request)
            return
        request.settimeout(3)
        try:
            super().process_request(request, address)
        except Exception:
            self.slots.release()
            raise

    def process_request_thread(self, request, address):
        try:
            super().process_request_thread(request, address)
        finally:
            self.slots.release()

    def get_request(self):
        connection, address = super().get_request()
        connection.settimeout(3)
        connection = context.wrap_socket(connection, server_side=True, do_handshake_on_connect=False)
        with lock:
            if stopping.is_set():
                connection.close()
                raise OSError('owner stopped')
            active.add(connection)
        try:
            connection.do_handshake()
            return connection, address
        except OSError:
            self.shutdown_request(connection)
            if not stopping.is_set():
                record('errors', 'TLS handshake')
            raise

    def shutdown_request(self, request):
        try:
            super().shutdown_request(request)
        finally:
            with lock:
                active.discard(request)


server = BoundedServer(('127.0.0.1', 443), ClosedMirror)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.minimum_version = ssl.TLSVersion.TLSv1_2
context.load_cert_chain('/fixture/server.crt', '/fixture/server.key')


def stop_from_owner():
    global interrupted
    if sys.stdin.readline() == 'stop\n':
        stopping.set()
        with lock:
            connections = list(active)
            interrupted = len(connections)
        for connection in connections:
            try:
                connection.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
        server.shutdown()


with ready.open('x') as stream:
    stream.write('ready')
threading.Thread(target=stop_from_owner, daemon=True).start()
try:
    server.serve_forever()
finally:
    server.server_close()
    result = {'samples': samples, 'counts': counts, 'servedUnique': len(served), 'upstream': False, 'interrupted': interrupted,
              'samplesComplete': not truncated and all(counts[k] == len(samples[k]) for k in counts)}
    data = json.dumps(result).encode()
    require(len(data) <= 32 * 1024, 'report byte bound')
    with report.open('xb') as stream:
        stream.write(data)
