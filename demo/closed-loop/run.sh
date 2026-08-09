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

# A minimal, hand-driven closed loop across real hardware: SENSE the spectrum
# with an SDR, DECIDE from the sensed value, ACTUATE the mesh through the
# operator's reconcile loop, then VERIFY the change by sensing again. It is the
# physical proof-of-concept behind the Phase 6 vision (sensed occupancy proposes
# a config change).
#
# It is a PROOF OF CONCEPT with a deliberately simple, fixed policy, NOT
# autonomous operation. Real autonomy is gated behind the signed-autonomy and
# safety-kernel work (ADR 0002); this script makes one decision, actuates once,
# and stops. The mesh transmit it induces is application-layer Meshtastic text on
# a license-free band; the SDR is receive-only.
#
# It orchestrates two machines: the SDR on a sensor host reached over SSH, and a
# Meshtastic board on this machine over serial. Configure it with environment
# variables (see the defaults below), then:
#
#   sh demo/closed-loop/run.sh
#
# Requirements on this machine: the nephmeshctl and reconcile-demo binaries
# (built from operators/meshtastic-operator), a Meshtastic CLI, and the export
# helper. On the sensor host: hackrf_sweep. See demo/closed-loop/README.md.

set -eu

SENSOR_SSH="${SENSOR_SSH:-kilo@192.168.44.84}"
COM_PORT="${COM_PORT:-COM3}"
FREQ_MIN="${FREQ_MIN:-902}"
FREQ_MAX="${FREQ_MAX:-928}"
BAND="${BAND:-ism-915-us}"
# Above this peak power (relative dB) the mesh channel is treated as active.
THRESHOLD_DBM="${THRESHOLD_DBM:--35}"
# The preset to move to when the channel reads active. LONG_MODERATE uses a
# narrower bandwidth, which relocates the channel slot (an observable change).
BUSY_PRESET="${BUSY_PRESET:-LONG_MODERATE}"
HOLD_PRESET="${HOLD_PRESET:-LONG_FAST}"

NEPHMESHCTL="${NEPHMESHCTL:-nephmeshctl}"
RECONCILE_DEMO="${RECONCILE_DEMO:-reconcile-demo}"
MESH_CLI="${MESH_CLI:-meshtastic}"
EXPORTER="${EXPORTER:?set EXPORTER to the argv of the config exporter, e.g. \"python .../hack/mesh-export.py\"}"

log() { printf '\n=== %s ===\n' "$1"; }

# reconcile applies a desired preset through the operator's real reconcile loop.
reconcile() { # preset
    # shellcheck disable=SC2086
    "$RECONCILE_DEMO" -preset "$1" -serial "$COM_PORT" -bin "$MESH_CLI" -exporter "$EXPORTER"
}

# Always return the node to its original preset, on any exit path, so a failure
# partway through never leaves the device relocated.
restore() { log "RESTORE: return the node to ${HOLD_PRESET}"; reconcile "$HOLD_PRESET" || true; }
trap restore EXIT

# sense runs one integrated sweep on the sensor host while keying a short mesh
# burst so the mesh is on the air, then reduces it to the band's summary line
# with nephmeshctl. It prints just the target band's text line.
sense() {
    _csv="$(mktemp)"
    ssh -o BatchMode=yes "$SENSOR_SSH" \
        "nohup sh -c 'timeout 30 hackrf_sweep -f ${FREQ_MIN}:${FREQ_MAX} -w 1000000 > /tmp/cl_sweep.csv 2>/dev/null' >/dev/null 2>&1 & echo sweeping" >/dev/null
    _n=1
    while [ "$_n" -le 6 ]; do
        "$MESH_CLI" --port "$COM_PORT" --sendtext "closed-loop-sense-$_n" >/dev/null 2>&1 || true
        _n=$((_n + 1))
    done
    sleep 6
    ssh -o BatchMode=yes "$SENSOR_SSH" 'cat /tmp/cl_sweep.csv' > "$_csv" 2>/dev/null
    "$NEPHMESHCTL" spectrum -f "$_csv" -o text | grep "^${BAND}" || true
    rm -f "$_csv"
}

# Field extractors from a band line like:
#   ism-915-us  902.000-928.000 MHz: ... peak -19.4 dB @ 906.500 MHz
peakDbmOf() { printf '%s' "$1" | grep -oE 'peak -?[0-9.]+ dB' | grep -oE '\-?[0-9.]+' | head -1; }
freqMhzOf() { printf '%s' "$1" | grep -oE '@ [0-9.]+ MHz' | grep -oE '[0-9.]+' | head -1; }

log "SENSE: sweep ${FREQ_MIN}-${FREQ_MAX} MHz with the mesh on the air"
SENSE_LINE="$(sense)"
PEAK="$(peakDbmOf "$SENSE_LINE")"
PEAKF="$(freqMhzOf "$SENSE_LINE")"
if [ -z "$PEAK" ]; then
    printf 'sense failed: no %s reading (is the sweep tool and the band covered?)\n' "$BAND" >&2
    exit 1
fi
printf 'sensed %s: peak %s dB at %s MHz\n' "$BAND" "$PEAK" "${PEAKF:-?}"

log "DECIDE: is the channel active (peak > ${THRESHOLD_DBM} dB)?"
ACTIVE="$(awk -v p="$PEAK" -v t="$THRESHOLD_DBM" 'BEGIN { print (p+0 > t+0) ? "yes" : "no" }')"
if [ "$ACTIVE" != "yes" ]; then
    printf 'channel is quiet (peak %s <= %s dB); HOLD, no actuation.\n' "$PEAK" "$THRESHOLD_DBM"
    exit 0
fi
printf 'channel is active (peak %s > %s dB); ACTUATE: move the mesh to %s.\n' "$PEAK" "$THRESHOLD_DBM" "$BUSY_PRESET"

log "ACTUATE: operator reconciles the T-Deck to ${BUSY_PRESET}"
reconcile "$BUSY_PRESET"

log "VERIFY: sense again; the mesh should have moved frequency"
VERIFY_LINE="$(sense)"
VPEAKF="$(freqMhzOf "$VERIFY_LINE")"
printf 'after actuation, %s peak is at %s MHz (was %s MHz)\n' "$BAND" "${VPEAKF:-?}" "${PEAKF:-?}"

log "closed loop complete: sensed, decided, actuated, verified (restore runs next)"
