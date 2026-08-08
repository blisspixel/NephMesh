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

# Enforces the transmit interlock (docs/security/threat-model.md): the
# project is receive-only by default, and any code path that could key a
# radio transmitter must be a deliberate, reviewed, opted-in exception,
# never something that slips in unnoticed.
#
# The check greps tracked source for known radio-transmit invocations and
# fails unless the line carries an explicit "transmit-ok: <reason>" marker
# that a reviewer had to write. This turns the threat model's central
# principle from prose into a wire.
#
# In scope (genuine RF transmit):
#   - SDR transmit tools: hackrf_transfer in TX mode, SoapySDR TX streams.
#     The HackRF is receive-only in this project; a TX call is critical.
#   - Programmatic radio-power or region escalation, which can move a radio
#     into an illegal or higher-power transmit mode.
# Out of scope: application-layer mesh messaging (--sendtext). That is the
# purpose of a messaging network and is harmless against a simulated node,
# so it is deliberately not matched.

set -eu

# Narrow on purpose: over-broad patterns train reviewers to rubber-stamp. Both
# hackrf_transfer TX modes are covered: -t (transmit from file) and -c (signal
# source / continuous-wave), which also keys the transmitter.
patterns='hackrf_transfer[^|]*-[tc]|SOAPY_SDR_TX|writeStream|--set-ham|--transmit|txPower|tx_power'

# Scan compiled-language sources and build files too, so a TX call in a Dockerfile
# RUN, a Makefile, or C/C++/Rust is not invisible to the gate.
files=$(git ls-files -- '*.go' '*.sh' '*.py' '*.yaml' '*.yml' '*.c' '*.cc' '*.cpp' '*.cxx' '*.h' '*.rs' '*Dockerfile*' '*Makefile*' 2>/dev/null || true)
[ -z "$files" ] && { echo "check-transmit: no source yet, OK"; exit 0; }

fail=0
for f in $files; do
    case "$f" in
        hack/check-transmit.sh) continue ;;  # names the patterns as data
    esac
    unmarked=$(grep -EnI "$patterns" "$f" 2>/dev/null | grep -v 'transmit-ok:' || true)
    if [ -n "$unmarked" ]; then
        echo "  $f: unmarked transmit entry point(s):"
        echo "$unmarked" | sed 's/^/    /'
        fail=1
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "check-transmit: FAILED (remove the call, or add a reviewed 'transmit-ok: <reason>' marker)"
    exit 1
fi
echo "check-transmit: OK"
