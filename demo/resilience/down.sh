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

# Tear down the resilience harness: every simulated node, the helper, the
# network, and the per-node volumes. Safe to run repeatedly.

set -eu

NET="${MESH_NET:-meshsim}"
export MSYS_NO_PATHCONV=1

# Remove every node container attached to the network, the helper, the operator
# (created by independence.sh), and any leftover sim* containers/volumes
# regardless of how many nodes were started.
docker rm -f meshcli operator >/dev/null 2>&1 || true
for c in $(docker ps -a --filter "name=sim" --format '{{.Names}}'); do
    case "$c" in
        sim[0-9]|sim[0-9][0-9]|sim[0-9][0-9][0-9]) docker rm -f "$c" >/dev/null 2>&1 || true ;;
    esac
done
for v in $(docker volume ls --filter "name=mesh" --format '{{.Name}}'); do
    case "$v" in
        mesh[0-9]|mesh[0-9][0-9]|mesh[0-9][0-9][0-9]) docker volume rm "$v" >/dev/null 2>&1 || true ;;
    esac
done
docker network rm "$NET" >/dev/null 2>&1 || true
echo "resilience harness torn down"
