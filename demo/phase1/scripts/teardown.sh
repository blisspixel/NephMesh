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

# Removes everything the Phase 1 demo created, including the namespace
# (and with it the PVC, so node identity is intentionally destroyed).

set -eu

NS=nephmesh
DIR=$(dirname "$0")/../manifests

# Same guard as demo.sh: do not delete namespace/nephmesh on a cloud cluster
# just because kubectl is pointed there.
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

kubectl -n "$NS" delete pod mqtt-watch sender --ignore-not-found
kubectl delete -f "$DIR" --ignore-not-found
kubectl wait --for=delete "namespace/$NS" --timeout=180s 2>/dev/null || true
echo "teardown complete; namespace $NS removed"
