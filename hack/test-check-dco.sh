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

set -eu

checker=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)/check-dco.sh
fixture=$(mktemp -d "${TMPDIR:-/tmp}/nephmesh-dco.XXXXXXXX")
trap 'rm -rf "$fixture"' EXIT HUP INT TERM
git init -q "$fixture"
cd "$fixture"

# These identities describe synthetic fixture objects, not a contribution.
export GIT_AUTHOR_NAME='Fixture Contributor'
export GIT_AUTHOR_EMAIL='contributor@example.invalid'
export GIT_COMMITTER_NAME='Fixture Contributor'
export GIT_COMMITTER_EMAIL='contributor@example.invalid'
tree=$(git mktree < /dev/null)
commit() { git -c commit.gpgsign=false commit-tree "$tree" "$@"; }
base=$(printf 'Existing base without a signoff\n' | commit)
signed=$(printf 'Signed contribution\n\nSigned-off-by: Fixture Contributor <contributor@example.invalid>\n' | commit -p "$base")
signed_tip=$(printf 'Another signed contribution\n\nSigned-off-by: Fixture Contributor <contributor@example.invalid>\n' | commit -p "$signed")
merge=$(printf 'Unsigned GitHub test merge\n' | commit -p "$base" -p "$signed_tip")
git update-ref refs/heads/main "$merge"
git symbolic-ref HEAD refs/heads/main
git update-ref refs/remotes/origin/main "$base"

sh "$checker" "$base" "$signed_tip"
echo 'all signed contributions with unsigned synthetic HEAD: PASS'

unsigned=$(printf 'Unsigned earlier contribution\n' | commit -p "$base")
signed_tip=$(printf 'Signed tip\n\nSigned-off-by: Fixture Contributor <contributor@example.invalid>\n' | commit -p "$unsigned")
if sh "$checker" "$base" "$signed_tip" > earlier.log 2>&1; then
    echo 'unsigned earlier contribution was accepted' >&2
    exit 1
fi
grep -qF "commit $unsigned lacks Signed-off-by" earlier.log
echo 'unsigned earlier contribution despite signed tip: refused'

if sh "$checker" "$base" "$unsigned" > tip.log 2>&1; then
    echo 'unsigned tip was accepted' >&2
    exit 1
fi
grep -qF "commit $unsigned lacks Signed-off-by" tip.log
echo 'unsigned tip: refused'

refuse() {
    if sh "$checker" "$@" > invalid.log 2>&1; then
        echo 'invalid contribution range was accepted' >&2
        exit 1
    fi
}
refuse
refuse "$base" main
refuse "$base" ''
refuse "$base" 1111111111111111111111111111111111111111
refuse "$base" "$tree"
echo 'missing, malformed, unavailable and noncommit objects: refused'

# A commit object can exist while its referenced parent is unavailable.
broken=$(printf 'tree %s\nparent %s\nauthor Fixture Contributor <contributor@example.invalid> 1 +0000\ncommitter Fixture Contributor <contributor@example.invalid> 1 +0000\n\nSigned-off-by: Fixture Contributor <contributor@example.invalid>\n' \
    "$tree" 1111111111111111111111111111111111111111 | git hash-object -t commit -w --stdin)
refuse "$base" "$broken"
echo 'unreadable contribution history: refused'
echo 'test-check-dco: OK'
