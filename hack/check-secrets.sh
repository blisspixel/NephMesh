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

# Assume-breach gate: a deployable manifest must never embed well-known default
# credentials. Shipping the Meshtastic default channel key means "anyone can
# read this channel"; shipping the public broker's default account (heavily
# rate-limited, world-readable) is a silent footgun. A real deployment supplies
# its own key (by Secret reference) and its own broker.
#
# Scope is deployable manifests (packages/ and demo/*/manifests/). Comment lines
# and Markdown are excluded, so the docs and code comments that legitimately
# name these defaults as facts do not trip the gate; only an actual embedded
# value fails.
set -eu

# The full default channel PSK, the public broker host, and its default account.
# The full default channel PSK (base64 and hex expansion), the 0x01 shorthand
# as a YAML psk value, the public broker host, and its default account.
# AQ== is only matched as a psk value so random base64 does not trip the gate.
patterns='1PG7OiApB1nwvP\+rz05pAQ==|d4f1bb3a20290759f0bcffabcd4e6901|psk:[[:space:]]*["'\'']?AQ==["'\'']?[[:space:]]*$|mqtt\.meshtastic\.org|meshdev|large4cats'

# Tracked plus not-yet-committed files, matching check-manifests, so a new
# manifest with a default credential fails locally before git add.
list_manifests() {
    {
        git ls-files -- 'packages' 'demo'
        git ls-files --others --exclude-standard -- 'packages' 'demo'
    } | grep -E '\.(yaml|yml)$' | grep -vE '(^|/)[^/]*\.md$' | sort -u
}

hits=""
for f in $(list_manifests); do
    [ -f "$f" ] || continue
    found=$(grep -nE "$patterns" "$f" | grep -vE '^[0-9]+:[[:space:]]*#' || true)
    if [ -n "$found" ]; then
        hits="$hits
$f:
$found"
    fi
done

if [ -n "$hits" ]; then
	echo "default credentials embedded in a deployable manifest (supply your own):"
	echo "$hits"
	exit 1
fi

echo "check-secrets: OK"
