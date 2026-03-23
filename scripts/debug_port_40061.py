import os
import socket
import sys

try:
    import psutil
except Exception as exc:  # pragma: no cover
    print(f"psutil import failed: {exc}")
    sys.exit(1)


def main() -> int:
    target_port = 40061
    seen = False
    for conn in psutil.net_connections(kind="tcp"):
        if conn.laddr and conn.laddr.port == target_port and conn.status == psutil.CONN_LISTEN:
            seen = True
            pid = conn.pid
            print(f"listener pid={pid}")
            if pid:
                try:
                    proc = psutil.Process(pid)
                    print(f"name={proc.name()}")
                    print(f"exe={proc.exe()}")
                    print(f"cmdline={' '.join(proc.cmdline())}")
                    print(f"cwd={proc.cwd()}")
                except Exception as exc:
                    print(f"process inspect failed: {exc}")
    if not seen:
        print("no listener on 40061")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
