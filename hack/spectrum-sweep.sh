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

# Receive-only spectrum sweep via hackrf_sweep. The HackRF (One or Pro) is
# receive-only in this project (see docs/security/threat-model.md and the
# transmit interlock in hack/check-transmit.sh); hackrf_sweep only receives, it
# never keys the transmitter. It prints rtl_power/hackrf_sweep CSV to stdout,
# which `nephmeshctl spectrum` reduces to per-band occupancy.
#
# Usage:
#   sh hack/spectrum-sweep.sh [freq_min_mhz] [freq_max_mhz] [bin_width_hz] [seconds] > sweep.csv
#
# Defaults integrate the US 915 MHz ISM band (902-928 MHz) at 1 MHz bins for 3
# seconds. Integrating matters: validated against a real HackRF, a single pass
# gave a noisy occupancy (30-46% swing, 8 dB noise-floor jitter), while a few
# seconds of sweeps settled to a stable estimate. Pass 0 seconds for a single
# pass.
#   sh hack/spectrum-sweep.sh > sweep.csv
#   nephmeshctl spectrum -f sweep.csv -o text
#
# A wider survey of the common ISM ranges, integrated for 5 seconds:
#   sh hack/spectrum-sweep.sh 400 960 1000000 5 > survey.csv
#
# See docs/guides/spectrum-validation.md for the full runbook, including the
# Linux-host policy for USB SDRs.

set -eu

FMIN=${1:-902}
FMAX=${2:-928}
BINW=${3:-1000000}
SECONDS_INTEGRATE=${4:-3}

if ! command -v hackrf_sweep >/dev/null 2>&1; then
    echo "hackrf_sweep not found; install the HackRF host tools (e.g. apt install hackrf)" >&2
    exit 1
fi
if ! hackrf_info >/dev/null 2>&1; then
    echo "no HackRF detected; check the USB connection and host udev rules (53-hackrf.rules)" >&2
    exit 1
fi

# -f low:high in MHz, -w bin width in Hz. All receive-only.
if [ "${SECONDS_INTEGRATE}" -le 0 ] 2>/dev/null; then
    echo "sweeping ${FMIN}-${FMAX} MHz at ${BINW} Hz bins (receive-only, one pass)" >&2
    # -1 does one sweep then exits.
    exec hackrf_sweep -f "${FMIN}:${FMAX}" -w "${BINW}" -1
fi

echo "sweeping ${FMIN}-${FMAX} MHz at ${BINW} Hz bins for ${SECONDS_INTEGRATE}s (receive-only, integrating)" >&2
# Integrate many passes over a window, then stop. timeout returns 124 when it
# ends the run, which is the normal, successful case here.
if command -v timeout >/dev/null 2>&1; then
    timeout "${SECONDS_INTEGRATE}" hackrf_sweep -f "${FMIN}:${FMAX}" -w "${BINW}" || {
        ec=$?
        [ "$ec" -eq 124 ] && exit 0
        exit "$ec"
    }
else
    # No timeout available: fall back to a single pass rather than run forever.
    echo "timeout(1) not found; falling back to a single pass" >&2
    exec hackrf_sweep -f "${FMIN}:${FMAX}" -w "${BINW}" -1
fi
