#!/usr/bin/env python3
import argparse
import json
import pathlib
import re
import sys


def hostname(value):
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.-]{0,252}", value):
        raise ValueError()
    return value.split(".")[0].lower()


def recipe_release(image):
    if image is None:
        return None
    if not isinstance(image, str) or not re.fullmatch(r"[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", image):
        raise ValueError()
    name = image.rsplit("/", 1)[-1]
    if not name.endswith((".img", ".tar.zst")):
        return None
    match = re.match(r"^(ubuntu|debian|rocky|freebsd)-(\d+(?:\.\d+)?)(?=[-_.])", name)
    return (match[1], match[2]) if match else None


def read_recipes(paths):
    result = {}
    for path in paths:
        data = json.loads(path.read_text())
        if not isinstance(data, dict) or not data:
            raise ValueError()
        for name, fields in data.items():
            host = hostname(name)
            if host in result or not isinstance(fields, dict) or set(fields) != {"kind", "node", "image"}:
                raise ValueError()
            if fields["kind"] not in ("vm", "lxc"):
                raise ValueError()
            hostname(fields["node"])
            result[host] = recipe_release(fields["image"])
    if not result:
        raise ValueError()
    return result


def read_observations(path):
    data = json.loads(path.read_text())
    if data.get("status") != "success" or data["data"]["resultType"] != "vector":
        raise ValueError()
    result = {}
    for series in data["data"]["result"]:
        labels = series["metric"]
        host = hostname(labels["nodename"])
        os_id, version = labels["id"], labels["version_id"]
        if not isinstance(os_id, str) or not re.fullmatch(r"[a-z][a-z0-9_]*", os_id):
            raise ValueError()
        if not isinstance(version, str) or not re.fullmatch(r"[A-Za-z0-9.+_-]{1,64}", version):
            raise ValueError()
        if float(series["value"][1]) != 1:
            raise ValueError()
        result.setdefault(host, set()).add((os_id, version))
    return result


def matches(recipe, observed):
    os_id, version = recipe
    actual_id, actual_version = observed
    if os_id != actual_id:
        return False
    if os_id in ("debian", "rocky"):
        return actual_version == version or actual_version.startswith(version + ".")
    if os_id == "freebsd":
        actual_version = actual_version.split("-", 1)[0]
    return actual_version == version


def audit(recipes, observations):
    rows = []
    for host, recipe in sorted(recipes.items()):
        observed = observations.get(host, set())
        if recipe is None:
            status = "unsupported-recipe"
        elif not observed:
            status = "unobserved"
        elif len(observed) != 1:
            status = "ambiguous-observation"
        else:
            status = "match" if matches(recipe, next(iter(observed))) else "mismatch"
        rows.append((host, status, recipe, next(iter(observed)) if len(observed) == 1 else None))
    return rows


def main():
    parser = argparse.ArgumentParser(description="Audit creation recipes against an OS observation snapshot.")
    parser.add_argument("--recipes", nargs="+", type=pathlib.Path, required=True)
    parser.add_argument("--observations", type=pathlib.Path, required=True)
    args = parser.parse_args()
    try:
        rows = audit(read_recipes(args.recipes), read_observations(args.observations))
    except Exception:
        print("Invalid or unreadable audit input; export only guest_os_recipes and the documented OS query.", file=sys.stderr)
        return 2
    print("HOST\tSTATUS\tRECIPE_OS\tOBSERVED_OS")
    for host, status, recipe, observed in rows:
        print("\t".join((host, status, " ".join(recipe) if recipe else "-", " ".join(observed) if observed else "-")))
    mismatch = sum(row[1] == "mismatch" for row in rows)
    incomplete = sum(row[1] not in ("match", "mismatch") for row in rows)
    print(f"Audited={len(rows)} matched={len(rows) - mismatch - incomplete} mismatched={mismatch} incomplete={incomplete}")
    return 1 if mismatch else 3 if incomplete else 0


if __name__ == "__main__":
    sys.exit(main())
