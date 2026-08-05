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

// Package secret holds sensitive material (a broker password, a channel PSK)
// in a type that cannot leak through the usual accidental paths: fmt verbs,
// structured logging, JSON, or an error string. The real bytes come out only
// through Reveal, called at the single point that writes device configuration.
package secret

// Value wraps a secret string. Its String, GoString, MarshalJSON, and
// MarshalLog (the logr structured-logging hook) all render a fixed placeholder
// instead of the secret, so logging or formatting a Value, a struct containing
// one, or an error wrapping one never discloses it. Only Reveal returns the
// real value; every call site that reveals is a deliberate, auditable point.
type Value struct {
	secret string
}

const placeholder = "[REDACTED]"

// New wraps a secret string.
func New(s string) Value { return Value{secret: s} }

// Reveal returns the underlying secret. This is the one disclosing accessor;
// keep its callers few and obvious (the config-write path), never a log line.
func (v Value) Reveal() string { return v.secret }

// IsZero reports whether no secret is held, so callers can omit an unset field
// without revealing anything.
func (v Value) IsZero() bool { return v.secret == "" }

// String renders the placeholder (empty stays empty so an unset value formats
// cleanly), covering %v, %s, and any fmt.Stringer consumer.
func (v Value) String() string {
	if v.secret == "" {
		return ""
	}
	return placeholder
}

// GoString covers %#v, which would otherwise print the struct field verbatim.
func (v Value) GoString() string { return "secret.Value{" + v.String() + "}" }

// MarshalJSON covers JSON encoders, including structured loggers that serialize
// fields as JSON.
func (v Value) MarshalJSON() ([]byte, error) {
	if v.secret == "" {
		return []byte(`""`), nil
	}
	return []byte(`"` + placeholder + `"`), nil
}

// MarshalLog is the logr hook: loggers that understand it use this instead of
// reflecting over the struct, so a Value logged as a field stays redacted.
func (v Value) MarshalLog() any { return v.String() }
