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

# The receive-only spectrum exporter. Build context is the repo root so the
# sibling api module resolves through the go.mod replace directive:
#
#   docker build -f operators/meshtastic-operator/cmd/spectrum-exporter/Dockerfile \
#     -t ghcr.io/blisspixel/nephmesh-spectrum-exporter:dev .
#
# The default in-cluster deployment runs in -replay mode (simulation-first, no
# radio). The image also carries hackrf_sweep for the real sensor path, which
# runs on a host with the SDR attached, outside the hardened cluster deployment
# (device access is a reviewed host deviation; see packages/spectrum-exporter).

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY api/ api/
COPY operators/meshtastic-operator/ operators/meshtastic-operator/
WORKDIR /src/operators/meshtastic-operator
RUN CGO_ENABLED=0 go build -o /out/spectrum-exporter ./cmd/spectrum-exporter

FROM debian:stable-slim
# hackrf_sweep for the real sensor path. The default deployment is receive-only
# and replay-driven, so an image without a radio still publishes real metrics.
RUN apt-get update \
    && apt-get install -y --no-install-recommends hackrf ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN useradd --create-home --uid 65532 app
USER 65532
COPY --from=build /out/spectrum-exporter /usr/local/bin/spectrum-exporter
EXPOSE 9808
ENTRYPOINT ["spectrum-exporter"]
