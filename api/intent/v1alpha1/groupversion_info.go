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

// Package v1alpha1 contains the NephMesh outcome-level intent API types. A
// CommunicationIntent declares desired mesh communications above the per-device
// configuration, and a compiler lowers it into MeshtasticNode specs. Today the
// compiler runs report-only: it writes the proposed rendering and a feasibility
// verdict to status and never actuates. This realizes ADR 0001 (MeshtasticNode
// is the compiled output of a higher-level intent), the first, deliberately safe
// slice of the design doctrine's intent layer.
//
// The nephmesh.io domain is a provisional placeholder, as in the mesh group.
//
// +kubebuilder:object:generate=true
// +groupName=intent.nephmesh.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	// Group is the API group for outcome-level communications intent.
	Group = "intent.nephmesh.io"
	// Version is the API version.
	Version = "v1alpha1"
	// CommunicationIntentKind is exported for object references.
	CommunicationIntentKind = "CommunicationIntent"
)

var (
	// GroupVersion is the group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
