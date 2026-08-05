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

// Package v1alpha1 contains the NephMesh mesh radio API types.
//
// The nephmesh.io domain is a provisional placeholder: an API group is a
// stable DNS-style string, not a claim that the domain is registered. While
// these types are v1alpha1 and pre-alpha, renaming the group is cheap; this is
// revisited before any public or 1.0 release. See docs/plans/crd-api-design.md.
//
// +kubebuilder:object:generate=true
// +groupName=mesh.nephmesh.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	// Group is the API group for mesh radio intent.
	Group = "mesh.nephmesh.io"
	// Version is the API version.
	Version = "v1alpha1"
	// MeshtasticNodeKind is exported so KRM functions can build object
	// references without importing reflection.
	MeshtasticNodeKind = "MeshtasticNode"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
