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

# Enforces the AGENTS.md writing rules: no em dashes, no emojis, no AI
# attribution trailers. It scans tracked files AND not-yet-committed files
# (untracked, respecting .gitignore), so a brand-new file is checked before it
# is committed rather than passing locally and failing in CI once tracked. This
# script exempts itself (it contains the forbidden patterns as data).

set -eu

fail=0

# Em dash (U+2014) and the robot emoji, built with printf so this file does not
# contain them literally.
emdash=$(printf '\342\200\224')
robot=$(printf '\360\237\244\226')

# Tracked plus not-yet-committed files, deduplicated. This closes the "passes
# locally until git add" gap for new files.
list_files() {
    {
        git ls-files
        git ls-files --others --exclude-standard
    } | sort -u
}

for f in $(list_files); do
    # This script carries the forbidden patterns as data; never scan it.
    case "$f" in
        hack/check-style.sh) continue ;;
    esac
    [ -f "$f" ] || continue
    # Skip binary files (images, fonts): grep -I finds no text lines in them, so
    # their bytes cannot false-positive against the emoji ranges below.
    grep -Iq . -- "$f" 2>/dev/null || continue

    if grep -qF "$emdash" -- "$f" 2>/dev/null; then
        echo "em dash found in: $f"
        fail=1
    fi

    # AI attribution. Patterns are the concrete trailer forms, not bare tool
    # names, so factual mentions of AI tooling in docs stay legal.
    for pat in 'Co-Authored-By:' 'Generated with [' "$robot"; do
        if grep -qF "$pat" -- "$f" 2>/dev/null; then
            echo "forbidden attribution pattern found in: $f"
            fail=1
        fi
    done

    # Common emoji blocks, checked with perl when available (CI has it).
    if command -v perl >/dev/null 2>&1; then
        if perl -CSD -ne 'exit 1 if /[\x{1F300}-\x{1FAFF}\x{2700}-\x{27BF}\x{2B00}-\x{2BFF}\x{FE0F}]/' "$f" 2>/dev/null; then :; else
            echo "emoji found: $f"
            fail=1
        fi
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "check-style: FAILED"
    exit 1
fi
echo "check-style: OK"
