import json
import shlex
import subprocess
import argparse


def run() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--message", type=str, required=True)
    parser.add_argument("--command", type=str, required=True)
    parser.add_argument("--working_directory", type=str, required=True)

    args = parser.parse_args()

    try:
        git_args = shlex.split(args.command)
    except ValueError as e:
        print(json.dumps({"ok": False, "error": f"Invalid command syntax: {e}"}))
        return

    try:
        result = subprocess.run(
            ["git"] + git_args,
            cwd=args.working_directory,
            capture_output=True,
            text=True,
            timeout=30,
        )

        print(json.dumps({
            "ok": True,
            "exit_code": result.returncode,
            "stdout": result.stdout,
            "stderr": result.stderr,
        }))

    except subprocess.TimeoutExpired:
        print(json.dumps({"ok": False, "error": "Git command timed out"}))

    except Exception as e:
        print(json.dumps({"ok": False, "error": str(e)}))


run()
