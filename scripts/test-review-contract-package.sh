#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
dist_root="${1:-}"

python3 - "$repo_root" "$dist_root" <<'PY'
import hashlib
import json
import pathlib
import sys
import tarfile
import zipfile

repo = pathlib.Path(sys.argv[1]).resolve()
dist_arg = sys.argv[2]
contract_root = pathlib.PurePosixPath("contracts/review-integration/v1")
expected_contract = [
	contract_root / "fixtures/capabilities-v1.1.fixture.json",
    contract_root / "fixtures/capabilities.fixture.json",
    contract_root / "fixtures/failure.fixture.json",
    contract_root / "fixtures/operation.fixture.json",
    contract_root / "fixtures/start.fixture.json",
    contract_root / "fixtures/status-ambiguous.fixture.json",
    contract_root / "fixtures/status-corrupted.fixture.json",
    contract_root / "fixtures/status-recover.fixture.json",
    contract_root / "fixtures/status-unrelated.fixture.json",
    contract_root / "fixtures/status.fixture.json",
	contract_root / "schemas/capabilities-v1.1.schema.json",
    contract_root / "schemas/capabilities.schema.json",
    contract_root / "schemas/failure.schema.json",
    contract_root / "schemas/operation.schema.json",
    contract_root / "schemas/projection.schema.json",
    contract_root / "schemas/result-artifact.schema.json",
    contract_root / "schemas/start.schema.json",
    contract_root / "schemas/status.schema.json",
]
expected_names = sorted(path.as_posix() for path in expected_contract)

source_root = repo / pathlib.Path(contract_root.as_posix())
source_names = sorted(
    path.relative_to(repo).as_posix()
    for path in source_root.rglob("*")
    if path.is_file()
)
assert source_names == expected_names, (
    f"review contract inventory mismatch\ngot={source_names}\nwant={expected_names}"
)
source_hashes = {
    name: hashlib.sha256((repo / name).read_bytes()).hexdigest()
    for name in expected_names
}

if not dist_arg:
    print("review contract source inventory: PASS")
    raise SystemExit(0)

dist = pathlib.Path(dist_arg)
if not dist.is_absolute():
    dist = (repo / dist).resolve()
artifacts_path = dist / "artifacts.json"
artifacts = json.loads(artifacts_path.read_text(encoding="utf-8"))
archives = [artifact for artifact in artifacts if artifact.get("type") == "Archive"]
expected_platforms = {
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("windows", "amd64"),
    ("windows", "arm64"),
}
platforms = {(item.get("goos"), item.get("goarch")) for item in archives}
assert platforms == expected_platforms and len(archives) == len(expected_platforms), (
    f"release archive matrix mismatch: {sorted(platforms)}"
)

checksums = {}
for line in (dist / "checksums.txt").read_text(encoding="utf-8").splitlines():
    fields = line.split()
    if len(fields) == 2:
        checksums[fields[1].lstrip("*")] = fields[0]

def archive_path(item):
    candidate = pathlib.Path(item["path"])
    if candidate.is_absolute():
        return candidate
    for path in (repo / candidate, dist / candidate, dist / candidate.name):
        if path.is_file():
            return path
    raise AssertionError(f"archive path does not exist: {candidate}")

def archive_payloads(path):
    if path.suffix == ".zip":
        with zipfile.ZipFile(path) as archive:
            names = [name.lstrip("./") for name in archive.namelist() if not name.endswith("/")]
            return names, {name.lstrip("./"): archive.read(name) for name in archive.namelist() if not name.endswith("/")}
    with tarfile.open(path, "r:*") as archive:
        members = [member for member in archive.getmembers() if member.isfile()]
        names = [member.name.lstrip("./") for member in members]
        return names, {
            member.name.lstrip("./"): archive.extractfile(member).read()
            for member in members
        }

for artifact in archives:
    path = archive_path(artifact)
    names, payloads = archive_payloads(path)
    packaged_contract = sorted(name for name in names if name.startswith(contract_root.as_posix() + "/"))
    assert packaged_contract == expected_names, (
        f"{path.name} contract inventory mismatch\ngot={packaged_contract}\nwant={expected_names}"
    )
    for name, expected_hash in source_hashes.items():
        actual_hash = hashlib.sha256(payloads[name]).hexdigest()
        assert actual_hash == expected_hash, f"{path.name}:{name} checksum mismatch"
    documentation = "docs/review-integration.md"
    assert documentation in payloads, f"{path.name} is missing {documentation}"
    assert hashlib.sha256(payloads[documentation]).digest() == hashlib.sha256((repo / documentation).read_bytes()).digest(), (
        f"{path.name}:{documentation} checksum mismatch"
    )
    archive_hash = hashlib.sha256(path.read_bytes()).hexdigest()
    assert checksums.get(path.name) == archive_hash, f"{path.name} is missing or incorrect in checksums.txt"

print(f"review contract release archives: PASS ({len(archives)} archives, {len(expected_names)} contract files each)")
PY
