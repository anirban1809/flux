import json
import subprocess
import argparse


def run() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--message", type=str, required=True)
    parser.add_argument("--command", type=str, required=True)
    parser.add_argument("--working_directory", type=str, default=".")
    parser.add_argument("--timeout_seconds", type=int, default=10)

    args = parser.parse_args()

    print(args.command.replace('"',"\""))
    try:
        result = subprocess.run(
            ["bash", "-lc", args.command.replace('"',"\"")],
            cwd=args.working_directory,
            capture_output=True,
            text=True,
            timeout=args.timeout_seconds
        )

        payload = {
            "ok": result.returncode == 0,
            "exit_code": result.returncode,
            "stdout": result.stdout,
            "stderr": result.stderr
        }
        if result.returncode != 0:
            payload["error"] = f"Command exited with status {result.returncode}"

        print(json.dumps(payload))

    except subprocess.TimeoutExpired as e:
        print(json.dumps({
            "ok": False,
            "error": "Command timed out",
            "stdout": e.stdout or "",
            "stderr": e.stderr or ""
        }))

    except Exception as e:
        print(json.dumps({
            "ok": False,
            "error": str(e)
        }))


run()
