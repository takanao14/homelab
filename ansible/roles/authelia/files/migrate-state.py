#!/usr/bin/env python3
from contextlib import closing
import json
import os
from pathlib import Path
import shutil
import sqlite3
import stat
import subprocess
import sys
import tempfile
import time
import urllib.request


def service(action):
    result = subprocess.run(
        ['/usr/bin/systemctl', action] + ([] if action == 'daemon-reload' else ['authelia.service']),
        capture_output=True, timeout=60,
    )
    if result.returncode:
        raise ValueError()


def regular_file(path, required=False):
    try:
        info = Path(path).lstat()
    except FileNotFoundError:
        if required:
            raise ValueError()
        return False
    if not stat.S_ISREG(info.st_mode):
        raise ValueError()
    return True


def database_files(path, required=False):
    regular_file(path, required)
    for suffix in ('-wal', '-shm', '-journal'):
        regular_file(Path(str(path) + suffix))


def database_copy(source, target):
    source, target = Path(source), Path(target)
    database_files(source, required=True)
    database_files(target)
    temporary = None
    try:
        with closing(sqlite3.connect(f'file:{source}?mode=ro', uri=True)) as src:
            if src.execute('PRAGMA integrity_check').fetchone()[0] != 'ok':
                raise ValueError()
            descriptor, name = tempfile.mkstemp(prefix='.authelia-state-', dir=target.parent)
            os.close(descriptor)
            temporary = Path(name)
            with closing(sqlite3.connect(temporary)) as dst:
                src.backup(dst)
                dst.commit()
                if dst.execute('PRAGMA integrity_check').fetchone()[0] != 'ok':
                    raise ValueError()
        if target.exists():
            # Both services are stopped; checkpoint before replacing an old WAL database.
            with closing(sqlite3.connect(target)) as previous:
                if previous.execute('PRAGMA wal_checkpoint(TRUNCATE)').fetchone()[0] != 0:
                    raise ValueError()
            info = metadata(target)
            restore_metadata(temporary, info)
        os.replace(temporary, target)
        temporary = None
        for suffix in ('-wal', '-shm'):
            Path(str(target) + suffix).unlink(missing_ok=True)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def metadata(path):
    info = path.stat()
    return {'uid': info.st_uid, 'gid': info.st_gid, 'mode': stat.S_IMODE(info.st_mode)}


def restore_metadata(path, info):
    os.chown(path, info['uid'], info['gid'])
    os.chmod(path, info['mode'])


def migrate(action, config, data, secrets, unit, backup, uid, gid):
    config, data, secrets, unit, backup = map(Path, (config, data, secrets, unit, backup))
    paths = (config, data, secrets, unit, backup)
    if any(not p.is_absolute() or any(part.is_symlink() for part in (p, *p.parents)) for p in paths):
        raise ValueError()
    roots = [p.resolve() for p in (config, data, secrets)]
    if any(a == b or a in b.parents or b in a.parents
           for index, a in enumerate(roots) for b in roots[index + 1:]):
        raise ValueError()
    if any(p in backup.resolve().parents or backup.resolve() in p.parents for p in roots):
        raise ValueError()
    if action == 'prepare':
        if any(backup.iterdir()) or (data / 'db.sqlite3').exists():
            raise ValueError()
        regular_file(data / 'notification.txt')
        os.chmod(backup, 0o700)
        paths = [config, secrets, config / 'configuration.yml', config / 'db.sqlite3']
        paths += [secrets / n for n in (
            'session_secret', 'storage_encryption_key', 'reset_password_jwt_secret', 'ldap_password'
        )]
        if regular_file(config / 'notification.txt'):
            paths.append(config / 'notification.txt')
        if not config.is_dir() or not secrets.is_dir():
            raise ValueError()
        for path in paths[2:]:
            regular_file(path, required=True)
        regular_file(unit, required=True)
        saved = {str(p): metadata(p) for p in paths}
        (backup / 'metadata.json').write_text(json.dumps(saved))
        shutil.copy2(config / 'configuration.yml', backup / 'configuration.yml')
        shutil.copy2(unit, backup / 'authelia.container')
        # Mark before stopping so rescue also covers a partially failed stop.
        (backup / 'started').touch(mode=0o600)
        service('stop')
        regular_file(config / 'notification.txt')
        database_copy(config / 'db.sqlite3', backup / 'db.sqlite3')
        os.chmod(backup / 'db.sqlite3', 0o600)
        data.mkdir(mode=0o700, exist_ok=True)
        database_copy(config / 'db.sqlite3', data / 'db.sqlite3')
        regular_file(data / 'notification.txt')
        if (config / 'notification.txt').exists():
            shutil.copy2(config / 'notification.txt', data / 'notification.txt')
        for name in ('db.sqlite3', 'notification.txt'):
            path = data / name
            if path.exists():
                os.chown(path, uid, gid)
                os.chmod(path, 0o600)
        os.chown(data, uid, gid)
        os.chmod(data, 0o700)
        (backup / 'prepared').touch(mode=0o600)
    elif action == 'rollback':
        if (backup / 'rolled-back').exists():
            return
        if not (backup / 'started').exists():
            return
        service('stop')
        saved = json.loads((backup / 'metadata.json').read_text())
        # Preserve writes made after cutover instead of restoring the old snapshot.
        if (backup / 'prepared').exists():
            database_files(data / 'db.sqlite3', required=True)
            regular_file(data / 'notification.txt', required=str(config / 'notification.txt') in saved)
            regular_file(config / 'notification.txt')
            database_copy(data / 'db.sqlite3', config / 'db.sqlite3')
            if (data / 'notification.txt').exists():
                shutil.copy2(data / 'notification.txt', config / 'notification.txt')
        shutil.copy2(backup / 'configuration.yml', config / 'configuration.yml')
        shutil.copy2(backup / 'authelia.container', unit)
        for name, info in saved.items():
            restore_metadata(Path(name), info)
        if str(config / 'notification.txt') not in saved and (config / 'notification.txt').exists():
            os.chown(config / 'notification.txt', saved[str(config)]['uid'], saved[str(config)]['gid'])
            os.chmod(config / 'notification.txt', 0o600)
        service('daemon-reload')
        service('start')
        for attempt in range(15):
            try:
                with urllib.request.urlopen('http://127.0.0.1:9091/api/health', timeout=2) as response:
                    if response.status == 200:
                        break
            except Exception:
                pass
            time.sleep(1)
        else:
            raise ValueError()
        (backup / 'rolled-back').touch(mode=0o600)
        # The legacy DB is authoritative again; force explicit migration on a retry.
        for name in ('db.sqlite3', 'db.sqlite3-wal', 'db.sqlite3-shm', 'notification.txt'):
            (data / name).unlink(missing_ok=True)
    else:
        raise ValueError()


if __name__ == '__main__':
    try:
        action, config, data, secrets, unit, backup, uid, gid = sys.argv[1:]
        migrate(action, config, data, secrets, unit, backup, int(uid), int(gid))
        print(json.dumps({'check': 'state_migration', 'success': True}))
    except Exception:
        print(json.dumps({'check': 'state_migration', 'success': False}))
        sys.exit(1)
