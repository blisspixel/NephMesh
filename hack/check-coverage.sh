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

# Enforces a minimum test coverage of meaningful code. Run from a Go module
# directory. Generated deepcopy files and the thin cmd/main entrypoint are
# excluded from the denominator so the gate measures hand-written logic, not
# machine-generated boilerplate or the manager wiring that only an integration
# test can exercise.
#
# Usage: sh hack/check-coverage.sh [min-percent]   (default 80)

set -eu

MIN=${1:-80}
prof=$(mktemp)
filtered=$(mktemp)
trap 'rm -f "$prof" "$filtered"' EXIT

go test ./... -coverprofile="$prof" >/dev/null

# Keep the mode line. Drop generated deepcopy and thin cmd main entrypoints
# (cmd/main.go, cmd/<name>/main.go). Other cmd files (apply_spec.go) stay in
# the denominator so their tests count.
head -1 "$prof" > "$filtered"
grep -v -E 'zz_generated|/cmd/[^/]+/main\.go|/cmd/main\.go' "$prof" | tail -n +2 >> "$filtered" || true

pct=$(go tool cover -func="$filtered" | awk '/^total:/ {gsub(/%/,"",$3); print $3}')
[ -z "$pct" ] && { echo "check-coverage: no coverage measured"; exit 1; }

meets=$(awk -v p="$pct" -v m="$MIN" 'BEGIN { print (p+0 >= m+0) ? "yes" : "no" }')
if [ "$meets" != "yes" ]; then
    echo "check-coverage: FAILED (meaningful coverage ${pct}% < ${MIN}%)"
    go tool cover -func="$filtered" | grep -v '100.0%' | tail -20
    exit 1
fi
echo "check-coverage: OK (${pct}% >= ${MIN}%)"
