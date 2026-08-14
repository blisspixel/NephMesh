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

"""Write channel settings to a Meshtastic node, keeping keys out of argv and logs.

The pre-shared keys are read from a JSON file (given by --channels-file), never
the command line, matching how the operator writes the scalar config to a
`--configure` file rather than passing secrets as arguments. Each channel's "psk"
is one of: "default" (the public default key, the single byte 0x01), "none" (no
key), or "base64:<b64>" (an explicit raw key). Setting channels reboots the
device, the same as a scalar config apply.

    mesh-apply.py --serial COM3 --channels-file chans.json
    mesh-apply.py --host 127.0.0.1:14403 --channels-file chans.json

The file is a JSON array of objects: index, name, psk, uplinkEnabled,
downlinkEnabled. This script never prints channel names or key material; on
success it prints only a count.
"""

import argparse
import base64
import json
import sys

from meshtastic.protobuf import channel_pb2


def as_bool(value, default=False):
    """Parse a JSON boolean without treating the string 'false' as True."""
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return value != 0
    if isinstance(value, str):
        s = value.strip().lower()
        if s in ("true", "yes", "on", "1"):
            return True
        if s in ("false", "no", "off", "0", ""):
            return False
    raise ValueError("not a boolean: %r" % (value,))


def psk_bytes(directive: str) -> bytes:
    if directive == "none":
        return b""
    if directive == "default":
        return b"\x01"
    if directive.startswith("base64:"):
        # validate=True so a malformed key raises rather than being silently
        # mangled (b64decode otherwise discards stray non-alphabet characters and
        # writes a different key to the device).
        return base64.b64decode(directive[len("base64:") :], validate=True)
    raise ValueError("unknown psk directive")


def split_host_port(host):
    """Split a host into (host, port), handling host:port and bracketed IPv6.

    A bare host or a bracketless IPv6 literal (multiple colons) has no port.
    """
    if host.startswith("[") and "]" in host:  # [2001:db8::1]:4403
        addr, _, rest = host[1:].partition("]")
        return addr, (rest[1:] if rest.startswith(":") else "")
    if host.count(":") == 1:
        addr, _, port = host.partition(":")
        return addr, port
    return host, ""


def make_iface(serial, host):
    if serial:
        from meshtastic.serial_interface import SerialInterface

        return SerialInterface(serial)
    from meshtastic.tcp_interface import TCPInterface

    addr, port = split_host_port(host)
    if not port:
        return TCPInterface(addr)
    try:
        portnum = int(port)
    except ValueError:
        raise ValueError("invalid TCP port %r in host %r" % (port, host))
    return TCPInterface(addr, portNumber=portnum)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    group = ap.add_mutually_exclusive_group(required=True)
    group.add_argument("--serial", help="serial port, e.g. COM3 or /dev/ttyACM0")
    group.add_argument("--host", help="TCP host, e.g. 127.0.0.1:14403")
    ap.add_argument("--channels-file", required=True, help="JSON array of channels to write")
    args = ap.parse_args()

    with open(args.channels_file, "r", encoding="utf-8") as fh:
        channels = json.load(fh)

    # First pass: parse and validate every channel (index and psk directive)
    # BEFORE connecting, so a malformed entry aborts with zero device writes rather
    # than leaving the mesh half-configured after a mid-loop crash.
    parsed = []
    for ch in channels:
        parsed.append(
            {
                "index": int(ch["index"]),
                "name": ch.get("name", ""),
                "psk": psk_bytes(ch.get("psk", "default")),
                "uplink": as_bool(ch.get("uplinkEnabled"), False),
                "downlink": as_bool(ch.get("downlinkEnabled"), False),
            }
        )

    iface = make_iface(args.serial, args.host)
    try:
        node = iface.localNode
        node_channels = node.channels
        if node_channels is None:
            raise RuntimeError("device returned no channel set")
        slots = len(node_channels)
        # Bounds-check every index before writing anything, so an out-of-range
        # index cannot leave earlier channels already written.
        for ch in parsed:
            if not 0 <= ch["index"] < slots:
                raise IndexError("channel index %d out of range 0..%d" % (ch["index"], slots - 1))
        # Prefer a settings transaction so every channel is committed together
        # (one reboot) rather than writeChannel leaving a prefix applied if a
        # later slot fails. Older library builds have no such API; fall back.
        begin = getattr(node, "beginSettingsTransaction", None)
        commit = getattr(node, "commitSettingsTransaction", None)
        use_txn = callable(begin) and callable(commit)
        if use_txn:
            begin()
        for ch in parsed:
            idx = ch["index"]
            c = node_channels[idx]
            c.role = (
                channel_pb2.Channel.Role.PRIMARY
                if idx == 0
                else channel_pb2.Channel.Role.SECONDARY
            )
            c.settings.name = ch["name"]
            c.settings.psk = ch["psk"]
            c.settings.uplink_enabled = ch["uplink"]
            c.settings.downlink_enabled = ch["downlink"]
            node.writeChannel(idx)
        if use_txn:
            commit()
    finally:
        iface.close()

    # Never echo names or keys; a count is enough for the operator to log.
    sys.stdout.write("applied %d channel(s)\n" % len(parsed))
    return 0


if __name__ == "__main__":
    sys.exit(main())
