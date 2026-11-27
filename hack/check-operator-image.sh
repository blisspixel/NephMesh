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

# Audit both platform images from the exact OCI index being considered for release.
# Requires Docker (including arm64 execution), regctl, jq and pip-audit on PATH.
set -eu

archive=${1:?usage: check-operator-image.sh operator.tar}
tmp=$(mktemp -d)
layout="ocidir://$tmp/layout:candidate"
tag="nephmesh-image-check:$$"
cleanup() {
    docker image rm "$tag" >/dev/null 2>&1 || :
    rm -rf "$tmp"
}
trap cleanup EXIT
regctl image import "$layout" "$archive"
regctl manifest get "$layout" --format raw-body > "$tmp/index.json"
trivy=aquasec/trivy:0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb

for arch in amd64 arm64; do
    platform="linux/$arch"
    digest=$(regctl image digest --platform "$platform" "$layout")
    attestation=$(jq -er --arg digest "$digest" '
        .manifests[] | select(.annotations["vnd.docker.reference.digest"] == $digest)
        | .digest' "$tmp/index.json")
    regctl manifest get "${layout%:*}@$attestation" --format raw-body > "$tmp/evidence.json"
    jq -e '[.layers[].annotations["in-toto.io/predicate-type"]] |
        any(. == "https://spdx.dev/Document") and any(startswith("https://slsa.dev/provenance/"))' \
        "$tmp/evidence.json" >/dev/null

    regctl image export --platform "$platform" --name "$tag" "$layout" "$tmp/platform.tar"
    docker load -i "$tmp/platform.tar" >/dev/null
    test "$(docker image inspect "$tag" --format '{{.Os}}/{{.Architecture}}')" = "$platform"
    docker run --rm --network none --read-only --platform "$platform" "$tag" --help >/dev/null 2>&1
    docker run --rm --network none --read-only --platform "$platform" --entrypoint meshtastic "$tag" --version
    docker run --rm --network none --read-only --platform "$platform" --entrypoint python3 "$tag" -c '
from importlib import metadata
from pathlib import Path
import os
import meshtastic.tcp_interface
assert os.getuid() == 65532
for name in ("mesh-apply.py", "mesh-export.py"):
    assert (Path("/usr/local/lib/nephmesh") / name).is_file()
for package in metadata.distributions():
    print("{}=={}".format(package.metadata["Name"], package.version))
' > "$tmp/installed.txt"
    pip-audit --disable-pip --no-deps -r "$tmp/installed.txt"
    docker run --rm -v "$tmp:/image:ro" -v nephmesh-trivy-cache:/root/.cache/trivy "$trivy" \
        image --input /image/platform.tar --scanners vuln --severity HIGH,CRITICAL --exit-code 1 --no-progress
done
