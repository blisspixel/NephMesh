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

# Fail if any GitHub Action is pinned to a mutable tag or branch instead of a
# full 40-character commit SHA. A tag like @v4 is a supply-chain risk: whoever
# controls the action's repo can move the tag to malicious code that then runs
# with the workflow's token. The SHA is the trust anchor; the version in the
# trailing "# vX.Y.Z" comment is documentation. Dependabot keeps both current
# (see .github/dependabot.yml). Local composite actions (./path) are exempt.
set -eu

dir=".github/workflows"
[ -d "$dir" ] || { echo "check-actions: no $dir, nothing to check"; exit 0; }

violations=$(
  for f in "$dir"/*.yaml "$dir"/*.yml; do
    [ -f "$f" ] || continue
    awk -v file="$f" '
      /uses:/ {
        line = $0
        sub(/.*uses:[ \t]*/, "", line)
        gsub(/["'\'']/, "", line)
        split(line, tok, /[ \t]+/)
        spec = tok[1]
        if (spec == "" || spec ~ /^\.\// || spec ~ /^docker:\/\//) next
        ref = ""
        if (index(spec, "@") > 0) {
          n = split(spec, parts, "@")
          ref = parts[n]
        }
        if (ref !~ /^[0-9a-f]{40}$/) {
          printf "%s:%d: %s\n", file, FNR, spec
        }
      }
    ' "$f"
  done
)

if [ -n "$violations" ]; then
  echo "Unpinned GitHub Actions (pin each to a 40-character commit SHA):"
  echo "$violations"
  exit 1
fi

# Every workflow must declare top-level permissions. A missing declaration
# inherits the repository default token scope, which is usually broader than
# the workflow needs; scoping it down limits blast radius on compromise.
missing_perms=$(
  for f in "$dir"/*.yaml "$dir"/*.yml; do
    [ -f "$f" ] || continue
    grep -qE '^permissions:' "$f" || echo "$f"
  done
)

if [ -n "$missing_perms" ]; then
  echo "Workflows missing a top-level 'permissions:' block (declare least privilege):"
  echo "$missing_perms"
  exit 1
fi

echo "check-actions: OK"
