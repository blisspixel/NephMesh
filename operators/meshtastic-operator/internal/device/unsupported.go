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

package device

import (
	"context"
	"fmt"
)

// Unsupported is a Client for a transport the operator does not yet implement
// (serial and viaGateway are planned). Every method reports the device as
// unreachable, so a node selecting such a transport is surfaced as not ready
// with a clear reason rather than silently mishandled or panicking. It is a
// deliberate graceful-degradation stand-in, not a placeholder.
type Unsupported struct {
	Transport string
}

func (u *Unsupported) err() error {
	return fmt.Errorf("%w: transport %q is not implemented yet", ErrUnreachable, u.Transport)
}

func (u *Unsupported) ExportConfig(context.Context) (map[string]any, error) { return nil, u.err() }
func (u *Unsupported) Apply(context.Context, map[string]any) error          { return u.err() }
func (u *Unsupported) Reboot(context.Context) error                         { return u.err() }
func (u *Unsupported) Info(context.Context) (Info, error)                   { return Info{}, u.err() }
