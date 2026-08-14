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

# Build the meshtastic-operator image locally. Does not push, sign, or
# generate an SBOM. Those land with the registry publish step (roadmap
# 1.0 gap). Context is the repo root so the api module replace resolves.
#
#   sh hack/build-operator-image.sh
#   sh hack/build-operator-image.sh nephmesh-meshtastic-operator:dev

set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
TAG=${1:-nephmesh-meshtastic-operator:local}

cd "$ROOT"
docker build -f operators/meshtastic-operator/Dockerfile -t "$TAG" .
printf 'built %s (local only, not pushed)\n' "$TAG"
