"""Quiesced encrypted backup and non-destructive isolated restore verification."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time


def run(*args, capture=False):
    result = subprocess.run(args, check=True, stdout=subprocess.PIPE if capture else None)
    return result.stdout.decode() if capture else None


def sql(query):
    return run("psql", "-XAt", "-v", "ON_ERROR_STOP=1", "-c", query, capture=True).strip()


def counts():
    names = sql("SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename").splitlines()
    return {name: int(sql('SELECT count(*) FROM public."' + name.replace('"', '""') + '"')) for name in names}


def digest(path):
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def manifest(directory):
    return {str(p.relative_to(directory)): {"bytes": p.stat().st_size, "sha256": digest(p)}
            for p in sorted(directory.rglob("*")) if p.is_file() and p.name != "manifest.json"}


def backup():
    if os.environ.get("BACKUP_WRITERS_STOPPED") != "1":
        raise RuntimeError("Use scripts/release-backup.mjs to stop and verify writers first")
    root = Path("/work/snapshot")
    root.mkdir()  # Fresh tmpfs per run; never overwrite an earlier snapshot.
    started = time.time()
    for item in Path("/source/blobs").iterdir():
        if item.is_symlink() or not item.is_file() or len(item.name) != 64 or any(c not in "0123456789abcdef" for c in item.name):
            raise RuntimeError("Unexpected object in private blob storage; inspect before backup")
    run("pg_dump", "--format=custom", "--no-owner", "--no-acl", "--file=" + str(root / "database.dump"))
    shutil.copytree("/source/blobs", root / "blobs", symlinks=False)
    data = {"schema_version": 1, "captured_at": started, "tables": counts(), "files": manifest(root)}
    (root / "manifest.json").write_text(json.dumps(data, sort_keys=True))
    run("restic", "backup", str(root), "--tag", "ascend-consistent", "--host", "ascend", "--quiet")
    run("restic", "check", "--quiet")
    print(json.dumps({"backup_verified": True, "seconds": round(time.time()-started, 2), "files": len(data["files"])}))


def restore():
    # Never restores over an existing database, or accepts the production DB name.
    if os.environ.get("PGHOST") != "restore-db" or os.environ.get("PGDATABASE") != "ascend_restore":
        raise RuntimeError("Restore is restricted to restore-db/ascend_restore")
    if int(sql("SELECT count(*) FROM pg_tables WHERE schemaname='public'")):
        raise RuntimeError("Restore database is not empty; retain it for inspection")
    started = time.time()
    run("restic", "check", "--read-data", "--quiet")
    run("restic", "restore", "latest", "--tag", "ascend-consistent", "--host", "ascend", "--target", "/work/restored", "--verify")
    root = Path("/work/restored/work/snapshot")
    data = json.loads((root / "manifest.json").read_text())
    if data.get("schema_version") != 1 or manifest(root) != data["files"]:
        raise RuntimeError("Backup manifest or checksum mismatch")
    run("pg_restore", "--exit-on-error", "--no-owner", "--no-acl", "--dbname=" + os.environ["PGDATABASE"], str(root / "database.dump"))
    if counts() != data["tables"]:
        raise RuntimeError("Restored table counts differ")
    attachments = sql("SELECT storage_key || ' ' || checksum FROM attachments WHERE deleted_at IS NULL").splitlines()
    for entry in attachments:
        key, checksum = entry.split(" ", 1)
        if len(key) != 64 or any(c not in "0123456789abcdef" for c in key) or digest(root / "blobs" / key) != checksum:
            raise RuntimeError("Restored attachment differs from database")
    # Persist restored files in a dedicated volume, never the live blob volume.
    shutil.copytree(root / "blobs", "/restore/blobs", dirs_exist_ok=True)
    print(json.dumps({"restore_verified": True, "tables": len(data["tables"]), "attachments": len(attachments),
                      "rto_seconds": round(time.time()-started, 2), "snapshot_age_seconds": round(started-data["captured_at"], 2)}))


if __name__ == "__main__":
    Path(os.environ.get("RESTIC_CACHE_DIR", "/tmp/restic")).mkdir(parents=True, exist_ok=True)
    command = sys.argv[1] if len(sys.argv) == 2 else ""
    if command == "init":
        run("restic", "init")
    elif command == "backup":
        backup()
    elif command == "restore-drill":
        restore()
    elif command == "retention-preview":
        run("restic", "forget", "--host", "ascend", "--tag", "ascend-consistent", "--keep-daily", "30", "--keep-weekly", "12", "--dry-run")
    else:
        raise SystemExit("Use init, backup, restore-drill or retention-preview")
