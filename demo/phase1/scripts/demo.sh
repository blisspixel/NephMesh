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

# In-container paths passed to kubectl exec (for example /tmp/sub.log) are
# wrapped inside sh -c "..." strings rather than passed as bare arguments,
# so Git Bash on Windows does not rewrite them into host paths. This keeps
# the script correct on Linux, macOS, and Git Bash without a global
# MSYS_NO_PATHCONV, which would break docker build context paths.

NS=nephmesh
DIR=$(dirname "$0")/../manifests
MSG="hello nephmesh $(date +%s)"

step() { printf '\n== %s\n' "$1"; }

# Refuse to apply to a cloud or production-looking context unless the caller
# names it. kind/k3d/k3s/minikube/desktop are the $0 path; a stray kubeconfig
# pointed at GKE/EKS/AKS must not get a nephmesh namespace.
require_safe_kube_context() {
    ctx=$(kubectl config current-context 2>/dev/null || true)
    if [ -z "$ctx" ]; then
        echo "kubectl has no current context" >&2
        exit 1
    fi
    echo "kubectl context: $ctx"
    if [ -n "${NEPHMESH_CONTEXT:-}" ] && [ "$ctx" != "$NEPHMESH_CONTEXT" ]; then
        echo "refusing: current context '$ctx' != NEPHMESH_CONTEXT='$NEPHMESH_CONTEXT'" >&2
        exit 1
    fi
    case "$ctx" in
        *prod*|*Prod*|*PROD*|gke_*|arn:aws:*|*eks*|*aks*)
            if [ "${NEPHMESH_ALLOW_CONTEXT:-}" != "1" ]; then
                echo "refusing to change cluster '$ctx' (looks like a cloud or production context)." >&2
                echo "set NEPHMESH_CONTEXT=$ctx and NEPHMESH_ALLOW_CONTEXT=1 if this is intentional." >&2
                exit 1
            fi
            ;;
    esac
}

require_safe_kube_context

step "0/6 build and load the pinned CLI image (no runtime PyPI dependency)"
sh "$(dirname "$0")/build-cli-image.sh"

step "1/6 apply manifests"
# The namespace goes first: kubectl apply -f <dir> processes files in
# alphabetical order, and everything else lands inside the namespace.
kubectl apply -f "$DIR/namespace.yaml"
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
# Restart the node before sending. The config-apply reboot churn leaves the
# meshtasticd sim's single-client device API in an unreliable state; a fresh
# node comes up with a clean listener and, importantly, re-establishes its
# MQTT client at boot (the module thread only starts at boot). The node
# reloads its applied config from the PVC, so it comes back configured.
kubectl -n "$NS" rollout restart deploy/meshnode-sim
kubectl -n "$NS" rollout status deploy/meshnode-sim --timeout=180s
# The subscriber restarts mosquitto_sub if it drops (the client can emit a
# transient bad-descriptor error), appending to a file we read later. This
# removes the race between a one-shot subscriber and the sender.
kubectl -n "$NS" run mqtt-watch --image=eclipse-mosquitto:2 --restart=Never -- \
    sh -c "while true; do mosquitto_sub -h mosquitto -t 'msh/#' -v >>/tmp/sub.log 2>&1; sleep 1; done"
kubectl -n "$NS" wait --for=condition=Ready pod/mqtt-watch --timeout=120s
# The sender waits for the device API to accept a connection before sending,
# the same reachability poll the config Job uses, then sends exactly once
# (the API is single-client and dislikes rapid reconnection). One send is
# enough because the subscriber is already listening and dedupes nothing.
kubectl -n "$NS" run sender --image=nephmesh/meshtastic-cli:2.7.11 \
    --image-pull-policy=IfNotPresent --restart=Never --command -- \
    python -c "
import socket, subprocess, sys, time
host = 'meshnode-sim'
for _ in range(60):
    try:
        socket.create_connection((host, 4403), timeout=5).close()
        break
    except OSError:
        time.sleep(3)
else:
    sys.exit('device API never became reachable')
sys.exit(subprocess.run(['meshtastic', '--host', host, '--sendtext', '$MSG']).returncode)
"
kubectl -n "$NS" wait --for=jsonpath='{.status.phase}'=Succeeded pod/sender --timeout=180s

step "6/6 verify the message reached MQTT"
# grep -a: the protobuf topics carry binary payloads on the same stream.
tries=0
until kubectl -n "$NS" exec mqtt-watch -- sh -c "grep -aF '$MSG' /tmp/sub.log" >/dev/null 2>&1; do
    tries=$((tries + 1))
    if [ "$tries" -gt 20 ]; then
        echo "GATE FAILED: message not seen on MQTT within 60s"
        kubectl -n "$NS" exec mqtt-watch -- sh -c "cat /tmp/sub.log" 2>&1 | tail -20
        exit 1
    fi
    sleep 3
done
echo "message observed on MQTT topics:"
kubectl -n "$NS" exec mqtt-watch -- sh -c "grep -aF '$MSG' /tmp/sub.log" | head -4

printf '\nPHASE 1 GATE: PASS\n'
printf 'Teardown with: sh %s/../scripts/teardown.sh\n' "$(dirname "$0")/../manifests"
