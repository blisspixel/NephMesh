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

# The operator execs the Meshtastic CLI, so the CLI must live in the same
# container (a sidecar cannot be exec'd into). The final image therefore
# carries the pinned CLI alongside the Go binary and runs as a non-root user.
# Build context is the repo root so the sibling api module resolves through the
# go.mod replace directive.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY api/ api/
COPY operators/meshtastic-operator/ operators/meshtastic-operator/
WORKDIR /src/operators/meshtastic-operator
RUN CGO_ENABLED=0 go build -o /out/meshtastic-operator ./cmd

FROM python:3.13-slim
ARG MESHTASTIC_VERSION=2.7.11
RUN useradd --create-home --uid 65532 app
USER 65532
ENV HOME=/home/app \
    PATH=/home/app/.local/bin:/usr/local/bin:/usr/bin:/bin
RUN pip install --no-cache-dir --user "meshtastic[cli]==${MESHTASTIC_VERSION}"
COPY --from=build /out/meshtastic-operator /usr/local/bin/meshtastic-operator
ENTRYPOINT ["meshtastic-operator"]
