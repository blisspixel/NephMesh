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

# Check actual PR contributions, not the unsigned merge created by GitHub.
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: check-dco.sh <PR base SHA> <PR head SHA>" >&2
    exit 1
fi
base=$1
head=$2
for sha in "$base" "$head"; do
    case "$sha" in
        ''|*[!0-9a-f]*) echo "invalid PR commit SHA: $sha" >&2; exit 1 ;;
    esac
    if [ "${#sha}" -ne 40 ] || [ "$(git cat-file -t "$sha" 2>/dev/null)" != commit ]; then
        echo "PR commit object unavailable: $sha" >&2
        exit 1
    fi
done

commits=$(git rev-list "$base..$head")
missing=0
for sha in $commits; do
    if ! git log -1 --format=%B "$sha" | grep -q '^Signed-off-by: '; then
        echo "commit $sha lacks Signed-off-by"
        missing=1
    fi
done
exit "$missing"
