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

# The cleanup of a temporary package must preserve a failing renderer's status.
set -eu

checker=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)/check-packages.sh
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
cd "$fixture"
mkdir -p packages/invalid
printf 'invalid: [\n' > packages/invalid/Kptfile
if sh "$checker" > render.log 2>&1; then
    echo 'check-packages accepted an invalid Kptfile' >&2
    exit 1
fi
grep -qF 'render FAILED: packages/invalid' render.log
echo 'test-check-packages: invalid package refused'
