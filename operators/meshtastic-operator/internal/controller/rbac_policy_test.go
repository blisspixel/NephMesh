/*
Copyright 2026 The NephMesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Assume-breach control-proving tests for the shipped RBAC. They read the
// real ClusterRole the operator deploys with (not a copy) and prove the
// least-privilege boundary holds: an added grant, a wildcard, a Secret rule,
// or an escalation verb fails the build here. This guards the earlier removal
// of a cluster-wide Secret grant against silent regression.
package controller_test

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

// shippedRBACPath is the deployed ClusterRole, relative to this package
// directory (operators/meshtastic-operator/internal/controller). Reading the
// real artifact means drift cannot hide behind a test-only copy.
const shippedRBACPath = "../../../../packages/meshtastic-operator/rbac.yaml"

func loadClusterRoles(t *testing.T, path string) []rbacv1.ClusterRole {
	t.Helper()
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var roles []rbacv1.ClusterRole
	dec := k8syaml.NewYAMLOrJSONDecoder(f, 4096)
	for {
		// Every document decodes into a ClusterRole; non-ClusterRole
		// documents (ServiceAccount, ClusterRoleBinding) populate only
		// TypeMeta and are filtered out by Kind.
		var obj rbacv1.ClusterRole
		if err := dec.Decode(&obj); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode %s: %v", path, err)
		}
		if obj.Kind == "ClusterRole" {
			roles = append(roles, obj)
		}
	}
	return roles
}

// ruleKey canonicalizes a PolicyRule into an order-insensitive comparison key,
// so rule and field ordering in the YAML does not affect the assertion.
func ruleKey(r rbacv1.PolicyRule) string {
	parts := [][]string{
		append([]string(nil), r.APIGroups...),
		append([]string(nil), r.Resources...),
		append([]string(nil), r.Verbs...),
		append([]string(nil), r.ResourceNames...),
		append([]string(nil), r.NonResourceURLs...),
	}
	segs := make([]string, len(parts))
	for i, p := range parts {
		sort.Strings(p)
		segs[i] = strings.Join(p, ",")
	}
	return strings.Join(segs, "|")
}

func TestShippedRBACMatchesLeastPrivilegeAllowlist(t *testing.T) {
	roles := loadClusterRoles(t, shippedRBACPath)
	if len(roles) != 1 {
		t.Fatalf("expected exactly one ClusterRole in the shipped package, got %d", len(roles))
	}

	// The complete allowlist: read the MeshtasticNode resources it
	// reconciles and update their status and finalizers. Nothing else.
	want := []rbacv1.PolicyRule{
		{APIGroups: []string{"mesh.nephmesh.io"}, Resources: []string{"meshtasticnodes"}, Verbs: []string{"get", "list", "watch", "update", "patch"}},
		{APIGroups: []string{"mesh.nephmesh.io"}, Resources: []string{"meshtasticnodes/status"}, Verbs: []string{"get", "update", "patch"}},
		{APIGroups: []string{"mesh.nephmesh.io"}, Resources: []string{"meshtasticnodes/finalizers"}, Verbs: []string{"update"}},
	}

	gotKeys := map[string]bool{}
	for _, r := range roles[0].Rules {
		gotKeys[ruleKey(r)] = true
	}
	wantKeys := map[string]bool{}
	for _, r := range want {
		wantKeys[ruleKey(r)] = true
	}
	for k := range wantKeys {
		if !gotKeys[k] {
			t.Errorf("least-privilege allowlist is missing an expected rule: %s", k)
		}
	}
	for k := range gotKeys {
		if !wantKeys[k] {
			t.Errorf("unexpected RBAC grant (least-privilege regression): %s", k)
		}
	}
}

func TestShippedRBACForbidsSecretsWildcardEscalation(t *testing.T) {
	forbiddenVerbs := map[string]bool{"*": true, "escalate": true, "bind": true, "impersonate": true}
	roles := loadClusterRoles(t, shippedRBACPath)
	if len(roles) == 0 {
		t.Fatal("no ClusterRole found in the shipped package")
	}
	for _, role := range roles {
		for _, r := range role.Rules {
			for _, res := range r.Resources {
				if res == "*" || res == "secrets" || strings.HasPrefix(res, "secrets/") {
					t.Errorf("ClusterRole %s grants forbidden resource %q", role.Name, res)
				}
			}
			for _, g := range r.APIGroups {
				if g == "*" {
					t.Errorf("ClusterRole %s uses a wildcard apiGroup", role.Name)
				}
			}
			for _, v := range r.Verbs {
				if forbiddenVerbs[v] {
					t.Errorf("ClusterRole %s grants forbidden verb %q", role.Name, v)
				}
			}
		}
	}
}
