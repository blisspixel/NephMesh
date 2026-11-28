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

# Create the private, disposable Git destination for the single-cluster demo.
set -eu
umask 077
if [ "$(kubectl config current-context)" != kind-nephmesh-porch ]; then
    echo 'select the disposable kind-nephmesh-porch context first' >&2
    exit 1
fi

git_ns=porch-demo
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# Keep a single credential across retries within this disposable cluster.
kubectl -n default get secret porch-site-git -o name --ignore-not-found > "$tmp/existing"
if [ ! -s "$tmp/existing" ]; then
    openssl rand -hex 32 | tr -d '\n' > "$tmp/password"
    test "$(wc -c < "$tmp/password")" -eq 64
    kubectl -n default create secret generic porch-site-git --type=kubernetes.io/basic-auth \
        --from-literal=username=porchdemo --from-file=password="$tmp/password"
fi
kubectl create namespace "$git_ns" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "$git_ns" apply -f "$(dirname "$0")/git.yaml"
kubectl -n "$git_ns" rollout status deployment/git --timeout=3m

# Pass the demo credential over stdin. The repository API is reached on pod loopback.
kubectl -n default get secret porch-site-git -o jsonpath='{.data.password}' > "$tmp/encoded"
test -s "$tmp/encoded"
bootstrap=$(cat <<'BOOTSTRAP'
    password=$(base64 -d)
    if ! gitea admin user list | awk '$2=="porchdemo" {found=1} END {exit !found}'; then
        gitea admin user create --username porchdemo --email porch@example.invalid \
            --password "$password" --must-change-password=false
    fi
    auth=$(mktemp)
    trap 'rm -f "$auth"' EXIT
    printf "machine 127.0.0.1 login porchdemo password %s\n" "$password" > "$auth"
    base=http://127.0.0.1:3000/api/v1
    code=$(curl -sS -o /dev/null -w "%{http_code}" --netrc-file "$auth" "$base/repos/porchdemo/site")
    case "$code" in
        200) ;;
        404) curl -fsS --netrc-file "$auth" -H "Content-Type: application/json" \
                 -d '{"name":"site","private":true,"auto_init":true,"default_branch":"main"}' \
                 "$base/user/repos" > /dev/null ;;
        *) echo "Git repository lookup failed with HTTP $code" >&2; exit 1 ;;
    esac
BOOTSTRAP
)
kubectl -n "$git_ns" exec -i deploy/git -- sh -eu -c "$bootstrap" < "$tmp/encoded"
echo "Site repository ready at http://git.$git_ns.svc.cluster.local:3000/porchdemo/site.git"
