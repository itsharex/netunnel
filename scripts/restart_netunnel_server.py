from __future__ import annotations

import json
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SERVER_DIR = ROOT / "src" / "netunnel-server"
SERVER_EXE = SERVER_DIR / "netunnel-server-dev.exe"
POWERSHELL = r"C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe"
PORTS = (40061, 40062, 40063, 50000, 50001)
LOG_PATH = ROOT / "src" / "netunnel-desktop-tauri" / "logs" / "server-restart.log"


def run_powershell(script: str) -> str:
    completed = subprocess.run(
        [POWERSHELL, "-NoProfile", "-Command", script],
        capture_output=True,
        text=True,
        check=False,
    )
    if completed.returncode != 0:
        raise RuntimeError(completed.stderr.strip() or completed.stdout.strip() or f"powershell failed: {script}")
    return completed.stdout.strip()


def stop_pid(pid: int) -> None:
    try:
        run_powershell(f"Stop-Process -Id {pid} -Force -ErrorAction Stop")
    except RuntimeError as exc:
        if "Cannot find a process" not in str(exc):
            raise


def list_server_pids() -> list[int]:
    script = (
        "$procs = Get-CimInstance Win32_Process | Where-Object { "
        "$_.CommandLine -and ("
        "$_.CommandLine -like '*netunnel-server*' -or "
        "$_.CommandLine -like '*cmd/server*'"
        ") }; "
        "$procs | ForEach-Object { $_.ProcessId }"
    )
    output = run_powershell(script)
    if not output:
        return []
    pids = []
    for line in output.splitlines():
        line = line.strip()
        if line.isdigit():
            pids.append(int(line))
    return pids


def list_listener_pids() -> list[int]:
    port_list = ",".join(str(port) for port in PORTS)
    script = (
        f"$ports = @({port_list}); "
        '$netstat = Join-Path $env:SystemRoot "System32\\netstat.exe"; '
        '$lines = & $netstat -ano -p tcp; '
        '$pids = @(); '
        'foreach ($line in $lines) { '
        '  $text = $line.Trim(); '
        '  foreach ($port in $ports) { '
        '    if ($text -match (":{0}\\s+.*LISTENING\\s+(\\d+)$" -f $port)) { $pids += $Matches[1] } '
        '  } '
        '} '
        '$pids | Sort-Object -Unique'
    )
    output = run_powershell(script)
    if not output:
        return []
    pids = []
    for line in output.splitlines():
        line = line.strip()
        if line.isdigit():
            pids.append(int(line))
    return pids


def is_port_open(port: int) -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.settimeout(0.5)
        return sock.connect_ex(("127.0.0.1", port)) == 0


def start_server() -> None:
    LOG_PATH.parent.mkdir(parents=True, exist_ok=True)
    build = subprocess.run(
        ["go", "build", "-a", "-o", str(SERVER_EXE), "./cmd/server"],
        cwd=SERVER_DIR,
        capture_output=True,
        text=True,
        check=False,
    )
    if build.returncode != 0:
        raise RuntimeError(build.stderr.strip() or build.stdout.strip() or "go build failed")
    with LOG_PATH.open("ab") as handle:
        subprocess.Popen(
            [str(SERVER_EXE)],
            cwd=SERVER_DIR,
            stdout=handle,
            stderr=subprocess.STDOUT,
        )


def wait_for_route(url: str, timeout: float = 30.0) -> tuple[int, str]:
    start = time.time()
    last_error = ""
    while time.time() - start < timeout:
        try:
            with urllib.request.urlopen(url, timeout=2) as response:
                return response.getcode(), response.read().decode("utf-8", errors="replace")
        except urllib.error.HTTPError as exc:
            return exc.code, exc.read().decode("utf-8", errors="replace")
        except Exception as exc:  # noqa: BLE001
            last_error = str(exc)
            time.sleep(1)
    raise TimeoutError(last_error or f"timeout waiting for {url}")


def main() -> int:
    stopped = []
    target_pids = sorted(set(list_server_pids() + list_listener_pids()))
    for pid in target_pids:
        stop_pid(pid)
        stopped.append({"pid": pid})

    deadline = time.time() + 10
    while time.time() < deadline:
        if not any(is_port_open(port) for port in PORTS):
            break
        time.sleep(0.5)

    start_server()
    health_code, _ = wait_for_route("http://127.0.0.1:40061/healthz")
    plans_code, plans_body = wait_for_route("http://127.0.0.1:40061/api/v1/billing/plans")
    print(
        json.dumps(
            {
                "stopped": stopped,
                "health_code": health_code,
                "plans_code": plans_code,
                "plans_body_preview": plans_body[:300],
                "log_path": str(LOG_PATH),
            },
            ensure_ascii=False,
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
