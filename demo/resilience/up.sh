#!/bin/sh
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

# Bring up a hardware-free multi-node Meshtastic mesh for the resilience harness.
# Each node is a meshtasticd instance in simulated-radio mode (-s, no radio); the
# nodes mesh over the firmware's real UDP multicast transport (group 239.0.0.69,
# port 4403), which is a genuine product feature of the Linux build, so a text
# sent on one node is carried to the others with no RF. This is the data plane.
# The node TCP APIs (4403) stay free for a management plane (the operator) to
# configure them, which is the separation the control-plane-independence test
# depends on.
#
# Usage:
#   sh demo/resilience/up.sh [node-count]   # default 3
#   sh demo/resilience/down.sh              # tear it all down
#
# Requires Docker. Nothing here transmits over the air: the radios are simulated.

set -eu

NODES="${1:-3}"
IMG="${MESH_IMG:-meshtastic/meshtasticd:beta-debian}"
NET="${MESH_NET:-meshsim}"

# Git Bash rewrites in-container absolute paths (a -d /var/... argument becomes a
# Windows path) unless this is set; the same guard the kpt render uses.
export MSYS_NO_PATHCONV=1

log() { printf '\n>>> %s\n' "$1"; }

log "Network ${NET}"
docker network inspect "$NET" >/dev/null 2>&1 || docker network create "$NET" >/dev/null

# Clear nodes/volumes from any previous, possibly larger, run so stale sim
# containers do not keep transmitting on the shared channel and skew results.
# Docker name filters are substring/regex against the stored name (/sim1).
# A leading ^ does not match. List by the sim/mesh prefix and keep only
# sim<digits> / mesh<digits> so a previous larger run cannot stay on the net.
for c in $(docker ps -a --filter 'name=sim' --format '{{.Names}}'); do
    case "$c" in
        sim[0-9]|sim[0-9][0-9]|sim[0-9][0-9][0-9]) docker rm -f "$c" >/dev/null 2>&1 || true ;;
    esac
done
for v in $(docker volume ls --filter 'name=mesh' --format '{{.Name}}'); do
    case "$v" in
        mesh[0-9]|mesh[0-9][0-9]|mesh[0-9][0-9][0-9]) docker volume rm "$v" >/dev/null 2>&1 || true ;;
    esac
done

log "Starting ${NODES} simulated nodes"
i=1
while [ "$i" -le "$NODES" ]; do
    name="sim$i"
    # Distinct MAC per node yields a distinct node id (!....).
    hwid="$(printf 'dc2c6e0000%02x' "$i")"
    docker rm -f "$name" >/dev/null 2>&1 || true
    docker volume rm "mesh$i" >/dev/null 2>&1 || true
    # --restart is required: a Meshtastic config change reboots the device by
    # exiting the process; without a restart policy the container stays down.
    docker run -d --name "$name" --restart unless-stopped --network "$NET" \
        -v "mesh$i:/var/lib/meshtasticd" --entrypoint meshtasticd "$IMG" \
        -s -d /var/lib/meshtasticd -p 4403 -h "$hwid" >/dev/null
    echo "  ${name} (hwid ${hwid})"
    i=$((i + 1))
done
echo "  started; the helper waits for each API before configuring it"

log "Helper container (Meshtastic CLI for setup and the probe)"
docker rm -f meshcli >/dev/null 2>&1 || true
docker run -d --name meshcli --network "$NET" python:3.13-slim sleep infinity >/dev/null
docker exec meshcli pip install -q meshtastic >/dev/null 2>&1 || {
    echo "  failed to install the Meshtastic CLI in the helper (needs egress)"; exit 1; }
# Copy the probe in so the helper can run it against the mesh. The host-side
# source must be a Windows-style path on Git Bash, since MSYS_NO_PATHCONV leaves
# /c/... unconverted and docker cp then mangles it; the container-side target
# needs that same guard to stay /probe.py.
probe_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && { pwd -W 2>/dev/null || pwd; })"
if [ -f "$probe_dir/probe.py" ]; then
    docker cp "$probe_dir/probe.py" meshcli:/probe.py >/dev/null 2>&1 || \
        echo "  (could not copy probe.py into the helper; copy it in manually)"
fi

# Wait for a node's API to answer (up to ~60s); non-zero on timeout. Polling
# instead of a fixed sleep so a slow boot on a loaded host is not a silent race.
wait_api() {
    _n=0
    while [ "$_n" -lt 30 ]; do
        docker exec meshcli meshtastic --host "$1" --info >/dev/null 2>&1 && return 0
        _n=$((_n + 1))
        sleep 2
    done
    return 1
}

log "Enabling the UDP multicast transport on each node (reboots them)"
# Wait for each node's API before configuring it: a config change reboots the
# node, so a blind --set against a not-yet-booted API silently no-ops and leaves
# that node off the mesh. enabled_protocols bit 0x1 = UDP_BROADCAST; the setting
# persists in the node's volume. Both the enable and the reboot are verified.
i=1
while [ "$i" -le "$NODES" ]; do
    wait_api "sim$i" || { echo "  sim$i: API never came up; aborting"; exit 1; }
    docker exec meshcli meshtastic --host "sim$i" --set network.enabled_protocols 1 >/dev/null 2>&1 \
        || { echo "  sim$i: failed to enable UDP; aborting"; exit 1; }
    echo "  sim$i: UDP enabled"
    i=$((i + 1))
done

echo "  waiting for reboots to settle..."
sleep 4
i=1
while [ "$i" -le "$NODES" ]; do
    wait_api "sim$i" || { echo "  sim$i: did not return after its reboot; aborting"; exit 1; }
    i=$((i + 1))
done

log "Mesh up: ${NODES} nodes on ${NET}, helper 'meshcli' ready"
echo "Measure delivery ratio, e.g.:"
echo "  MSYS_NO_PATHCONV=1 docker exec meshcli python /probe.py --sender sim1 --receivers sim2,sim3 --count 20 --interval 1"
