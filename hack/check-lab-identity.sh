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

# Fail if tracked or staged-to-commit text names lab identity: LAN addresses,
# hostnames, hardware node ids, personal home paths, or USB bus/dev from a
# particular plug-in. Product-class names (a Jetson, a T-Deck) stay legal;
# "the USB host in this lab" does not. Screenshots are binary and are not
# scanned here: recapture or redact them by hand if a node id appears.

set -eu

# This script carries the forbidden patterns as data; never scan it.
# Patterns are specific lab fingerprints, not RFC1918 or COM-port examples.
patterns='192\.168\.44|kilo@|!0c3a5f2c|!00439107|C:\\\\Users\\\\nicks|C:/Users/nicks|/home/kilo|nvpmodel|jetson clocks|text-generation-webui|001/005'

fail=0

list_files() {
    {
        git ls-files
        git ls-files --others --exclude-standard
    } | sort -u
}

for f in $(list_files); do
    case "$f" in
        hack/check-lab-identity.sh) continue ;;
    esac
    [ -f "$f" ] || continue
    grep -Iq . -- "$f" 2>/dev/null || continue
    if grep -nEi "$patterns" -- "$f" >/dev/null 2>&1; then
        echo "lab identity found in: $f"
        grep -nEi "$patterns" -- "$f" || true
        fail=1
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "check-lab-identity: FAILED (hostnames, LAN addresses, device ids, and personal paths stay out of the repo)"
    exit 1
fi
echo "check-lab-identity: OK"
