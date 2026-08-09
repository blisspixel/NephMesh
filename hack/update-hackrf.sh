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

# Build and install the latest HackRF host tools from source on a sensor host
# (for example a Jetson Orin Nano), so a HackRF Pro runs natively instead of the
# legacy HackRF-One-compatible mode the distro package provides.
#
# Why: Ubuntu's apt "hackrf" is years old (2021.03) and predates HackRF Pro
# support entirely, so it reports the Pro as an "Unknown Board ID" and runs it in
# legacy mode. Great Scott Gadgets added HackRF Pro support in 2026.01.1 and fixed
# Pro spectrum inversion and tuning in 2026.01.2; the host-side fixes and the new
# board recognition come from these tools. The firmware fixes (spectrum inversion)
# need the matching firmware flashed onto the device, which is a deliberate,
# opt-in step here because flashing is a device write.
#
# Usage, on the sensor host:
#   bash update-hackrf.sh            # build and install the latest host tools
#   bash update-hackrf.sh --flash    # also flash the matching firmware (device write)
#
# It uses sudo only for the package steps and the optional flash, and prompts for
# the password once. Receive-only project note: this updates tooling and firmware;
# it does not transmit.

set -eu

# Pinned to a known release; override with HACKRF_REF=vX.Y.Z if a newer one ships.
HACKRF_REF="${HACKRF_REF:-v2026.01.3}"

FLASH=0
[ "${1:-}" = "--flash" ] && FLASH=1

log() { printf '\n>>> %s\n' "$1"; }
die() { printf 'ERROR: %s\n' "$1" >&2; exit 1; }

command -v git >/dev/null 2>&1 || true # git is installed below if missing

log "Removing the stale apt hackrf packages (the source build replaces them)"
sudo apt-get remove -y hackrf libhackrf0 2>/dev/null || true

log "Installing build dependencies (sudo)"
sudo apt-get update
sudo apt-get install -y build-essential cmake pkg-config git libusb-1.0-0-dev libfftw3-dev

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

log "Cloning greatscottgadgets/hackrf at ${HACKRF_REF}"
if ! git clone --depth 1 --branch "${HACKRF_REF}" https://github.com/greatscottgadgets/hackrf.git "$WORK/hackrf"; then
    die "could not clone tag ${HACKRF_REF}; check the tag name at https://github.com/greatscottgadgets/hackrf/releases and re-run with HACKRF_REF=<tag>"
fi

log "Building the host tools"
mkdir -p "$WORK/hackrf/host/build"
cd "$WORK/hackrf/host/build"
cmake .. -DCMAKE_INSTALL_PREFIX=/usr/local >/dev/null
make -j"$(nproc)"

log "Installing the host tools (sudo)"
sudo make install
sudo ldconfig
# Refresh udev rules the source install placed, so non-root access keeps working.
sudo udevadm control --reload-rules 2>/dev/null || true
sudo udevadm trigger 2>/dev/null || true

log "Installed host tool versions"
hash -r 2>/dev/null || true
hackrf_info 2>&1 | head -8 || true

# Stage the prebuilt firmware next to the user so a flash is one clear command.
FW_SRC="$WORK/hackrf/firmware-bin"
FW_DEST="$HOME/hackrf-firmware-${HACKRF_REF}"
if [ -d "$FW_SRC" ]; then
    mkdir -p "$FW_DEST"
    cp "$FW_SRC"/*.bin "$FW_DEST"/ 2>/dev/null || true
fi

if [ "$FLASH" -eq 1 ]; then
    log "Flashing firmware (device write)"
    # Prefer a Pro-specific image when the board reports as a Pro; otherwise stop
    # rather than flash a guess, since the wrong image can brick until DFU recovery.
    BIN=""
    if hackrf_info 2>/dev/null | grep -qi "pro"; then
        BIN="$(ls "$FW_DEST"/*pro*.bin 2>/dev/null | head -1 || true)"
    fi
    if [ -z "$BIN" ]; then
        BIN="$(ls "$FW_DEST"/hackrf_one_usb.bin 2>/dev/null | head -1 || true)"
    fi
    if [ -z "$BIN" ]; then
        log "Could not confidently pick a firmware image. Available images:"
        ls -1 "$FW_DEST" 2>/dev/null || echo "  (none staged)"
        die "flash aborted; run hackrf_spiflash -w <one of the files above> yourself after checking the board type with hackrf_info"
    fi
    printf 'About to flash: %s\nPress Enter to continue, or Ctrl-C to abort.' "$BIN"
    read -r _
    hackrf_spiflash -w "$BIN"
    log "Flash complete. Power-cycle the HackRF (unplug/replug the USB-C cable), then run hackrf_info."
else
    log "Host tools updated. To also fix the Pro firmware (spectrum inversion, tuning):"
    echo "  1) confirm the board:   hackrf_info"
    echo "  2) staged firmware in:  ${FW_DEST}"
    echo "  3) flash deliberately:  bash update-hackrf.sh --flash   (or run hackrf_spiflash -w <image> yourself)"
    echo "  Flashing is a device write; it is recoverable via DFU but do it deliberately."
fi

log "Done."
