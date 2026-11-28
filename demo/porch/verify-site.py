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

"""Check the published demo settings through a fresh, read-only device session."""

import socket

from meshtastic.protobuf import config_pb2
from meshtastic.tcp_interface import TCPInterface

with TCPInterface(hostname="meshnode-sim") as radio:
    node = radio.localNode
    region = config_pb2.Config.LoRaConfig.RegionCode.Name(node.localConfig.lora.region)
    mqtt = node.moduleConfig.mqtt
    broker_matches = mqtt.address == socket.gethostbyname("mosquitto")
    uplink = node.channels[0].settings.uplink_enabled
    print(f"region={region} mqtt_enabled={mqtt.enabled} site_broker={broker_matches} primary_uplink={uplink}")
    if not (region == "US" and mqtt.enabled and broker_matches and uplink):
        raise SystemExit(1)
