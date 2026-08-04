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

# Fails if any tracked source file lacks the Apache-2.0 license header.
# Markdown, LICENSE, dotfiles, and testdata fixtures are exempt.

set -eu

fail=0
for f in $(git ls-files '*.go' '*.sh' '*.mk' 'Makefile' '*/Makefile' 'Dockerfile' '*/Dockerfile' 'demo/**/*.yaml' 'packages/**/*.yaml' '.github/**/*.yaml' '.github/**/*.yml'); do
    case "$f" in
        */testdata/*) continue ;;
    esac
    if ! head -20 "$f" | grep -q "The NephMesh Authors"; then
        echo "missing license header: $f"
        fail=1
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "check-headers: FAILED"
    exit 1
fi
echo "check-headers: OK"
