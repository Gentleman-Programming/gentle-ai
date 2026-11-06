import base64
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import tarfile
import tempfile

PINS = [
    ('/frozen-codegraph-main.tgz', '@colbymchenry/codegraph', 'sha512-xZayboJt6T3QBzBsBsI0eJ3J/tN4vYs6LMuvrxJVdM71yQFjrFKFeV3KsbicAgkj6AHnTXxUNk26g9BYwkNMgg=='),
    ('/frozen-codegraph-linux.tgz', '@colbymchenry/codegraph-linux-x64', 'sha512-bjfa+0IyjbxpZz7ghznN4qMPoYVSiZafeZu/rXGdg3VI+kEPOPZrk2vmtWIU9tBPGVJbeU2Kwa/0Z2DkUGOJ9A=='),
]


def sri(path):
    digest = hashlib.sha512()
    with open(path, 'rb') as stream:
        while chunk := stream.read(1024 * 1024):
            digest.update(chunk)
    return 'sha512-' + base64.b64encode(digest.digest()).decode()


def inventory(root):
    result = {}
    for path in sorted(root.rglob('*')):
        assert not path.is_symlink()
        st = path.stat()
        result[str(path.relative_to(root))] = (
            st.st_mode, st.st_ino, st.st_size if path.is_file() else None,
            st.st_mtime_ns,
            hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else None,
        )
    return result


with open('/opt/node/bin/node', 'rb') as stream:
    assert hashlib.file_digest(stream, 'sha256').hexdigest() == '41a74efb34cbde5c7632cdac0cf8bd1a14d0b8d73dc1e82755014d9a9ce70f5c', 'refuse an unauthenticated caller runtime'
version = subprocess.run(['/opt/node/bin/node', '--version'], check=True, capture_output=True, text=True, timeout=5)
assert version.stdout.strip() == 'v24.18.0'
print('FROZEN_CALLER_NODE_SHA_VERSION_PASS 24.18.0', flush=True)

with tempfile.TemporaryDirectory(prefix='codegraph-frozen-probe-') as temporary:
    root = Path(temporary)
    home, project, modules = root / 'home', root / 'project', root / 'node_modules'
    for path in (home, project, modules):
        path.mkdir(mode=0o700)
    subprocess.run(['/usr/bin/git', 'init', '-q', str(project)], check=True,
                   env={'HOME': str(home), 'PATH': '/usr/bin:/bin'})
    for archive, name, pin in PINS:
        assert Path(archive).stat().st_size <= 128 * 1024 * 1024
        assert sri(archive) == pin, 'refuse unauthenticated package execution'
        target = modules / name
        target.mkdir(mode=0o700, parents=True)
        with tarfile.open(archive, 'r:gz') as tar:
            total = 0
            for member in tar:
                path = PurePosixPath(member.name)
                assert path.parts[0] == 'package' and '..' not in path.parts
                assert member.isdir() or member.isfile(), 'refuse archive links/special objects'
                total += member.size
                assert total <= 300 * 1024 * 1024
                # Explicit recipe characterization: authenticate the complete archive,
                # but do not materialize its oversized alternate Node runtime.
                if name.endswith('-linux-x64') and member.name == 'package/node':
                    assert member.isfile() and member.size == 123438592
                    print('VENDORED_NODE_EXCLUDED bytes=123438592', flush=True)
                    continue
                assert member.size <= 32 * 1024 * 1024
                output = target.joinpath(*path.parts[1:])
                if member.isdir():
                    output.mkdir(mode=0o700, parents=True, exist_ok=True)
                else:
                    output.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
                    with tar.extractfile(member) as stream, output.open('xb') as dest:
                        while chunk := stream.read(1024 * 1024):
                            dest.write(chunk)
                    output.chmod(0o700 if member.mode & 0o111 else 0o600)
        metadata = json.loads((target / 'package.json').read_text())
        assert metadata['name'] == name and metadata['version'] == '1.4.1'
        print('FROZEN_CODEGRAPH_SRI_PASS', name, flush=True)
    before = inventory(root)
    env = {'HOME': str(home), 'PATH': '/opt/node/bin:/usr/bin:/bin',
           'CODEGRAPH_NO_DOWNLOAD': '1', 'CODEGRAPH_INSTALL_DIR': str(home / '.codegraph'),
           'CODEGRAPH_TELEMETRY': '0', 'DO_NOT_TRACK': '1'}
    cli = modules / '@colbymchenry/codegraph-linux-x64/lib/dist/bin/codegraph.js'
    assert not (modules / '@colbymchenry/codegraph-linux-x64/node').exists()
    commands = [
        ['/opt/node/bin/node', '--liftoff-only', str(cli), '--version'],
        ['/opt/node/bin/node', '--liftoff-only', str(cli), 'status', str(project), '--json'],

    ]
    for index, command in enumerate(commands):
        run = subprocess.run(command, cwd=project, env=env, capture_output=True,
                             text=True, timeout=20)
        print(json.dumps({'command': command, 'exit': run.returncode,
                          'stdout': run.stdout, 'stderr': run.stderr}), flush=True)
        assert run.returncode == 0
        if index == 0:
            assert run.stdout.strip() == '1.4.1'
        elif index == 1:
            status = json.loads(run.stdout)
            assert status['initialized'] is False and status['version'] == '1.4.1'
            assert status['projectPath'] == str(project) and status['lastIndexed'] is None
    assert not (project / '.codegraph').exists(), 'no automatic indexing'
    assert inventory(root) == before, 'CLI health readback changed the owned tree'
    print('FROZEN_CODEGRAPH_SHELL_NODE_PASS no_index=true tree_unchanged=true vendored_node=false materialized_file_bound=32MiB', flush=True)
