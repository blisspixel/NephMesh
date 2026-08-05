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

# Renders every kpt package and fails if any pipeline errors. Rendering is
# done on a throwaway copy so the authored source stays pre-render (with its
# blueprint-default namespaces), which is what a blueprint repo should hold.
#
# Runtime: uses kpt from PATH when present (Linux, CI). Otherwise falls back
# to the pinned nephmesh/kpt-runner image with the Docker socket mounted, so
# a Windows or macOS developer gets the identical toolchain. kpt runs its KRM
# functions as containers, so a working Docker daemon is required either way.

set -eu

KPT_RUNNER_IMAGE=${KPT_RUNNER_IMAGE:-nephmesh/kpt-runner:v1.0.0-beta.67}

pkgs=$(find packages -name Kptfile -exec dirname {} \; 2>/dev/null | sort)
[ -z "$pkgs" ] && { echo "check-packages: no packages yet, OK"; exit 0; }

render_native() {
    tmp=$(mktemp -d)
    cp -r "$1" "$tmp/pkg"
    kpt fn render "$tmp/pkg" >/dev/null
    rm -rf "$tmp"
}

render_container() {
    # Render in place inside the container against a copied /work/pkg, so the
    # host source is untouched. MSYS_NO_PATHCONV keeps Git Bash from rewriting
    # the in-container paths.
    winpwd=$(pwd -W 2>/dev/null || pwd)
    MSYS_NO_PATHCONV=1 docker run --rm \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v "$winpwd":/src:ro \
        --entrypoint sh \
        "$KPT_RUNNER_IMAGE" -c \
        "cp -r /src/$1 /work/pkg && kpt fn render /work/pkg >/dev/null"
}

if command -v kpt >/dev/null 2>&1; then
    runner=render_native
else
    runner=render_container
fi

fail=0
for p in $pkgs; do
    if $runner "$p"; then
        echo "  rendered OK: $p"
    else
        echo "  render FAILED: $p"
        fail=1
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "check-packages: FAILED"
    exit 1
fi
echo "check-packages: OK"
