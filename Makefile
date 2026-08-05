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

# Root Makefile. Grows the upstream default-*.mk fragment pattern as Go
# modules land (docs/plans/engineering-conventions.md, section 6). For now it
# carries the Phase 0 checks and the Phase 1 demo entry points.

.PHONY: check check-headers check-style check-manifests check-transmit demo-phase1 demo-phase1-down help

check: check-headers check-style check-manifests check-transmit ## Run all repo checks

check-headers: ## Verify Apache-2.0 headers on source files
	@sh hack/check-headers.sh

check-style: ## Enforce AGENTS.md writing rules (no em dashes, emojis, attribution)
	@sh hack/check-style.sh

check-manifests: ## Enforce the threat model: never expose the control surface
	@sh hack/check-manifests.sh

check-transmit: ## Enforce the transmit interlock: no unmarked RF-transmit entry points
	@sh hack/check-transmit.sh

demo-phase1: ## Apply the Phase 1 virtual mesh demo to the current kube context
	@sh demo/phase1/scripts/demo.sh

demo-phase1-down: ## Tear the Phase 1 demo down
	@sh demo/phase1/scripts/teardown.sh

help: ## Show targets
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-20s %s\n", $$1, $$2}'
