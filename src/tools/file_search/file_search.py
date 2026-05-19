import argparse
import fnmatch
import json
import os
import re
import shutil
import subprocess
from pathlib import Path


MAX_MATCHES = 200
SKIP_DIRS = {
    ".git",
    ".hg",
    ".svn",
    ".venv",
    "node_modules",
    "vendor",
    "dist",
    "build",
}


def list_files(root: Path) -> list[str]:
    if shutil.which("rg"):
        result = subprocess.run(
            ["rg", "--files"],
            cwd=str(root),
            capture_output=True,
            text=True,
        )
        if result.returncode in (0, 1):
            return [line for line in result.stdout.splitlines() if line]

    files: list[str] = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [name for name in dirnames if name not in SKIP_DIRS]
        for filename in filenames:
            full_path = Path(dirpath) / filename
            files.append(str(full_path.relative_to(root)))
    return files


def matches_query(path: str, query: str) -> bool:
    normalized = path.replace(os.sep, "/")
    basename = os.path.basename(normalized)

    if fnmatch.fnmatch(normalized, query) or fnmatch.fnmatch(basename, query):
        return True

    if query.lower() in normalized.lower():
        return True

    try:
        return re.search(query, normalized, re.IGNORECASE) is not None
    except re.error:
        return False


def run() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--message", type=str, required=True)
    parser.add_argument("--query", type=str, required=True)
    parser.add_argument("--path", type=str, required=True)

    args = parser.parse_args()
    root = Path(args.path).expanduser()

    if not args.query.strip():
        print(json.dumps({"ok": False, "error": "query cannot be empty"}))
        return

    if not root.exists() or not root.is_dir():
        print(json.dumps({"ok": False, "error": f"path is not a directory: {args.path}"}))
        return

    try:
        matches = [
            {"file": path, "content": ""}
            for path in list_files(root)
            if matches_query(path, args.query)
        ]

        print(json.dumps({
            "ok": True,
            "matches": matches[:MAX_MATCHES],
            "truncated": len(matches) > MAX_MATCHES,
            "count": len(matches),
        }))
    except Exception as e:
        print(json.dumps({"ok": False, "error": str(e)}))


run()
