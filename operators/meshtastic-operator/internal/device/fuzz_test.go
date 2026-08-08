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

import "testing"

// The CLI output parsers consume untrusted data: a compromised or spoofed
// device on the 4403 API, or a hostile mesh participant whose traffic shapes
// what the CLI prints, controls these bytes. The property under fuzz is simply
// that parsing never panics and always terminates, so malformed device output
// can never crash the operator. Crashers the engine finds are written to
// testdata/fuzz and become permanent regression tests. The seed corpus mixes
// valid, malformed, and truncated inputs so mutations explore the error paths,
// not just the happy path.

func FuzzParseExportConfig(f *testing.F) {
	f.Add(exportMarker + "\nconfig:\n  lora:\n    region: US\n")
	f.Add("connection chatter before\n" + exportMarker + "\n{}")
	f.Add(exportMarker)
	f.Add("")
	f.Add("not yaml at all: : : ][")
	f.Add(exportMarker + "\nconfig:\n  lora:\n    region: US\n  device:\n    role: ROUTER\n")
	f.Fuzz(func(t *testing.T, out string) {
		// Property: never panics, always returns. The result is not asserted;
		// robustness against arbitrary bytes is the point.
		_, _ = parseExportConfig(out)
	})
}

func FuzzParseInfo(f *testing.F) {
	f.Add(`Nodes: { "!6e000001": {} }`)
	f.Add("Owner: NephMesh (NM)")
	f.Add("")
	f.Add("!tooShort")
	f.Add("!6e000001 !aabbccdd extra tokens")
	f.Fuzz(func(t *testing.T, out string) {
		_ = parseInfo(out)
	})
}

func FuzzLooksUnreachable(f *testing.F) {
	f.Add("Error connecting to host:[Errno 110] Connection timed out")
	f.Add("Set lora.region to US")
	f.Add("")
	f.Fuzz(func(t *testing.T, out string) {
		_ = looksUnreachable(out, false)
		_ = looksUnreachable(out, true)
	})
}
