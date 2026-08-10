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

# Control-plane-independence proof: show, with a number, that the mesh keeps
# carrying traffic when the management plane and its host are gone. This is the
# README's load-bearing claim ("a management layer, not a runtime dependency of
# the mesh"), demonstrated rather than asserted.
#
# The scenario:
#   1. bring up a hardware-free UDP mesh (data plane);
#   2. the operator's real reconcile loop (reconcile-demo, the same Converge
#      state machine the controller runs) configures a node over its API, so the
#      management plane genuinely manages the mesh;
#   3. begin measuring delivery ratio between other nodes (the monitor survives);
#   4. destroy the management plane mid-run (kill its container: the cluster and
#      the site running it are gone);
#   5. reduce the log to a before/after verdict with `nephmeshctl resilience`.
#
# Because the data plane (UDP multicast) never depended on the operator, this
# operationalizes the claim, turning it from asserted into shown; it does not
# discover a hidden dependency. Nothing here transmits over the air.
#
# Usage:  sh demo/resilience/independence.sh
# Requires Docker and Go. Runs a few minutes. Tear down with down.sh.

set -eu

export MSYS_NO_PATHCONV=1
HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && { pwd -W 2>/dev/null || pwd; })"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && { pwd -W 2>/dev/null || pwd; })"
MOD="$ROOT/operators/meshtastic-operator"
NET="${MESH_NET:-meshsim}"
# A scratch dir, kept as a Windows-style path so both the Windows Go toolchain
# and docker cp accept it (a bare /tmp/... path breaks both on Git Bash).
WORK_U="$(mktemp -d)"
WORK="$(cd "$WORK_U" && { pwd -W 2>/dev/null || pwd; })"
trap 'rm -rf "$WORK_U"' EXIT

log() { printf '\n=== %s ===\n' "$1"; }

log "1/5 Bring up the mesh (data plane)"
sh "$HERE/up.sh" 3

log "2/5 Build the operator and the reducer"
# reconcile-demo runs in a Linux container as the management plane; nephmeshctl
# reduces the log on this host.
( cd "$MOD" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$WORK/reconcile-demo" ./cmd/reconcile-demo )
( cd "$MOD" && go build -o "$WORK/nephmeshctl.exe" ./cmd/nephmeshctl )

log "3/5 Operator configures sim1 (the management plane manages the mesh)"
docker rm -f operator >/dev/null 2>&1 || true
docker run -d --name operator --network "$NET" python:3.13-slim sleep infinity >/dev/null
docker exec operator pip install -q meshtastic >/dev/null 2>&1
docker cp "$WORK/reconcile-demo" operator:/reconcile-demo >/dev/null
# Apply a benign owner change (no preset change, so it does not disturb the
# channel) through the operator's real reconcile loop: export, diff, apply drift,
# reboot, re-verify. sim1 reboots once here, before measurement starts.
docker exec operator /reconcile-demo -host sim1 -bin meshtastic \
    -owner "NephMesh Field 01" -owner-short NF01 2>&1 | grep -E 'reconcile-demo|step|converged' | tail -6 || true
echo "  waiting for sim1 to settle after its reboot..."
sleep 14

log "4/5 Measure delivery, then destroy the management plane mid-run"
# Start the probe in the surviving monitor (meshcli), writing its event log.
docker exec -d meshcli sh -c 'python /probe.py --sender sim1 --receivers sim2,sim3 --count 24 --interval 3 > /probe.jsonl 2>/dev/null'
echo "  probe running; measuring baseline..."
sleep 40
KILL_T="$(date +%s)"
echo "  perturbation at t=${KILL_T}: destroying the management plane (operator + its host)"
docker rm -f operator >/dev/null 2>&1 || true
echo "  mesh continues; measuring after..."
sleep 45  # let the probe finish its run and settle

log "5/5 Verdict"
docker cp meshcli:/probe.jsonl "$WORK/probe.jsonl" >/dev/null
"$WORK/nephmeshctl.exe" resilience -at "$KILL_T" -f "$WORK/probe.jsonl" -o text
echo
echo "Tear down with: sh demo/resilience/down.sh  (and: docker rm -f operator)"
