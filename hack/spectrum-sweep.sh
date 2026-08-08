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
#   sh hack/spectrum-sweep.sh [freq_min_mhz] [freq_max_mhz] [bin_width_hz] > sweep.csv
#
# Defaults sweep the US 915 MHz ISM band (902-928 MHz) at 1 MHz bins, one pass:
#   sh hack/spectrum-sweep.sh > sweep.csv
#   nephmeshctl spectrum -f sweep.csv -o text
#
# A wider survey of the common ISM ranges:
#   sh hack/spectrum-sweep.sh 400 960 1000000 > survey.csv
#
# See docs/guides/spectrum-validation.md for the full runbook, including the
# Linux-host policy for USB SDRs.

set -eu

FMIN=${1:-902}
FMAX=${2:-928}
BINW=${3:-1000000}

if ! command -v hackrf_sweep >/dev/null 2>&1; then
    echo "hackrf_sweep not found; install the HackRF host tools (e.g. apt install hackrf)" >&2
    exit 1
fi
if ! hackrf_info >/dev/null 2>&1; then
    echo "no HackRF detected; check the USB connection and host udev rules (53-hackrf.rules)" >&2
    exit 1
fi

echo "sweeping ${FMIN}-${FMAX} MHz at ${BINW} Hz bins (receive-only, one pass)" >&2
# -f low:high in MHz, -w bin width in Hz, -1 one sweep then exit. Receive-only.
exec hackrf_sweep -f "${FMIN}:${FMAX}" -w "${BINW}" -1
