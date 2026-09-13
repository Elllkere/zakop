#!/usr/bin/env python3
"""Long-lived paced TCP echo flows. Run endpoints off the router.

Each flow sends --mbps in each direction; no reconnect hides broken connections.
This models sustained bidirectional forwarded TCP, not the RDP application protocol.
"""
import argparse
import concurrent.futures
import json
import socket
import threading
import time


def positive(value):
    result = float(value)
    if not 0 < result <= 43200:
        raise argparse.ArgumentTypeError("must be >0 and <=43200")
    return result


def echo(conn):
    with conn:
        conn.settimeout(60)
        while True:
            data = conn.recv(16384)
            if not data:
                return
            conn.sendall(data)


def server(args):
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        listener.bind((args.listen, args.port))
        listener.listen(128)
        slots = threading.BoundedSemaphore(128)

        def handle(conn):
            try:
                echo(conn)
            except OSError as error:
                print(json.dumps({"server_error": str(error)}), flush=True)
            finally:
                slots.release()

        print(json.dumps({"listening": args.listen, "port": args.port}), flush=True)
        while True:
            conn, _ = listener.accept()
            if not slots.acquire(blocking=False):
                conn.close()
                continue
            threading.Thread(target=handle, args=(conn,), daemon=True).start()


def flow(args, number):
    rate = args.mbps * 1_000_000 / 8
    # Small bursts, including low-bandwidth runs. Payload is known test data.
    chunk = bytes([number % 251]) * max(1, min(16384, int(rate / 20)))
    sent = 0
    received = 0
    errors = []
    start = time.monotonic()
    with socket.create_connection((args.host, args.port), timeout=15) as conn:
        conn.settimeout(30)
        conn.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)

        def receive():
            nonlocal received
            try:
                while True:
                    data = conn.recv(65536)
                    if not data:
                        break
                    if data != bytes([number % 251]) * len(data):
                        raise RuntimeError("payload corruption")
                    received += len(data)
            except (OSError, RuntimeError) as error:
                errors.append(str(error))

        reader = threading.Thread(target=receive)
        reader.start()
        try:
            while time.monotonic() - start < args.duration and not errors:
                conn.sendall(chunk)
                sent += len(chunk)
                delay = start + sent / rate - time.monotonic()
                while delay > 0 and time.monotonic() - start < args.duration and not errors:
                    time.sleep(min(delay, 0.1))
                    delay = start + sent / rate - time.monotonic()
            conn.shutdown(socket.SHUT_WR)
        finally:
            reader.join(timeout=35)
            if reader.is_alive():
                conn.shutdown(socket.SHUT_RDWR)
                reader.join()
                errors.append("receive stalled")
    result = {"flow": number, "seconds": round(time.monotonic() - start, 3),
              "sent": sent, "received": received, "errors": errors}
    print(json.dumps(result), flush=True)
    if errors or sent == 0 or received != sent:
        raise RuntimeError("flow failed; no automatic reconnect")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="mode", required=True)
    serve = sub.add_parser("server")
    serve.add_argument("--listen", default="127.0.0.1")
    serve.add_argument("--port", type=int, default=33890)
    client = sub.add_parser("client")
    client.add_argument("--host", required=True)
    client.add_argument("--port", type=int, default=33890)
    client.add_argument("--duration", type=positive, default=3600)
    client.add_argument("--mbps", type=positive, default=1)
    client.add_argument("--flows", type=int, choices=range(1, 129), default=1)
    args = parser.parse_args()
    if not 1 <= args.port <= 65535:
        parser.error("port must be 1..65535")
    if args.mode == "server":
        server(args)
    else:
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.flows) as pool:
            list(pool.map(lambda number: flow(args, number), range(args.flows)))


if __name__ == "__main__":
    main()
