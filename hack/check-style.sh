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

# Enforces the AGENTS.md writing rules across tracked files:
# no em dashes, no emojis, no AI attribution trailers.
# This script exempts itself (it contains the forbidden patterns as data).

set -eu

fail=0
exclude=':!hack/check-style.sh'

# Em dash (U+2014), built with printf so this file does not contain one.
emdash=$(printf '\342\200\224')
hits=$(git grep -lI "$emdash" -- . "$exclude" || true)
if [ -n "$hits" ]; then
    echo "em dashes found in:"
    echo "$hits"
    fail=1
fi

# AI attribution. Patterns are the concrete trailer forms, not bare tool
# names, so factual mentions of AI tooling in docs stay legal.
for pat in 'Co-Authored-By:' 'Generated with \[' "$(printf '\360\237\244\226')"; do
    hits=$(git grep -lI "$pat" -- . "$exclude" || true)
    if [ -n "$hits" ]; then
        echo "forbidden attribution pattern found in:"
        echo "$hits"
        fail=1
    fi
done

# Common emoji blocks, checked with perl when available (CI has it). Binary
# files (images, fonts) are skipped: git grep -I finds nothing in them, so
# their bytes cannot false-positive against the emoji ranges.
if command -v perl >/dev/null 2>&1; then
    for f in $(git ls-files | grep -v '^hack/check-style\.sh$'); do
        git grep -Iq . -- "$f" 2>/dev/null || continue
        if perl -CSD -ne 'exit 1 if /[\x{1F300}-\x{1FAFF}\x{2700}-\x{27BF}\x{2B00}-\x{2BFF}\x{FE0F}]/' "$f" 2>/dev/null; then :; else
            echo "emoji found: $f"
            fail=1
        fi
    done
fi

if [ "$fail" -ne 0 ]; then
    echo "check-style: FAILED"
    exit 1
fi
echo "check-style: OK"
