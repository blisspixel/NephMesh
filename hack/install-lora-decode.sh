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

# gr-lora_sdr must be built for the SAME Python GNU Radio uses, which is not
# necessarily the system default python3 (a box may default to a newer Python
# than the one GNU Radio's apt package installed its bindings into). Detect the
# Python that can import gnuradio and build against exactly that, so this works
# regardless of the machine's default Python.
log "Detecting the Python that GNU Radio uses"
GR_PYTHON=""
for py in python3.10 python3.11 python3.12 python3.13 python3; do
    if command -v "$py" >/dev/null 2>&1 && "$py" -c "import gnuradio" >/dev/null 2>&1; then
        GR_PYTHON="$(command -v "$py")"
        break
    fi
done
[ -n "$GR_PYTHON" ] || die "no Python can 'import gnuradio'; check the gnuradio install"
GR_PYVER="$("$GR_PYTHON" -c 'import sys; print("%d.%d" % sys.version_info[:2])')"
echo "GNU Radio uses ${GR_PYTHON} (Python ${GR_PYVER})"

# The pybind11 bindings need that Python's dev headers. Install them if the
# system default differs from GNU Radio's Python and its headers are missing.
if [ ! -f "/usr/include/python${GR_PYVER}/Python.h" ]; then
    log "Installing python${GR_PYVER}-dev headers (sudo)"
    sudo apt-get install -y "python${GR_PYVER}-dev" \
        || die "gr-lora_sdr needs headers for Python ${GR_PYVER} (the one GNU Radio uses); install python${GR_PYVER}-dev"
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

log "Building gr-lora_sdr (${GR_LORA_REF}) from source for Python ${GR_PYVER}"
git clone --depth 1 --branch "${GR_LORA_REF}" https://github.com/tapparelj/gr-lora_sdr.git "$WORK/gr-lora_sdr" \
    || die "clone gr-lora_sdr failed; check the ref at https://github.com/tapparelj/gr-lora_sdr"
mkdir -p "$WORK/gr-lora_sdr/build"
cd "$WORK/gr-lora_sdr/build"
# Pin the Python (both the modern and legacy cmake variables) and make pybind11
# use FindPython, so it locates the real headers instead of a hardcoded path
# that may point at a Python whose dev package is not installed.
cmake .. -DCMAKE_INSTALL_PREFIX=/usr/local \
    -DPython_EXECUTABLE="$GR_PYTHON" \
    -DPython3_EXECUTABLE="$GR_PYTHON" \
    -DPYTHON_EXECUTABLE="$GR_PYTHON" \
    -DPYBIND11_FINDPYTHON=ON >/dev/null
make -j"$(nproc)"
log "Installing gr-lora_sdr (sudo)"
sudo make install
sudo ldconfig

log "Checking the module imports"
if "$GR_PYTHON" -c "import gnuradio.lora_sdr; print('gr-lora_sdr import OK')"; then
    log "Done. The receive-only LoRa decode toolchain is installed."
    echo "GNU Radio's Python is ${GR_PYTHON}; run the decoder with that interpreter:"
    echo "  ${GR_PYTHON} hack/lora-decode.py --help"
else
    die "gr-lora_sdr installed but does not import under ${GR_PYTHON}; check the install prefix / PYTHONPATH"
fi
