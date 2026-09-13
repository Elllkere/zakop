#!/usr/bin/env python3
"""Local TCP smoke test; no router, namespaces, or firewall modifications."""
import json
from pathlib import Path
import socket
import subprocess
import sys


def main():
    script = str(Path(__file__).with_name("dnat-stress.py"))
    with socket.socket() as reserve:
        reserve.bind(("127.0.0.1", 0))
        port = reserve.getsockname()[1]
    server = subprocess.Popen([sys.executable, script, "server", "--port", str(port)],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        # Startup is observed by connecting; client retries are only in this setup probe.
        import time
        for _ in range(50):
            if server.poll() is not None:
                raise RuntimeError(server.communicate())
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=0.1):
                    break
            except OSError:
                time.sleep(0.02)
        else:
            raise RuntimeError("server startup timeout")
        result = subprocess.run([sys.executable, script, "client", "--host", "127.0.0.1",
                                 "--port", str(port), "--duration", "1", "--flows", "3", "--mbps", "0.05"],
                                capture_output=True, text=True, timeout=45, check=True)
        rows = [json.loads(line) for line in result.stdout.splitlines()]
        assert len(rows) == 3
        assert all(row["sent"] > 0 and row["sent"] == row["received"] and not row["errors"] for row in rows)
        print("PASS: 3 long-lived loopback TCP flows, paced send, verified bidirectional bytes")
    finally:
        server.terminate()
        try:
            server.wait(timeout=5)
        except subprocess.TimeoutExpired:
            server.kill()
            server.wait()


if __name__ == "__main__":
    main()
