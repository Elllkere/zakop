#!/usr/bin/env python3
"""Integration harness for a disposable Linux VM with root/CAP_NET_ADMIN.

Uses generated Zakop nft + sing-box JSON and real sing-box; never host fw4.
Creates only uniquely named namespaces; does not touch the host ruleset/routes.
Requires iproute2, nft, conntrack, sysctl, Python3, zakopd and sing-box >=1.12.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import uuid


def command(*args, input=None, check=True):
    return subprocess.run(args, input=input, text=True, capture_output=True,
                          check=check, timeout=30).stdout


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--zakopd", required=True)
    parser.add_argument("--sing-box", required=True)
    parser.add_argument("--duration", type=int, default=60)
    parser.add_argument("--flows", type=int, default=4)
    parser.add_argument("--mbps", type=float, default=1)
    parser.add_argument("--reloads", type=int, default=3,
                        help="reload generated Zakop nft during the connection")
    args = parser.parse_args()
    if not 5 <= args.duration <= 43200 or not 1 <= args.flows <= 128 or args.mbps <= 0 or args.reloads < 0:
        parser.error("duration=5..43200, flows=1..128, mbps>0, reloads>=0")
    zakopd, singbox = str(Path(args.zakopd).resolve()), str(Path(args.sing_box).resolve())
    stress = str(Path(__file__).with_name("dnat-stress.py").resolve())
    prefix = "zk" + uuid.uuid4().hex[:7]
    internet, router, lan = [prefix + x for x in ("i", "r", "l")]
    created, children = [], []

    def ns(name, *argv, **kwargs):
        return command("ip", "netns", "exec", name, *argv, **kwargs)

    def start(name, *argv, **kwargs):
        child = subprocess.Popen(["ip", "netns", "exec", name, *argv], **kwargs)
        children.append(child)
        return child

    def counter(name):
        doc = json.loads(ns(router, "nft", "-j", "list", "counter", "inet", "observe", name))
        return next(obj["counter"]["packets"] for obj in doc["nftables"] if "counter" in obj)

    try:
        with tempfile.TemporaryDirectory(prefix="zakop-dnat-") as work:
            work = Path(work)
            for name in (internet, router, lan):
                command("ip", "netns", "add", name)
                created.append(name)
                ns(name, "ip", "link", "set", "lo", "up")
            # Create veth entirely inside the test router namespace.
            for peer, iface, router_addr, peer_addr in (
                (internet, "wan", "11.0.0.1/24", "11.0.0.2/24"),
                (lan, "lan", "192.168.8.1/24", "192.168.8.2/24"),
            ):
                ns(router, "ip", "link", "add", iface, "type", "veth", "peer", "name", "peer")
                ns(router, "ip", "link", "set", "peer", "netns", peer)
                ns(router, "ip", "addr", "add", router_addr, "dev", iface)
                ns(router, "ip", "link", "set", iface, "up")
                ns(peer, "ip", "addr", "add", peer_addr, "dev", "peer")
                ns(peer, "ip", "link", "set", "peer", "up")
                ns(peer, "ip", "route", "add", "default", "via", router_addr.split("/")[0])
            ns(router, "sysctl", "-qw", "net.ipv4.ip_forward=1")
            for iface in ("all", "default", "wan", "lan"):
                ns(router, "sysctl", "-qw", f"net.ipv4.conf.{iface}.rp_filter=0")
            cfg = work / "zakop"
            cfg.write_text("""config main 'main'
 option routing_mode 'global'
 option manage_dnsmasq '0'
 option fakeip_enabled '0'
 list lan_subnet '192.168.8.0/24'
 list lan_iface 'lan'
config outbound 'test_proxy'
 option type 'trojan'
 option server '11.0.0.2'
 option port '9443'
 option password 'synthetic-test-only'
""")
            ns(router, zakopd, "compile", "-config", str(cfg), "-out-dir", str(work))
            ns(router, singbox, "check", "-c", str(work / "sing-box.json"))
            with (work / "singbox.log").open("w") as log:
                sb = start(router, singbox, "run", "-c", str(work / "sing-box.json"), stdout=log, stderr=log)
                ns(router, "ip", "rule", "add", "fwmark", "0x101", "table", "101")
                ns(router, "ip", "route", "add", "local", "default", "dev", "lo", "table", "101")
                ns(router, "nft", "-c", "-f", str(work / "zakop.nft"))
                ns(router, "nft", "-f", str(work / "zakop.nft"))
                ns(router, "nft", "-f", "-", input="""
table inet observe {
 counter dnat_reply { }
 counter marked_reply { }
 counter input_rdp { }
 counter wan_reply { }
 chain nat { type nat hook prerouting priority dstnat; policy accept;
   iifname "wan" tcp dport 33890 dnat ip to 192.168.8.2:33890
 }
 chain post_mangle { type filter hook prerouting priority -149; policy accept;
   iifname "lan" tcp sport 33890 ct direction reply ct status dnat counter name dnat_reply
   iifname "lan" tcp sport 33890 meta mark 0x101 counter name marked_reply
 }
 chain input { type filter hook input priority filter; policy accept;
   tcp sport 33890 counter name input_rdp
   tcp dport 33890 counter name input_rdp
 }
 chain forward { type filter hook forward priority filter; policy accept;
   iifname "lan" oifname "wan" tcp sport 33890 ct status dnat counter name wan_reply
 }
}
""")
                with (work / "server.log").open("w") as slog:
                    start(lan, sys.executable, stress, "server", "--listen", "192.168.8.2", stdout=slog, stderr=slog)
                    time.sleep(1)
                    if sb.poll() is not None:
                        raise RuntimeError("sing-box exited: " + (work / "singbox.log").read_text())
                    # Positive control: a LAN TCP flow must reach the actual TProxy socket.
                    ns(lan, sys.executable, "-c", "import socket; s=socket.create_connection(('11.0.0.2',44444),5); s.close()")
                    success = ns(router, "nft", "list", "counter", "inet", "zakop", "debug_tproxy_success")
                    import re
                    if int(re.search(r"packets (\d+)", success)[1]) == 0:
                        raise RuntimeError("positive TProxy control did not increment")
                    # Let the control socket drain before measuring RDP only.
                    time.sleep(2)
                    ns(router, "nft", "reset", "counter", "inet", "zakop", "debug_tproxy_success")
                    client = start(internet, sys.executable, stress, "client", "--host", "11.0.0.1",
                                   "--duration", str(args.duration), "--flows", str(args.flows), "--mbps", str(args.mbps))
                    deadline = time.monotonic() + args.duration + 60
                    reload_at = time.monotonic() + args.duration / (args.reloads + 1)
                    reloaded = 0
                    while client.poll() is None:
                        if time.monotonic() > deadline:
                            raise RuntimeError("stress timeout")
                        time.sleep(1)
                        if counter("marked_reply") or counter("input_rdp"):
                            raise RuntimeError("DNAT packet marked or delivered locally")
                        # Preserve baseline before each reload resets anonymous counters.
                        success = ns(router, "nft", "list", "counter", "inet", "zakop", "debug_tproxy_success")
                        if int(re.search(r"packets (\d+)", success)[1]):
                            raise RuntimeError("unexpected TProxy traffic during DNAT test")
                        if reloaded < args.reloads and time.monotonic() >= reload_at:
                            # Reproduce current production delete/load gap, not an invented atomic path.
                            ns(router, "nft", "delete", "table", "inet", "zakop")
                            ns(router, "nft", "-f", str(work / "zakop.nft"))
                            reloaded += 1
                            reload_at += args.duration / (args.reloads + 1)
                    if client.returncode != 0:
                        raise RuntimeError("stress client failed")
                    if counter("dnat_reply") == 0 or counter("wan_reply") == 0:
                        raise RuntimeError("no DNAT reply/WAN forwarding observed")
                    if counter("marked_reply") or counter("input_rdp"):
                        raise RuntimeError("DNAT path violated")
                    rules = ns(router, "nft", "list", "chain", "inet", "zakop", "from_lan")
                    match = re.search(r"ct status dnat counter packets (\d+)", rules)
                    if not match or int(match[1]) == 0:
                        raise RuntimeError("Zakop DNAT bypass counter did not grow")
                    print(ns(router, "conntrack", "-L", "-p", "tcp"))
                    print(json.dumps({"result": "PASS", "dnat_reply": counter("dnat_reply"),
                                      "wan_reply": counter("wan_reply"), "reloads": reloaded}))
    finally:
        for child in reversed(children):
            if child.poll() is None:
                child.terminate()
                try:
                    child.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait()
        for name in reversed(created):
            command("ip", "netns", "delete", name, check=False)


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        print(error.stdout, error.stderr, file=sys.stderr)
        raise
