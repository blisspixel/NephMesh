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

"""Emit a Meshtastic node's live configuration in the --export-config format,
reading the config that streams on connect rather than re-requesting it.

The CLI's own `meshtastic --export-config` re-requests the full config and
channel set over admin messages, which hangs on some real devices over a USB
serial link (verified against a T-Deck, firmware 2.7.26: --info returns in
seconds, --export-config never completes). The device client uses this helper
for the export step so the operator can reconcile over serial as well as TCP.
It emits only the fields the operator manages, which is all the drift diff
needs, in the marker-prefixed YAML the operator already parses.

    mesh-export.py --serial COM3
    mesh-export.py --host 192.168.1.42
"""

import argparse
import hashlib
import sys

import yaml
from meshtastic.protobuf import config_pb2


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    group = ap.add_mutually_exclusive_group(required=True)
    group.add_argument("--serial", help="serial port, e.g. COM3 or /dev/ttyACM0")
    group.add_argument("--host", help="TCP host, e.g. 192.168.1.42")
    args = ap.parse_args()

    if args.serial:
        from meshtastic.serial_interface import SerialInterface

        iface = SerialInterface(args.serial)
    else:
        from meshtastic.tcp_interface import TCPInterface

        iface = TCPInterface(args.host)

    try:
        lc = iface.localNode.localConfig
        region = config_pb2.Config.LoRaConfig.RegionCode.Name(lc.lora.region)
        preset = config_pb2.Config.LoRaConfig.ModemPreset.Name(lc.lora.modem_preset)
        role = config_pb2.Config.DeviceConfig.Role.Name(lc.device.role)

        doc: dict = {"config": {"lora": {"region": region}, "device": {"role": role}}}
        # modemPreset is only meaningful when the device is preset-based; emit it
        # only then, so it does not read as drift on a bandwidth/spread-factor node.
        if lc.lora.use_preset:
            doc["config"]["lora"]["modemPreset"] = preset

        # MQTT, if the module config streamed on connect. Wrapped so a device
        # that does not return it does not break the export.
        try:
            mc = iface.localNode.moduleConfig.mqtt
            mqtt: dict = {"enabled": bool(mc.enabled)}
            if mc.address:
                mqtt["address"] = mc.address
            if mc.username:
                mqtt["username"] = mc.username
            if mc.root:
                mqtt["root"] = mc.root
            mqtt["encryptionEnabled"] = bool(mc.encryption_enabled)
            mqtt["jsonEnabled"] = bool(mc.json_enabled)
            mqtt["tlsEnabled"] = bool(mc.tls_enabled)
            doc["module_config"] = {"mqtt": mqtt}
        except Exception:  # noqa: BLE001 - module config is best-effort
            pass

        try:
            user = iface.getMyUser()
            if user:
                if user.get("longName"):
                    doc["owner"] = user["longName"]
                if user.get("shortName"):
                    doc["owner_short"] = user["shortName"]
        except Exception:  # noqa: BLE001 - owner is best-effort
            pass

        # Channels, keyed by slot index. The pre-shared key is emitted only as a
        # SHA-256 hash so the raw key never leaves the device: the operator
        # compares this against the hash of the declared key (resolved from a
        # Secret) to detect drift without ever handling the key here. Disabled
        # slots (role 0) are skipped. Wrapped so a device that does not stream
        # channels does not break the export.
        try:
            channels = []
            for ch in iface.localNode.channels or []:
                if ch.role == 0:  # DISABLED
                    continue
                s = ch.settings
                psk_hash = hashlib.sha256(bytes(s.psk)).hexdigest() if len(s.psk) else ""
                channels.append(
                    {
                        "index": int(ch.index),
                        "name": s.name,
                        "pskHash": psk_hash,
                        "uplinkEnabled": bool(s.uplink_enabled),
                        "downlinkEnabled": bool(s.downlink_enabled),
                    }
                )
            if channels:
                doc["channels"] = channels
        except Exception:  # noqa: BLE001 - channels are best-effort
            pass
    finally:
        iface.close()

    # The marker the operator's parser looks for, then the document.
    sys.stdout.write("# start of Meshtastic configure yaml\n")
    sys.stdout.write(yaml.dump(doc, default_flow_style=False, sort_keys=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
