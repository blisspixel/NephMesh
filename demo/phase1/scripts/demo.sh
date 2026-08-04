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

# Phase 1 demo, the 0.1 gate. Needs only kubectl pointed at a cluster
# (kind, k3d, or k3s). Everything else runs in-cluster, so the host needs
# no Python and no Meshtastic CLI. POSIX sh; on Windows run under WSL2 or
# Git Bash.

set -eu

NS=nephmesh
DIR=$(dirname "$0")/../manifests
MSG="hello nephmesh $(date +%s)"

step() { printf '\n== %s\n' "$1"; }

step "1/6 apply manifests"
kubectl apply -f "$DIR"

step "2/6 wait for workloads"
kubectl -n "$NS" wait --for=condition=Available deploy/mosquitto --timeout=300s
kubectl -n "$NS" wait --for=condition=Available deploy/meshnode-sim --timeout=300s

step "3/6 wait for declarative config to converge (device reboots are expected)"
kubectl -n "$NS" wait --for=condition=Complete job/meshnode-configure --timeout=600s
kubectl -n "$NS" logs job/meshnode-configure | tail -5

step "4/6 idempotency: re-running the applier must be a no-op"
kubectl -n "$NS" delete job meshnode-configure
kubectl apply -f "$DIR/configure-job.yaml"
kubectl -n "$NS" wait --for=condition=Complete job/meshnode-configure --timeout=600s
if kubectl -n "$NS" logs job/meshnode-configure | grep -q "already converged"; then
    echo "idempotency: OK"
else
    echo "idempotency: FAILED (applier did not report convergence)"
    exit 1
fi

step "5/6 send a message and watch the MQTT topics"
kubectl -n "$NS" delete pod mqtt-watch sender --ignore-not-found >/dev/null 2>&1
kubectl -n "$NS" run mqtt-watch --image=eclipse-mosquitto:2 --restart=Never -- \
    sh -c "mosquitto_sub -h mosquitto -t 'msh/#' -v"
kubectl -n "$NS" wait --for=condition=Ready pod/mqtt-watch --timeout=120s
kubectl -n "$NS" run sender --image=python:3.13-slim --restart=Never -- \
    sh -c "pip install --quiet 'meshtastic[cli]' && meshtastic --host meshnode-sim --sendtext '$MSG'"
kubectl -n "$NS" wait --for=jsonpath='{.status.phase}'=Succeeded pod/sender --timeout=300s

step "6/6 verify the message reached MQTT"
tries=0
until kubectl -n "$NS" logs mqtt-watch | grep -F "$MSG" >/dev/null 2>&1; do
    tries=$((tries + 1))
    if [ "$tries" -gt 20 ]; then
        echo "GATE FAILED: message not seen on MQTT within 60s"
        kubectl -n "$NS" logs mqtt-watch | tail -20
        exit 1
    fi
    sleep 3
done
echo "message observed on MQTT topics:"
kubectl -n "$NS" logs mqtt-watch | grep -F "$MSG" | head -4

printf '\nPHASE 1 GATE: PASS\n'
printf 'Teardown with: sh %s/../scripts/teardown.sh\n' "$(dirname "$0")/../manifests"
