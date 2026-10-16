"""Own one isolated Core process and terminate its group when transport closes."""
import os
import signal
import subprocess
import sys
import threading

core, task_home, root, endpoint = sys.argv[1:]
if not endpoint.startswith("http://127.0.0.1:") or not endpoint.endswith("/v1"):
    raise ValueError("Fixture endpoint must be loopback")
environment = {"PATH": "/usr/bin:/bin", "USERPROFILE": task_home,
               "NEKO_PROVIDER": "openai_compat", "NEKO_BASE_URL": endpoint,
               "NEKO_MODEL": "neko-controlled-fixture", "NEKO_API_KEY": "fixture-only",
               "NEKO_AUTO_UPDATE": "0", "NEKO_AUTO_UPDATE_CHECK": "false",
               "NEKO_MODE": "auto", "NEKO_MAX_STEPS": "6", "NEKO_MAX_RETRIES": "0",
               "NEKO_OFFLINE_RETRY_SECONDS": "0", "NEKO_TIMEOUT_SECONDS": "20",
               "NEKO_COMPLETION_SOUND": "false"}
process = subprocess.Popen([core, "acp", "--host-profile", "nekobot"], cwd=root,
                           env=environment, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                           stderr=subprocess.DEVNULL, start_new_session=True)


def output():
    try:
        for line in process.stdout:
            sys.stdout.buffer.write(line)
            sys.stdout.buffer.flush()
    finally:
        os._exit(1)


threading.Thread(target=output, daemon=True).start()
try:
    for line in sys.stdin.buffer:
        process.stdin.write(line)
        process.stdin.flush()
finally:
    try:
        process.stdin.close()
        process.wait(timeout=3)
    except ProcessLookupError:
        pass
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=2)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
