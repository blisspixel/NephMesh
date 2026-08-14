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

# Replays the MeshToad plus T-Deck bench: observe both radios, send one
# application text each way over LoRa, and optionally apply then restore a
# secondary channel on the MeshToad. It is a hand-run script, not autonomy
# and not a cluster install.
#
# Preconditions: an SSH tunnel so MESH_HOST reaches the sensor host's
# localhost 4403 (ssh -N -L 14403:127.0.0.1:4403 "$SENSOR_SSH"), the handheld
# on COM_PORT, meshtasticd already running. See demo/meshtoad-gateway/README.md.
#
#   sh demo/meshtoad-gateway/run.sh

set -eu

MESH_HOST="${MESH_HOST:-127.0.0.1:14403}"
COM_PORT="${COM_PORT:?set COM_PORT to the handheld serial device}"
MESH_CLI="${MESH_CLI:-meshtastic}"
RECONCILE_DEMO="${RECONCILE_DEMO:-reconcile-demo}"
EXPORTER="${EXPORTER:?set EXPORTER to the argv of the config exporter, e.g. python operators/meshtastic-operator/hack/mesh-export.py}"
APPLIER="${APPLIER:-}"
DO_CHANNELS="${DO_CHANNELS:-0}"
MSG_OUT="${MSG_OUT:-nephmesh-phase2-gate}"
MSG_BACK="${MSG_BACK:-nephmesh-phase2-return}"

log() { printf '\n=== %s ===\n' "$1"; }

need_cli() {
    if ! command -v "$MESH_CLI" >/dev/null 2>&1; then
        printf 'missing %s on PATH\n' "$MESH_CLI" >&2
        exit 2
    fi
}

listen_until() { # host_or_port flag value message logfile seconds
    _flag=$1
    _value=$2
    _want=$3
    _log=$4
    _secs=$5
    : >"$_log"
    "$MESH_CLI" "$_flag" "$_value" --listen >>"$_log" 2>&1 &
    _pid=$!
    _n=0
    while [ "$_n" -lt "$_secs" ]; do
        if grep -F "$_want" "$_log" >/dev/null 2>&1; then
            kill "$_pid" >/dev/null 2>&1 || true
            wait "$_pid" >/dev/null 2>&1 || true
            return 0
        fi
        _n=$((_n + 1))
        sleep 1
    done
    kill "$_pid" >/dev/null 2>&1 || true
    wait "$_pid" >/dev/null 2>&1 || true
    return 1
}

need_cli

log "OBSERVE: MeshToad at ${MESH_HOST}"
"$RECONCILE_DEMO" -host "$MESH_HOST" -exporter "$EXPORTER" -observe

log "OBSERVE: T-Deck at ${COM_PORT}"
"$RECONCILE_DEMO" -serial "$COM_PORT" -exporter "$EXPORTER" -observe

_work="${TMPDIR:-/tmp}/nephmesh-meshtoad-$$"
mkdir -p "$_work"
trap 'rm -rf "$_work"' EXIT

log "RF: T-Deck -> MeshToad (${MSG_OUT})"
listen_until --host "$MESH_HOST" "$MSG_OUT" "$_work/listen-toad.log" 40 &
_lp=$!
sleep 8
"$MESH_CLI" --port "$COM_PORT" --sendtext "$MSG_OUT" --ack
if ! wait "$_lp"; then
    printf 'MeshToad did not receive %s over LoRa\n' "$MSG_OUT" >&2
    exit 1
fi
printf 'heard %s on MeshToad\n' "$MSG_OUT"

log "RF: MeshToad -> T-Deck (${MSG_BACK})"
listen_until --port "$COM_PORT" "$MSG_BACK" "$_work/listen-tdeck.log" 40 &
_lp=$!
sleep 8
"$MESH_CLI" --host "$MESH_HOST" --sendtext "$MSG_BACK" --ack
if ! wait "$_lp"; then
    printf 'T-Deck did not receive %s over LoRa\n' "$MSG_BACK" >&2
    exit 1
fi
printf 'heard %s on T-Deck\n' "$MSG_BACK"

if [ "$DO_CHANNELS" = "1" ]; then
    if [ -z "$APPLIER" ]; then
        printf 'DO_CHANNELS=1 needs APPLIER set to mesh-apply.py\n' >&2
        exit 2
    fi
    log "CHANNEL: apply relief on MeshToad, then delete it"
    "$RECONCILE_DEMO" -host "$MESH_HOST" -exporter "$EXPORTER" -applier "$APPLIER" \
        -region US -channel-name relief -channel-key "demo-relief-key!" -channel-index 1
    "$MESH_CLI" --host "$MESH_HOST" --ch-index 1 --ch-del
fi

log "done"
