#!/usr/bin/env sh
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

# Reproducible operator demo, hardware-free and $0. It stands up a simulated
# Meshtastic radio (real firmware, no RF), then runs the operator's real reconcile
# loop to bring the device to a declared intent, including a secure private
# channel, watches it detect drift, apply only what changed, reboot, and
# re-verify to Ready, and then proves idempotency by running it again with zero
# further writes.
#
# Prerequisites: docker, go (1.25.x), and the Meshtastic CLI plus library
# (`pip install "meshtastic[cli]"`). Override the CLI and python with MESH_BIN and
# NEPHMESH_PY if they are not `meshtastic` and `python` on your PATH.
#
# Run from the operator module directory:
#     sh ../../demo/operator/run.sh
set -eu

HOST=127.0.0.1:14403
IMAGE=meshtastic/meshtasticd:beta-debian
PY="${NEPHMESH_PY:-python}"
CLI="${MESH_BIN:-meshtastic}"
EXPORTER="$PY hack/mesh-export.py"
APPLIER="$PY hack/mesh-apply.py"

cleanup() { docker rm -f nephmesh-demo >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== bringing up a simulated Meshtastic radio (real firmware, no RF) =="
cleanup
# --restart=always matters: applying config reboots meshtasticd, which exits, and
# the container must bring it back for the loop to re-verify.
docker run -d --name nephmesh-demo --restart=always -p 14403:4403 "$IMAGE" \
  meshtasticd --sim --fsdir=/var/lib/meshtasticd --port=4403 >/dev/null
printf "waiting for the device API"
i=0
while [ "$i" -lt 30 ]; do
  if "$PY" -c "import socket; socket.create_connection(('127.0.0.1',14403),timeout=2).close()" 2>/dev/null; then break; fi
  printf "."; sleep 1; i=$((i + 1))
done
echo " ready"
sleep 6

echo
echo "== reconcile declared intent onto the device (region, preset, owner, and a secure channel) =="
MESH_BIN="$CLI" go run ./cmd/reconcile-demo \
  -host "$HOST" \
  -region US -preset MEDIUM_SLOW -owner "NephMesh Field 01" -owner-short NF01 \
  -exporter "$EXPORTER" -applier "$APPLIER" \
  -channel-name relief -channel-key "demo-relief-key" -channel-index 1

echo
echo "== run it again: the device is already in the declared state, so nothing is written =="
MESH_BIN="$CLI" go run ./cmd/reconcile-demo \
  -host "$HOST" \
  -region US -preset MEDIUM_SLOW -owner "NephMesh Field 01" -owner-short NF01 \
  -exporter "$EXPORTER" -applier "$APPLIER" \
  -channel-name relief -channel-key "demo-relief-key" -channel-index 1

echo
echo "== done. The operator declared intent, converged the radio (including a secure channel), and was idempotent on re-run. =="
