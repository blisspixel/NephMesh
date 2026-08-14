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

# Builds the pinned Meshtastic CLI image the demo uses, and loads it into
# the local cluster so no pod pulls from PyPI at runtime. Detects kind,
# k3d, and k3s. A cluster that cannot be loaded is a hard failure: the
# demo Job would otherwise ImagePullBackOff on a host-only tag.

set -eu

IMAGE=nephmesh/meshtastic-cli:2.7.11
DIR=$(CDPATH= cd "$(dirname "$0")/../images/meshtastic-cli" && pwd)

echo "building $IMAGE"
docker build -t "$IMAGE" "$DIR"

ctx=$(kubectl config current-context 2>/dev/null || echo "")
case "$ctx" in
    kind-*)
        cluster=${ctx#kind-}
        echo "loading $IMAGE into kind cluster '$cluster'"
        kind load docker-image "$IMAGE" --name "$cluster"
        ;;
    k3d-*)
        cluster=${ctx#k3d-}
        echo "loading $IMAGE into k3d cluster '$cluster'"
        k3d image import "$IMAGE" -c "$cluster"
        ;;
    *)
        if command -v k3s >/dev/null 2>&1; then
            echo "importing $IMAGE into k3s containerd"
            docker save "$IMAGE" | sudo k3s ctr images import -
        else
            echo "context '$ctx' is not kind/k3d and k3s was not found."
            echo "Load $IMAGE into the cluster yourself, or point kubectl at kind/k3d/k3s."
            exit 1
        fi
        ;;
esac
echo "done"
