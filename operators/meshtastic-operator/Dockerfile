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

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
ARG TARGETARCH
WORKDIR /src
COPY api/ api/
COPY operators/meshtastic-operator/ operators/meshtastic-operator/
WORKDIR /src/operators/meshtastic-operator
RUN CGO_ENABLED=0 GOARCH="$TARGETARCH" go build -trimpath -buildvcs=false -o /out/meshtastic-operator ./cmd

# The CLI needs Python <3.15. Only the build stage needs pip.
FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS runtime
RUN apk add --no-cache python3=3.14.10-r2

FROM runtime AS cli
COPY operators/meshtastic-operator/requirements.txt /tmp/requirements.txt
RUN apk add --no-cache py3-pip=26.1.2-r1 \
    && python3 -m venv --without-pip /opt/cli \
    && pip --python /opt/cli install --no-cache-dir --require-hashes -r /tmp/requirements.txt

FROM runtime
RUN addgroup -g 65532 app && adduser -D -u 65532 -G app app
COPY --from=cli /opt/cli /opt/cli
ENV HOME=/home/app \
    PATH=/opt/cli/bin:/usr/local/bin:/usr/bin:/bin
USER 65532:65532
COPY --from=build /out/meshtastic-operator /usr/local/bin/meshtastic-operator
# The operator execs these helpers: the exporter emits the managed config
# including the channel set (which the CLI's --export-config does not), and the
# applier writes channels through a file so keys never reach the command line.
COPY operators/meshtastic-operator/hack/mesh-export.py operators/meshtastic-operator/hack/mesh-apply.py /usr/local/lib/nephmesh/
ENTRYPOINT ["meshtastic-operator"]
