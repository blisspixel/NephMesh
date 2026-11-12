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

package plan

import (
	"fmt"
	"sort"
)

// fieldSchema describes the CommunicationIntent fields the CRD schema defines.
// A scalar has no children. An array of objects sets elem. Unknown keys are
// warnings: a structural CRD prunes them at admission rather than rejecting
// the object, so plan reports them and still compiles the known fields.
type fieldSchema struct {
	children map[string]*fieldSchema
	elem     *fieldSchema
}

func objectSchema(children map[string]*fieldSchema) *fieldSchema {
	return &fieldSchema{children: children}
}

func arraySchema(elem *fieldSchema) *fieldSchema {
	return &fieldSchema{elem: elem}
}

var scalarSchema = &fieldSchema{}

var secretRefSchema = objectSchema(map[string]*fieldSchema{
	"key":      scalarSchema,
	"name":     scalarSchema,
	"optional": scalarSchema,
})

var specFieldSchema = objectSchema(map[string]*fieldSchema{
	"region":               scalarSchema,
	"approvedModemPresets": scalarSchema,
	"role":                 scalarSchema,
	"channels": arraySchema(objectSchema(map[string]*fieldSchema{
		"index":           scalarSchema,
		"name":            scalarSchema,
		"pskSecretRef":    secretRefSchema,
		"uplinkEnabled":   scalarSchema,
		"downlinkEnabled": scalarSchema,
	})),
	"expectedTraffic": objectSchema(map[string]*fieldSchema{
		"messagesPerMinutePerNode": scalarSchema,
		"payloadBytes":             scalarSchema,
	}),
	"nodes": arraySchema(objectSchema(map[string]*fieldSchema{
		"name": scalarSchema,
		"connection": objectSchema(map[string]*fieldSchema{
			"tcp": objectSchema(map[string]*fieldSchema{
				"host": scalarSchema,
				"port": scalarSchema,
			}),
			"serial": objectSchema(map[string]*fieldSchema{
				"device": scalarSchema,
			}),
			"viaGateway": objectSchema(map[string]*fieldSchema{
				"dest": scalarSchema,
				"gatewayRef": objectSchema(map[string]*fieldSchema{
					"name": scalarSchema,
				}),
			}),
		}),
		"owner": objectSchema(map[string]*fieldSchema{
			"shortName": scalarSchema,
			"longName":  scalarSchema,
		}),
	})),
})

// unknownFields lists schema keys the document carries and the CRD does not
// define. Paths are rooted at spec, including for a bare spec, so a warning
// names the field an apply would prune. Metadata and status are not walked:
// they are not compiler input, and metadata has an open key set.
func unknownFields(probe map[string]any) []string {
	if len(probe) == 0 {
		return nil
	}
	var warns []string
	if _, hasSpec := probe["spec"]; hasSpec {
		keys := make([]string, 0, len(probe))
		for k := range probe {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch k {
			case "apiVersion", "kind", "metadata", "spec", "status":
			default:
				warns = append(warns, "unknown field "+k)
			}
		}
		walkUnknown("spec", probe["spec"], specFieldSchema, &warns)
	} else {
		walkUnknown("spec", probe, specFieldSchema, &warns)
	}
	if len(warns) == 0 {
		return nil
	}
	sort.Strings(warns)
	return warns
}

func walkUnknown(path string, value any, schema *fieldSchema, warns *[]string) {
	if schema == nil {
		return
	}
	if schema.elem != nil {
		items, ok := value.([]any)
		if !ok {
			return
		}
		for i, item := range items {
			walkUnknown(fmt.Sprintf("%s[%d]", path, i), item, schema.elem, warns)
		}
		return
	}
	if len(schema.children) == 0 {
		return
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		child, known := schema.children[k]
		next := path + "." + k
		if !known {
			*warns = append(*warns, "unknown field "+next)
			continue
		}
		walkUnknown(next, obj[k], child, warns)
	}
}
