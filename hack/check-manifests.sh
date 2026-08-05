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

# Deterministic security gate for Kubernetes manifests, enforcing the
# threat model's "never expose the control surface" boundary. The device
# API (4403) and MQTT (1883) are unauthenticated and must stay
# cluster-internal.
#
# Scope: every tracked and every not-yet-committed YAML in the repo (not
# just demo/ and packages/), because a manifest is a manifest wherever it
# lives, and a contributor's new file must be checked before it is added.
#
# Matching is intentionally liberal: it accepts quoting and both block and
# flow YAML styles, since an attacker (or an honest mistake) is not obliged
# to use the tidy form. False positives are preferable to a bypass; a
# legitimate exception is a reviewed change to this script, not a quiet
# reformat that slips past it.

set -eu

# Tracked YAML plus not-yet-committed YAML (untracked, respecting
# .gitignore), deduplicated. This closes the "passes locally until git
# add" gap.
list_yaml() {
    {
        git ls-files -- '*.yaml' '*.yml'
        git ls-files --others --exclude-standard -- '*.yaml' '*.yml'
    } | sort -u
}

files=$(list_yaml)
[ -z "$files" ] && { echo "check-manifests: no manifests yet, OK"; exit 0; }

fail=0
flag() { echo "  $1"; fail=1; }

for f in $files; do
    # Expose-the-workload predicates. Each allows optional quote/brace and
    # surrounding whitespace so "NodePort", { type: NodePort }, and block
    # style all match.
    if grep -Eiq '(^|[[:space:],{])type:[[:space:]]*"?(NodePort|LoadBalancer)"?' "$f"; then
        flag "$f: Service type NodePort/LoadBalancer (control surface must stay ClusterIP)"
    fi
    if grep -Eiq '(^|[[:space:],{])externalIPs:' "$f"; then
        flag "$f: Service externalIPs (routes the control surface off-cluster)"
    fi
    if grep -Eiq '(^|[[:space:],{])hostPort:' "$f"; then
        flag "$f: hostPort (binds a port on the node, bypassing cluster isolation)"
    fi
    if grep -Eiq '(^|[[:space:],{])(hostNetwork|hostPID|hostIPC):[[:space:]]*"?true"?' "$f"; then
        flag "$f: host namespace sharing (exposes node network/process space)"
    fi
    if grep -Eiq '(^|[[:space:],{])privileged:[[:space:]]*"?true"?' "$f"; then
        flag "$f: privileged container (must be an explicit, reviewed deviation)"
    fi
    if grep -Eiq '(^|[[:space:],{])allowPrivilegeEscalation:[[:space:]]*"?true"?' "$f"; then
        flag "$f: allowPrivilegeEscalation true"
    fi
    if grep -Eiq '(^|[[:space:],{])hostPath:' "$f"; then
        flag "$f: hostPath volume (reaches into the node filesystem)"
    fi
    if grep -Eiq '(SYS_ADMIN|NET_ADMIN|NET_RAW|SYS_RAWIO|SYS_PTRACE)' "$f"; then
        flag "$f: dangerous Linux capability requested"
    fi
    # Ingress and Gateway API resources front a Service to the outside; the
    # control surface must never be behind one.
    if grep -Eiq '^kind:[[:space:]]*"?(Ingress|TCPRoute|UDPRoute|Gateway)"?' "$f"; then
        flag "$f: Ingress/Gateway resource (must not front the control surface)"
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "check-manifests: FAILED"
    exit 1
fi
echo "check-manifests: OK"
