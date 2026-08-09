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

# Install the receive-only LoRa decode toolchain on a sensor host: GNU Radio, a
# HackRF source (SoapySDR/gr-osmosdr), and gr-lora_sdr (the LoRa PHY decoder).
# This turns the SDR from an energy sensor into a receiver that can read
# Meshtastic packets off the air, an independent, out-of-band witness of the
# mesh. It only receives; nothing here transmits.
#
# Usage, on the sensor host (Ubuntu 22.04 arm64 tested):
#   bash install-lora-decode.sh
#
# It uses sudo for the package installs and the library install, and prompts for
# the password once. The gr-lora_sdr build takes several minutes.

set -eu

GR_LORA_REF="${GR_LORA_REF:-master}"

log() { printf '\n>>> %s\n' "$1"; }
die() { printf 'ERROR: %s\n' "$1" >&2; exit 1; }

log "Installing GNU Radio, the HackRF source, and build tools (sudo)"
sudo apt-get update
# gnuradio + dev headers, the osmosdr/SoapySDR HackRF source, and the build deps
# gr-lora_sdr needs (cmake, a compiler, and its libraries).
sudo apt-get install -y \
    gnuradio gnuradio-dev \
    gr-osmosdr soapysdr-tools soapysdr-module-hackrf libhackrf-dev \
    cmake build-essential pkg-config git \
    libsndfile1-dev liblog4cpp5-dev libgmp-dev

log "Verifying GNU Radio and the HackRF source"
gnuradio-config-info --version || die "gnuradio did not install"
SoapySDRUtil --find 2>/dev/null | grep -qi hackrf && echo "HackRF visible to SoapySDR" || \
    echo "note: HackRF not seen by SoapySDR yet (plug it in / replug); decode needs it"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

log "Building gr-lora_sdr (${GR_LORA_REF}) from source"
git clone --depth 1 --branch "${GR_LORA_REF}" https://github.com/tapparelj/gr-lora_sdr.git "$WORK/gr-lora_sdr" \
    || die "clone gr-lora_sdr failed; check the ref at https://github.com/tapparelj/gr-lora_sdr"
mkdir -p "$WORK/gr-lora_sdr/build"
cd "$WORK/gr-lora_sdr/build"
cmake .. -DCMAKE_INSTALL_PREFIX=/usr/local >/dev/null
make -j"$(nproc)"
log "Installing gr-lora_sdr (sudo)"
sudo make install
sudo ldconfig

log "Checking the module imports"
if python3 -c "import gnuradio.lora_sdr; print('gr-lora_sdr import OK')"; then
    log "Done. The receive-only LoRa decode toolchain is installed."
    echo "Next: run the decoder (hack/lora-decode.py) tuned to the mesh's preset and channel frequency."
else
    die "gr-lora_sdr installed but does not import; check PYTHONPATH / the install prefix"
fi
