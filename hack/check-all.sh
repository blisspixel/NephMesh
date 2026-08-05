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

# One command that runs every gate, for any agent or human, before committing.
# A group whose tooling is absent is skipped with a clear message rather than
# failing, so this is safe to run on any machine. Run from the repo root.

set -u

ROOT=$(pwd)
fail=0
group() { printf '\n== %s\n' "$1"; }
note()  { printf '   %s\n' "$1"; }

group "repo gates"
for g in check-headers check-style check-manifests check-transmit; do
    sh "$ROOT/hack/$g.sh" || fail=1
done

group "go modules"
if command -v go >/dev/null 2>&1; then
    for m in api operators/meshtastic-operator; do
        note "module: $m"
        (cd "$m" && go build ./... && go vet ./... && go test ./... >/dev/null) || fail=1
        (cd "$m" && sh "$ROOT/hack/check-coverage.sh" 80) || fail=1
        if command -v golangci-lint >/dev/null 2>&1; then
            (cd "$m" && golangci-lint run ./...) || fail=1
        else
            note "golangci-lint not installed; lint skipped"
        fi
    done
else
    note "go not installed; Go modules skipped"
fi

group "kpt packages"
if command -v docker >/dev/null 2>&1; then
    sh "$ROOT/hack/check-packages.sh" || fail=1
else
    note "docker not installed; package render skipped"
fi

printf '\n'
if [ "$fail" -eq 0 ]; then
    echo "check-all: OK"
else
    echo "check-all: FAILED"
    exit 1
fi
