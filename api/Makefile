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

# api types module. controller-gen is pinned so generation is reproducible.
CONTROLLER_GEN_VERSION ?= v0.19.0
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
PKG := ./mesh/v1alpha1

.PHONY: generate manifests build vet lint test coverage verify help

generate: ## Regenerate deepcopy code
	$(CONTROLLER_GEN) object:headerFile=../hack/boilerplate.go.txt paths=$(PKG)

manifests: ## Regenerate CRD manifests
	$(CONTROLLER_GEN) crd output:crd:artifacts:config=config/crd/bases paths=$(PKG)

build: ## Compile
	go build ./...

vet: ## go vet
	go vet ./...

lint: ## golangci-lint (shared root config)
	golangci-lint run ./...

test: ## Run unit tests with coverage
	go test ./... -cover

coverage: ## Enforce the meaningful-coverage floor (generated excluded)
	sh ../hack/check-coverage.sh 80

verify: generate manifests ## Fail if generated code or manifests are stale
	@git diff --exit-code -- mesh config >/dev/null 2>&1 || \
		{ echo "generated code or CRDs are stale; run make -C api generate manifests"; exit 1; }
	@echo "verify: generated artifacts up to date"

help: ## Show targets
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'
