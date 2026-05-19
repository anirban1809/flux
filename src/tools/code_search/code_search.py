import argparse
import json
import shutil
import subprocess
from pathlib import Path


MAX_MATCHES = 200


def run_rg(query: str, root: Path) -> tuple[list[dict], bool]:
    command = [
        "rg",
        "--json",
        "--ignore-case",
        "--line-number",
        "--column",
        "--",
        query,
    ]
    result = subprocess.run(
        command,
        cwd=str(root),
        capture_output=True,
        text=True,
    )

    if result.returncode not in (0, 1):
        raise RuntimeError(result.stderr.strip() or "ripgrep failed")

    matches: list[dict] = []
    for line in result.stdout.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue

        if event.get("type") != "match":
            continue

        data = event.get("data", {})
        path = data.get("path", {}).get("text", "")
        lines = data.get("lines", {}).get("text", "").rstrip("\n")
        line_number = data.get("line_number")

        submatches = data.get("submatches") or []
        column = submatches[0].get("start") + 1 if submatches else None

        matches.append({
            "file": path,
            "line": line_number,
            "column": column,
            "content": lines.strip(),
        })

    return matches, len(matches) > MAX_MATCHES


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

    if not shutil.which("rg"):
        print(json.dumps({"ok": False, "error": "ripgrep (rg) is required for code_search"}))
        return

    try:
        matches, truncated = run_rg(args.query, root)
        print(json.dumps({
            "ok": True,
            "matches": matches[:MAX_MATCHES],
            "truncated": truncated,
            "count": len(matches),
        }))
    except Exception as e:
        print(json.dumps({"ok": False, "error": str(e)}))


run()
