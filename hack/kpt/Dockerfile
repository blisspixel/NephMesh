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

# A reproducible environment for `kpt fn render`. kpt needs git on PATH and,
# to run KRM functions, the docker CLI plus a mounted Docker socket. Pinning
# kpt here means local Windows or macOS runs and CI render the packages with
# the exact same toolchain. Base is docker:cli (Alpine with the docker CLI).

FROM docker:29.10.1-cli

ARG KPT_VERSION=v1.0.0-beta.67
ARG KPT_SHA256=5769ef31efb5a18a1b94e1e766443c685aabd72f9f52bfc83079e1f4b1c3d532
RUN apk add --no-cache git curl \
    && curl -fsSL -o /tmp/kpt.tar.gz "https://github.com/kptdev/kpt/releases/download/${KPT_VERSION}/kpt_linux_amd64-${KPT_VERSION#v}.tar.gz" \
    && echo "${KPT_SHA256}  /tmp/kpt.tar.gz" | sha256sum -c - \
    && tar -xzf /tmp/kpt.tar.gz -C /usr/local/bin kpt \
    && rm /tmp/kpt.tar.gz \
    && kpt version

WORKDIR /work
ENTRYPOINT ["kpt"]
CMD ["fn", "render", "--help"]
