#!/usr/bin/env python3
# Copyright 2026 The NephMesh Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Delivery-ratio probe for the resilience harness.

It originates a run of sequenced broadcast texts from one node and counts how
many arrive at each of the other nodes, over their device APIs, then reports the
delivery ratio and latency. This is the measurement instrument the harness uses
to turn "resilient" from an adjective into a number, before and after a
perturbation (a killed node, a killed management plane, a congested channel).

It only observes and originates application-layer mesh text; it drives no radio.
Run it in the helper container the bring-up script starts:

    docker exec meshcli python /probe.py --sender sim1 --receivers sim2,sim3 \\
        --count 20 --interval 1

Each received text is attributed to the interface it actually arrived on, so
counts are correct with any number of receivers. Emits one `EVENT {...}` line per
send and receipt (a JSONL event log a reducer can bucket around a perturbation
timestamp) and a final `SUMMARY {...}` line.
"""

import argparse
import json
import sys
import threading
import time
import uuid

from pubsub import pub
import meshtastic.tcp_interface


def main():
    ap = argparse.ArgumentParser(description="Measure mesh delivery ratio (receive-only observation).")
    ap.add_argument("--sender", required=True, help="node hostname that originates the traffic")
    ap.add_argument("--receivers", required=True, help="comma-separated node hostnames to count receipts on")
    ap.add_argument("--count", type=int, default=20, help="number of messages to originate")
    # 3s is comfortably within the default preset's airtime ceiling, so a healthy
    # mesh delivers ~100%. Drive it lower to deliberately saturate the channel and
    # watch delivery fall: that is the airtime commons made visible, not a bug.
    ap.add_argument("--interval", type=float, default=3.0, help="seconds between originated messages")
    ap.add_argument("--warmup", type=float, default=5.0, help="seconds to let interfaces settle before sending")
    ap.add_argument("--schedule", default="", help='offered-rate schedule "interval:count,..." overriding --count/--interval, to vary load across phases (baseline, degraded, adapted); each segment change prints a BOUNDARY line')
    args = ap.parse_args()

    receivers = [r for r in args.receivers.split(",") if r]
    if not receivers:
        ap.error("no receivers given")

    # A unique id per run, baked into every message, so stale messages from a
    # previous run (still rebroadcasting in the mesh, or replayed to a freshly
    # connected interface) are ignored instead of corrupting the count.
    run_id = uuid.uuid4().hex[:8]
    prefix = "probe-%s-" % run_id

    lock = threading.Lock()
    got = {}      # (seq, node) -> t_recv
    events = []   # ordered event log

    # Attribute each receipt to the interface it arrived on. The Meshtastic
    # pubsub is process-global, so every subscribed callback fires for every
    # interface; mapping id(interface) -> node keeps per-receiver counts honest.
    iface_of = {}

    def on_receive(packet, interface):
        node = iface_of.get(id(interface))
        if node is None:
            return
        d = packet.get("decoded", {}) or {}
        if d.get("portnum") != "TEXT_MESSAGE_APP":
            return
        text = d.get("text", "")
        if not text.startswith(prefix):
            return
        try:
            seq = int(text[len(prefix):])
        except ValueError:
            return
        now = time.time()
        with lock:
            if (seq, node) not in got:
                got[(seq, node)] = now
                events.append({"ev": "recv", "seq": seq, "node": node, "t": now})

    pub.subscribe(on_receive, "meshtastic.receive")

    open_ifaces = []
    for node in receivers:
        iface = meshtastic.tcp_interface.TCPInterface(hostname=node)
        iface_of[id(iface)] = node
        open_ifaces.append(iface)
    sender = meshtastic.tcp_interface.TCPInterface(hostname=args.sender)
    open_ifaces.append(sender)
    # The UDP transport and the interfaces need a moment before the first send is
    # reliably carried; sending too early silently drops (learned empirically).
    time.sleep(args.warmup)

    # The send schedule is either the single --count/--interval, or a multi-segment
    # --schedule "interval:count,..." that changes the offered rate mid-run to
    # drive the mesh over its airtime budget and back. Each segment transition
    # prints a BOUNDARY line with the wall-clock time for `nephmeshctl resilience
    # -phases`; the reducer ignores non-EVENT lines.
    schedule = []
    if args.schedule:
        for seg in args.schedule.split(","):
            iv, _, ct = seg.partition(":")
            schedule.append((float(iv), int(ct)))
    else:
        schedule.append((args.interval, args.count))

    sent = {}
    seq = 0
    for si, (interval, count) in enumerate(schedule):
        if si > 0:
            sys.stdout.write("BOUNDARY %f\n" % time.time())
            sys.stdout.flush()
        for _ in range(count):
            sender.sendText("%s%d" % (prefix, seq))
            now = time.time()
            sent[seq] = now
            events.append({"ev": "sent", "seq": seq, "node": args.sender, "t": now})
            seq += 1
            time.sleep(interval)
    time.sleep(4.0)  # let the last in-flight messages land

    expected = len(sent) * len(receivers)
    delivered = len(got)
    latencies = sorted(got[(s, n)] - sent[s] for (s, n) in got if s in sent)
    summary = {
        "sender": args.sender,
        "receivers": receivers,
        "sent": len(sent),
        "expected": expected,
        "delivered": delivered,
        "delivery_ratio": round(delivered / expected, 4) if expected else 0.0,
        "latency_ms_p50": round(latencies[len(latencies) // 2] * 1000, 1) if latencies else None,
        "latency_ms_max": round(latencies[-1] * 1000, 1) if latencies else None,
    }

    for e in events:
        sys.stdout.write("EVENT " + json.dumps(e) + "\n")
    sys.stdout.write("SUMMARY " + json.dumps(summary) + "\n")
    sys.stdout.flush()

    for iface in open_ifaces:
        try:
            iface.close()
        except Exception:
            pass


if __name__ == "__main__":
    main()
